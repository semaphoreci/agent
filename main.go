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

	for _, c := range services {
		template += `  ` + c.Name + ":\n"
		template += `    image: "` + c.Image + `"` + "\n"
	}

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

	fmt.Println("* Docker services are ready")
}

func run(job Job) {
	build(job.Services)

	fmt.Println("* Running commands")
	cmd := exec.Command("bash", "-c", "docker-compose -f /tmp/dc1 run main bash -c '"+job.Commands[0].Directive+"'")

	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error creating StdoutPipe for Cmd", err)
		os.Exit(1)
	}

	scanner := bufio.NewScanner(cmdReader)
	go func() {
		for scanner.Scan() {
			fmt.Printf("output | %s\n", scanner.Text())
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
			Command{Directive: "echo here"},
		},
	}

	run(j1)
}
