package listener

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"time"

	selfhostedapi "github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
	log "github.com/sirupsen/logrus"
)

type Listener struct {
	HearthBeater *HearthBeater
	JobProcessor *JobProcessor
	Config       Config
	Client       *selfhostedapi.Api
}

type Config struct {
	Endpoint           string
	RegisterRetryLimit int
	Token              string
	Scheme             string
}

func Start(config Config, logger io.Writer) (*Listener, error) {
	listener := &Listener{
		Config: config,
		Client: selfhostedapi.New(config.Scheme, config.Endpoint, config.Token),
	}

	listener.DisplayHelloMessage()

	log.Info("Starting Agent")
	log.Info("Registering Agent")
	err := listener.Register()
	if err != nil {
		return listener, err
	}

	// fmt.Println("* Starting to Send HearthBeats")
	// hbEndpoint := "http://" + listener.Config.Endpoint + "/api/v1/self_hosted_agents/hearthbeat"
	// hearthbeater, err := StartHeartBeater(hbEndpoint)
	// if err != nil {
	// 	return listener, err
	// }

	log.Info("Starting to poll for jobs")
	jobProcessor, err := StartJobProcessor(listener.Client)
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

	for i := 0; i < l.Config.RegisterRetryLimit; i++ {
		resp, err := l.Client.Register(req)
		if err != nil {
			log.Error(err)
			time.Sleep(1 * time.Second)
			continue
		}

		l.Client.SetAccessToken(resp.Token)

		return nil
	}

	return fmt.Errorf("failed to register agent")
}
