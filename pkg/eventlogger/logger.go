package eventlogger

import (
	"bufio"
	"encoding/json"
	"os"
	"time"

	"github.com/tidwall/gjson"

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
		return "", err
	}

	defer tmpFile.Close()

	bufferedWriter := bufio.NewWriterSize(tmpFile, 64*1024)
	err = l.Backend.ReadAndProcess(func(b []byte) error {
		return l.writePlain(bufferedWriter, b)
	})

	if err != nil {
		return "", err
	}

	err = bufferedWriter.Flush()
	if err != nil {
		return "", err
	}

	return tmpFile.Name(), nil
}

func (l *Logger) writePlainCustom(writer *bufio.Writer, event []byte) error {
	r := gjson.ParseBytes(event)
	switch eventType := r.Get("event").Str; {
	case eventType == "cmd_started":
		if _, err := writer.WriteString(r.Get("directive").Str + "\n"); err != nil {
			return err
		}
	case eventType == "cmd_output":
		if _, err := writer.WriteString(r.Get("output").Str); err != nil {
			return err
		}
	default:
		// We can ignore all the other event types here
	}

	return nil
}

func (l *Logger) writePlain(writer *bufio.Writer, event []byte) error {
	var object map[string]interface{}
	err := json.Unmarshal(event, &object)
	if err != nil {
		return err
	}

	switch eventType := object["event"].(string); {
	case eventType == "cmd_started":
		if _, err := writer.WriteString(object["directive"].(string) + "\n"); err != nil {
			return err
		}
	case eventType == "cmd_output":
		if _, err := writer.WriteString(object["output"].(string)); err != nil {
			return err
		}
	default:
		// We can ignore all the other event types here
	}

	return nil
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
