package eventlogger

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
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

	bytes, err := os.ReadFile(file)
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

func Benchmark__GeneratePlainLogs(b *testing.B) {
	//
	// We do not want to account for this setup time in our benchmark
	// so we stop the timer here, while we are creating the file backend
	// and generating and writing the log events to it.
	//
	b.StopTimer()
	tmpFileName := filepath.Join(os.TempDir(), fmt.Sprintf("logs_%d.json", time.Now().UnixNano()))
	backend, _ := NewFileBackend(tmpFileName, DefaultMaxSizeInBytes)
	require.Nil(b, backend.Open())
	logger, _ := NewLogger(backend)

	//
	// Write a lot of log events into our file backend.
	// In this case, 1M `cmd_output` log events with a random string in it.
	//
	buf := make([]byte, 45)
	expected := []string{}
	expected = append(expected, "echo hello")
	generateLogEventsWithOutputGenerator(b, 1000000, backend, func() string {
		// #nosec
		_, err := rand.Read(buf)
		require.NoError(b, err)
		o := base64.URLEncoding.EncodeToString(buf)
		expected = append(expected, o)
		return o
	})

	expected = append(expected, "")

	//
	// Actually run the benchmark.
	// We start the timer at the beginning of the iteration,
	// and stop it right after logger.GeneratePlainTextFile() returns,
	// because we only want to account for the amount of time it takes
	// for that function to run, but we also want to assert the output is correct.
	//
	for i := 0; i < b.N; i++ {
		b.StartTimer()
		file, err := logger.GeneratePlainTextFile()

		b.StopTimer()
		require.NoError(b, err)
		require.FileExists(b, file)
		bytes, err := os.ReadFile(file)
		require.NoError(b, err)
		assert.Equal(b, expected, strings.Split(string(bytes), "\n"))

		os.Remove(file)
	}

	require.NoError(b, logger.Close())
}
