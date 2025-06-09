package eventlogger

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// We buffer the output events before flushing them for 100ms, for redacting reasons.
// Since the output is received here in chunks, it could be
// that a bigger secret is not put into a single output event.
const OutputBufferingWindow = 100 * time.Millisecond
const OutputBufferMax = 4 * 1024

type Logger struct {
	Backend Backend
	Options LoggerOptions

	bufferedOutput bytes.Buffer
	lastBufferedAt *time.Time
	bufferLock     sync.Mutex
}

func NewLogger(backend Backend, options LoggerOptions) (*Logger, error) {
	return &Logger{
		Backend: backend,
		Options: options,
	}, nil
}

func (l *Logger) Open() error {
	return l.Backend.Open()
}

func (l *Logger) Close() error {
	return l.Backend.Close()
}

func (l *Logger) CloseWithOptions(options CloseOptions) error {
	return l.Backend.CloseWithOptions(options)
}

func (l *Logger) GeneratePlainTextFileIn(directory string) (string, error) {
	tmpFile, err := os.CreateTemp(directory, "*.txt")
	if err != nil {
		return "", fmt.Errorf("error creating plain text file: %v", err)
	}

	defer tmpFile.Close()

	bufferedWriter := bufio.NewWriterSize(tmpFile, 64*1024)
	err = l.Backend.Iterate(func(event []byte) error {
		var object map[string]interface{}
		err := json.Unmarshal(event, &object)
		if err != nil {
			return fmt.Errorf("error unmarshaling log event '%s': %v", string(event), err)
		}

		switch eventType := object["event"].(string); {
		case eventType == "cmd_started":
			if _, err := bufferedWriter.WriteString(object["directive"].(string) + "\n"); err != nil {
				return fmt.Errorf("error writing to output: %v", err)
			}
		case eventType == "cmd_output":
			if _, err := bufferedWriter.WriteString(object["output"].(string)); err != nil {
				return fmt.Errorf("error writing to output: %v", err)
			}
		default:
			// We can ignore all the other event types here
		}

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("error iterating on log backend: %v", err)
	}

	err = bufferedWriter.Flush()
	if err != nil {
		return "", fmt.Errorf("error flushing buffered writer: %v", err)
	}

	return tmpFile.Name(), nil
}

/*
 * Convert the JSON logs file into a plain text one.
 * Note: the caller must delete the generated plain text file after it's done with it.
 */
func (l *Logger) GeneratePlainTextFile() (string, error) {
	return l.GeneratePlainTextFileIn(os.TempDir())
}

func (l *Logger) LogJobStarted() {
	event := &JobStartedEvent{
		Timestamp: int(time.Now().Unix()),
		Event:     "job_started",
	}

	err := l.Backend.Write(event)
	if err != nil {
		log.Errorf("Error writing job_started log: %v", err)
	}
}

func (l *Logger) LogJobFinished(result string) {
	event := &JobFinishedEvent{
		Timestamp: int(time.Now().Unix()),
		Event:     "job_finished",
		Result:    result,
	}

	err := l.Backend.Write(event)
	if err != nil {
		log.Errorf("Error writing job_finished log: %v", err)
	}
}

func (l *Logger) LogCommandStarted(directive string) {
	event := &CommandStartedEvent{
		Timestamp: int(time.Now().Unix()),
		Event:     "cmd_started",
		Directive: directive,
	}

	err := l.Backend.Write(event)
	if err != nil {
		log.Errorf("Error writing cmd_started log: %v", err)
	}
}

func (l *Logger) LogCommandOutput(output string) {
	l.bufferLock.Lock()
	defer l.bufferLock.Unlock()

	now := time.Now()
	_, err := l.bufferedOutput.WriteString(output)
	if err != nil {
		log.Errorf("Error writing cmd_output log: %v", err)
		return
	}

	//
	// If we are above our max buffer size, we flush.
	//
	if l.bufferedOutput.Len() > OutputBufferMax {
		l.lastBufferedAt = &now
		l.flushBufferedOutput()
		return
	}

	//
	// If we are outside our current buffering window, we flush.
	//
	if l.lastBufferedAt != nil && l.lastBufferedAt.Add(OutputBufferingWindow).After(now) {
		l.lastBufferedAt = &now
		l.flushBufferedOutput()
		return
	}

	//
	// Otherwise, we just update our last buffered at time.
	//
	l.lastBufferedAt = &now
}

func (l *Logger) flushBufferedOutput() {
	//
	// Make sure we clear the buffered output events before returning
	//
	defer func() {
		l.bufferedOutput.Reset()
	}()

	//
	// Redact the output before flushing it to the backend.
	// Also, we chunk the output into multiple events.
	//
	output := l.redact()
	chunkSize := 256
	outputLen := len(output)

	for i := 0; i < outputLen; i += chunkSize {
		end := i + chunkSize
		if end > outputLen {
			end = outputLen
		}

		chunk := output[i:end]
		event := &CommandOutputEvent{
			Timestamp: int(l.lastBufferedAt.Unix()),
			Event:     "cmd_output",
			Output:    string(chunk),
		}

		err := l.Backend.Write(event)
		if err != nil {
			log.Errorf("Error writing cmd_output log: %v", err)
			return
		}
	}
}

func (l *Logger) redact() []byte {
	out := bytes.Clone(l.bufferedOutput.Bytes())
	for _, v := range l.Options.RedactableValues {
		if bytes.Contains(out, v) {
			out = bytes.ReplaceAll(out, v, []byte("[REDACTED]"))
		}
	}

	for _, r := range l.Options.RedactableRegexes {
		out = r.ReplaceAll(out, []byte("[REDACTED]"))
	}

	return out
}

func (l *Logger) LogCommandFinished(directive string, exitCode int, startedAt int, finishedAt int) {
	l.bufferLock.Lock()
	defer l.bufferLock.Unlock()

	l.flushBufferedOutput()

	event := &CommandFinishedEvent{
		Timestamp:  int(time.Now().Unix()),
		Event:      "cmd_finished",
		Directive:  directive,
		ExitCode:   exitCode,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}

	err := l.Backend.Write(event)
	if err != nil {
		log.Errorf("Error writing cmd_finished log: %v", err)
	}
}
