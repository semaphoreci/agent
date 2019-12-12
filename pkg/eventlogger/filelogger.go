package eventlogger

import "os"

type fileLogger struct {
	EventLogger

	path string
	file *os.File
}

func newFileLogger(path string) (*fileLogger, error) {
	return &fileLogger{path: path}, nil
}

func (l *fileLogger) Open() error {
	file, err := os.Create(l.path)
	if err != nil {
		return nil
	}

	l.file = file

	return nil
}

func Close() error {
	return nil
}
