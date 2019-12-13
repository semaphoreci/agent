package eventlogger

import (
	"fmt"
)

type InMemoryBackend struct {
	Events []interface{}
}

func NewInMemoryBackend() (*InMemoryBackend, error) {
	return &InMemoryBackend{}, nil
}

func (l *InMemoryBackend) Open() error {
	return nil
}

func (l *InMemoryBackend) Write(event interface{}) error {
	l.Events = append(l.Events, event)

	return nil
}

func (l *InMemoryBackend) Close() error {
	return nil
}

func (l *InMemoryBackend) SimplifiedEvents() []string {
	events := []string{}

	for _, event := range l.Events {
		switch e := event.(type) {
		case *JobStartedEvent:
			events = append(events, "job_started")
		case *JobFinishedEvent:
			events = append(events, "job_finished")
		case *CommandStartedEvent:
			events = append(events, "directive: "+e.Directive)
		case *CommandOutputEvent:
			events = append(events, e.Output)
		case *CommandFinishedEvent:
			events = append(events, fmt.Sprintf("Exit Code: %d", e.ExitCode))
		default:
			panic("Unknown shell event")
		}
	}

	return events
}
