package listener

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/semaphoreci/agent/pkg/config"
	"github.com/semaphoreci/agent/pkg/eventlogger"
	"github.com/semaphoreci/agent/pkg/kubernetes"
	selfhostedapi "github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
	osinfo "github.com/semaphoreci/agent/pkg/osinfo"
	"github.com/semaphoreci/agent/pkg/retry"
	log "github.com/sirupsen/logrus"
)

type Listener struct {
	JobProcessor *JobProcessor
	Config       Config
	Client       *selfhostedapi.API
}

type Config struct {
	Endpoint                         string
	RegisterRetryLimit               int
	GetJobRetryLimit                 int
	CallbackRetryLimit               int
	Token                            string
	Scheme                           string
	ShutdownHookPath                 string
	PreJobHookPath                   string
	PostJobHookPath                  string
	DisconnectAfterJob               bool
	JobID                            string
	DisconnectAfterIdleSeconds       int
	InterruptionGracePeriod          int
	EnvVars                          []config.HostEnvVar
	FileInjections                   []config.FileInjection
	FailOnMissingFiles               bool
	UploadJobLogs                    string
	FailOnPreJobHookError            bool
	SourcePreJobHook                 bool
	ExitOnShutdown                   bool
	AgentVersion                     string
	UserAgent                        string
	AgentName                        string
	KubernetesExecutor               bool
	KubernetesPodSpec                string
	KubernetesImageValidator         *kubernetes.ImageValidator
	KubernetesPodStartTimeoutSeconds int
	KubernetesLabels                 map[string]string
	KubernetesDefaultImage           string
}

func Start(httpClient *http.Client, config Config) (*Listener, error) {
	listener := &Listener{
		Config: config,
		Client: selfhostedapi.New(httpClient, config.Scheme, config.Endpoint, config.Token, config.UserAgent),
	}

	listener.DisplayHelloMessage()
	setCustomLogFormatter(config.AgentName)

	log.Info("Starting Agent")
	log.Info("Registering Agent")
	err := listener.Register(config.AgentName)
	if err != nil {
		return listener, err
	}

	// We re-set the agent name in the custom log formatter,
	// to ensure that names assigned by the Semaphore control plane
	// are also added to the agent's custom log formatter, after registration.
	setCustomLogFormatter(listener.Config.AgentName)

	log.Info("Starting to poll for jobs")
	jobProcessor, err := StartJobProcessor(httpClient, listener.Client, listener.Config)
	if err != nil {
		return listener, err
	}

	listener.JobProcessor = jobProcessor

	return listener, nil
}

func setCustomLogFormatter(agentName string) {
	// If the name is a URL, which will be followed by the Semaphore control plane.
	// The actual name used for the agent will be returned by the Semaphore
	// control in the registration response. So, while the name is a URL,
	// we initially the URL host in the log context until we get a name from the Semaphore control plane.
	if u, err := url.ParseRequestURI(agentName); err == nil {
		formatter := eventlogger.CustomFormatter{
			AgentName: fmt.Sprintf("[%s]", u.Host),
		}

		log.SetFormatter(&formatter)
		return
	}

	// If it's not a URL, just use the name itself.
	formatter := eventlogger.CustomFormatter{AgentName: agentName}
	log.SetFormatter(&formatter)
}

// only used during tests
func (l *Listener) Stop() {
	l.JobProcessor.Shutdown(ShutdownReasonRequested, 0)
}

// only used during tests
func (l *Listener) Interrupt() {
	l.JobProcessor.InterruptedAt = time.Now().Unix()
}

func (l *Listener) DisplayHelloMessage() {
	fmt.Println("                                      ")
	fmt.Println("                 00000000000          ")
	fmt.Println("               0000000000000000       ")
	fmt.Println("             00000000000000000000     ")
	fmt.Println("          00000000000    0000000000   ")
	fmt.Println("   11     00000000    11   000000000  ")
	fmt.Println(" 111111   000000   1111111   000000   ")
	fmt.Println("111111111   00   111111111     00     ")
	fmt.Println("  111111111    1111111111             ")
	fmt.Println("    1111111111111111111               ")
	fmt.Println("      111111111111111                 ")
	fmt.Println("         111111111                    ")
	fmt.Println("                                      ")
}

func (l *Listener) Register(name string) error {
	req := &selfhostedapi.RegisterRequest{
		Version:                 l.Config.AgentVersion,
		Name:                    name,
		PID:                     os.Getpid(),
		OS:                      osinfo.Name(),
		Arch:                    osinfo.Arch(),
		Hostname:                osinfo.Hostname(),
		SingleJob:               l.Config.DisconnectAfterJob,
		IdleTimeout:             l.Config.DisconnectAfterIdleSeconds,
		InterruptionGracePeriod: l.Config.InterruptionGracePeriod,
		JobID:                   l.Config.JobID,
	}

	err := retry.RetryWithConstantWait(retry.RetryOptions{
		Task:                 "Register",
		MaxAttempts:          l.Config.RegisterRetryLimit,
		DelayBetweenAttempts: time.Second,
		Fn: func() error {
			resp, err := l.Client.Register(req)
			if err != nil {
				return err
			}

			l.Config.AgentName = resp.Name
			l.Client.SetAccessToken(resp.Token)
			return nil
		},
	})

	if err != nil {
		return fmt.Errorf("failed to register agent: %v", err)
	}

	return nil
}
