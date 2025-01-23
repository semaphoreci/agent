package listener

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	jobs "github.com/semaphoreci/agent/pkg/jobs"
	"github.com/semaphoreci/agent/pkg/kubernetes"
	selfhostedapi "github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
	"github.com/semaphoreci/agent/pkg/random"
	"github.com/semaphoreci/agent/pkg/retry"
	"github.com/semaphoreci/agent/pkg/shell"
	log "github.com/sirupsen/logrus"
)

func StartJobProcessor(httpClient *http.Client, apiClient *selfhostedapi.API, config Config) (*JobProcessor, error) {
	p := &JobProcessor{
		HTTPClient:                       httpClient,
		APIClient:                        apiClient,
		UserAgent:                        config.UserAgent,
		LastSuccessfulSync:               time.Now(),
		forceSyncCh:                      make(chan bool),
		State:                            selfhostedapi.AgentStateWaitingForJobs,
		DisconnectRetryAttempts:          100,
		GetJobRetryAttempts:              config.GetJobRetryLimit,
		CallbackRetryAttempts:            config.CallbackRetryLimit,
		ShutdownHookPath:                 config.ShutdownHookPath,
		PreJobHookPath:                   config.PreJobHookPath,
		PostJobHookPath:                  config.PostJobHookPath,
		EnvVars:                          config.EnvVars,
		FileInjections:                   config.FileInjections,
		FailOnMissingFiles:               config.FailOnMissingFiles,
		UploadJobLogs:                    config.UploadJobLogs,
		FailOnPreJobHookError:            config.FailOnPreJobHookError,
		SourcePreJobHook:                 config.SourcePreJobHook,
		ExitOnShutdown:                   config.ExitOnShutdown,
		KubernetesExecutor:               config.KubernetesExecutor,
		KubernetesPodSpec:                config.KubernetesPodSpec,
		KubernetesImageValidator:         config.KubernetesImageValidator,
		KubernetesPodStartTimeoutSeconds: config.KubernetesPodStartTimeoutSeconds,
		KubernetesLabels:                 config.KubernetesLabels,
		KubernetesDefaultImage:           config.KubernetesDefaultImage,
	}

	go p.Start()

	p.SetupInterruptHandler()

	return p, nil
}

type JobProcessor struct {

	// Job processor state
	HTTPClient         *http.Client
	APIClient          *selfhostedapi.API
	State              selfhostedapi.AgentState
	CurrentJobID       string
	CurrentJobResult   selfhostedapi.JobResult
	CurrentJob         *jobs.Job
	LastSyncErrorAt    *time.Time
	LastSuccessfulSync time.Time
	InterruptedAt      int64
	ShutdownReason     ShutdownReason
	mutex              sync.Mutex
	forceSyncCh        chan (bool)

	// Job processor config
	DisconnectRetryAttempts          int
	GetJobRetryAttempts              int
	CallbackRetryAttempts            int
	ShutdownHookPath                 string
	PreJobHookPath                   string
	PostJobHookPath                  string
	StopSync                         bool
	EnvVars                          []config.HostEnvVar
	FileInjections                   []config.FileInjection
	FailOnMissingFiles               bool
	UploadJobLogs                    string
	UserAgent                        string
	FailOnPreJobHookError            bool
	SourcePreJobHook                 bool
	ExitOnShutdown                   bool
	KubernetesExecutor               bool
	KubernetesPodSpec                string
	KubernetesImageValidator         *kubernetes.ImageValidator
	KubernetesPodStartTimeoutSeconds int
	KubernetesLabels                 map[string]string
	KubernetesDefaultImage           string
}

func (p *JobProcessor) Start() {
	go p.SyncLoop()
}

func (p *JobProcessor) SyncLoop() {
	for {
		if p.StopSync {
			break
		}

		nextSyncInterval := p.Sync()
		log.Infof("Waiting %v for next sync...", nextSyncInterval)

		// Here, we wait for the delay sent in the API to pass
		// or we sync again before the delay has passed, if needed.
		select {
		case <-p.forceSyncCh:
			log.Debug("Forcing sync due to state change")
		case <-time.After(nextSyncInterval):
			log.Debug("Delay requested by API expired")
		}
	}
}

