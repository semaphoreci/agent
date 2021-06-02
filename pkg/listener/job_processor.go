package listener

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	jobs "github.com/semaphoreci/agent/pkg/jobs"
	selfhostedapi "github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
)

func StartJobProcessor(apiClient *selfhostedapi.Api) (*JobProcessor, error) {
	p := &JobProcessor{
		ApiClient:             apiClient,
		LastSuccesfulSync:     time.Now(),
		ConsecutiveSyncErrors: 0,
		State: selfhostedapi.AgentStateWaitingForJobs,
	}

	go p.Start()

	return p, nil
}

type JobProcessor struct {
	ApiClient             *selfhostedapi.Api
	State                 selfhostedapi.AgentState
	CurrentJobID          string
	CurrentJob            *jobs.Job
	ConsecutiveSyncErrors int
	LastSyncErrorAt       *time.Time
	LastSuccesfulSync     time.Time
}

func (p *JobProcessor) Start() {
	for {
		p.Sync()
		time.Sleep(5 * time.Second)
	}
}

func (p *JobProcessor) Sync() {
	request := &selfhostedapi.SyncRequest{
		State: p.State,
		JobID: p.CurrentJobID,
	}

	response, err := p.ApiClient.Sync(request)
	if err != nil {
		fmt.Println("[SYNC ERR] Failed to sync with API.")
		fmt.Println("[SYNC ERR] " + err.Error())

		now := time.Now()

		p.ConsecutiveSyncErrors += 1
		p.LastSyncErrorAt = &now

		if p.ConsecutiveSyncErrors > 10 && time.Now().Add(-10*time.Minute).After(p.LastSuccesfulSync) {
			panic("AAAA")
		}

		return
	}

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
		os.Exit(1)
	}
}

func (p *JobProcessor) RunJob(jobID string) {
	p.CurrentJobID = jobID
	p.State = selfhostedapi.AgentStateRunningJob

	jobRequest, err := p.ApiClient.GetJob(p.CurrentJobID)
	if err != nil {
		panic(err)
	}

	job, err := jobs.NewJob(jobRequest)
	if err != nil {
		panic("bbb")
	}

	p.CurrentJob = job

	go p.StreamLogs(job)
	go job.Run()
}

func (p *JobProcessor) StopJob(jobID string) {
	p.CurrentJobID = jobID
	p.State = selfhostedapi.AgentStateStoppingJob

	p.CurrentJob.Stop()
}

func (p *JobProcessor) StreamLogs(job *jobs.Job) {
	lastEventStreamed := 0
	ticker := time.NewTicker(1 * time.Second)

	go func() {
		for {
			<-ticker.C

			events, err := p.StreamLogsBatch(lastEventStreamed)
			if err != nil {
				fmt.Println(err)
				continue
			}

			fmt.Println("Logs streamed")
			lastEventStreamed += events

			if p.CurrentJob.Finished {
				p.State = selfhostedapi.AgentStateFinishedJob
				p.CurrentJob.JobLogArchived = true
				return
			}
		}
	}()
}

func (p *JobProcessor) StreamLogsBatch(lastEventStreamed int) (int, error) {
	buf := new(bytes.Buffer)

	logFile := p.CurrentJob.Logger.Backend.(*eventlogger.FileBackend)
	logFile.Stream(lastEventStreamed, buf)

	events := len(strings.Split(buf.String(), "\n\n")) - 1

	err := p.ApiClient.Logs(p.CurrentJobID, buf)
	if err != nil {
		fmt.Println(err)
		return 0, err
	}

	return events, nil
}
