package eventlogger

import (
	"encoding/json"
	"log"
	"os"
)

type fileBackend struct {
	path string
	file *os.File
}

func newFileBackend(path string) (*fileBackend, error) {
	return &fileBackend{path: path}, nil
}

func (l *fileBackend) Open() error {
	file, err := os.Create(l.path)
	if err != nil {
		return nil
	}

	l.file = file

	return nil
}

func (l *fileBackend) Write(event interface{}) error {
	jsonString, _ := json.Marshal(event)

	l.file.Write([]byte(jsonString))
	l.file.Write([]byte("\n"))

	log.Printf("%s", jsonString)

	return nil
}

func (l *fileBackend) Close() error {
	return nil
}

func (l *fileBackend) Read(from, to int) []string {
	return []string{}
}