func (p *JobProcessor) Sync() time.Duration {
	request := &selfhostedapi.SyncRequest{
		State:         p.State,
		JobID:         p.CurrentJobID,
		JobResult:     p.CurrentJobResult,
		InterruptedAt: p.InterruptedAt,
	}

	response, err := p.APIClient.Sync(request)
	if err != nil {
		p.HandleSyncError(err)
		return p.defaultSyncInterval()
	}

	p.LastSuccessfulSync = time.Now()
	p.ProcessSyncResponse(response)
	return p.findNextSyncInterval(response)
}

func (p *JobProcessor) findNextSyncInterval(response *selfhostedapi.SyncResponse) time.Duration {
	if response.NextSyncAfter > 0 {
		return time.Duration(response.NextSyncAfter) * time.Millisecond
	}

	log.Debug("No next_sync_at field on sync response - using default interval")
	return p.defaultSyncInterval()
}

func (p *JobProcessor) defaultSyncInterval() time.Duration {
	d, _ := random.DurationInRange(3000, 6000)
	return *d
}

func (p *JobProcessor) HandleSyncError(err error) {
	log.Errorf("[SYNC ERR] Failed to sync with API: %v", err)

	now := time.Now()

	p.LastSyncErrorAt = &now

	if time.Now().Add(-10 * time.Minute).After(p.LastSuccessfulSync) {
		log.Error("Unable to sync with Semaphore for over 10 minutes.")
		p.Shutdown(ShutdownReasonUnableToSync, 1)
	}
}

func (p *JobProcessor) ProcessSyncResponse(response *selfhostedapi.SyncResponse) {
	switch response.Action {
	case selfhostedapi.AgentActionContinue:
		// continue what I'm doing, no action needed
		return

	case selfhostedapi.AgentActionRunJob:
		go p.RunJob(response.JobID)
		return

	case selfhostedapi.AgentActionStopJob:
		go p.StopJob(response.JobID)
		return

	case selfhostedapi.AgentActionShutdown:
		log.Infof("Agent shutdown requested by Semaphore due to: %s", response.ShutdownReason)
		p.Shutdown(ShutdownReasonFromAPI(response.ShutdownReason), 0)

	case selfhostedapi.AgentActionWaitForJobs:
		p.WaitForJobs()
	}
}

func (p *JobProcessor) RunJob(jobID string) {
	p.State = selfhostedapi.AgentStateStartingJob
	p.CurrentJobID = jobID

	jobRequest, err := p.getJobWithRetries(p.CurrentJobID)
	if err != nil {
		log.Errorf("Could not get job %s: %v", jobID, err)
		p.JobFinished(selfhostedapi.JobResultFailed)
		return
	}

	job, err := jobs.NewJobWithOptions(&jobs.JobOptions{
		Request:                          jobRequest,
		Client:                           p.HTTPClient,
		ExposeKvmDevice:                  false,
		FileInjections:                   p.FileInjections,
		FailOnMissingFiles:               p.FailOnMissingFiles,
		SelfHosted:                       true,
		UseKubernetesExecutor:            p.KubernetesExecutor,
		PodSpecDecoratorConfigMap:        p.KubernetesPodSpec,
		KubernetesPodStartTimeoutSeconds: p.KubernetesPodStartTimeoutSeconds,
		KubernetesLabels:                 p.KubernetesLabels,
		KubernetesImageValidator:         p.KubernetesImageValidator,
		KubernetesDefaultImage:           p.KubernetesDefaultImage,
		UploadJobLogs:                    p.UploadJobLogs,
		UserAgent:                        p.UserAgent,
		RefreshTokenFn: func() (string, error) {
			return p.APIClient.RefreshToken()
		},
	})

	if err != nil {
		log.Errorf("Could not construct job %s: %v", jobID, err)
		p.JobFinished(selfhostedapi.JobResultFailed)
		return
	}

	p.State = selfhostedapi.AgentStateRunningJob
	p.CurrentJob = job

	go job.RunWithOptions(jobs.RunOptions{
		EnvVars:               p.EnvVars,
		PreJobHookPath:        p.PreJobHookPath,
		PostJobHookPath:       p.PostJobHookPath,
		FailOnPreJobHookError: p.FailOnPreJobHookError,
		SourcePreJobHook:      p.SourcePreJobHook,
		CallbackRetryAttempts: p.CallbackRetryAttempts,
		OnJobFinished:         p.JobFinished,
	})
}

