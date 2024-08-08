package eventlogger

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/semaphoreci/agent/pkg/random"
	"github.com/semaphoreci/agent/pkg/retry"
	log "github.com/sirupsen/logrus"
)

const (
	MaxLinesPerRequest           = 2000
	MaxFlushTimeoutInSeconds     = 900
	DefaultFlushTimeoutInSeconds = 60
)

type HTTPBackend struct {
	client      *http.Client
	fileBackend FileBackend
	startFrom   int
	config      HTTPBackendConfig
	stop        bool
	flush       bool
	useArtifact bool
}

type HTTPBackendConfig struct {
	URL                   string
	Token                 string
	LinesPerRequest       int
	FlushTimeoutInSeconds int
	RefreshTokenFn        func() (string, error)
}

func NewHTTPBackend(config HTTPBackendConfig) (*HTTPBackend, error) {
	if config.LinesPerRequest <= 0 || config.LinesPerRequest > MaxLinesPerRequest {
		return nil, fmt.Errorf("config.LinesPerRequest must be between 1 and %d", MaxLinesPerRequest)
	}

	if config.FlushTimeoutInSeconds <= 0 || config.FlushTimeoutInSeconds > MaxFlushTimeoutInSeconds {
		return nil, fmt.Errorf("config.FlushTimeoutInSeconds must be between 1 and %d", MaxFlushTimeoutInSeconds)
	}

	path := filepath.Join(os.TempDir(), fmt.Sprintf("job_log_%d.json", time.Now().UnixNano()))

	// The API will instruct the HTTP backend when to stop
	// streaming logs due to their size hitting the limits.
	// We don't need to impose any limits on the underlying file backend.
	fileBackend, err := NewFileBackend(path, math.MaxInt32)
	if err != nil {
		return nil, err
	}

	httpBackend := HTTPBackend{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		fileBackend: *fileBackend,
		startFrom:   0,
		config:      config,
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

func (l *HTTPBackend) Read(startFrom, maxLines int, writer io.Writer) (int, error) {
	return l.fileBackend.Read(startFrom, maxLines, writer)
}

func (l *HTTPBackend) Iterate(fn func([]byte) error) error {
	return l.fileBackend.Iterate(fn)
}

func (l *HTTPBackend) push() {
	log.Infof("Logs will be pushed to %s", l.config.URL)

	for {

		/*
		 * Check if streaming is necessary. There are three cases where it isn't necessary anymore:
		 *   1. The job has exhausted the amount of log space it has available.
		 *      The API will reject all subsequent attempts, so we just stop trying.
		 *   2. The job is finished and all the logs were already pushed.
		 *   3. The job is finished, not all logs were pushed, but we gave up because it was taking too long.
		 */
		if l.stop {
			break
		}

		/*
		 * Wait for the appropriate amount of time
		 * before trying to send the next batch of logs.
		 */
		delay := l.delay()
		log.Infof("Waiting %v to push next batch of logs...", delay)
		time.Sleep(delay)

		/*
		 * Send the next batch of logs.
		 * If an error occurs, it will be retried in the next tick,
		 * so there is no need to retry requests that failed here.
		 */
		err := l.newRequest()
		if err != nil {
			log.Errorf("Error pushing logs: %v", err)
		}
	}

	log.Info("Stopped pushing logs.")
}

/*
 * The delay between log requests.
 * Note that this isn't a rate,
 * but a delay between the end of one request and the start of the next one.
 */
func (l *HTTPBackend) delay() time.Duration {

	/*
	 * if we are flushing,
	 * we use a tighter range of 500ms - 1000ms.
	 */
	if l.flush {
		delay, _ := random.DurationInRange(250, 500)
		return *delay
	}

	/*
	 * if we are not flushing,
	 * we use a wider range of 1500ms - 3000ms.
	 */
	delay, _ := random.DurationInRange(1500, 3000)
	return *delay
}

func (l *HTTPBackend) newRequest() error {
	buffer := bytes.NewBuffer([]byte{})
	nextStartFrom, err := l.fileBackend.Read(l.startFrom, l.config.LinesPerRequest, buffer)
	if err != nil {
		return err
	}

	/*
	 * If no more logs are found, we may be in two scenarios:
	 *   1. The job is not done, so more logs might be generated. We just skip until there is some new logs.
	 *   2. The job is done, so no more logs will be generated. We stop pushing altogether.
	 */
	if l.startFrom == nextStartFrom {
		if l.flush {
			log.Infof("No more logs to flush - stopping")
			l.stop = true
			return nil
		}

		log.Infof("No logs to push - skipping")
		return nil
	}

	log.Infof("Pushing next batch of logs with %d log events...", (nextStartFrom - l.startFrom))
	url := fmt.Sprintf("%s?start_from=%d", l.config.URL, l.startFrom)
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
		l.stop = true
		l.useArtifact = true
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

func (l *HTTPBackend) CloseWithOptions(options CloseOptions) error {
	/*
	 * Try to flush all the remaining logs.
	 * We wait for them to be flushed for a period of time (60s).
	 * If they are not yet completely flushed after that period of time, we give up.
	 */
	l.flush = true

	log.Printf("Waiting for all logs to be flushed...")
	err := retry.RetryWithConstantWait(retry.RetryOptions{
		Task:                 "wait for logs to be flushed",
		MaxAttempts:          l.config.FlushTimeoutInSeconds,
		DelayBetweenAttempts: time.Second,
		HideError:            true,
		Fn: func() error {
			if l.stop {
				return nil
			}

			return fmt.Errorf("not fully flushed")
		},
	})

	if options.OnClose != nil {
		options.OnClose(l.useArtifact)
	}

	if err != nil {
		log.Errorf("Could not push all logs to %s - giving up", l.config.URL)
	}

	l.stop = true
	return l.fileBackend.Close()
}

func (l *HTTPBackend) Close() error {
	return l.CloseWithOptions(CloseOptions{})
}
