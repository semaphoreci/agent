package eventlogger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const MaxStagedOutputEvents = 3

type Logger struct {
	Backend Backend
	Options LoggerOptions

	/*
	 * We stage a few of these output events before sending them to the backend,
	 * for redacting reasons. Since the output is received here in chunks, it could be
	 * that a bigger secret is not put into a single output event.
	 * TODO: do I need a lock here?
	 */
	StagedOutputEvents []CommandOutputEvent
}

func NewLogger(backend Backend, options LoggerOptions) (*Logger, error) {
	return &Logger{Backend: backend, Options: options}, nil
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

	l.StagedOutputEvents = append(l.StagedOutputEvents, *event)

	//
	// If we still didn't hit the max number of staged output events,
	// we don't send anything to the backend yet.
	//
	if len(l.StagedOutputEvents) < MaxStagedOutputEvents {
		return
	}

	l.FlushStagedOutput()
}

func (l *Logger) FlushStagedOutput() {
	//
	// Make sure we clear the staged output events before returning
	//
	defer func() {
		l.StagedOutputEvents = []CommandOutputEvent{}
	}()

	//
	// Otherwise, try to redact the output.
	// If the output is redacted, combine all the staged events into a single event
	// with the new redacted output, and send it to the backend.
	//
	redacted, output := l.redactOutput()
	if redacted {
		event := &CommandOutputEvent{
			Timestamp: l.StagedOutputEvents[0].Timestamp,
			Event:     "cmd_output",
			Output:    output,
		}

		err := l.Backend.Write(event)
		if err != nil {
			log.Errorf("Error writing cmd_output log: %v", err)
		}

		return
	}

	//
	// Output was not redacted, just send it as is.
	//
	for _, stagedEvent := range l.StagedOutputEvents {
		err := l.Backend.Write(stagedEvent)
		if err != nil {
			log.Errorf("Error writing cmd_output log: %v", err)
		}
	}
}

func (l *Logger) AllStagedOutput() string {
	o := ""
	for _, v := range l.StagedOutputEvents {
		o += v.Output
	}

	return o
}

func (l *Logger) redactOutput() (bool, string) {
	redacted := false
	output := l.AllStagedOutput()

	for _, v := range l.Options.RedactableValues {
		if strings.Contains(output, v) {
			output = strings.ReplaceAll(output, v, "[REDACTED]")
			redacted = true
		}
	}

	return redacted, output
}

func (l *Logger) LogCommandFinished(directive string, exitCode int, startedAt int, finishedAt int) {
	l.FlushStagedOutput()

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
