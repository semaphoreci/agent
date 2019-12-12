package eventlogger

type EventLogger interface {
	Open() error

	LogJobStarted() error
	LogJobFinished(directive string) error

	LogCommandStarted(directive string) error
	LogCommandOutput(string) error
	LogCommandFinished(directive string, exitCode int, startedAt int, finishedAt int) error

	Close() error
}

var DefaultLogFilePath = "/tmp/job_log.json"

func Default() (EventLogger, error) {
	logger, err := newFileLogger(DefaultLogFilePath)
	if err != nil {
		return nil, err
	}

	err = logger.Open()
	if err != nil {
		return nil, err
	}

	return logger, nil
}
