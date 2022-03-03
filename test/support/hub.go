package testsupport

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
	"github.com/semaphoreci/agent/pkg/retry"
)

type HubMockServer struct {
	Server                    *httptest.Server
	Handler                   http.Handler
	JobRequest                *api.JobRequest
	LogsURL                   string
	RegisterRequest           *selfhostedapi.RegisterRequest
	RegisterAttemptRejections int
	RegisterAttempts          int
	ShouldShutdown            bool
	Disconnected              bool
	RunningJob                bool
	FinishedJob               bool
}

func NewHubMockServer() *HubMockServer {
	return &HubMockServer{
		RegisterAttempts: -1,
	}
}

func (m *HubMockServer) Init() {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.Path; {
		case strings.Contains(path, "/register"):
			m.handleRegisterRequest(w, r)
		case strings.Contains(path, "/sync"):
			m.handleSyncRequest(w, r)
		case strings.Contains(path, "/disconnect"):
			fmt.Printf("[HUB MOCK] Received disconnect request\n")
			m.Disconnected = true
			w.WriteHeader(200)
		case strings.Contains(path, "/jobs/"):
			m.handleGetJobRequest(w, r)
		}
	}))

	m.Server = mockServer
}

func (m *HubMockServer) handleRegisterRequest(w http.ResponseWriter, r *http.Request) {
	m.RegisterAttempts++
	if m.RegisterAttempts < m.RegisterAttemptRejections {
		fmt.Printf("[HUB MOCK] Attempts: %d, Rejections: %d, rejecting...\n", m.RegisterAttempts, m.RegisterAttemptRejections)
		w.WriteHeader(500)
	}

	request := selfhostedapi.RegisterRequest{}
	bytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("[HUB MOCK] Error reading register request body: %v\n", err)
		w.WriteHeader(500)
		return
	}

	err = json.Unmarshal(bytes, &request)
	if err != nil {
		fmt.Printf("[HUB MOCK] Error unmarshaling register request: %v\n", err)
		w.WriteHeader(500)
		return
	}

	fmt.Printf("[HUB MOCK] Received register request: %v\n", request)
	m.RegisterRequest = &request

	registerResponse := &selfhostedapi.RegisterResponse{
		Name:  request.Name,
		Token: "token",
	}

	response, err := json.Marshal(registerResponse)
	if err != nil {
		fmt.Printf("[HUB MOCK] Error marshaling register response: %v\n", err)
		w.WriteHeader(500)
		return
	}

	w.Write(response)
}

func (m *HubMockServer) handleSyncRequest(w http.ResponseWriter, r *http.Request) {
	request := selfhostedapi.SyncRequest{}
	bytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("[HUB MOCK] Error reading sync request body: %v\n", err)
		w.WriteHeader(500)
		return
	}

	err = json.Unmarshal(bytes, &request)
	if err != nil {
		fmt.Printf("[HUB MOCK] Error unmarshaling sync request: %v\n", err)
		w.WriteHeader(500)
		return
	}

	fmt.Printf("[HUB MOCK] Received sync request: %v\n", request)

	syncResponse := selfhostedapi.SyncResponse{
		Action: selfhostedapi.AgentActionContinue,
	}

	switch request.State {
	case selfhostedapi.AgentStateWaitingForJobs:
		if m.ShouldShutdown {
			syncResponse.Action = selfhostedapi.AgentActionShutdown
		}

		if m.JobRequest != nil {
			syncResponse.Action = selfhostedapi.AgentActionRunJob
			syncResponse.JobID = m.JobRequest.ID
		}

	case selfhostedapi.AgentStateRunningJob:
		m.RunningJob = true

		if m.ShouldShutdown {
			syncResponse.Action = selfhostedapi.AgentActionStopJob
			syncResponse.JobID = m.JobRequest.ID
		}

	case selfhostedapi.AgentStateFinishedJob:
		m.JobRequest = nil
		m.FinishedJob = true

		if m.ShouldShutdown {
			syncResponse.Action = selfhostedapi.AgentActionShutdown
		} else {
			syncResponse.Action = selfhostedapi.AgentActionWaitForJobs
		}

	case selfhostedapi.AgentStateFailedToFetchJob,
		selfhostedapi.AgentStateFailedToConstructJob,
		selfhostedapi.AgentStateFailedToSendCallback:
		syncResponse.Action = selfhostedapi.AgentActionWaitForJobs
	}

	response, err := json.Marshal(syncResponse)
	if err != nil {
		fmt.Printf("[HUB MOCK] Error marshaling sync response: %v\n", err)
		w.WriteHeader(500)
		return
	}

	w.Write(response)
}

func (m *HubMockServer) handleGetJobRequest(w http.ResponseWriter, r *http.Request) {
	if m.JobRequest == nil {
		fmt.Printf("[HUB MOCK] No jobRequest in use\n")
		w.WriteHeader(404)
	} else {
		response, err := json.Marshal(m.JobRequest)
		if err != nil {
			fmt.Printf("[HUB MOCK] Error marshaling job request: %v\n", err)
			w.WriteHeader(500)
		} else {
			w.Write(response)
		}
	}
}

func (m *HubMockServer) UseLogsURL(URL string) {
	m.LogsURL = URL
}

func (m *HubMockServer) AssignJob(jobRequest *api.JobRequest) {
	m.JobRequest = jobRequest
}

func (m *HubMockServer) RejectRegisterAttempts(times int) {
	m.RegisterAttemptRejections = times
}

func (m *HubMockServer) URL() string {
	return m.Server.URL
}

func (m *HubMockServer) Host() string {
	return m.Server.Listener.Addr().String()
}

func (m *HubMockServer) WaitUntilRunningJob(attempts int, wait time.Duration) error {
	return retry.RetryWithConstantWait("WaitUntilRunningJob", attempts, wait, func() error {
		if !m.RunningJob {
			return fmt.Errorf("still not running job")
		}

		return nil
	})
}

func (m *HubMockServer) WaitUntilFinishedJob(attempts int, wait time.Duration) error {
	return retry.RetryWithConstantWait("WaitUntilFinishedJob", attempts, wait, func() error {
		if !m.FinishedJob {
			return fmt.Errorf("still not finished job")
		}

		return nil
	})
}

func (m *HubMockServer) WaitUntilDisconnected(attempts int, wait time.Duration) error {
	return retry.RetryWithConstantWait("WaitUntilDisconnected", attempts, wait, func() error {
		if !m.Disconnected {
			return fmt.Errorf("still not disconnected")
		}

		return nil
	})
}

func (m *HubMockServer) WaitUntilRegistered() error {
	return retry.RetryWithConstantWait("WaitUntilRegistered", 10, time.Second, func() error {
		if m.RegisterRequest == nil {
			return fmt.Errorf("still not registered")
		}

		return nil
	})
}

func (m *HubMockServer) GetRegisterRequest() *selfhostedapi.RegisterRequest {
	return m.RegisterRequest
}

func (m *HubMockServer) ScheduleShutdown() {
	m.ShouldShutdown = true
}

func (m *HubMockServer) Close() {
	m.Server.Close()
}
