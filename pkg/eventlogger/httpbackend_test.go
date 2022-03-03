package eventlogger

import (
	"fmt"
	"testing"
	"time"

	testsupport "github.com/semaphoreci/agent/test/support"
	"github.com/stretchr/testify/assert"
)

func Test__LogsArePushedToHTTPEndpoint(t *testing.T) {
	mockServer := testsupport.NewLoghubMockServer()
	mockServer.Init()

	httpBackend, err := NewHTTPBackend(mockServer.URL(), "token")
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

	assert.Equal(t, []string{
		fmt.Sprintf(`{"event":"job_started","timestamp":%d}`, timestamp),
		fmt.Sprintf(`{"event":"cmd_started","timestamp":%d,"directive":"echo hello"}`, timestamp),
		fmt.Sprintf(`{"event":"cmd_output","timestamp":%d,"output":"hello\n"}`, timestamp),
		fmt.Sprintf(`{"event":"cmd_finished","timestamp":%d,"directive":"echo hello","exit_code":0,"started_at":%d,"finished_at":%d}`, timestamp, timestamp, timestamp),
		fmt.Sprintf(`{"event":"job_finished","timestamp":%d,"result":"passed"}`, timestamp),
	}, mockServer.GetLogs())

	mockServer.Close()
}
