package listener

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/semaphoreci/agent/pkg/api"
	jobs "github.com/semaphoreci/agent/pkg/jobs"
	selfhostedapi "github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
)

func StartJobProcessor(apiClient *selfhostedapi.Api) (*JobProcessor, error) {
	p := &JobProcessor{
		ApiClient:         apiClient,
		LastSuccesfulSync: time.Now(),
		State:             selfhostedapi.AgentStateWaitingForJobs,
	}

	go p.Start()

	p.SetupInteruptHandler()

	return p, nil
}

type JobProcessor struct {
	ApiClient               *selfhostedapi.Api
	State                   selfhostedapi.AgentState
	CurrentJobID            string
	CurrentJob              *jobs.Job
	LastSyncErrorAt         *time.Time
	LastSuccesfulSync       time.Time
	DisconnectRetryAttempts int
}

func (p *JobProcessor) Start() {
	p.SyncLoop()
}

func (p *JobProcessor) SyncLoop() {
	for {
		p.Sync()
		time.Sleep(5 * time.Second)
	}
}

func (p *JobProcessor) Sync() {
	request := &selfhostedapi.SyncRequest{State: p.State, JobID: p.CurrentJobID}

	response, err := p.ApiClient.Sync(request)
	if err != nil {
		p.HandleSyncError(err)
		return
	}

	p.ProcessSyncResponse(response)
}

func (p *JobProcessor) HandleSyncError(err error) {
	fmt.Println("[SYNC ERR] Failed to sync with API.")
	fmt.Println("[SYNC ERR] " + err.Error())

	now := time.Now()

	p.LastSyncErrorAt = &now

	if time.Now().Add(-10 * time.Minute).After(p.LastSuccesfulSync) {
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
		p.CurrentJobID = ""
		p.CurrentJob = nil
		p.State = selfhostedapi.AgentStateWaitingForJobs
	}
}

func (p *JobProcessor) RunJob(jobID string) {
	p.CurrentJobID = jobID
	p.State = selfhostedapi.AgentStateRunningJob

	jobRequest, err := p.getJobWithRetries(p.CurrentJobID)
	if err != nil {
		panic(err)
	}

	job, err := jobs.NewJob(jobRequest)
	if err != nil {
		panic("bbb")
	}

	p.CurrentJob = job

	go job.Run(p.JobFinished)
}

func (p *JobProcessor) getJobWithRetries(jobID string) (*api.JobRequest, error) {
	retries := 10

	for {
		log.Printf("Getting job %s", jobID)

		jobRequest, err := p.ApiClient.GetJob(jobID)
		if err == nil {
			return jobRequest, err
		}

		if retries > 0 {
			retries--
			time.Sleep(3 * time.Second)
			continue
		}

		return nil, err
	}
}

func (p *JobProcessor) StopJob(jobID string) {
	p.CurrentJobID = jobID
	p.State = selfhostedapi.AgentStateStoppingJob

	p.CurrentJob.Stop()
}

func (p *JobProcessor) JobFinished() {
	p.CurrentJobID = ""
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
	fmt.Println("Diconnecting the Agent from Semaphore")

	success := false

	for i := 0; i < p.DisconnectRetryAttempts; i++ {
		resp, err := p.ApiClient.Disconnect()

		if err == nil {
			success = true
			break
		} else {
			fmt.Printf("Disconnect Error. Response: %s, Error: %s\n", resp, err.Error())
			time.Sleep(1 * time.Second)
			continue
		}
	}

	if success {
		fmt.Println("Disconnected.")
	} else {
		fmt.Printf("Failed to disconnect from Semaphore even after %d tries\n", p.DisconnectRetryAttempts)
	}
}

func (p *JobProcessor) Shutdown(reason string, code int) {
	p.disconnect()

	fmt.Println()
	fmt.Println()
	fmt.Println()
	fmt.Println(reason)
	fmt.Println("Shutting down... Good bye!")
	os.Exit(code)
}
