package eventlogger

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/semaphoreci/agent/pkg/api"
)

const LoggerMethodPull = "pull"
const LoggerMethodPush = "push"

func CreateLogger(request *api.JobRequest) (*Logger, error) {
	switch request.Logger.Method {
	case LoggerMethodPull:
		return Default()
	case LoggerMethodPush:
		return DefaultHTTP(request)
	default:
		return nil, fmt.Errorf("unknown logger type")
	}
}

func Default() (*Logger, error) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("job_log_%d.json", time.Now().UnixNano()))
	backend, err := NewFileBackend(path)
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

func DefaultHTTP(request *api.JobRequest) (*Logger, error) {
	if request.Logger.URL == "" {
		return nil, errors.New("HTTP logger needs a URL")
	}

	backend, err := NewHTTPBackend(request.Logger.URL, request.Logger.Token)
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
