package eventlogger

func Default() (*Logger, error) {
	backend, err := newFileBackend("/tmp/job_log.json")
	if err != nil {
		return nil, err
	}

	logger, err := NewLogger(backend)
	if err != nil {
		return nil, err
	}

	err = logger.Open()
	if err != nil {
		return nil, err
	}

	return logger, nil
}

func DefaultTestLogger() *Logger {
	backend, err := newFileBackend("/tmp/job_log_test.json")
	if err != nil {
		panic(err)
	}

	logger, err := NewLogger(backend)
	if err != nil {
		panic(err)
	}

	err = logger.Open()
	if err != nil {
		panic(err)
	}

	return logger
}
