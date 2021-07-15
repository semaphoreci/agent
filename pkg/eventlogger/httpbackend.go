package eventlogger

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

type HttpBackend struct {
	client      *http.Client
	url         string
	token       string
	fileBackend FileBackend
	startFrom   int
	streamChan  chan bool
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

	httpBackend.startStreaming()

	return &httpBackend, nil
}

func (l *HttpBackend) Open() error {
	return l.fileBackend.Open()
}

func (l *HttpBackend) Write(event interface{}) error {
	return l.fileBackend.Write(event)
}

func (l *HttpBackend) startStreaming() {
	log.Debugf("Logs will be streamed to %s", l.url)

	ticker := time.NewTicker(time.Second)
	l.streamChan = make(chan bool)

	go func() {
		for {
			select {
			case <-ticker.C:
				l.streamLogs()
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

func (l *HttpBackend) streamLogs() {
	buffer := bytes.NewBuffer([]byte{})
	nextStartFrom, err := l.fileBackend.Stream(l.startFrom, buffer)
	if err != nil {
		log.Errorf("Error reading logs from file: %v", err)
		return
	}

	if l.startFrom == nextStartFrom {
		// no logs to stream
		return
	}

	url := fmt.Sprintf("%s?start_from=%d", l.url, l.startFrom)
	request, err := http.NewRequest("POST", url, buffer)
	if err != nil {
		log.Errorf("Error creating streaming log request to %s: %v", url, err)
		return
	}

	request.Header.Set("Content-Type", "text/plain")
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", l.token))
	response, err := l.client.Do(request)
	if err != nil {
		log.Errorf("Error streaming logs to %s: %v", url, err)
		return
	}

	if response.StatusCode != 200 {
		log.Errorf("Request to %s failed: %s", url, response.Status)
		return
	}

	l.startFrom = nextStartFrom
}

func (l *HttpBackend) Close() error {
	l.stopStreaming()
	l.streamLogs()
	return l.fileBackend.Close()
}
