package eventlogger

import (
	"fmt"
	"strings"
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

func (l *InMemoryBackend) SimplifiedEvents(includeOutput bool) []string {
	events := []string{}

	for _, event := range l.Events {
		switch e := event.(type) {
		case *JobStartedEvent:
			events = append(events, "job_started")
		case *JobFinishedEvent:
			events = append(events, "job_finished: "+e.Result)
		case *CommandStartedEvent:
			events = append(events, "directive: "+e.Directive)
		case *CommandOutputEvent:
			if includeOutput {
				events = append(events, e.Output)
			}
		case *CommandFinishedEvent:
			events = append(events, fmt.Sprintf("Exit Code: %d", e.ExitCode))
		default:
			panic("Unknown shell event")
		}
	}

	return events
}

func (l *InMemoryBackend) SimplifiedEventsWithoutDockerPull() []string {
	logs := l.SimplifiedEvents(true)

	start := 0

	for i, l := range logs {
		if strings.Contains(l, "Pulling docker images") {
			start = i
			break
		}
	}

	end := start

	for i, l := range logs[start:] {
		if strings.Contains(l, "Exit Code") {
			end = i
			break
		}
	}

	return append([]string{logs[start]}, logs[end:]...)
}
