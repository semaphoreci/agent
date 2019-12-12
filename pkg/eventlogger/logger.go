package eventlogger

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
	event := &JobStartedEvent{}

	return l.Backend.Write(event)
}

func (l *Logger) LogJobFinished(directive string) error {
	return nil
}

func (l *Logger) LogCommandStarted(directive string) error {
	return nil
}

func (l *Logger) LogCommandOutput(string) error {
	return nil
}

func (l *Logger) LogCommandFinished(directive string, exitCode int, startedAt int, finishedAt int) error {
	return nil
}
