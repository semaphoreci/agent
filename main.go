package main

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	jobs "github.com/semaphoreci/agent/pkg/jobs"
	server "github.com/semaphoreci/agent/pkg/server"
	pflag "github.com/spf13/pflag"

	daemon "github.com/sevlyar/go-daemon"
)

var VERSION = "dev"

// command line flags
var daemonizeFlag bool
var authTokenSecret string
var port int
var host string
var tlsCertPath string
var tlsKeyPath string

func main() {
	pflag.BoolVarP(&daemonizeFlag, "daemonize", "d", false, "Daemonize the agent")
	pflag.StringVar(&authTokenSecret, "auth-token-secret", "", "Auth token for accessing the server")
	pflag.IntVar(&port, "port", 8000, "Port of the server")
	pflag.StringVar(&host, "host", "0.0.0.0", "Host of the server")
	pflag.StringVar(&tlsCertPath, "tls-cert-path", "server.crt", "TLS Certificate path")
	pflag.StringVar(&tlsKeyPath, "tls-key-path", "server.key", "TLS Private key path")

	pflag.Parse()

	if daemonizeFlag {
		daemonize()
	}

	// Initialize global randomness
	rand.Seed(time.Now().UnixNano())

	logfile := OpenLogfile()
	log.SetOutput(logfile)
	log.SetFlags(log.Ldate | log.Lmicroseconds | log.Lshortfile)

	action := os.Args[1]
	switch action {
	case "serve":
		RunServer(logfile)
	case "run":
		RunSingleJob()
	case "version":
		fmt.Println(VERSION)
	}
}

func daemonize() {
	context := &daemon.Context{
		PidFileName: "/tmp/agent.pid",
		PidFilePerm: 0644,
		WorkDir:     "./",
	}

	child, _ := context.Reborn()

	if child != nil {
		os.Exit(0) // close the parent
	} else {
		defer context.Release()
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
	if authTokenSecret == "" {
		log.Fatal("Auth token is empty")
	}

	server.NewServer(
		host,
		port,
		tlsCertPath,
		tlsKeyPath,
		VERSION,
		logfile,
		[]byte(authTokenSecret),
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
