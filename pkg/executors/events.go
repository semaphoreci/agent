package executors

import "time"

type EventHandler func(interface{})

type CommandStartedEvent struct {
	Timestamp int
	Directive string
}

type CommandOutputEvent struct {
	Timestamp int
	Output    string
}

type CommandFinishedEvent struct {
	Timestamp  int
	ExitCode   int
	Directive  string
	StartedAt  int
	FinishedAt int
}

func NewCommandStartedEvent(directive string) *CommandStartedEvent {
	return &CommandStartedEvent{
		Timestamp: int(time.Now().Unix()),
		Directive: directive,
	}
}

func NewCommandOutputEvent(output string) *CommandOutputEvent {
	return &CommandOutputEvent{
		Timestamp: int(time.Now().Unix()),
		Output:    output,
	}
}

func NewCommandFinishedEvent(directive string, exitCode int, startedAt int, finishedAt int) *CommandFinishedEvent {
	return &CommandFinishedEvent{
		Timestamp:  int(time.Now().Unix()),
		Directive:  directive,
		ExitCode:   exitCode,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}
}
