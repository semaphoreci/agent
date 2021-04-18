package listener

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	jobs "github.com/semaphoreci/agent/pkg/jobs"
)

func StartJobProcessor(endpoint string) (*JobProcessor, error) {
	p := &JobProcessor{
		Endpoint: endpoint,
	}

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

		job.Run()
	}
}

func (p *JobProcessor) AcquireJob() (*api.JobRequest, error) {
	resp, err := http.Post(p.Endpoint, "application/json", bytes.NewBuffer([]byte("{}")))
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
