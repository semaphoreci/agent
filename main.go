package main

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/mitchellh/panicwrap"
	watchman "github.com/renderedtext/go-watchman"
	api "github.com/semaphoreci/agent/pkg/api"
	jobs "github.com/semaphoreci/agent/pkg/jobs"
	server "github.com/semaphoreci/agent/pkg/server"
	pflag "github.com/spf13/pflag"
)

var VERSION = "dev"

func main() {
	logFile := OpenLogfile()
	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Lmicroseconds | log.Lshortfile)

	exitStatus, err := panicwrap.BasicWrap(panicHandler)
	if err != nil {
		panic(err)
	}

	// If exitStatus >= 0, then we're the parent process and the panicwrap
	// re-executed ourselves and completed. Just exit with the proper status.
	if exitStatus >= 0 {
		os.Exit(exitStatus)
	}

	// Otherwise, exitStatus < 0 means we're the child. Continue executing as normal...
	// Initialize global randomness
	rand.Seed(time.Now().UnixNano())

	action := os.Args[1]

	switch action {
	case "serve":
		RunServer(logFile)
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
	authTokenSecret := pflag.String("auth-token-secret", "", "Auth token for accessing the server")
	port := pflag.Int("port", 8000, "Port of the server")
	host := pflag.String("host", "0.0.0.0", "Host of the server")
	tlsCertPath := pflag.String("tls-cert-path", "server.crt", "TLS Certificate path")
	tlsKeyPath := pflag.String("tls-key-path", "server.key", "TLS Private key path")
	statsdHost := pflag.String("statsd-host", "", "Metrics Host")
	statsdPort := pflag.String("statsd-port", "", "Metrics port")
	statsdNamespace := pflag.String("statsd-namespace", "agent.prod", "The prefix to be added to every StatsD metric")

	pflag.Parse()

	if *authTokenSecret == "" {
		log.Fatal("Auth token is empty")
	}

	if *statsdHost != "" && *statsdPort != "" {
		// Initialize watchman
		err := watchman.Configure(*statsdHost, *statsdPort, *statsdNamespace)
		if err != nil {
			log.Printf("(err) Failed to configure statsd connection with watchman. Error: %s", err.Error())
		}
	}

	server.NewServer(
		*host,
		*port,
		*tlsCertPath,
		*tlsKeyPath,
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

func panicHandler(output string) {
	log.Printf("Child agent process panicked:\n\n%s\n", output)
	os.Exit(1)
}
