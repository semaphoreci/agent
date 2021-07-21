package eventlogger

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/semaphoreci/agent/pkg/retry"
	log "github.com/sirupsen/logrus"
)

type HttpBackend struct {
	client      *http.Client
	url         string
	token       string
	fileBackend FileBackend
	startFrom   int
	streamChan  chan bool
	pushLock    sync.Mutex
}

func NewHttpBackend(url, token string) (*HttpBackend, error) {
	fileBackend, err := NewFileBackend("/tmp/job_log.json")
	if err != nil {
		return nil, err
	}

	httpBackend := HttpBackend{
		client:      &http.Client{},
		url:         url,
		token:       token,
		fileBackend: *fileBackend,
		startFrom:   0,
	}

	httpBackend.startPushingLogs()

	return &httpBackend, nil
}

func (l *HttpBackend) Open() error {
	return l.fileBackend.Open()
}

func (l *HttpBackend) Write(event interface{}) error {
	return l.fileBackend.Write(event)
}

func (l *HttpBackend) startPushingLogs() {
	log.Debugf("Logs will be pushed to %s", l.url)

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

func (l *HttpBackend) stopStreaming() {
	if l.streamChan != nil {
		close(l.streamChan)
	}

	log.Debug("Stopped streaming logs")
}

func (l *HttpBackend) pushLogs() error {
	l.pushLock.Lock()
	defer l.pushLock.Unlock()

	buffer := bytes.NewBuffer([]byte{})
	nextStartFrom, err := l.fileBackend.Stream(l.startFrom, buffer)
	if err != nil {
		return err
	}

	if l.startFrom == nextStartFrom {
		log.Debugf("No logs to push - skipping")
		// no logs to stream
		return nil
	}

	url := fmt.Sprintf("%s?start_from=%d", l.url, l.startFrom)
	log.Debugf("Pushing logs to %s", url)
	request, err := http.NewRequest("POST", url, buffer)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "text/plain")
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", l.token))
	response, err := l.client.Do(request)
	if err != nil {
		return err
	}

	if response.StatusCode != 200 {
		return fmt.Errorf("request to %s failed: %s", url, response.Status)
	}

	l.startFrom = nextStartFrom
	return nil
}

func (l *HttpBackend) Close() error {
	l.stopStreaming()

	err := retry.RetryWithConstantWait("Push logs", 5, time.Second, func() error {
		return l.pushLogs()
	})

	if err != nil {
		log.Errorf("Could not push all logs to %s: %v", l.url, err)
	} else {
		log.Infof("All logs successfully pushed to %s", l.url)
	}

	return l.fileBackend.Close()
}
