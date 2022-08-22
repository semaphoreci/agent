package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/mitchellh/panicwrap"
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
	logfile := OpenLogfile()
	log.SetOutput(logfile)
	log.SetFormatter(new(eventlogger.CustomFormatter))
	log.SetLevel(getLogLevel())

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
	// #nosec
	f, err := os.OpenFile(getLogFilePath(), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)

	if err != nil {
		log.Fatal(err)
	}

	return io.MultiWriter(f, os.Stdout)
}

func getLogLevel() log.Level {
	logLevel := os.Getenv("SEMAPHORE_AGENT_LOG_LEVEL")
	if logLevel == "" {
		return log.InfoLevel
	}

	level, err := log.ParseLevel(logLevel)
	if err != nil {
		log.Fatalf("Log level %s not supported", logLevel)
	}

	return level
}

func getLogFilePath() string {
	logFilePath := os.Getenv("SEMAPHORE_AGENT_LOG_FILE_PATH")
	if logFilePath == "" {
		return filepath.Join(os.TempDir(), "agent_log")
	}

	parentDirectory := path.Dir(logFilePath)
	err := os.MkdirAll(parentDirectory, 0644)
	if err != nil {
		log.Panicf("Could not create directories to place log file in '%s': %v", logFilePath, err)
	}

	return logFilePath
}

func RunListener(httpClient *http.Client, logfile io.Writer) {
	configFile := pflag.String(config.ConfigFile, "", "Config file")
	_ = pflag.String(config.Name, "", "Name to use for the agent. If not set, a default random one is used.")
	_ = pflag.String(config.Endpoint, "", "Endpoint where agents are registered")
	_ = pflag.String(config.Token, "", "Registration token")
	_ = pflag.Bool(config.NoHTTPS, false, "Use http for communication")
	_ = pflag.String(config.ShutdownHookPath, "", "Shutdown hook path")
	_ = pflag.String(config.PreJobHookPath, "", "Pre-job hook path")
	_ = pflag.Bool(config.DisconnectAfterJob, false, "Disconnect after job")
	_ = pflag.Int(config.DisconnectAfterIdleTimeout, 0, "Disconnect after idle timeout, in seconds")
	_ = pflag.StringSlice(config.EnvVars, []string{}, "Export environment variables in jobs")
	_ = pflag.StringSlice(config.Files, []string{}, "Inject files into container, when using docker compose executor")
	_ = pflag.Bool(config.FailOnMissingFiles, false, "Fail job if files specified using --files are missing")
	_ = pflag.Bool(config.FailOnPreJobHookError, false, "Fail job if pre-job hook fails")

	pflag.Parse()

	if *configFile != "" {
		loadConfigFile(*configFile)
	}

	err := viper.BindPFlags(pflag.CommandLine)
	if err != nil {
		log.Fatalf("Error binding pflags: %v", err)
	}

	validateConfiguration()

	if viper.GetString(config.Endpoint) == "" {
		log.Fatal("Semaphore endpoint was not specified. Exiting...")
	}

	if viper.GetString(config.Token) == "" {
		log.Fatal("Agent registration token was not specified. Exiting...")
	}

	if viper.GetInt(config.DisconnectAfterIdleTimeout) < 0 {
		log.Fatal("Idle timeout can't be negative. Exiting...")
	}

	scheme := "https"
	if viper.GetBool(config.NoHTTPS) {
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

	agentName := getAgentName()
	formatter := eventlogger.CustomFormatter{AgentName: agentName}
	log.SetFormatter(&formatter)

	config := listener.Config{
		AgentName:                  agentName,
		Endpoint:                   viper.GetString(config.Endpoint),
		Token:                      viper.GetString(config.Token),
		RegisterRetryLimit:         30,
		GetJobRetryLimit:           10,
		CallbackRetryLimit:         60,
		Scheme:                     scheme,
		ShutdownHookPath:           viper.GetString(config.ShutdownHookPath),
		PreJobHookPath:             viper.GetString(config.PreJobHookPath),
		DisconnectAfterJob:         viper.GetBool(config.DisconnectAfterJob),
		DisconnectAfterIdleSeconds: viper.GetInt(config.DisconnectAfterIdleTimeout),
		EnvVars:                    hostEnvVars,
		FileInjections:             fileInjections,
		FailOnMissingFiles:         viper.GetBool(config.FailOnMissingFiles),
		FailOnPreJobHookError:      viper.GetBool(config.FailOnPreJobHookError),
		AgentVersion:               VERSION,
		ExitOnShutdown:             true,
	}

	go func() {
		_, err := listener.Start(httpClient, config)
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

func validateConfiguration() {
	contains := func(list []string, item string) bool {
		for _, x := range list {
			if x == item {
				return true
			}
		}

		return false
	}

	for _, key := range viper.AllKeys() {
		if !contains(config.ValidConfigKeys, key) {
			log.Fatalf("Unrecognized option '%s'. Exiting...", key)
		}
	}
}

func getAgentName() string {
	agentName := viper.GetString(config.Name)
	if agentName != "" {
		if len(agentName) < 8 || len(agentName) > 64 {
			log.Fatalf("The agent name should have between 8 and 64 characters. '%s' has %d.", agentName, len(agentName))
		}

		return agentName
	}

	log.Infof("Agent name was not assigned - using a random one.")
	randomName, err := randomName()
	if err != nil {
		log.Fatalf("Error generating name for agent: %v", err)
	}

	return randomName
}

func ParseEnvVars() ([]config.HostEnvVar, error) {
	vars := []config.HostEnvVar{}
	for _, envVar := range viper.GetStringSlice(config.EnvVars) {
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
	for _, file := range viper.GetStringSlice(config.Files) {
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

func panicHandler(output string) {
	log.Printf("Child agent process panicked:\n\n%s\n", output)
	os.Exit(1)
}

// base64 gives you 4 chars every 3 bytes, we want 20 chars, so 15 bytes
const nameLength = 15

func randomName() (string, error) {
	buffer := make([]byte, nameLength)
	_, err := rand.Read(buffer)

	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(buffer), nil
}
