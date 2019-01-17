package main

import (
	"fmt"
	"io/ioutil"
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

type JobRequest struct {
	Commands []Command `yaml:"commands"`
	EnvVars  []EnvVar  `yaml:"env_vars"`
	Files    []File    `yaml:"file"`
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

	var job Job

	err = yaml.Unmarshal(yamlFile, &job)
	if err != nil {
		return nil, err
	}

	return &job, nil
}

func (job *Job) Run() {
	fmt.Printf("%+v\n", job.Request)

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
}
