package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"

	"github.com/ghodss/yaml"
)

type Command struct {
	Directive string `yaml:"directive"`
}

type EnvVar struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type File struct {
	Path    string `yaml:"name"`
	Content string `yaml:"content"`
	Mode    string `yaml:"mode"`
}

type Callbacks struct {
	Started  string `yaml:"mode"`
	Finished string `yaml:"mode"`
}

type JobRequest struct {
	Commands  []Command `yaml:"commands"`
	EnvVars   []EnvVar  `yaml:"env_vars"`
	Files     []File    `yaml:"file"`
	Callbacks Callbacks `yaml:"callbacks"`
}

type Job struct {
	Request JobRequest
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func NewJobFromYaml(path string) (*Job, error) {
	filename, _ := filepath.Abs(path)
	yamlFile, err := ioutil.ReadFile(filename)

	if err != nil {
		return nil, err
	}

	fmt.Printf("%s\n", yamlFile)

	var jobRequest JobRequest

	err = yaml.Unmarshal(yamlFile, &jobRequest)
	if err != nil {
		return nil, err
	}

	return &Job{Request: jobRequest}, nil
}

func (job *Job) Run() {
	fmt.Printf("%+v\n", job.Request)

	job.SendStartedCallback()

	shell := NewShell()

	shell.Run(job.Request, func(event interface{}) {
		switch e := event.(type) {
		case CommandStartedShellEvent:
			fmt.Printf("command %d | Running: %s\n", e.CommandIndex, e.Command)
		case CommandOutputShellEvent:
			fmt.Printf("command %d | %s\n", e.CommandIndex, e.Output)
		case CommandFinishedShellEvent:
			fmt.Printf("command %d | exit status: %d\n", e.CommandIndex, e.ExitStatus)
		default:
			panic("Unknown shell event")
		}
	})

	job.SendFinishedCallback("passed")
}

func (job *Job) SendStartedCallback() error {
	payload := `{"port": 22}`

	return job.SendCallback(job.Request.Callbacks.Started, payload)
}

func (job *Job) SendFinishedCallback(result string) error {
	payload := fmt.Sprintf(`{"result": "%s"}`, result)

	return job.SendCallback(job.Request.Callbacks.Finished, payload)
}

func (job *Job) SendCallback(url string, payload string) error {
	fmt.Printf("Sending started callback: %s with %+v\n", url, payload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer([]byte(payload)))

	fmt.Printf("%+v\n", resp)

	return err
}
