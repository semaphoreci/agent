package listener

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/semaphoreci/agent/pkg/config"
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
	DisconnectAfterJob               bool
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
	AgentName                        string
	KubernetesExecutor               bool
	KubernetesPodSpec                string
	KubernetesImageValidator         *kubernetes.ImageValidator
	KubernetesPodStartTimeoutSeconds int
}

func Start(httpClient *http.Client, config Config) (*Listener, error) {
	listener := &Listener{
		Config: config,
		Client: selfhostedapi.New(httpClient, config.Scheme, config.Endpoint, config.Token),
	}

	listener.DisplayHelloMessage()

	log.Info("Starting Agent")
	log.Info("Registering Agent")
	err := listener.Register(config.AgentName)
	if err != nil {
		return listener, err
	}

	log.Info("Starting to poll for jobs")
	jobProcessor, err := StartJobProcessor(httpClient, listener.Client, listener.Config)
	if err != nil {
		return listener, err
	}

	listener.JobProcessor = jobProcessor

	return listener, nil
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

			l.Client.SetAccessToken(resp.Token)
			return nil
		},
	})

	if err != nil {
		return fmt.Errorf("failed to register agent: %v", err)
	}

	return nil
}
