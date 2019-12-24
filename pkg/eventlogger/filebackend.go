package eventlogger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
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

func (l *FileBackend) Stream(startLine int, writter io.Writer) error {
	fd, err := os.OpenFile(l.path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return err
	}
	defer fd.Close()

	reader := bufio.NewReader(fd)
	lineIndex := 0

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				return err
			}

			break
		}

		if lineIndex < startLine {
			continue
		}

		fmt.Fprintln(writter, line)

		lineIndex++
	}

	return nil
}
