package eventlogger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
)

type Logger struct {
	Backend Backend
}

func NewLogger(backend Backend) (*Logger, error) {
	return &Logger{Backend: backend}, nil
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
	event := &CommandOutputEvent{
		Timestamp: int(time.Now().Unix()),
		Event:     "cmd_output",
		Output:    output,
	}

	err := l.Backend.Write(event)
	if err != nil {
		log.Errorf("Error writing cmd_output log: %v", err)
	}
}

func (l *Logger) LogCommandFinished(directive string, exitCode int, startedAt int, finishedAt int) {
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
