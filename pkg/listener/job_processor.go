package listener

import (
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/semaphoreci/agent/pkg/api"
	jobs "github.com/semaphoreci/agent/pkg/jobs"
	selfhostedapi "github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
	"github.com/semaphoreci/agent/pkg/retry"
	log "github.com/sirupsen/logrus"
)

func StartJobProcessor(httpClient *http.Client, apiClient *selfhostedapi.Api) (*JobProcessor, error) {
	p := &JobProcessor{
		HttpClient:         httpClient,
		ApiClient:          apiClient,
		LastSuccessfulSync: time.Now(),
		State:              selfhostedapi.AgentStateWaitingForJobs,

		SyncInterval:            5 * time.Second,
		DisconnectRetryAttempts: 100,
	}

	go p.Start()

	p.SetupInteruptHandler()

	return p, nil
}

type JobProcessor struct {
	HttpClient              *http.Client
	ApiClient               *selfhostedapi.Api
	State                   selfhostedapi.AgentState
	CurrentJobID            string
	CurrentJob              *jobs.Job
	SyncInterval            time.Duration
	LastSyncErrorAt         *time.Time
	LastSuccessfulSync      time.Time
	DisconnectRetryAttempts int

	StopSync bool
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
		State: p.State,
		JobID: p.CurrentJobID,
	}

	response, err := p.ApiClient.Sync(request)
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
		p.Shutdown("Unable to sync with Semaphore for over 10 minutes.", 1)
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
		p.Shutdown("Agent Shutdown requested by Semaphore", 0)

	case selfhostedapi.AgentActionWaitForJobs:
		p.WaitForJobs()
	}
}

func (p *JobProcessor) RunJob(jobID string) {
	p.State = selfhostedapi.AgentStateStartingJob
	jobRequest, err := p.getJobWithRetries(jobID)
	if err != nil {
		log.Errorf("Could not get job %s: %v", jobID, err)
		p.State = selfhostedapi.AgentStateFailedToFetchJob
		return
	}

	job, err := jobs.NewJob(jobRequest, p.HttpClient)
	if err != nil {
		log.Errorf("Could not construct job %s: %v", jobID, err)
		p.State = selfhostedapi.AgentStateFailedToConstructJob
		return
	}

	p.State = selfhostedapi.AgentStateRunningJob
	p.CurrentJobID = jobID
	p.CurrentJob = job

	go job.RunWithCallbacks(p.JobFinished, func() {
		p.State = selfhostedapi.AgentStateFailedToSendCallback
	})
}

func (p *JobProcessor) getJobWithRetries(jobID string) (*api.JobRequest, error) {
	var jobRequest *api.JobRequest
	err := retry.RetryWithConstantWait("Get job", 10, 3*time.Second, func() error {
		job, err := p.ApiClient.GetJob(jobID)
		if err != nil {
			return err
		} else {
			jobRequest = job
			return nil
		}
	})

	return jobRequest, err
}

func (p *JobProcessor) StopJob(jobID string) {
	p.CurrentJobID = jobID
	p.State = selfhostedapi.AgentStateStoppingJob

	p.CurrentJob.Stop()
}

func (p *JobProcessor) JobFinished() {
	p.State = selfhostedapi.AgentStateFinishedJob
}

func (p *JobProcessor) WaitForJobs() {
	p.CurrentJobID = ""
	p.CurrentJob = nil
	p.State = selfhostedapi.AgentStateWaitingForJobs
}

func (p *JobProcessor) SetupInteruptHandler() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		p.Shutdown("Ctrl+C pressed in Terminal", 0)
	}()
}

func (p *JobProcessor) disconnect() {
	p.StopSync = true
	log.Info("Disconnecting the Agent from Semaphore")

	err := retry.RetryWithConstantWait("Disconnect", p.DisconnectRetryAttempts, time.Second, func() error {
		_, err := p.ApiClient.Disconnect()
		return err
	})

	if err != nil {
		log.Errorf("Failed to disconnect from Semaphore even after %d tries: %v", p.DisconnectRetryAttempts, err)
	} else {
		log.Info("Disconnected.")
	}
}

func (p *JobProcessor) Shutdown(reason string, code int) {
	log.Println()
	p.disconnect()

	log.Println()
	log.Println()
	log.Println()
	log.Info(reason)
	log.Info("Shutting down... Good bye!")
	os.Exit(code)
}
