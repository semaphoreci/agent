package listener

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	jobs "github.com/semaphoreci/agent/pkg/jobs"
	selfhostedapi "github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
	"github.com/semaphoreci/agent/pkg/retry"
	"github.com/semaphoreci/agent/pkg/shell"
	log "github.com/sirupsen/logrus"
)

func StartJobProcessor(httpClient *http.Client, apiClient *selfhostedapi.API, config Config) (*JobProcessor, error) {
	p := &JobProcessor{
		HTTPClient:              httpClient,
		APIClient:               apiClient,
		LastSuccessfulSync:      time.Now(),
		State:                   selfhostedapi.AgentStateWaitingForJobs,
		SyncInterval:            5 * time.Second,
		DisconnectRetryAttempts: 100,
		GetJobRetryAttempts:     config.GetJobRetryLimit,
		CallbackRetryAttempts:   config.CallbackRetryLimit,
		ShutdownHookPath:        config.ShutdownHookPath,
		EnvVars:                 config.EnvVars,
		FileInjections:          config.FileInjections,
		FailOnMissingFiles:      config.FailOnMissingFiles,
		ExitOnShutdown:          config.ExitOnShutdown,
	}

	go p.Start()

	p.SetupInterruptHandler()

	return p, nil
}

type JobProcessor struct {
	HTTPClient              *http.Client
	APIClient               *selfhostedapi.API
	State                   selfhostedapi.AgentState
	CurrentJobID            string
	CurrentJobResult        selfhostedapi.JobResult
	CurrentJob              *jobs.Job
	SyncInterval            time.Duration
	LastSyncErrorAt         *time.Time
	LastSuccessfulSync      time.Time
	DisconnectRetryAttempts int
	GetJobRetryAttempts     int
	CallbackRetryAttempts   int
	ShutdownHookPath        string
	StopSync                bool
	EnvVars                 []config.HostEnvVar
	FileInjections          []config.FileInjection
	FailOnMissingFiles      bool
	ExitOnShutdown          bool
	ShutdownReason          ShutdownReason
}

func (p *JobProcessor) Start() {
	go p.SyncLoop()
}

func (p *JobProcessor) SyncLoop() {
	for {
		if p.StopSync {
			break
		}

		p.Sync()
		time.Sleep(p.SyncInterval)
	}
}

func (p *JobProcessor) Sync() {
	request := &selfhostedapi.SyncRequest{
		State:     p.State,
		JobID:     p.CurrentJobID,
		JobResult: p.CurrentJobResult,
	}

	response, err := p.APIClient.Sync(request)
	if err != nil {
		p.HandleSyncError(err)
		return
	}

	p.LastSuccessfulSync = time.Now()
	p.ProcessSyncResponse(response)
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
		log.Info("Agent shutdown requested by Semaphore due to: %s", response.ShutdownReason)
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
		Request:            jobRequest,
		Client:             p.HTTPClient,
		ExposeKvmDevice:    false,
		FileInjections:     p.FileInjections,
		FailOnMissingFiles: p.FailOnMissingFiles,
		SelfHosted:         true,
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
		CallbackRetryAttempts: p.CallbackRetryAttempts,
		FileInjections:        p.FileInjections,
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
	p.CurrentJobID = jobID
	p.State = selfhostedapi.AgentStateStoppingJob

	p.CurrentJob.Stop()
}

func (p *JobProcessor) JobFinished(result selfhostedapi.JobResult) {
	p.State = selfhostedapi.AgentStateFinishedJob
	p.CurrentJobResult = result
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
		log.Info("Ctrl+C pressed in Terminal")
		p.Shutdown(ShutdownReasonInterrupted, 0)
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

	// #nosec
	if runtime.GOOS == "windows" {
		args := append(shell.Args(), p.ShutdownHookPath)
		cmd = exec.Command(shell.Executable(), args...)
	} else {
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
