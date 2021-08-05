package listener

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/semaphoreci/agent/pkg/config"
	selfhostedapi "github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
	"github.com/semaphoreci/agent/pkg/retry"
	log "github.com/sirupsen/logrus"
)

type Listener struct {
	JobProcessor *JobProcessor
	Config       Config
	Client       *selfhostedapi.Api
}

type Config struct {
	Endpoint           string
	RegisterRetryLimit int
	Token              string
	Scheme             string
	ShutdownHookPath   string
	DisconnectAfterJob bool
	EnvVars            []config.HostEnvVar
}

func Start(httpClient *http.Client, config Config, logger io.Writer) (*Listener, error) {
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

func (l *Listener) Name() string {
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	randBytes := make([]byte, 10)

	_, err = rand.Read(randBytes)
	if err != nil {
		panic(err)
	}

	randSuffix := fmt.Sprintf("%x", randBytes)

	return "sh-" + hostname + "-" + randSuffix
}

func (l *Listener) Register() error {
	req := &selfhostedapi.RegisterRequest{
		Name: l.Name(),
		OS:   "Ubuntu",
	}

	err := retry.RetryWithConstantWait("Register", l.Config.RegisterRetryLimit, time.Second, func() error {
		resp, err := l.Client.Register(req)
		if err != nil {
			return err
		} else {
			l.Client.SetAccessToken(resp.Token)
			return nil
		}
	})

	if err != nil {
		return fmt.Errorf("failed to register agent: %v", err)
	}

	return nil
}
