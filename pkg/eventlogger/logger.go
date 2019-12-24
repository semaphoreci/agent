package eventlogger

import "time"

type Logger struct {
	Backend Backend
}

func NewLogger(backend Backend) (*Logger, error) {
	return &Logger{Backend: backend}, nil
}

func (l *Logger) Open() error {
	return l.Backend.Open()
}

func (l *Logger) Close() error {
	return l.Backend.Close()
}

func (l *Logger) LogJobStarted() error {
	event := &JobStartedEvent{
		Timestamp: int(time.Now().Unix()),
		Event:     "job_started",
	}

	return l.Backend.Write(event)
}

func (l *Logger) LogJobFinished(result string) error {
	event := &JobFinishedEvent{
		Timestamp: int(time.Now().Unix()),
		Event:     "job_finished",
		Result:    result,
	}

	return l.Backend.Write(event)
}

func (l *Logger) LogCommandStarted(directive string) error {
	event := &CommandStartedEvent{
		Timestamp: int(time.Now().Unix()),
		Event:     "cmd_started",
		Directive: directive,
	}

	return l.Backend.Write(event)
}

func (l *Logger) LogCommandOutput(output string) error {
	event := &CommandOutputEvent{
		Timestamp: int(time.Now().Unix()),
		Event:     "cmd_output",
		Output:    output,
	}

	return l.Backend.Write(event)
}

func (l *Logger) LogCommandFinished(directive string, exitCode int, startedAt int, finishedAt int) error {
	event := &CommandFinishedEvent{
		Timestamp:  int(time.Now().Unix()),
		Event:      "cmd_finished",
		Directive:  directive,
		ExitCode:   exitCode,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}

	return l.Backend.Write(event)
}
