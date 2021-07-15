package eventlogger

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"time"
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
	log.Printf("Start streaming logs to %s", l.url)

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

	log.Printf("Stopped streaming logs to %s", l.url)
}

func (l *HttpBackend) streamLogs() {
	buffer := bytes.NewBuffer([]byte{})
	nextStartFrom, err := l.fileBackend.Stream(l.startFrom, buffer)
	if err != nil {
		log.Printf("Error reading logs from file: %v", err)
		return
	}

	if l.startFrom == nextStartFrom {
		// no logs to stream
		return
	}

	url := fmt.Sprintf("%s?start_from=%d", l.url, l.startFrom)
	request, err := http.NewRequest("POST", url, buffer)
	if err != nil {
		log.Printf("Error creating streaming log request to %s: %v", url, err)
		return
	}

	request.Header.Set("Content-Type", "text/plain")
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", l.token))
	response, err := l.client.Do(request)
	if err != nil {
		log.Printf("Error streaming logs to %s: %v", url, err)
		return
	}

	if response.StatusCode != 200 {
		log.Printf("Log streaming request got %s response", response.Status)
		return
	}

	l.startFrom = nextStartFrom
}

func (l *HttpBackend) Close() error {
	l.stopStreaming()
	l.streamLogs()
	return l.fileBackend.Close()
}
