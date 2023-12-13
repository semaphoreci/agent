package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
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
	"github.com/semaphoreci/agent/pkg/kubernetes"
	listener "github.com/semaphoreci/agent/pkg/listener"
	server "github.com/semaphoreci/agent/pkg/server"
	slices "github.com/semaphoreci/agent/pkg/slices"
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
	err := os.MkdirAll(parentDirectory, 0640)
	if err != nil {
		log.Panicf("Could not create directories to place log file in '%s': %v", logFilePath, err)
	}

	return logFilePath
}

func RunListener(httpClient *http.Client, logfile io.Writer) {
	configFile := pflag.String(config.ConfigFile, "", "Config file")
	_ = pflag.String(config.Name, "", "Name to use for the agent. If not set, a default random one is used.")
	_ = pflag.String(config.NameFromEnv, "", "Specify name to use for the agent, using an environment variable. If --name and --name-from-env are empty, a random one is generated.")
	_ = pflag.String(config.Endpoint, "", "Endpoint where agents are registered")
	_ = pflag.String(config.Token, "", "Registration token")
	_ = pflag.Bool(config.NoHTTPS, false, "Use http for communication")
	_ = pflag.String(config.ShutdownHookPath, "", "Shutdown hook path")
	_ = pflag.String(config.PreJobHookPath, "", "Pre-job hook path")
	_ = pflag.String(config.PostJobHookPath, "", "Post-job hook path")
	_ = pflag.Bool(config.DisconnectAfterJob, false, "Disconnect after job")
	_ = pflag.String(config.RunJob, "", "Request a specific job to run")
	_ = pflag.Int(config.DisconnectAfterIdleTimeout, 0, "Disconnect after idle timeout, in seconds")
	_ = pflag.Int(config.InterruptionGracePeriod, 0, "The grace period, in seconds, to wait after receiving an interrupt signal")
	_ = pflag.StringSlice(config.EnvVars, []string{}, "Export environment variables in jobs")
	_ = pflag.StringSlice(config.Files, []string{}, "Inject files into container, when using docker compose executor")
	_ = pflag.Bool(config.FailOnMissingFiles, false, "Fail job if files specified using --files are missing")
	_ = pflag.String(config.UploadJobLogs, config.UploadJobLogsConditionNever, "When should the agent upload the job logs as a job artifact. Default is never.")
	_ = pflag.Bool(config.FailOnPreJobHookError, false, "Fail job if pre-job hook fails")
	_ = pflag.Bool(config.SourcePreJobHook, false, "Execute pre-job hook in the current shell (using 'source <script>') instead of in a new shell (using 'bash <script>')")
	_ = pflag.Bool(config.KubernetesExecutor, false, "Use Kubernetes executor")
	_ = pflag.String(config.KubernetesPodSpec, "", "Use a Kubernetes configmap to decorate the pod created to run the Semaphore job")
	_ = pflag.StringSlice(config.KubernetesAllowedImages, []string{}, "List of regexes for allowed images to use for the Kubernetes executor")
	_ = pflag.Int(
		config.KubernetesPodStartTimeout,
		config.DefaultKubernetesPodStartTimeout,
		fmt.Sprintf("Timeout for the pod to be ready, in seconds. Default is %d.", config.DefaultKubernetesPodStartTimeout),
	)

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

	if viper.GetInt(config.KubernetesPodStartTimeout) < 0 {
		log.Fatal("Kubernetes pod start timeout can't be negative. Exiting...")
	}

	scheme := "https"
	if viper.GetBool(config.NoHTTPS) {
		scheme = "http"
	}

	hostEnvVars, err := ParseEnvVars()
	if err != nil {
		log.Fatalf("Error parsing --env-vars: %v", err)
	}

	fileInjections, err := ParseFiles(viper.GetStringSlice(config.Files))
	if err != nil {
		log.Fatalf("Error parsing --files: %v", err)
	}

	config := listener.Config{
		AgentName:                        getAgentName(),
		Endpoint:                         viper.GetString(config.Endpoint),
		Token:                            viper.GetString(config.Token),
		RegisterRetryLimit:               30,
		GetJobRetryLimit:                 10,
		CallbackRetryLimit:               60,
		Scheme:                           scheme,
		ShutdownHookPath:                 viper.GetString(config.ShutdownHookPath),
		PreJobHookPath:                   viper.GetString(config.PreJobHookPath),
		PostJobHookPath:                  viper.GetString(config.PostJobHookPath),
		DisconnectAfterJob:               viper.GetBool(config.DisconnectAfterJob),
		RunJob:                           viper.GetString(config.RunJob),
		DisconnectAfterIdleSeconds:       viper.GetInt(config.DisconnectAfterIdleTimeout),
		InterruptionGracePeriod:          viper.GetInt(config.InterruptionGracePeriod),
		EnvVars:                          hostEnvVars,
		FileInjections:                   fileInjections,
		FailOnMissingFiles:               viper.GetBool(config.FailOnMissingFiles),
		UploadJobLogs:                    viper.GetString(config.UploadJobLogs),
		FailOnPreJobHookError:            viper.GetBool(config.FailOnPreJobHookError),
		SourcePreJobHook:                 viper.GetBool(config.SourcePreJobHook),
		AgentVersion:                     VERSION,
		ExitOnShutdown:                   true,
		KubernetesExecutor:               viper.GetBool(config.KubernetesExecutor),
		KubernetesPodSpec:                viper.GetString(config.KubernetesPodSpec),
		KubernetesImageValidator:         createImageValidator(viper.GetStringSlice(config.KubernetesAllowedImages)),
		KubernetesPodStartTimeoutSeconds: viper.GetInt(config.KubernetesPodStartTimeout),
	}

	go func() {
		_, err := listener.Start(httpClient, config)
		if err != nil {
			log.Panicf("Could not start agent: %v", err)
		}
	}()

	select {}
}

