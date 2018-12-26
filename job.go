package main

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"

	"github.com/ghodss/yaml"
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

func buildExecutor(services []Container) {
	template := ""
	template += `version: "2"` + "\n"
	template += `services:` + "\n"

	main := services[0]
	template += `  ` + main.Name + ":\n"
	template += `    image: "` + main.Image + `"` + "\n"

	if len(services) > 1 {
		template += `    links:` + "\n"

		// first we add links
		for _, c := range services[1:] {
			template += `      - ` + c.Name + "\n"
		}

		// then we define the rest of the services
		for _, c := range services[1:] {
			template += `  ` + c.Name + ":\n"
			template += `    image: "` + c.Image + `"` + "\n"
		}
	}

	fmt.Println(template)

	fmt.Println("* Creating docker-compose template")
	err := ioutil.WriteFile("/tmp/dc1", []byte(template), 0644)
	check(err)

	cmd := exec.Command("bash", "-c", "docker-compose -f /tmp/dc1 pull --include-deps")

	fmt.Println("* Starting docker compose")
	err = cmd.Start()
	check(err)

	fmt.Println("* Waiting for build to finish")
	err = cmd.Wait()
	check(err)

	fmt.Println("* Docker Compose Up")
}

func (job *Job) Run() {
	buildExecutor(job.Services)

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
