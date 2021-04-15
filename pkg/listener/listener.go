package listener

import (
	"io"
)

type Listener struct {
	HearthBeater *HearthBeater
	JobProcessor *JobProcessor
}

func Start(logger io.Writer) (*Listener, error) {
	listener := &Listener{}

	err := listener.Register()
	if err != nil {
		return listener, err
	}

	hearthbeater, err := StartHeartBeater()
	if err != nil {
		return listener, err
	}

	jobProcessor, err := StartJobProcessor()
	if err != nil {
		return listener, err
	}

	listener.HearthBeater = hearthbeater
	listener.JobProcessor = jobProcessor

	return listener, nil
}

func (l *Listener) Register() error {
	return nil
}