func createImageValidator(expressions []string) *kubernetes.ImageValidator {
	imageValidator, err := kubernetes.NewImageValidator(expressions)
	if err != nil {
		log.Panicf("Error creating image validator: %v", err)
	}

	return imageValidator
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
	for _, key := range viper.AllKeys() {
		if !slices.Contains(config.ValidConfigKeys, key) {
			log.Fatalf("Unrecognized option '%s'. Exiting...", key)
		}
	}

	if viper.GetString(config.RunJob) != "" && !viper.GetBool(config.DisconnectAfterJob) {
		log.Fatalf("%s can only be used if %s is also used. Exiting...", config.RunJob, config.DisconnectAfterJob)
	}

	uploadJobLogs := viper.GetString(config.UploadJobLogs)
	if !slices.Contains(config.ValidUploadJobLogsCondition, uploadJobLogs) {
		log.Fatalf(
			"Unsupported value '%s' for '%s'. Allowed values are: %v. Exiting...",
			uploadJobLogs,
			config.UploadJobLogs,
			config.ValidUploadJobLogsCondition,
		)
	}
}

func getAgentName() string {
	// --name configuration parameter was specified.
	agentName := viper.GetString(config.Name)
	if agentName != "" {
		if err := validateAgentName(agentName); err != nil {
			log.Fatalf("Agent name validation failed: %v", err)
		}

		return agentName
	}

	// --name-from-env configuration parameter was passed.
	// We need to fetch the actual name from the environment variable.
	envVarName := viper.GetString(config.NameFromEnv)
	if envVarName != "" {
		agentName := os.Getenv(envVarName)
		if err := validateAgentName(agentName); err != nil {
			log.Fatalf("Agent name validation failed: %v", err)
		}

		return agentName
	}

	// No name was specified - we generate a random one.
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
	preJobHookPath := pflag.String(config.PreJobHookPath, "", "The path to a pre-job hook script")
	files := pflag.StringSlice(config.Files, []string{}, "Inject files into container, when using docker compose executor")

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

	fileInjections, err := ParseFiles(*files)
	if err != nil {
		log.Fatalf("Error parsing --files: %v", err)
	}

	server.NewServer(server.ServerConfig{
		Host:           *host,
		Port:           *port,
		TLSCertPath:    *tlsCertPath,
		TLSKeyPath:     *tlsKeyPath,
		Version:        VERSION,
		LogFile:        logfile,
		JWTSecret:      []byte(*authTokenSecret),
		HTTPClient:     httpClient,
		PreJobHookPath: *preJobHookPath,
		FileInjections: fileInjections,
	}).Serve()
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

	// #nosec
	_, err := rand.Read(buffer)

	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(buffer), nil
}

func validateAgentName(name string) error {
	// Do not apply length restriction on URLs.
	if _, err := url.ParseRequestURI(name); err == nil {
		return nil
	}

	// Not a URL - apply length restriction.
	if len(name) < 8 || len(name) > 80 {
		return fmt.Errorf("name should have between 8 and 80 characters. '%s' has %d", name, len(name))
	}

	return nil
}
