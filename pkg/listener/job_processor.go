package listener

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	jobs "github.com/semaphoreci/agent/pkg/jobs"
	selfhostedapi "github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
)

func StartJobProcessor(apiClient *selfhostedapi.Api) (*JobProcessor, error) {
	p := &JobProcessor{
		ApiClient:             apiClient,
		LastSuccesfulSync:     time.Now(),
		ConsecutiveSyncErrors: 0,
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
		now := time.Now()

		p.ConsecutiveSyncErrors += 1
		p.LastSyncErrorAt = &now

		if p.ConsecutiveSyncErrors > 10 && time.Now().Add(-10*time.Minute).After(p.LastSuccesfulSync) {
			panic("AAAA")
		}
	}

	switch response.Action {
	case selfhostedapi.AgentActionContinue:
		// continue what I'm doing, no action needed
		return

	case selfhostedapi.AgentActionRunJob:
		p.RunJob(response.JobID)
		return

	case selfhostedapi.AgentActionStopJob:
		p.StopJob(response.JobID)
		return

	case selfhostedapi.AgentActionShutdown:
		os.Exit(1)
	}

}

func (p *JobProcessor) RunJob(jobID string) {
	p.CurrentJobID = jobID
	p.State = selfhostedapi.AgentStateRunningJob

	job, _ := jobs.NewJob(&api.JobRequest{})

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
		}
	}()
}

func (p *JobProcessor) StreamLogsBatch(lastEventStreamed int) (int, error) {
	buf := new(bytes.Buffer)

	logFile := p.CurrentJob.Logger.Backend.(*eventlogger.FileBackend)
	logFile.Stream(lastEventStreamed, buf)

	events := len(strings.Split(buf.String(), "\n\n")) - 1

	p.ApiClient.Logs(buf)

	return events, nil
}
