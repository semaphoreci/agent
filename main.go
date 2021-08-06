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
	"github.com/spf13/viper"
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
	if logLevel != "" {
		level, err := log.ParseLevel(logLevel)
		if err != nil {
			log.Fatalf("Log level %s not supported", logLevel)
		}

		return level
	} else {
		return log.InfoLevel
	}
}

func RunListener(httpClient *http.Client, logfile io.Writer) {
	configFile := pflag.String(config.CONFIG_FILE, "", "Config file")
	_ = pflag.String(config.ENDPOINT, "", "Endpoint where agents are registered")
	_ = pflag.String(config.TOKEN, "", "Registration token")
	_ = pflag.Bool(config.NO_HTTPS, false, "Use http for communication")
	_ = pflag.String(config.SHUTDOWN_HOOK_PATH, "", "Shutdown hook path")
	_ = pflag.Bool(config.DISCONNECT_AFTER_JOB, false, "Disconnect after job")
	_ = pflag.StringSlice(config.ENV_VARS, []string{}, "Export environment variables in jobs")
	_ = pflag.StringSlice(config.FILES, []string{}, "Inject files into container, when using docker compose executor")
	_ = pflag.Bool(config.FAIL_ON_MISSING_FILES, false, "Fail job if files specified using --files are missing")

	pflag.Parse()

	if *configFile != "" {
		loadConfigFile(*configFile)
	}

	viper.BindPFlags(pflag.CommandLine)

	if viper.GetString(config.ENDPOINT) == "" {
		log.Fatal("Semaphore endpoint was not specified. Exiting...")
	}

	if viper.GetString(config.TOKEN) == "" {
		log.Fatal("Agent registration token was not specified. Exiting...")
	}

	scheme := "https"
	if viper.GetBool(config.NO_HTTPS) {
		scheme = "http"
	}

	hostEnvVars, err := ParseEnvVars()
	if err != nil {
		log.Fatalf("Error parsing --env-vars: %v", err)
	}

	fileInjections, err := ParseFiles()
	if err != nil {
		log.Fatalf("Error parsing --files: %v", err)
	}

	config := listener.Config{
		Endpoint:           viper.GetString(config.ENDPOINT),
		Token:              viper.GetString(config.TOKEN),
		RegisterRetryLimit: 30,
		Scheme:             scheme,
		ShutdownHookPath:   viper.GetString(config.SHUTDOWN_HOOK_PATH),
		DisconnectAfterJob: viper.GetBool(config.DISCONNECT_AFTER_JOB),
		EnvVars:            hostEnvVars,
		FileInjections:     fileInjections,
		FailOnMissingFiles: viper.GetBool(config.FAIL_ON_MISSING_FILES),
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

func loadConfigFile(configFile string) {
	viper.SetConfigFile(configFile)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Fatalf("Couldn't find config file %s: %v", configFile, err)
		} else {
			log.Fatalf("Error reading config file %s: %v", configFile, err)
		}
	}
}

func ParseEnvVars() ([]config.HostEnvVar, error) {
	vars := []config.HostEnvVar{}
	for _, envVar := range viper.GetStringSlice(config.ENV_VARS) {
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

func ParseFiles() ([]config.FileInjection, error) {
	fileInjections := []config.FileInjection{}
	for _, file := range viper.GetStringSlice(config.FILES) {
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
