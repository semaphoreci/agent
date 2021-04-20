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

func StartJobProcessor(endpoint string) (*JobProcessor, error) {
	p := &JobProcessor{Endpoint: endpoint}

	go p.Start()

	return p, nil
}

type JobProcessor struct {
	Endpoint string
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
		go p.PollJobStatus(job)

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

	events := len(strings.Split(buf.String(), "\n\n")) - 1
	url := p.JobLogStreamUrl(job)

	fmt.Println(buf.String())
	fmt.Println(events)

	resp, err := http.Post(url, "application/json", buf)
	if err != nil {
		return 0, err
	}

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("failed to stream logs")
	}

	return events, nil
}

func (p *JobProcessor) AcquireJobUrl() string {
	return "http://" + p.Endpoint + "/acquire"
}

func (p *JobProcessor) JobLogStreamUrl(job *jobs.Job) string {
	return fmt.Sprintf("http://%s/jobs/%s/logs", p.Endpoint, job.Request.ID)
}

func (p *JobProcessor) AcquireJob() (*api.JobRequest, error) {
	url := p.AcquireJobUrl()
	payload := bytes.NewBuffer([]byte("{}"))

	resp, err := http.Post(url, "application/json", payload)
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

func (p *JobProcessor) JobStatusUrl(jobID string) string {
	return fmt.Sprintf("http://%s/jobs/%s/status", p.Endpoint, jobID)
}

func (p *JobProcessor) PollJobStatus(job *jobs.Job) {
	ticker := time.NewTicker(1 * time.Second)

	go func() {
		for {
			select {
			case <-ticker.C:
				fmt.Println("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
				status, err := p.GetJobStatus(job)
				fmt.Println(status)
				fmt.Println(err)

				if err != nil {
					fmt.Println(err)
					continue
				}

				if status == "stopping" {
					fmt.Println("DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD")
					job.Stop()
				}
			}
		}
	}()
}

func (p *JobProcessor) GetJobStatus(job *jobs.Job) (string, error) {
	url := p.JobStatusUrl(job.Request.ID)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}

	if resp.StatusCode == 404 {
		return "", fmt.Errorf("no job")
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	return string(body), nil
}
