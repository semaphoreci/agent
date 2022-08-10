package eventlogger

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test__GeneratePlainLogs(t *testing.T) {
	tmpFileName := filepath.Join(os.TempDir(), fmt.Sprintf("logs_%d.json", time.Now().UnixNano()))
	backend, _ := NewFileBackend(tmpFileName)
	assert.Nil(t, backend.Open())
	logger, _ := NewLogger(backend)
	generateLogEvents(t, 10, backend)

	file, err := logger.GeneratePlainTextFile()
	assert.NoError(t, err)
	assert.FileExists(t, file)

	bytes, err := ioutil.ReadFile(file)
	assert.NoError(t, err)

	lines := strings.Split(string(bytes), "\n")
	assert.Equal(t, []string{
		"echo hello",
		"hello",
		"hello",
		"hello",
		"hello",
		"hello",
		"hello",
		"hello",
		"hello",
		"hello",
		"hello",
		"",
	}, lines)

	assert.NoError(t, logger.Close())
	os.Remove(file)
}
