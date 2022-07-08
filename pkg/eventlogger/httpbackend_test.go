package eventlogger

import (
	"testing"
	"time"

	testsupport "github.com/semaphoreci/agent/test/support"
	"github.com/stretchr/testify/assert"
)

func Test__LogsArePushedToHTTPEndpoint(t *testing.T) {
	mockServer := testsupport.NewLoghubMockServer()
	mockServer.Init()

	httpBackend, err := NewHTTPBackend(mockServer.URL(), "token", func() (string, error) { return "", nil })
	assert.Nil(t, err)
	assert.Nil(t, httpBackend.Open())

	timestamp := int(time.Now().Unix())
	assert.Nil(t, httpBackend.Write(&JobStartedEvent{Timestamp: timestamp, Event: "job_started"}))
	assert.Nil(t, httpBackend.Write(&CommandStartedEvent{Timestamp: timestamp, Event: "cmd_started", Directive: "echo hello"}))
	assert.Nil(t, httpBackend.Write(&CommandOutputEvent{Timestamp: timestamp, Event: "cmd_output", Output: "hello\n"}))
	assert.Nil(t, httpBackend.Write(&CommandFinishedEvent{
		Timestamp:  timestamp,
		Event:      "cmd_finished",
		Directive:  "echo hello",
		ExitCode:   0,
		StartedAt:  timestamp,
		FinishedAt: timestamp,
	}))
	assert.Nil(t, httpBackend.Write(&JobFinishedEvent{Timestamp: timestamp, Event: "job_finished", Result: "passed"}))

	// Wait until everything is pushed
	time.Sleep(2 * time.Second)

	err = httpBackend.Close()
	assert.Nil(t, err)

	eventObjects, err := TransformToObjects(mockServer.GetLogs())
	assert.Nil(t, err)

	simplifiedEvents, err := SimplifyLogEvents(eventObjects, true)
	assert.Nil(t, err)

	assert.Equal(t, []string{
		"job_started",

		"directive: echo hello",
		"hello\n",
		"Exit Code: 0",

		"job_finished: passed",
	}, simplifiedEvents)

	mockServer.Close()
}

func Test__TokenIsRefreshed(t *testing.T) {
	mockServer := testsupport.NewLoghubMockServer()
	mockServer.Init()

	tokenWasRefreshed := false

	httpBackend, err := NewHTTPBackend(mockServer.URL(), testsupport.ExpiredLogToken, func() (string, error) {
		tokenWasRefreshed = true
		return "some-new-and-shiny-valid-token", nil
	})

	assert.Nil(t, err)
	assert.Nil(t, httpBackend.Open())

	timestamp := int(time.Now().Unix())
	assert.Nil(t, httpBackend.Write(&JobStartedEvent{Timestamp: timestamp, Event: "job_started"}))
	assert.Nil(t, httpBackend.Write(&CommandStartedEvent{Timestamp: timestamp, Event: "cmd_started", Directive: "echo hello"}))
	assert.Nil(t, httpBackend.Write(&CommandOutputEvent{Timestamp: timestamp, Event: "cmd_output", Output: "hello\n"}))
	assert.Nil(t, httpBackend.Write(&CommandFinishedEvent{
		Timestamp:  timestamp,
		Event:      "cmd_finished",
		Directive:  "echo hello",
		ExitCode:   0,
		StartedAt:  timestamp,
		FinishedAt: timestamp,
	}))
	assert.Nil(t, httpBackend.Write(&JobFinishedEvent{Timestamp: timestamp, Event: "job_finished", Result: "passed"}))

	// Wait until everything is pushed
	time.Sleep(2 * time.Second)

	err = httpBackend.Close()
	assert.Nil(t, err)
	assert.True(t, tokenWasRefreshed)

	eventObjects, err := TransformToObjects(mockServer.GetLogs())
	assert.Nil(t, err)

	simplifiedEvents, err := SimplifyLogEvents(eventObjects, true)
	assert.Nil(t, err)

	assert.Equal(t, []string{
		"job_started",

		"directive: echo hello",
		"hello\n",
		"Exit Code: 0",

		"job_finished: passed",
	}, simplifiedEvents)

	mockServer.Close()
}
