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
	extraFields := f.formatFields(entry.Data)
	if extraFields == "" {
		log := fmt.Sprintf(
			"%s agent_name=%s : %s\n",
			entry.Time.UTC().Format(time.StampMilli),
			f.AgentName,
			entry.Message,
		)

		return []byte(log), nil
	}

	log := fmt.Sprintf(
		"%s agent_name=%s %s : %s\n",
		entry.Time.UTC().Format(time.StampMilli),
		f.AgentName,
		extraFields,
		entry.Message,
	)

	return []byte(log), nil
}

func (f *CustomFormatter) formatFields(fields log.Fields) string {
	result := []string{}
	for key, value := range fields {
		result = append(result, fmt.Sprintf("%s=%s", key, value))
	}

	return strings.Join(result, " ")
}
