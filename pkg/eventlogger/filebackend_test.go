package eventlogger

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testsupport "github.com/semaphoreci/agent/test/support"
	"github.com/stretchr/testify/assert"
)

func Test__LogsArePushedToFile(t *testing.T) {
	tmpFileName := filepath.Join(os.TempDir(), fmt.Sprintf("logs_%d.json", time.Now().UnixNano()))
	fileBackend, err := NewFileBackend(tmpFileName, DefaultMaxSizeInBytes)
	assert.Nil(t, err)
	assert.Nil(t, fileBackend.Open())

	timestamp := int(time.Now().Unix())
	assert.Nil(t, fileBackend.Write(&JobStartedEvent{Timestamp: timestamp, Event: "job_started"}))
	assert.Nil(t, fileBackend.Write(&CommandStartedEvent{Timestamp: timestamp, Event: "cmd_started", Directive: "echo hello"}))
	assert.Nil(t, fileBackend.Write(&CommandOutputEvent{Timestamp: timestamp, Event: "cmd_output", Output: "hello\n"}))
	assert.Nil(t, fileBackend.Write(&CommandFinishedEvent{
		Timestamp:  timestamp,
		Event:      "cmd_finished",
		Directive:  "echo hello",
		ExitCode:   0,
		StartedAt:  timestamp,
		FinishedAt: timestamp,
	}))
	assert.Nil(t, fileBackend.Write(&JobFinishedEvent{Timestamp: timestamp, Event: "job_finished", Result: "passed"}))

	bytes, err := ioutil.ReadFile(tmpFileName)
	assert.Nil(t, err)
	logs := strings.Split(string(bytes), "\n")

	assert.Equal(t, []string{
		fmt.Sprintf(`{"event":"job_started","timestamp":%d}`, timestamp),
		fmt.Sprintf(`{"event":"cmd_started","timestamp":%d,"directive":"echo hello"}`, timestamp),
		fmt.Sprintf(`{"event":"cmd_output","timestamp":%d,"output":"hello\n"}`, timestamp),
		fmt.Sprintf(`{"event":"cmd_finished","timestamp":%d,"directive":"echo hello","exit_code":0,"started_at":%d,"finished_at":%d}`, timestamp, timestamp, timestamp),
		fmt.Sprintf(`{"event":"job_finished","timestamp":%d,"result":"passed"}`, timestamp),
	}, testsupport.FilterEmpty(logs))

	err = fileBackend.Close()
	assert.Nil(t, err)
}