func (p *JobProcessor) getJobWithRetries(jobID string) (*api.JobRequest, error) {
	var jobRequest *api.JobRequest
	err := retry.RetryWithConstantWait(retry.RetryOptions{
		Task:                 "Get job",
		MaxAttempts:          p.GetJobRetryAttempts,
		DelayBetweenAttempts: 3 * time.Second,
		Fn: func() error {
			job, err := p.APIClient.GetJob(jobID)
			if err != nil {
				return err
			}

			jobRequest = job
			return nil
		},
	})

	return jobRequest, err
}

func (p *JobProcessor) StopJob(jobID string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// The job finished before the sync request returned a stop-job command.
	// Here, we don't do anything since the job is already finished and
	// a finished-job state will be reported in the next sync.
	if p.State == selfhostedapi.AgentStateFinishedJob {
		return
	}

	p.CurrentJobID = jobID
	p.State = selfhostedapi.AgentStateStoppingJob

	p.CurrentJob.Stop()
}

func (p *JobProcessor) JobFinished(result selfhostedapi.JobResult) {
	p.mutex.Lock()
	p.State = selfhostedapi.AgentStateFinishedJob
	p.CurrentJobResult = result
	p.forceSyncCh <- true
	p.mutex.Unlock()
}

func (p *JobProcessor) WaitForJobs() {
	p.CurrentJobID = ""
	p.CurrentJob = nil
	p.CurrentJobResult = ""
	p.State = selfhostedapi.AgentStateWaitingForJobs
}

func (p *JobProcessor) SetupInterruptHandler() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Info("Termination signal received")

		// When we receive an interruption signal
		// we tell the API about it, and let it tell the agent when to shut down.
		p.InterruptedAt = time.Now().Unix()
		p.forceSyncCh <- true
	}()
}

func (p *JobProcessor) disconnect() {
	p.StopSync = true
	log.Info("Disconnecting the Agent from Semaphore")

	err := retry.RetryWithConstantWait(retry.RetryOptions{
		Task:                 "Disconnect",
		MaxAttempts:          p.DisconnectRetryAttempts,
		DelayBetweenAttempts: time.Second,
		Fn: func() error {
			_, err := p.APIClient.Disconnect()
			return err
		},
	})

	if err != nil {
		log.Errorf("Failed to disconnect from Semaphore even after %d tries: %v", p.DisconnectRetryAttempts, err)
	} else {
		log.Info("Disconnected.")
	}
}

func (p *JobProcessor) Shutdown(reason ShutdownReason, code int) {
	p.ShutdownReason = reason

	p.disconnect()
	p.executeShutdownHook(reason)
	log.Infof("Agent shutting down due to: %s", reason)

	if p.ExitOnShutdown {
		os.Exit(code)
	}
}

func (p *JobProcessor) executeShutdownHook(reason ShutdownReason) {
	if p.ShutdownHookPath == "" {
		return
	}

	var cmd *exec.Cmd
	log.Infof("Executing shutdown hook from %s", p.ShutdownHookPath)

	if runtime.GOOS == "windows" {
		args := append(shell.Args(), p.ShutdownHookPath)
		// #nosec
		cmd = exec.Command(shell.Executable(), args...)
	} else {
		// #nosec
		cmd = exec.Command("bash", p.ShutdownHookPath)
	}

	cmd.Env = append(os.Environ(), fmt.Sprintf("SEMAPHORE_AGENT_SHUTDOWN_REASON=%s", reason))
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Error executing shutdown hook: %v", err)
		log.Errorf("Output: %s", string(output))
	} else {
		log.Infof("Output: %s", string(output))
	}
}
