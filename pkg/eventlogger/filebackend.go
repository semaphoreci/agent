package eventlogger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	log "github.com/sirupsen/logrus"
)

// We use 16MB as the default max size for a job log.
// This default value is used when the job request doesn't specify a limit.
const DefaultMaxSizeInBytes = 16777216

type FileBackend struct {
	path           string
	file           *os.File
	maxSizeInBytes int
}

func NewFileBackend(path string, maxSizeInBytes int) (*FileBackend, error) {
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
	jsonBytes, err := json.Marshal(event)
	if err != nil {
		return err
	}
	jsonBytes = append(jsonBytes, '\n')

	_, err = l.file.Write(jsonBytes)
	if err != nil {
		return err
	}

	log.Debugf("%s", jsonBytes)

	return nil
}

func (l *FileBackend) Close() error {
	return l.CloseWithOptions(CloseOptions{})
}

func (l *FileBackend) CloseWithOptions(options CloseOptions) error {
	err := l.file.Close()
	if err != nil {
		log.Errorf("Error closing file %s: %v\n", l.file.Name(), err)
		return err
	}

	if options.OnClose != nil {
		fileInfo, err := os.Stat(l.file.Name())
		if err != nil {
			log.Errorf("Couldn't stat file '%s': %v", l.file.Name(), err)
		} else {
			trimmed := fileInfo.Size() >= int64(l.maxSizeInBytes)
			options.OnClose(trimmed)
		}
	}

	log.Debugf("Removing %s\n", l.file.Name())
	if err := os.Remove(l.file.Name()); err != nil {
		log.Errorf("Error removing logger file %s: %v\n", l.file.Name(), err)
		return err
	}

	return nil
}

func (l *FileBackend) Read(startingLineNumber, maxLines int, writer io.Writer) (int, error) {
	fd, err := os.OpenFile(l.path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return startingLineNumber, err
	}

	reader := bufio.NewReader(fd)
	lineNumber := 0
	linesStreamed := 0

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				_ = fd.Close()
				return lineNumber, err
			}

			break
		}

		// If current line is before the starting line we are after, we just skip it.
		if lineNumber < startingLineNumber {
			lineNumber++
			continue
		}

		// Otherwise, we advance to the next line and stream the current line.
		lineNumber++
		fmt.Fprintln(writer, line)
		linesStreamed++

		// if we have streamed the number of lines we want, we stop.
		if linesStreamed == maxLines {
			break
		}
	}

	return lineNumber, fd.Close()
}
