package eventlogger

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
)

type CustomFormatter struct {
}

func (f *CustomFormatter) Format(entry *log.Entry) ([]byte, error) {
	log := fmt.Sprintf("%-20s: %s\n", entry.Time.UTC().Format(time.StampMilli), entry.Message)
	return []byte(log), nil
}
