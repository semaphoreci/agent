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

func CreateLogger(request *api.JobRequest, refreshTokenFn func() (string, error)) (*Logger, error) {
	switch request.Logger.Method {
	case LoggerMethodPull:
		return Default()
	case LoggerMethodPush:
		return DefaultHTTP(request, refreshTokenFn)
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

func DefaultHTTP(request *api.JobRequest, refreshTokenFn func() (string, error)) (*Logger, error) {
	if request.Logger.URL == "" {
		return nil, errors.New("HTTP logger needs a URL")
	}

	if refreshTokenFn == nil {
		return nil, errors.New("HTTP logger needs a refresh token function")
	}

	backend, err := NewHTTPBackend(HTTPBackendConfig{
		URL:             request.Logger.URL,
		Token:           request.Logger.Token,
		RefreshTokenFn:  refreshTokenFn,
		LinesPerRequest: 2000,
	})

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
