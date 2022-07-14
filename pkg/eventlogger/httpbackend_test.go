package eventlogger

import (
	"testing"
	"time"

	testsupport "github.com/semaphoreci/agent/test/support"
	"github.com/stretchr/testify/assert"
)

func Test__ArgumentsMustBeValid(t *testing.T) {
	t.Run("linesPerRequest cannot be unspecified or 0", func(t *testing.T) {
		backend, err := NewHTTPBackend(HTTPBackendConfig{
			URL:            "whatever",
			Token:          "token",
			RefreshTokenFn: func() (string, error) { return "", nil },
		})

		assert.Nil(t, backend)
		assert.ErrorContains(t, err, "must be between 1 and 2000")
	})

	t.Run("linesPerRequest cannot be negative", func(t *testing.T) {
		backend, err := NewHTTPBackend(HTTPBackendConfig{
			URL:             "whatever",
			Token:           "token",
			RefreshTokenFn:  func() (string, error) { return "", nil },
			LinesPerRequest: -1,
		})

		assert.Nil(t, backend)
		assert.ErrorContains(t, err, "must be between 1 and 2000")
	})

	t.Run("linesPerRequest cannot be above the maximum allowed", func(t *testing.T) {
		backend, err := NewHTTPBackend(HTTPBackendConfig{
			URL:             "whatever",
			Token:           "token",
			RefreshTokenFn:  func() (string, error) { return "", nil },
			LinesPerRequest: 10000,
		})

		assert.Nil(t, backend)
		assert.ErrorContains(t, err, "must be between 1 and 2000")
	})

	t.Run("FlushTimeoutInSeconds cannot be unspecified or 0", func(t *testing.T) {
		backend, err := NewHTTPBackend(HTTPBackendConfig{
			URL:             "whatever",
			Token:           "token",
			LinesPerRequest: MaxLinesPerRequest,
			RefreshTokenFn:  func() (string, error) { return "", nil },
		})

		assert.Nil(t, backend)
		assert.ErrorContains(t, err, "must be between 1 and 900")
	})

	t.Run("FlushTimeoutInSeconds cannot be negative", func(t *testing.T) {
		backend, err := NewHTTPBackend(HTTPBackendConfig{
			URL:                   "whatever",
			Token:                 "token",
			LinesPerRequest:       MaxLinesPerRequest,
			FlushTimeoutInSeconds: -1,
			RefreshTokenFn:        func() (string, error) { return "", nil },
		})

		assert.Nil(t, backend)
		assert.ErrorContains(t, err, "must be between 1 and 900")
	})

	t.Run("FlushTimeoutInSeconds cannot be above the maximum allowed", func(t *testing.T) {
		backend, err := NewHTTPBackend(HTTPBackendConfig{
			URL:                   "whatever",
			Token:                 "token",
			RefreshTokenFn:        func() (string, error) { return "", nil },
			LinesPerRequest:       MaxLinesPerRequest,
			FlushTimeoutInSeconds: 1000000,
		})

		assert.Nil(t, backend)
		assert.ErrorContains(t, err, "must be between 1 and 900")
	})
}

func Test__LogsArePushedToHTTPEndpoint(t *testing.T) {
	mockServer := testsupport.NewLoghubMockServer()
	mockServer.Init()

	httpBackend, err := NewHTTPBackend(HTTPBackendConfig{
		URL:             mockServer.URL(),
		Token:           "token",
		RefreshTokenFn:  func() (string, error) { return "", nil },
		LinesPerRequest: 20,
	})

	assert.Nil(t, err)
	assert.Nil(t, httpBackend.Open())

	generateLogEvents(t, 1, httpBackend)

	err = httpBackend.Close()
	assert.Nil(t, err)

	eventObjects, err := TransformToObjects(mockServer.GetLogs())
	assert.Nil(t, err)

	simplifiedEvents, err := SimplifyLogEvents(eventObjects, true)
	assert.Nil(t, err)

	assert.Equal(t, []string{
		"job_started",

		"directive: echo hello",
		"hello\n",
		"Exit Code: 0",

		"job_finished: passed",
	}, simplifiedEvents)

	mockServer.Close()
}

