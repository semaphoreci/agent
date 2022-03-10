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
	err := l.file.Close()
	if err != nil {
		log.Errorf("Error closing file %s: %v\n", l.file.Name(), err)
		return err
	}

	log.Debugf("Removing %s\n", l.file.Name())
	if err := os.Remove(l.file.Name()); err != nil {
		log.Errorf("Error removing logger file %s: %v\n", l.file.Name(), err)
		return err
	}

	return nil
}

func (l *FileBackend) Stream(startLine int, writer io.Writer) (int, error) {
	fd, err := os.OpenFile(l.path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return startLine, err
	}

	reader := bufio.NewReader(fd)
	lineIndex := 0

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				_ = fd.Close()
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

	return lineIndex, fd.Close()
}
