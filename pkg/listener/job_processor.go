package listener

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	jobs "github.com/semaphoreci/agent/pkg/jobs"
)

func StartJobProcessor(endpoints *JobProcessorEndpoints) (*JobProcessor, error) {
	p := &JobProcessor{Endpoints: endpoints}

	go p.Start()

	return p, nil
}

type JobProcessorEndpoints struct {
	AcquireJob string
	StreamLogs string
}

type JobProcessor struct {
	Endpoints *JobProcessorEndpoints
}

func (p *JobProcessor) Start() {
	for {
		request, err := p.AcquireJob()
		if err != nil {
			fmt.Println(err)

			time.Sleep(5 * time.Second)
			continue
		}

		job, err := jobs.NewJob(request)

		go p.StreamLogs(job)
		go p.PollForJobStop(job)

		job.Run()
	}
}

func (p *JobProcessor) StreamLogs(job *jobs.Job) {
	lastEventStreamed := 0
	ticker := time.NewTicker(1 * time.Second)

	go func() {
		for {
			select {
			case <-ticker.C:
				events, err := p.StreamLogsBatch(lastEventStreamed, job)
				if err != nil {
					fmt.Println(err)
					continue
				}

				fmt.Println("Logs streamed")
				lastEventStreamed += events
			}
		}
	}()
}

func (p *JobProcessor) StreamLogsBatch(lastEventStreamed int, job *jobs.Job) (int, error) {
	buf := new(bytes.Buffer)

	logFile := job.Logger.Backend.(*eventlogger.FileBackend)
	logFile.Stream(lastEventStreamed, buf)

	events := len(strings.Split(buf.String(), "\n"))

	resp, err := http.Post(p.Endpoints.StreamLogs, "application/json", buf)
	if err != nil {
		return 0, err
	}

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("failed to stream logs")
	}

	return events, nil
}

func (p *JobProcessor) AcquireJob() (*api.JobRequest, error) {
	resp, err := http.Post(p.Endpoints.AcquireJob, "application/json", bytes.NewBuffer([]byte("{}")))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("no job")
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	fmt.Println(string(body))

	request, err := api.NewRequestFromJSON(body)
	if err != nil {
		return nil, err
	}

	return request, nil
}

func (p *JobProcessor) PollForJobStop() error {
	resp, err := http.Post(p.Endpoints., "application/json", bytes.NewBuffer([]byte("{}")))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("no job")
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	fmt.Println(string(body))

	request, err := api.NewRequestFromJSON(body)
	if err != nil {
		return nil, err
	}

	return request, nil
}
