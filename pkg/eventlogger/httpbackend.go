package eventlogger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type HttpBackend struct {
	client     *http.Client
	url        string
	token      string
	buffer     []interface{}
	bufferSize int
	startFrom  int
}

type LogRequest struct {
	Events []interface{} `json:"events"`
}

func NewHttpBackend(url, token string, bufferSize int) (*HttpBackend, error) {
	if bufferSize <= 0 {
		return nil, errors.New("bufferSize needs to be greater than 0")
	}

	return &HttpBackend{
		client:     &http.Client{},
		url:        url,
		token:      token,
		bufferSize: bufferSize,
		buffer:     []interface{}{},
		startFrom:  0,
	}, nil
}

func (l *HttpBackend) Open() error {
	// does nothing
	return nil
}

func (l *HttpBackend) Write(event interface{}) error {
	l.buffer = append(l.buffer, event)
	if len(l.buffer) < l.bufferSize {
		return nil
	}

	_, err := l.send()
	if err != nil {
		return err
	}

	l.startFrom = l.startFrom + len(l.buffer)
	l.buffer = l.buffer[:0]
	return nil
}

func (l *HttpBackend) send() (*http.Response, error) {
	payload := LogRequest{Events: l.buffer}
	jsonString, _ := json.Marshal(payload)
	request, err := http.NewRequest("POST", fmt.Sprintf("%s?start_from=%d", l.url, l.startFrom), bytes.NewBuffer(jsonString))
	if err != nil {
		return nil, err
	}

	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", l.token))
	response, err := l.client.Do(request)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != 200 {
		return nil, fmt.Errorf("got %s response", response.Status)
	} else {
		return response, nil
	}
}

func (l *HttpBackend) Close() error {
	if len(l.buffer) > 0 {
		_, err := l.send()
		if err != nil {
			return err
		}

		l.buffer = l.buffer[:0]
		l.startFrom = len(l.buffer)
	}

	return nil
}
