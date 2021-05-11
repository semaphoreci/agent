package listener

import (
	"fmt"
	"io"

	selfhostedapi "github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
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
	Token string
}

func Start(config Config, logger io.Writer) (*Listener, error) {
	listener := &Listener{
		Config: config,
		Client: selfhostedapi.New(config.Endpoint, config.Token),
	}

	listener.DisplayHelloMessage()

	fmt.Println("* Starting Agent")
	fmt.Println("* Registering Agent")
	err := listener.Register()
	if err != nil {
		return listener, err
	}

	fmt.Println("* Starting to Send HearthBeats")
	hbEndpoint := "http://" + listener.Config.Endpoint + "/api/v1/self_hosted_agents/hearthbeat"
	hearthbeater, err := StartHeartBeater(hbEndpoint)
	if err != nil {
		return listener, err
	}

	fmt.Println("* Starting to poll for jobs")
	jobProcessor, err := StartJobProcessor(listener.Config.Endpoint)
	if err != nil {
		return listener, err
	}

	listener.HearthBeater = hearthbeater
	listener.JobProcessor = jobProcessor

	fmt.Println("* Acquiring job...")

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

func (l *Listener) Register() error {
	req := &selfhostedapi.RegisterRequest{}

	for i := 0; i < l.Config.RegisterRetryLimit; i++ {
		resp, err := l.Client.Register(req)
		if err != nil {
			fmt.Println(err)
			continue
		}

		fmt.Println(resp)
		return nil
	}

	return fmt.Errorf("failed to register agent")
}
