package eventlogger

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
)

const LoggerMethodPull = "pull"
const LoggerMethodPush = "push"

type LoggerOptions struct {
	Request        *api.JobRequest
	RefreshTokenFn func() (string, error)
	UserAgent      string
}

func CreateLogger(options LoggerOptions) (*Logger, error) {
	if options.Request == nil {
		return nil, fmt.Errorf("request is required")
	}

	switch options.Request.Logger.Method {
	case LoggerMethodPull:
		return Default(options.Request)
	case LoggerMethodPush:
		return DefaultHTTP(options)
	default:
		return nil, fmt.Errorf("unknown logger type")
	}
}

func Default(request *api.JobRequest) (*Logger, error) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("job_log_%d.json", time.Now().UnixNano()))

	maxSize := DefaultMaxSizeInBytes
	if request.Logger.MaxSizeInBytes > 0 {
		maxSize = request.Logger.MaxSizeInBytes
	}

	backend, err := NewFileBackend(path, maxSize)
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

func DefaultHTTP(options LoggerOptions) (*Logger, error) {
	request := options.Request
	if request.Logger.URL == "" {
		return nil, errors.New("HTTP logger needs a URL")
	}

	if options.RefreshTokenFn == nil {
		return nil, errors.New("HTTP logger needs a refresh token function")
	}

	backend, err := NewHTTPBackend(HTTPBackendConfig{
		URL:                   request.Logger.URL,
		Token:                 request.Logger.Token,
		RefreshTokenFn:        options.RefreshTokenFn,
		UserAgent:             options.UserAgent,
		LinesPerRequest:       MaxLinesPerRequest,
		FlushTimeoutInSeconds: DefaultFlushTimeoutInSeconds,
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
