package main

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/ghodss/yaml"
	"github.com/semaphoreci/agent/pkg/executor"
	"github.com/semaphoreci/agent/pkg/shell"
)

type Command struct {
	Directive string `yaml:"directive"`
}

type Container struct {
	Name  string `yaml:"name"`
	Image string `yaml:"image"`
}

type Job struct {
	Services []Container `yaml:"services"`
	Commands []Command   `yaml:"commands"`
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
	containers := []executor.Container{}

	for _, c := range job.Services {
		containers = append(containers, executor.Container{
			Name:  c.Name,
			Image: c.Image,
		})
	}

	dc, err := executor.NewDockerComposeExecutor(containers)

	fmt.Printf("eee")
	err = dc.Build()
	check(err)

	fmt.Printf("ccc")
	err = dc.Setup()
	check(err)
	fmt.Printf("ddd")

	commands := []string{}
	for _, c := range job.Commands {
		commands = append(commands, c.Directive)
	}

	fmt.Printf("aaa")
	sh := shell.NewShell(dc)
	fmt.Printf("bbb")

	sh.Run(commands, func(event interface{}) {
		switch e := event.(type) {
		case shell.CommandStartedShellEvent:
			fmt.Printf("command %d | Running: %s\n", e.CommandIndex, e.Command)
		case shell.CommandOutputShellEvent:
			fmt.Printf("command %d | %s\n", e.CommandIndex, e.Output)
		case shell.CommandFinishedShellEvent:
			fmt.Printf("command %d | exit status: %d\n", e.CommandIndex, e.ExitStatus)
		default:
			panic("Unknown shell event")
		}
	})
}
