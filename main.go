package main

import (
	"fmt"
	"io"
	"log"
	"os"

	api "github.com/semaphoreci/agent/pkg/api"
	jobs "github.com/semaphoreci/agent/pkg/jobs"
	server "github.com/semaphoreci/agent/pkg/server"
)

var VERSION = "dev"

func main() {
	action := os.Args[1]

	f, err := os.OpenFile("/tmp/agent_log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)

	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	mwriter := io.MultiWriter(f, os.Stdout)

	log.SetOutput(mwriter)

	switch action {
	case "serve":
		server.NewServer("0.0.0.0", 8000, VERSION, mwriter).Serve()

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
