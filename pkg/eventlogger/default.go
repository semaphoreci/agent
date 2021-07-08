package eventlogger

import (
	"errors"
	"fmt"

	"github.com/semaphoreci/agent/pkg/api"
)

const LoggerMethodPull = "pull"
const LoggerMethodPush = "push"
const LoggerDefaultBufferSize = 10

func CreateLogger(request *api.JobRequest) (*Logger, error) {
	switch request.Logger.Method {
	case LoggerMethodPull:
		return Default()
	case LoggerMethodPush:
		return DefaultHttp(request)
	default:
		return nil, fmt.Errorf("unknown logger type")
	}
}

func Default() (*Logger, error) {
	backend, err := NewFileBackend("/tmp/job_log.json")
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

func DefaultHttp(request *api.JobRequest) (*Logger, error) {
	if request.Logger.Url == "" {
		return nil, errors.New("HTTP logger needs a URL")
	}

	if request.Logger.BufferSize <= 0 {
		request.Logger.BufferSize = LoggerDefaultBufferSize
	}

	backend, err := NewHttpBackend(request.Logger.Url, request.Logger.Token, request.Logger.BufferSize)
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

func DefaultTestLogger() (*Logger, *InMemoryBackend) {
	backend, err := NewInMemoryBackend()
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

	return logger, backend
}
