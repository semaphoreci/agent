package main

import (
	"fmt"
	"os"

	api "github.com/semaphoreci/agent/pkg/api"
	jobs "github.com/semaphoreci/agent/pkg/jobs"
	server "github.com/semaphoreci/agent/pkg/server"
)

var VERSION = "dev"

func main() {
	action := os.Args[1]

	switch action {
	case "serve":
		server.NewServer("0.0.0.0", 8000, VERSION).Serve()

	case "run":
		request, err := api.NewRequestFromYamlFile(os.Args[2])

		if err != nil {
			panic(err)
		}

		job, err := jobs.NewJob(request)
		if err != nil {
			panic(err)
		}

		job.JobLogArchived = true

		job.Run()
	case "version":
		fmt.Println(VERSION)
	}

	fmt.Printf("AAAAAAAAAAAAAAAA")
}
