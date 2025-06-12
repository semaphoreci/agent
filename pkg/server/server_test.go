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

func Test__JobLogs(t *testing.T) {
	dummyKey := "dummykey"
	testServer := NewServer(ServerConfig{
		HTTPClient: http.DefaultClient,
		JWTSecret:  []byte(dummyKey),
	})

	token, err := generateToken(dummyKey)
	if !assert.NoError(t, err) {
		return
	}

	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	defer callbackServer.Close()

	t.Run("no active job -> 404", func(t *testing.T) {
		code, _ := getLogs(t, testServer, "job-0", token)
		assert.Equal(t, http.StatusNotFound, code)
	})

	t.Run("job running and job on request do not match -> 403", func(t *testing.T) {
		// Start a job with ID 'job-0'
		code, _ := postJob(t, testServer, nil, token, 0, callbackServer.URL)
		assert.Equal(t, http.StatusOK, code)

		code, _ = getLogs(t, testServer, "id-not-matching", token)
		assert.Equal(t, http.StatusForbidden, code)
	})

	t.Run("job running and job on request match -> 200", func(t *testing.T) {
		code, _ := getLogs(t, testServer, "job-0", token)
		assert.Equal(t, http.StatusOK, code)
	})
}

func Test__ServerStatus(t *testing.T) {
	dummyKey := "dummykey"
	testServer := NewServer(ServerConfig{
		HTTPClient: http.DefaultClient,
		JWTSecret:  []byte(dummyKey),
	})

	token, err := generateToken(dummyKey)
	if !assert.NoError(t, err) {
		return
	}

	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	defer callbackServer.Close()

	// no job yet
	assert.Equal(t, ServerStateWaitingForJob, getAgentStatus(t, testServer, token))

	// run job and assert state changes
	code, _ := postJob(t, testServer, nil, token, 0, callbackServer.URL)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, ServerStateJobReceived, getAgentStatus(t, testServer, token))
}

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

	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	defer callbackServer.Close()

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
			code, _ := postJob(t, testServer, nil, token, i, callbackServer.URL)
			codes[i] = code
		}(i)
	}

	wg.Wait()

	// Assert that only 1 request gets a 200, and all the other ones get a 422.
	assert.Equal(t, 1, countCodes(codes, http.StatusOK))
	assert.Equal(t, totalReq-1, countCodes(codes, http.StatusUnprocessableEntity))
}

func Test__RunJobAcceptsSameJobAgain(t *testing.T) {
	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	defer callbackServer.Close()

	dummyKey := "dummykey"
	testServer := NewServer(ServerConfig{
		HTTPClient: http.DefaultClient,
		JWTSecret:  []byte(dummyKey),

		// We intentionally make our server slower
		// to make these tests more reliable.
		BeforeRunJobFn: func() { time.Sleep(100 * time.Millisecond) },
	})

	request := &api.JobRequest{
		JobID: "same-job-id",
		Callbacks: api.Callbacks{
			Finished:         callbackServer.URL,
			TeardownFinished: callbackServer.URL,
		},
	}

	token, err := generateToken(dummyKey)
	if !assert.NoError(t, err) {
		return
	}

	// Run a bunch of requests concurrently,
	// with the same job ID, keeping track of their responses.
	var totalReq = 20
	var wg sync.WaitGroup
	codes := make([]int, totalReq)
	bodies := make([]string, totalReq)
	for i := 0; i < totalReq; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, b := postJob(t, testServer, request, token, i, callbackServer.URL)
			body := map[string]string{}
			err := json.Unmarshal(b.Bytes(), &body)
			if !assert.NoError(t, err) {
				return
			}

			codes[i] = code
			bodies[i] = body["message"]
		}(i)
	}

	wg.Wait()

	// Assert that all requests get a 200,
	// but only one of them, receive a "ok" message.
	// The other ones receives a "job is already running" message.
	assert.Equal(t, totalReq, countCodes(codes, http.StatusOK))
	assert.Equal(t, 1, countBodies(bodies, "ok"))
	assert.Equal(t, totalReq-1, countBodies(bodies, "job is already running"))
}

func getAgentStatus(t *testing.T, testServer *Server, token string) string {
	req, _ := http.NewRequest("GET", "/status", nil)
	req.Header.Add("Authorization", "Token "+token)
	rr := httptest.NewRecorder()
	testServer.router.ServeHTTP(rr, req)

	resp := map[string]string{}
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		return ""
	}

	return resp["state"]
}

func getLogs(t *testing.T, testServer *Server, jobID, token string) (int, *bytes.Buffer) {
	req, _ := http.NewRequest("GET", fmt.Sprintf("/jobs/%s/log", jobID), nil)
	req.Header.Add("Authorization", "Token "+token)
	rr := httptest.NewRecorder()
	testServer.router.ServeHTTP(rr, req)
	return rr.Code, rr.Body
}

func postJob(t *testing.T, testServer *Server, jobReq *api.JobRequest, token string, i int, callbackURL string) (int, *bytes.Buffer) {
	jobRequest := jobReq
	if jobRequest == nil {
		jobRequest = &api.JobRequest{
			JobID: fmt.Sprintf("job-%d", i),
			Callbacks: api.Callbacks{
				Finished:         callbackURL,
				TeardownFinished: callbackURL,
			},
		}
	}

	body, err := json.Marshal(&jobRequest)
	if !assert.NoError(t, err) {
		return -1, nil
	}

	req, _ := http.NewRequest("POST", "/jobs", bytes.NewReader(body))
	req.Header.Add("Authorization", "Token "+token)
	rr := httptest.NewRecorder()
	testServer.router.ServeHTTP(rr, req)
	return rr.Code, rr.Body
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

func countBodies(bodies []string, body string) int {
	count := 0
	for _, b := range bodies {
		if b == body {
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
