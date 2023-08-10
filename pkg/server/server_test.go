package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/semaphoreci/agent/pkg/api"
	"github.com/stretchr/testify/assert"
)

func Test__RunJobDoesNotAcceptMultipleJobs(t *testing.T) {
	dummyKey := "dummykey"
	testServer := NewServer(ServerConfig{
		HTTPClient: http.DefaultClient,
		JWTSecret:  []byte(dummyKey),

		// We intentionally make our server slower
		// to make these tests more reliable.
		BeforeRunJobFn: func() { time.Sleep(100 * time.Millisecond) },
	})

	token, err := generateToken(dummyKey)
	if !assert.NoError(t, err) {
		return
	}

	// Run a bunch of requests concurrently,
	// with a different job ID for each request,
	// keeping track of their responses.
	var totalReq = 20
	var wg sync.WaitGroup
	codes := make([]int, totalReq)
	for i := 0; i < totalReq; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = makeRequest(t, testServer, token, i)
		}(i)
	}

	wg.Wait()

	// Assert that only 1 request got a 200, and everything else got a 422.
	assert.Equal(t, 1, countCodes(codes, http.StatusOK))
	assert.Equal(t, totalReq-1, countCodes(codes, http.StatusUnprocessableEntity))
}

func makeRequest(t *testing.T, testServer *Server, token string, i int) int {
	request := api.JobRequest{
		JobID: fmt.Sprintf("job-%d", i),
		Callbacks: api.Callbacks{
			Finished:         "https://httpbin.org/status/200",
			TeardownFinished: "https://httpbin.org/status/200",
		},
	}

	body, err := json.Marshal(&request)
	if !assert.NoError(t, err) {
		return -1
	}

	req, _ := http.NewRequest("POST", "/jobs", bytes.NewReader(body))
	req.Header.Add("Authorization", "Token "+token)
	rr := httptest.NewRecorder()
	testServer.router.ServeHTTP(rr, req)
	return rr.Code
}

func countCodes(codes []int, code int) int {
	count := 0
	for _, c := range codes {
		if c == code {
			count++
		}
	}

	return count
}

func generateToken(key string) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
	})

	return token.SignedString([]byte(key))
}
