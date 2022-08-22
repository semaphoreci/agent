package eventlogger

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"strings"
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

/*
 * Convert the JSON logs file into a plain text one.
 * Note: the caller must delete the generated plain text file after it's done with it.
 */
func (l *Logger) GeneratePlainTextFile() (string, error) {
	tmpFile, err := ioutil.TempFile("", "*.txt")
	if err != nil {
		return "", err
	}

	defer tmpFile.Close()

	/*
	 * Since we are only doing this for possibly very big files,
	 * we read/write things in chunks to avoid keeping a lot of things in memory.
	 */
	startFrom := 0
	var buf bytes.Buffer
	for {
		nextStartFrom, err := l.Backend.Read(startFrom, 20000, &buf)
		if err != nil {
			return "", err
		}

		if nextStartFrom == startFrom {
			break
		}

		startFrom = nextStartFrom
		logEvents := strings.Split(buf.String(), "\n")
		logs, err := l.eventsToPlainLogLines(logEvents)
		if err != nil {
			return "", err
		}

		newLines := []byte(strings.Join(logs, ""))
		err = ioutil.WriteFile(tmpFile.Name(), newLines, 0755)
		if err != nil {
			return "", err
		}
	}

	return tmpFile.Name(), nil
}

func (l *Logger) eventsToPlainLogLines(logEvents []string) ([]string, error) {
	lines := []string{}
	var object map[string]interface{}

	for _, logEvent := range logEvents {
		if logEvent == "" {
			continue
		}

		err := json.Unmarshal([]byte(logEvent), &object)
		if err != nil {
			return []string{}, err
		}

		switch eventType := object["event"].(string); {
		case eventType == "cmd_started":
			lines = append(lines, object["directive"].(string)+"\n")
		case eventType == "cmd_output":
			lines = append(lines, object["output"].(string))
		default:
			// We can ignore all the other event types here
		}
	}

	return lines, nil
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
