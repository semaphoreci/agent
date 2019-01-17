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

type Job struct {
	Commands []Command `yaml:"commands"`
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
	commands := []string{}
	for _, c := range job.Commands {
		commands = append(commands, c.Directive)
	}

	shell := NewShell()

	shell.Run(commands, func(event interface{}) {
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
