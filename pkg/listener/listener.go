package listener

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/semaphoreci/agent/pkg/config"
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
	Endpoint                   string
	RegisterRetryLimit         int
	GetJobRetryLimit           int
	CallbackRetryLimit         int
	Token                      string
	Scheme                     string
	ShutdownHookPath           string
	PreJobHookPath             string
	DisconnectAfterJob         bool
	DisconnectAfterIdleSeconds int
	EnvVars                    []config.HostEnvVar
	FileInjections             []config.FileInjection
	FailOnMissingFiles         bool
	UploadTrimmedLogs          bool
	FailOnPreJobHookError      bool
	ExitOnShutdown             bool
	AgentVersion               string
}

func Start(httpClient *http.Client, config Config) (*Listener, error) {
	listener := &Listener{
		Config: config,
		Client: selfhostedapi.New(httpClient, config.Scheme, config.Endpoint, config.Token),
	}

	listener.DisplayHelloMessage()

	log.Info("Starting Agent")
	log.Info("Registering Agent")
	err := listener.Register()
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

// base64 gives you 4 chars every 3 bytes, we want 20 chars, so 15 bytes
const nameLength = 15

func (l *Listener) Name() (string, error) {
	buffer := make([]byte, nameLength)
	_, err := rand.Read(buffer)

	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(buffer), nil
}

func (l *Listener) Register() error {
	name, err := l.Name()
	if err != nil {
		log.Errorf("Error generating name for agent: %v", err)
		return err
	}

	req := &selfhostedapi.RegisterRequest{
		Version:     l.Config.AgentVersion,
		Name:        name,
		PID:         os.Getpid(),
		OS:          osinfo.Name(),
		Arch:        osinfo.Arch(),
		Hostname:    osinfo.Hostname(),
		SingleJob:   l.Config.DisconnectAfterJob,
		IdleTimeout: l.Config.DisconnectAfterIdleSeconds,
	}

	err = retry.RetryWithConstantWait(retry.RetryOptions{
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
