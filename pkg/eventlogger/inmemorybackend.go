package eventlogger

import (
	"io"
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

func (l *InMemoryBackend) Read(startFrom, maxLines int, writer io.Writer) (int, error) {
	return 0, nil
}

func (l *InMemoryBackend) Write(event interface{}) error {
	l.Events = append(l.Events, event)

	return nil
}

func (l *InMemoryBackend) Close() error {
	return nil
}

func (l *InMemoryBackend) CloseWithOptions(options CloseOptions) error {
	return nil
}

func (l *InMemoryBackend) SimplifiedEvents(includeOutput, useSingleOutputEvent bool) ([]string, error) {
	return SimplifyLogEvents(l.Events, SimplifyOptions{
		IncludeOutput:          includeOutput,
		UseSingleItemForOutput: useSingleOutputEvent,
	})
}

func (l *InMemoryBackend) SimplifiedEventsWithoutDockerPull() ([]string, error) {
	logs, err := l.SimplifiedEvents(true, false)
	if err != nil {
		return []string{}, err
	}

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

	return append([]string{logs[start]}, logs[end:]...), nil
}
