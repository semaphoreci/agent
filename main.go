package main

import (
	"flag"
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

	logfile := OpenLogfile()
	log.SetOutput(logfile)

	switch action {
	case "serve":
		RunServer(logfile)
	case "run":
		RunSingleJob()
	case "version":
		fmt.Println(VERSION)
	}
}

func OpenLogfile() io.Writer {
	f, err := os.OpenFile("/tmp/agent_log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)

	if err != nil {
		log.Fatal(err)
	}

	return io.MultiWriter(f, os.Stdout)
}

func RunServer(logfile io.Writer) {
	authTokenSecret := flag.String("auth-token-secret", "", "Auth token for accessing the server")
	port := flag.Int("port", 8000, "Port of the server")
	host := flag.String("host", "0.0.0.0", "Host of the server")

	flag.Parse()

	server.NewServer(
		*host,
		*port,
		VERSION,
		logfile,
		[]byte(*authTokenSecret),
	).Serve()
}

func RunSingleJob() {
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
}
