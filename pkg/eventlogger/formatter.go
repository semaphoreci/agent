package eventlogger

import (
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

type CustomFormatter struct {
	AgentName string
}

func (f *CustomFormatter) Format(entry *log.Entry) ([]byte, error) {
	parts := []string{}
	parts = append(parts, entry.Time.UTC().Format(time.StampMilli))

	if f.AgentName != "" {
		parts = append(parts, f.AgentName)
	}

	extraFields := f.formatFields(entry.Data)
	if extraFields != "" {
		parts = append(parts, extraFields)
	}

	parts = append(parts, ":")
	parts = append(parts, fmt.Sprintf("%s\n", entry.Message))
	return []byte(strings.Join(parts, " ")), nil
}

func (f *CustomFormatter) formatFields(fields log.Fields) string {
	result := []string{}
	for key, value := range fields {
		result = append(result, fmt.Sprintf("%s=%s", key, value))
	}

	return strings.Join(result, " ")
}
