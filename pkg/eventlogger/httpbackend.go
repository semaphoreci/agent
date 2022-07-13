package eventlogger

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/semaphoreci/agent/pkg/retry"
	log "github.com/sirupsen/logrus"
)

type HTTPBackend struct {
	client      *http.Client
	fileBackend FileBackend
	startFrom   int
	streamChan  chan bool
	pushLock    sync.Mutex
	config      HTTPBackendConfig
}

type HTTPBackendConfig struct {
	URL             string
	Token           string
	LinesPerRequest int
	RefreshTokenFn  func() (string, error)
}

func NewHTTPBackend(config HTTPBackendConfig) (*HTTPBackend, error) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("job_log_%d.json", time.Now().UnixNano()))
	fileBackend, err := NewFileBackend(path)
	if err != nil {
		return nil, err
	}

	httpBackend := HTTPBackend{
		client:      &http.Client{},
		fileBackend: *fileBackend,
		startFrom:   0,
		config:      config,
	}

	httpBackend.startPushingLogs()

	return &httpBackend, nil
}

func (l *HTTPBackend) Open() error {
	return l.fileBackend.Open()
}

func (l *HTTPBackend) Write(event interface{}) error {
	return l.fileBackend.Write(event)
}

func (l *HTTPBackend) startPushingLogs() {
	log.Debugf("Logs will be pushed to %s", l.config.URL)

	ticker := time.NewTicker(time.Second)
	l.streamChan = make(chan bool)

	go func() {
		for {
			select {
			case <-ticker.C:
				err := l.pushLogs()
				if err != nil {
					log.Errorf("Error pushing logs: %v", err)
					// we don't retry the request here because a new one will happen in 1s,
					// so we only retry these requests on Close()
				}
			case <-l.streamChan:
				ticker.Stop()
				return
			}
		}
	}()
}

func (l *HTTPBackend) stopStreaming() {
	if l.streamChan != nil {
		close(l.streamChan)
	}

	log.Debug("Stopped streaming logs")
}

func (l *HTTPBackend) pushLogs() error {
	l.pushLock.Lock()
	defer l.pushLock.Unlock()

	buffer := bytes.NewBuffer([]byte{})
	nextStartFrom, err := l.fileBackend.Stream(l.startFrom, l.config.LinesPerRequest, buffer)
	if err != nil {
		return err
	}

	if l.startFrom == nextStartFrom {
		log.Debugf("No logs to push - skipping")
		// no logs to stream
		return nil
	}

	url := fmt.Sprintf("%s?start_from=%d", l.config.URL, l.startFrom)
	log.Debugf("Pushing logs to %s", url)
	request, err := http.NewRequest("POST", url, buffer)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "text/plain")
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", l.config.Token))
	response, err := l.client.Do(request)
	if err != nil {
		return err
	}

	switch response.StatusCode {

	// Everything went fine,
	// just update the index and move on.
	case http.StatusOK:
		l.startFrom = nextStartFrom
		return nil

	// The token issued for the agent expired.
	// Try to refresh the token and try again.
	// Here, we only update the token, and we let the caller do the retrying.
	case http.StatusUnauthorized:
		newToken, err := l.config.RefreshTokenFn()
		if err != nil {
			return err
		}

		l.config.Token = newToken
		return fmt.Errorf("request to %s failed: %s", url, response.Status)

	// something else went wrong
	default:
		return fmt.Errorf("request to %s failed: %s", url, response.Status)
	}
}

func (l *HTTPBackend) Close() error {
	l.stopStreaming()

	err := retry.RetryWithConstantWait("Push logs", 5, time.Second, func() error {
		return l.pushLogs()
	})

	if err != nil {
		log.Errorf("Could not push all logs to %s: %v", l.config.URL, err)
	} else {
		log.Infof("All logs successfully pushed to %s", l.config.URL)
	}

	return l.fileBackend.Close()
}
