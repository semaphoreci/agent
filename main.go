package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
)

type Command struct {
	Directive string
}

type Container struct {
	Name  string
	Image string
}

type Job struct {
	Services []Container
	Commands []Command
	Epilogue []Command
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func build(services []Container) {
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

	cmd := exec.Command("bash", "-c", "docker-compose -f /tmp/dc1 build")

	fmt.Println("* Starting docker compose")
	err = cmd.Start()
	check(err)

	fmt.Println("* Waiting for build to finish")
	err = cmd.Wait()
	check(err)

	fmt.Println("* Docker Compose Up")
}

func run(job Job) {
	build(job.Services)

	fmt.Println("* Running commands")
	cmd := exec.Command("bash", "-c", "docker-compose -f /tmp/dc1 run main bash -c '"+job.Commands[0].Directive+"'")

	cmdStdoutReader, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error creating StdoutPipe for Cmd", err)
		os.Exit(1)
	}

	cmdStderrReader, err := cmd.StderrPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error creating StderrPipe for Cmd", err)
		os.Exit(1)
	}

	scanner1 := bufio.NewScanner(cmdStdoutReader)
	scanner2 := bufio.NewScanner(cmdStderrReader)

	go func() {
		for scanner1.Scan() {
			fmt.Printf("output | %s\n", scanner1.Text())
		}
	}()

	go func() {
		for scanner2.Scan() {
			fmt.Printf("output | %s\n", scanner2.Text())
		}
	}()

	err = cmd.Start()
	check(err)

	err = cmd.Wait()
	check(err)
}

func main() {
	j1 := Job{
		Services: []Container{
			Container{Name: "main", Image: "ubuntu"},
			Container{Name: "db", Image: "postgres"},
		},
		Commands: []Command{
			Command{Directive: "set -x && echo here && apt-get update && apt-get install -y postgresql-client && createdb -h db -p 5432 -U postgres testdb3 && echo Database testdb3 created"},
		},
	}

	run(j1)
}
