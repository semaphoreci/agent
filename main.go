package main

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	watchman "github.com/renderedtext/go-watchman"
	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	"github.com/semaphoreci/agent/pkg/eventlogger"
	jobs "github.com/semaphoreci/agent/pkg/jobs"
	listener "github.com/semaphoreci/agent/pkg/listener"
	server "github.com/semaphoreci/agent/pkg/server"
	log "github.com/sirupsen/logrus"
	pflag "github.com/spf13/pflag"
)

var VERSION = "dev"

func main() {
	// Initialize global randomness
	rand.Seed(time.Now().UnixNano())

	action := os.Args[1]

	logfile := OpenLogfile()
	log.SetOutput(logfile)
	log.SetFormatter(new(eventlogger.CustomFormatter))
	log.SetLevel(getLogLevel())

	httpClient := &http.Client{}

	switch action {
	case "start":
		RunListener(httpClient, logfile)
	case "serve":
		RunServer(httpClient, logfile)
	case "run":
		RunSingleJob(httpClient)
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

func getLogLevel() log.Level {
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		return log.InfoLevel
	}

	level, err := log.ParseLevel(logLevel)
	if err != nil {
		log.Fatalf("Log level %s not supported", logLevel)
	}

	return level
}

func RunListener(httpClient *http.Client, logfile io.Writer) {
	endpoint := pflag.String("endpoint", "", "Endpoint where agents are registered")
	token := pflag.String("token", "", "Registration token")
	noHTTPS := pflag.Bool("no-https", false, "Use http for communication")
	shutdownHookPath := pflag.String("shutdown-hook-path", "", "Shutdown hook path")
	disconnectAfterJob := pflag.Bool("disconnect-after-job", false, "Disconnect after job")
	envVars := pflag.StringSlice("env-vars", []string{}, "Export environment variables in jobs")
	files := pflag.StringSlice("files", []string{}, "Inject files into container, when using docker compose executor")
	failOnMissingFiles := pflag.Bool("fail-on-missing-files", false, "Fail job if files specified using --files are missing")

	pflag.Parse()

	scheme := "https"
	if *noHTTPS {
		scheme = "http"
	}

	hostEnvVars, err := ParseEnvVars(*envVars)
	if err != nil {
		log.Fatalf("Error parsing --env-vars: %v", err)
	}

	fileInjections, err := ParseFiles(*files)
	if err != nil {
		log.Fatalf("Error parsing --files: %v", err)
	}

	config := listener.Config{
		Endpoint:           *endpoint,
		RegisterRetryLimit: 30,
		Token:              *token,
		Scheme:             scheme,
		ShutdownHookPath:   *shutdownHookPath,
		DisconnectAfterJob: *disconnectAfterJob,
		EnvVars:            hostEnvVars,
		FileInjections:     fileInjections,
		FailOnMissingFiles: *failOnMissingFiles,
		AgentVersion:       VERSION,
	}

	go func() {
		_, err := listener.Start(httpClient, config, logfile)
		if err != nil {
			log.Panicf("Could not start agent: %v", err)
		}
	}()

	select {}
}

func ParseEnvVars(envVars []string) ([]config.HostEnvVar, error) {
	vars := []config.HostEnvVar{}
	for _, envVar := range envVars {
		nameAndValue := strings.Split(envVar, "=")
		if len(nameAndValue) != 2 {
			return nil, fmt.Errorf("%s is not a valid environment variable", envVar)
		}

		vars = append(vars, config.HostEnvVar{
			Name:  nameAndValue[0],
			Value: nameAndValue[1],
		})
	}

	return vars, nil
}

func ParseFiles(files []string) ([]config.FileInjection, error) {
	fileInjections := []config.FileInjection{}
	for _, file := range files {
		hostPathAndDestination := strings.Split(file, ":")
		if len(hostPathAndDestination) != 2 {
			return nil, fmt.Errorf("%s is not a valid file injection", file)
		}

		fileInjections = append(fileInjections, config.FileInjection{
			HostPath:    hostPathAndDestination[0],
			Destination: hostPathAndDestination[1],
		})
	}

	return fileInjections, nil
}

func RunServer(httpClient *http.Client, logfile io.Writer) {
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
			log.Errorf("Failed to configure statsd connection with watchman. Error: %s", err.Error())
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
		httpClient,
	).Serve()
}

func RunSingleJob(httpClient *http.Client) {
	request, err := api.NewRequestFromYamlFile(os.Args[2])

	if err != nil {
		panic(err)
	}

	job, err := jobs.NewJobWithOptions(&jobs.JobOptions{
		Request:         request,
		Client:          httpClient,
		ExposeKvmDevice: true,
		FileInjections:  []config.FileInjection{},
	})

	if err != nil {
		panic(err)
	}

	job.JobLogArchived = true

	job.Run()
}
