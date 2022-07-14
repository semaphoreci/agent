package eventlogger

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/semaphoreci/agent/pkg/retry"
	log "github.com/sirupsen/logrus"
)

const (
	// pushing logs while backend is open
	statePushing = "pushing"

	// pushing logs after backend was requested to be closed
	stateFlushing = "flushing"

	// stopped pushing logs after a "no more space" response from the API
	stateStopped = "stopped"

	// all logs were completely streamed to the API
	stateDone = "done"
)

type HTTPBackend struct {
	client      *http.Client
	fileBackend FileBackend
	startFrom   int
	pushLock    sync.Mutex
	config      HTTPBackendConfig
	state       string
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
		state:       statePushing,
	}

	go httpBackend.push()

	return &httpBackend, nil
}

func (l *HTTPBackend) Open() error {
	return l.fileBackend.Open()
}

func (l *HTTPBackend) Write(event interface{}) error {
	return l.fileBackend.Write(event)
}

// TODO: add noise
func (l *HTTPBackend) interval() time.Duration {
	if l.state == stateFlushing {
		return 500 * time.Millisecond
	}

	return time.Second
}

func (l *HTTPBackend) push() {
	log.Infof("Logs will be pushed to %s", l.config.URL)

	for {

		/*
		 * No more streaming is necessary.
		 * This happens after the job exhausts the amount of log space it has available.
		 * The API will simply reject any new requests, so we just stop.
		 */
		if l.state == stateStopped || l.state == stateDone {
			break
		}

		// wait for the appropriate amount
		// of time before sending a new request.
		time.Sleep(l.interval())

		err := l.newRequest()
		if err != nil {
			log.Errorf("Error pushing logs: %v", err)
			// we don't retry the request here because a new one
			// will happen after a new tick.
		}

	}

	log.Info("Stopped streaming logs.")
}

func (l *HTTPBackend) newRequest() error {
	l.pushLock.Lock()
	defer l.pushLock.Unlock()

	buffer := bytes.NewBuffer([]byte{})
	nextStartFrom, err := l.fileBackend.Stream(l.startFrom, l.config.LinesPerRequest, buffer)
	if err != nil {
		return err
	}

	// We decide what to do when there are
	// no logs to push based on the current state.
	if l.startFrom == nextStartFrom {

		// if the current state is flushing,
		// then the job is done, and no more logs will be written.
		if l.state == stateFlushing {
			l.state = stateDone
			return nil
		}

		// If not, we just keep streaming.
		log.Infof("No logs to push - skipping")
		return nil
	}

	url := fmt.Sprintf("%s?start_from=%d", l.config.URL, l.startFrom)
	log.Infof("Pushing logs to %s", url)
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

	// No more space is available for this job's logs.
	// The API will keep rejecting the requests if we keep sending them, so just stop.
	case http.StatusUnprocessableEntity:
		l.state = stateStopped
		return errors.New("no more space available for logs - stopping")

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
	// if logs already stopped being streamed,
	// there's no need to wait for anything to be flushed.
	if l.state == stateStopped {
		return l.fileBackend.Close()
	}

	l.state = stateFlushing
	err := retry.RetryWithConstantWait("wait for logs to be flushed", 60, time.Second, func() error {
		if l.state == stateDone {
			return nil
		}

		return fmt.Errorf("logs are not yet fully flushed")
	})

	if err != nil {
		log.Errorf("Could not push all logs to %s: %v", l.config.URL, err)
		l.state = stateStopped
	} else {
		log.Infof("All logs successfully pushed to %s", l.config.URL)
	}

	return l.fileBackend.Close()
}
