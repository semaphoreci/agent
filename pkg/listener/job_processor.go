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
		HTTPClient:                 httpClient,
		APIClient:                  apiClient,
		LastSuccessfulSync:         time.Now(),
		LastStateChangeAt:          time.Now(),
		State:                      selfhostedapi.AgentStateWaitingForJobs,
		SyncInterval:               5 * time.Second,
		DisconnectRetryAttempts:    100,
		GetJobRetryAttempts:        config.GetJobRetryLimit,
		CallbackRetryAttempts:      config.CallbackRetryLimit,
		ShutdownHookPath:           config.ShutdownHookPath,
		DisconnectAfterJob:         config.DisconnectAfterJob,
		DisconnectAfterIdleTimeout: config.DisconnectAfterIdleTimeout,
		EnvVars:                    config.EnvVars,
		FileInjections:             config.FileInjections,
		FailOnMissingFiles:         config.FailOnMissingFiles,
		ExitOnShutdown:             config.ExitOnShutdown,
	}

	go p.Start()

	p.SetupInterruptHandler()

	return p, nil
}

type JobProcessor struct {
	HTTPClient                 *http.Client
	APIClient                  *selfhostedapi.API
	State                      selfhostedapi.AgentState
	CurrentJobID               string
	CurrentJob                 *jobs.Job
	SyncInterval               time.Duration
	LastSyncErrorAt            *time.Time
	LastSuccessfulSync         time.Time
	LastStateChangeAt          time.Time
	DisconnectRetryAttempts    int
	GetJobRetryAttempts        int
	CallbackRetryAttempts      int
	ShutdownHookPath           string
	StopSync                   bool
	DisconnectAfterJob         bool
	DisconnectAfterIdleTimeout time.Duration
	EnvVars                    []config.HostEnvVar
	FileInjections             []config.FileInjection
	FailOnMissingFiles         bool
	ExitOnShutdown             bool
	ShutdownReason             ShutdownReason
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

func (p *JobProcessor) isIdle() bool {
	return p.State == selfhostedapi.AgentStateWaitingForJobs
}

func (p *JobProcessor) setState(newState selfhostedapi.AgentState) {
	p.State = newState
	p.LastStateChangeAt = time.Now()
}

func (p *JobProcessor) shutdownIfIdle() {
	if !p.isIdle() {
		return
	}

	if p.DisconnectAfterIdleTimeout == 0 {
		return
	}

	idleFor := time.Since(p.LastStateChangeAt)
	if idleFor > p.DisconnectAfterIdleTimeout {
		log.Infof("Agent has been idle for the past %v.", idleFor)
		p.Shutdown(ShutdownReasonIdle, 0)
	}
}

func (p *JobProcessor) Sync() {
	p.shutdownIfIdle()

	request := &selfhostedapi.SyncRequest{
		State: p.State,
		JobID: p.CurrentJobID,
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
		go p.StopJob()
		return

	case selfhostedapi.AgentActionShutdown:
		log.Info("Agent shutdown requested by Semaphore")
		p.Shutdown(ShutdownReasonRequested, 0)

	case selfhostedapi.AgentActionWaitForJobs:
		p.WaitForJobs()
	}
}

func (p *JobProcessor) RunJob(jobID string) {
	p.setState(selfhostedapi.AgentStateStartingJob)
	p.CurrentJobID = jobID

	jobRequest, err := p.getJobWithRetries(p.CurrentJobID)
	if err != nil {
		log.Errorf("Could not get job %s: %v", jobID, err)
		p.setState(selfhostedapi.AgentStateFailedToFetchJob)
		return
	}

	job, err := jobs.NewJobWithOptions(&jobs.JobOptions{
		Request:            jobRequest,
		Client:             p.HTTPClient,
		ExposeKvmDevice:    false,
		FileInjections:     p.FileInjections,
		FailOnMissingFiles: p.FailOnMissingFiles,
		SelfHosted:         true,
	})

	if err != nil {
		log.Errorf("Could not construct job %s: %v", jobID, err)
		p.setState(selfhostedapi.AgentStateFailedToConstructJob)
		return
	}

	p.setState(selfhostedapi.AgentStateRunningJob)
	p.CurrentJob = job

	go job.RunWithOptions(jobs.RunOptions{
		EnvVars:               p.EnvVars,
		CallbackRetryAttempts: p.CallbackRetryAttempts,
		FileInjections:        p.FileInjections,
		OnSuccessfulTeardown:  p.JobFinished,
		OnFailedTeardown: func() {
			p.setState(selfhostedapi.AgentStateFailedToSendCallback)
			if p.DisconnectAfterJob {
				p.Shutdown(ShutdownReasonJobFinished, 1)
			}
		},
	})
}

func (p *JobProcessor) getJobWithRetries(jobID string) (*api.JobRequest, error) {
	var jobRequest *api.JobRequest
	err := retry.RetryWithConstantWait("Get job", p.GetJobRetryAttempts, 3*time.Second, func() error {
		job, err := p.APIClient.GetJob(jobID)
		if err != nil {
			return err
		}

		jobRequest = job
		return nil
	})

	return jobRequest, err
}

func (p *JobProcessor) StopJob() {
	p.setState(selfhostedapi.AgentStateStoppingJob)
	p.CurrentJob.Stop()
}

func (p *JobProcessor) JobFinished() {
	p.setState(selfhostedapi.AgentStateFinishedJob)
	if p.DisconnectAfterJob {
		p.Shutdown(ShutdownReasonJobFinished, 0)
	}
}

func (p *JobProcessor) WaitForJobs() {
	p.CurrentJobID = ""
	p.CurrentJob = nil
	p.setState(selfhostedapi.AgentStateWaitingForJobs)
}

func (p *JobProcessor) SetupInterruptHandler() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Info("Interruption signal received")
		p.Shutdown(ShutdownReasonInterrupted, 0)
	}()
}

func (p *JobProcessor) disconnect() {
	p.StopSync = true
	log.Info("Disconnecting the Agent from Semaphore")

	err := retry.RetryWithConstantWait("Disconnect", p.DisconnectRetryAttempts, time.Second, func() error {
		_, err := p.APIClient.Disconnect()
		return err
	})

	if err != nil {
		log.Errorf("Failed to disconnect from Semaphore even after %d tries: %v", p.DisconnectRetryAttempts, err)
	} else {
		log.Info("Disconnected.")
	}
}

func (p *JobProcessor) Shutdown(reason ShutdownReason, code int) {
	p.ShutdownReason = reason

	// If there is a running job, we should make sure it is stopped
	// before we disconnect the agent completely.
	if p.CurrentJob != nil && p.State == selfhostedapi.AgentStateRunningJob {
		log.Infof("There is a currently running job, stopping it before disconnecting...")
		p.StopJob()
		p.waitUntilJobIsStopped()
	}

	p.disconnect()
	p.executeShutdownHook(reason)
	log.Infof("Agent shutting down due to: %s", reason)

	if p.ExitOnShutdown {
		os.Exit(code)
	}
}

func (p *JobProcessor) waitUntilJobIsStopped() {
	_ = retry.RetryWithConstantWait("wait for job to stop", 20, 500*time.Millisecond, func() error {
		if p.State == selfhostedapi.AgentStateFinishedJob {
			log.Infof("Current job was stopped.")
			return nil
		}

		return fmt.Errorf("current job was not yet stopped - agent is in '%s' state", p.State)
	})
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
