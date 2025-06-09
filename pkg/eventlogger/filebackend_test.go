package eventlogger

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	bytes, err := os.ReadFile(tmpFileName)
	assert.Nil(t, err)
	logs := strings.Split(string(bytes), "\n")

	assert.Equal(t, []string{
		fmt.Sprintf(`{"event":"job_started","timestamp":%d}`, timestamp),
		fmt.Sprintf(`{"event":"cmd_started","timestamp":%d,"directive":"echo hello"}`, timestamp),
		fmt.Sprintf(`{"event":"cmd_output","timestamp":%d,"output":"hello\n"}`, timestamp),
		fmt.Sprintf(`{"event":"cmd_finished","timestamp":%d,"directive":"echo hello","exit_code":0,"started_at":%d,"finished_at":%d}`, timestamp, timestamp, timestamp),
		fmt.Sprintf(`{"event":"job_finished","timestamp":%d,"result":"passed"}`, timestamp),
		"", // newline at the end of the file
	}, logs)

	err = fileBackend.Close()
	assert.Nil(t, err)
}

func Test__ReadDoesNotIncludeDoubleNewlines(t *testing.T) {
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

	w := new(bytes.Buffer)
	_, err = fileBackend.Read(0, 1000, w)
	assert.NoError(t, err)
	logs := strings.Split(w.String(), "\n")

	assert.Equal(t, []string{
		fmt.Sprintf(`{"event":"job_started","timestamp":%d}`, timestamp),
		fmt.Sprintf(`{"event":"cmd_started","timestamp":%d,"directive":"echo hello"}`, timestamp),
		fmt.Sprintf(`{"event":"cmd_output","timestamp":%d,"output":"hello\n"}`, timestamp),
		fmt.Sprintf(`{"event":"cmd_finished","timestamp":%d,"directive":"echo hello","exit_code":0,"started_at":%d,"finished_at":%d}`, timestamp, timestamp, timestamp),
		fmt.Sprintf(`{"event":"job_finished","timestamp":%d,"result":"passed"}`, timestamp),
		"", // newline at the end of the file
	}, logs)

	err = fileBackend.Close()
	assert.Nil(t, err)
}

func Test__CloseWithOptions(t *testing.T) {

	t.Run("trimmed logs", func(t *testing.T) {
		tmpFileName := filepath.Join(os.TempDir(), fmt.Sprintf("logs_%d.json", time.Now().UnixNano()))

		// The max is 50 bytes
		fileBackend, err := NewFileBackend(tmpFileName, 50)
		assert.Nil(t, err)
		assert.Nil(t, fileBackend.Open())

		timestamp := int(time.Now().Unix())
		assert.Nil(t, fileBackend.Write(&JobStartedEvent{Timestamp: timestamp, Event: "job_started"}))
		assert.Nil(t, fileBackend.Write(&CommandStartedEvent{Timestamp: timestamp, Event: "cmd_started", Directive: "echo hello"}))
		assert.Nil(t, fileBackend.Write(&CommandOutputEvent{Timestamp: timestamp, Event: "cmd_output", Output: "hello\n"}))

		logsWereTrimmed := false
		err = fileBackend.CloseWithOptions(CloseOptions{OnClose: func(b bool) { logsWereTrimmed = b }})
		assert.Nil(t, err)
		assert.True(t, logsWereTrimmed)
	})

	t.Run("no trimmed logs", func(t *testing.T) {
		tmpFileName := filepath.Join(os.TempDir(), fmt.Sprintf("logs_%d.json", time.Now().UnixNano()))

		// The max is 1M
		fileBackend, err := NewFileBackend(tmpFileName, 1024*1024)
		assert.Nil(t, err)
		assert.Nil(t, fileBackend.Open())

		timestamp := int(time.Now().Unix())
		assert.Nil(t, fileBackend.Write(&JobStartedEvent{Timestamp: timestamp, Event: "job_started"}))
		assert.Nil(t, fileBackend.Write(&CommandStartedEvent{Timestamp: timestamp, Event: "cmd_started", Directive: "echo hello"}))
		assert.Nil(t, fileBackend.Write(&CommandOutputEvent{Timestamp: timestamp, Event: "cmd_output", Output: "hello\n"}))

		logsWereTrimmed := false
		err = fileBackend.CloseWithOptions(CloseOptions{OnClose: func(b bool) {
			logsWereTrimmed = b
		}})

		assert.Nil(t, err)
		assert.False(t, logsWereTrimmed)
	})
}
