package eventlogger

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
)

type FileBackend struct {
	path string
	file *os.File
}

func NewFileBackend(path string) (*FileBackend, error) {
	return &FileBackend{path: path}, nil
}

func (l *FileBackend) Open() error {
	file, err := os.Create(l.path)
	if err != nil {
		return nil
	}

	l.file = file

	return nil
}

func (l *FileBackend) Write(event interface{}) error {
	jsonString, _ := json.Marshal(event)

	l.file.Write([]byte(jsonString))
	l.file.Write([]byte("\n"))

	log.Printf("%s", jsonString)

	return nil
}

func (l *FileBackend) Close() error {
	return nil
}

func (l *FileBackend) Read(from, to int) ([]string, error) {
	return []string{}, nil
}

func (l *FileBackend) ReadAll() ([]string, error) {
	file, err := os.Open(l.path)

	if err != nil {
		return []string{}, err
	}

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)
	var lines []string

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	file.Close()

	return lines, nil
}
