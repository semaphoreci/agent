package listener

import (
	"fmt"
	"io"
)

type Listener struct {
	HearthBeater *HearthBeater
	JobProcessor *JobProcessor
}

func Start(logger io.Writer) (*Listener, error) {
	listener := &Listener{}
	listener.DisplayHelloMessage()

	fmt.Println("* Starting Agent")
	fmt.Println("* Registering Agent")
	err := listener.Register()
	if err != nil {
		return listener, err
	}

	fmt.Println("* Starting to Send HearthBeats")
	hearthbeater, err := StartHeartBeater()
	if err != nil {
		return listener, err
	}

	fmt.Println("* Starting to poll for jobs")
	jobProcessor, err := StartJobProcessor()
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
	fmt.Println("111111111    0   111111111     00     ")
	fmt.Println("  1111111111   1111111111             ")
	fmt.Println("    1111111111111111111               ")
	fmt.Println("      111111111111111                 ")
	fmt.Println("         111111111                    ")
	fmt.Println("                                      ")
}

func (l *Listener) Register() error {
	return nil
}
