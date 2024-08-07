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
	"github.com/stretchr/testify/require"
)

func Test__GeneratePlainLogs(t *testing.T) {
	tmpFileName := filepath.Join(os.TempDir(), fmt.Sprintf("logs_%d.json", time.Now().UnixNano()))
	backend, _ := NewFileBackend(tmpFileName, DefaultMaxSizeInBytes)
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

func Benchmark__GeneratePlainLogsPerformance(b *testing.B) {
	fileName := filepath.Join("/Users/lucaspin/Desktop/118m-logs.json")
	backend, _ := NewFileBackend(fileName, DefaultMaxSizeInBytes)
	logger, _ := NewLogger(backend)
	tmpDir, err := os.MkdirTemp("", "gen-plain-logs-test-*")
	require.Nil(b, err)

	b.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f, err := logger.GeneratePlainTextFileIn(tmpDir)
		require.Nil(b, err)
		require.FileExists(b, f)
	}
}