func Test__RequestsAreCappedAtLinesPerRequest(t *testing.T) {
	mockServer := testsupport.NewLoghubMockServer()
	mockServer.Init()

	httpBackend, err := NewHTTPBackend(HTTPBackendConfig{
		URL:             mockServer.URL(),
		Token:           "token",
		RefreshTokenFn:  func() (string, error) { return "", nil },
		LinesPerRequest: 2,
	})

	assert.Nil(t, err)
	assert.Nil(t, httpBackend.Open())

	generateLogEvents(t, 10, httpBackend)
	_ = httpBackend.Close()

	// assert no more than 2 events were sent per batch
	for _, batchSize := range mockServer.GetBatchSizesUsed() {
		assert.LessOrEqual(t, batchSize, 2)
	}

	eventObjects, err := TransformToObjects(mockServer.GetLogs())
	assert.Nil(t, err)

	simplifiedEvents, err := SimplifyLogEvents(eventObjects, true)
	assert.Nil(t, err)

	assert.Equal(t, []string{
		"job_started",

		"directive: echo hello",
		"hello\n",
		"hello\n",
		"hello\n",
		"hello\n",
		"hello\n",
		"hello\n",
		"hello\n",
		"hello\n",
		"hello\n",
		"hello\n",
		"Exit Code: 0",

		"job_finished: passed",
	}, simplifiedEvents)

	mockServer.Close()
}

func Test__FlushingGivesUpAfterTimeout(t *testing.T) {
	mockServer := testsupport.NewLoghubMockServer()
	mockServer.Init()

	httpBackend, err := NewHTTPBackend(HTTPBackendConfig{
		URL:                   mockServer.URL(),
		Token:                 "token",
		RefreshTokenFn:        func() (string, error) { return "", nil },
		LinesPerRequest:       2,
		FlushTimeoutInSeconds: 10,
	})

	assert.Nil(t, err)
	assert.Nil(t, httpBackend.Open())

	// 1000+ log events at 2 per request
	// would take more time to flush everything that the timeout we give it.
	generateLogEvents(t, 1000, httpBackend)

	_ = httpBackend.Close()

	eventObjects, err := TransformToObjects(mockServer.GetLogs())
	assert.Nil(t, err)

	simplifiedEvents, err := SimplifyLogEvents(eventObjects, true)
	assert.Nil(t, err)

	// logs are incomplete
	assert.NotContains(t, simplifiedEvents, "job_finished: passed")

	mockServer.Close()
}

func Test__TokenIsRefreshed(t *testing.T) {
	mockServer := testsupport.NewLoghubMockServer()
	mockServer.Init()

	tokenWasRefreshed := false

	httpBackend, err := NewHTTPBackend(HTTPBackendConfig{
		URL:             mockServer.URL(),
		Token:           testsupport.ExpiredLogToken,
		LinesPerRequest: 20,
		RefreshTokenFn: func() (string, error) {
			tokenWasRefreshed = true
			return "some-new-and-shiny-valid-token", nil
		},
	})

	assert.Nil(t, err)
	assert.Nil(t, httpBackend.Open())

	generateLogEvents(t, 1, httpBackend)
	_ = httpBackend.Close()
	assert.True(t, tokenWasRefreshed)

	eventObjects, err := TransformToObjects(mockServer.GetLogs())
	assert.Nil(t, err)

	simplifiedEvents, err := SimplifyLogEvents(eventObjects, true)
	assert.Nil(t, err)

	assert.Equal(t, []string{
		"job_started",

		"directive: echo hello",
		"hello\n",
		"Exit Code: 0",

		"job_finished: passed",
	}, simplifiedEvents)

	mockServer.Close()
}

func generateLogEvents(t *testing.T, outputEventsCount int, backend *HTTPBackend) {
	timestamp := int(time.Now().Unix())

	assert.Nil(t, backend.Write(&JobStartedEvent{Timestamp: timestamp, Event: "job_started"}))
	assert.Nil(t, backend.Write(&CommandStartedEvent{Timestamp: timestamp, Event: "cmd_started", Directive: "echo hello"}))

	count := outputEventsCount
	for count > 0 {
		assert.Nil(t, backend.Write(&CommandOutputEvent{Timestamp: timestamp, Event: "cmd_output", Output: "hello\n"}))
		count--
	}

	assert.Nil(t, backend.Write(&CommandFinishedEvent{
		Timestamp:  timestamp,
		Event:      "cmd_finished",
		Directive:  "echo hello",
		ExitCode:   0,
		StartedAt:  timestamp,
		FinishedAt: timestamp,
	}))

	assert.Nil(t, backend.Write(&JobFinishedEvent{Timestamp: timestamp, Event: "job_finished", Result: "passed"}))
}
