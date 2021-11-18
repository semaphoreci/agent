package eventlogger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	log "github.com/sirupsen/logrus"
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
	jsonString, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = l.file.Write([]byte(jsonString))
	if err != nil {
		return err
	}

	_, err = l.file.Write([]byte("\n"))
	if err != nil {
		return err
	}

	log.Debugf("%s", jsonString)

	return nil
}

func (l *FileBackend) Close() error {
	return nil
}

func (l *FileBackend) Stream(startLine int, writer io.Writer) (int, error) {
	fd, err := os.OpenFile(l.path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return startLine, err
	}

	defer fd.Close()

	reader := bufio.NewReader(fd)
	lineIndex := 0

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				return lineIndex, err
			}

			break
		}

		if lineIndex < startLine {
			lineIndex++
			continue
		} else {
			lineIndex++
			fmt.Fprintln(writer, line)
		}
	}

	return lineIndex, nil
}
