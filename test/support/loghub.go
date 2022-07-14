package testsupport

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
)

const ExpiredLogToken = "expired-token"

type LoghubMockServer struct {
	Logs           []string
	BatchSizesUsed []int
	MaxSizeForLogs int
	Server         *httptest.Server
	Handler        http.Handler
}

func NewLoghubMockServer() *LoghubMockServer {
	return &LoghubMockServer{
		Logs: []string{},
	}
}

func (m *LoghubMockServer) Init() {
	mockServer := httptest.NewServer(http.HandlerFunc(m.handler))
	m.Server = mockServer
}

func (m *LoghubMockServer) SetMaxSizeForLogs(maxSize int) {
	m.MaxSizeForLogs = maxSize
}

func (m *LoghubMockServer) handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// if max size is set, and is big enough, send 422.
	if m.MaxSizeForLogs > 0 && len(m.Logs) >= m.MaxSizeForLogs {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	// just an easy way to mock the expired token scenario
	token, err := m.findToken(r)
	if err != nil || token == ExpiredLogToken {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("[LOGHUB MOCK] Error reading body: %v\n", err)
	}

	logs := FilterEmpty(strings.Split(string(body), "\n"))
	fmt.Printf("[LOGHUB MOCK] Received %d log events\n", len(logs))

	m.BatchSizesUsed = append(m.BatchSizesUsed, len(logs))
	m.Logs = append(m.Logs, logs...)

	w.WriteHeader(200)
}

func (m *LoghubMockServer) findToken(r *http.Request) (string, error) {
	reqToken := r.Header.Get("Authorization")
	if reqToken == "" {
		return "", fmt.Errorf("no token found")
	}

	splitToken := strings.Split(reqToken, "Bearer ")
	if len(splitToken) != 2 {
		return "", fmt.Errorf("malformed token")
	}

	return splitToken[1], nil
}

func (m *LoghubMockServer) GetLogs() []string {
	return m.Logs
}

func (m *LoghubMockServer) GetBatchSizesUsed() []int {
	return m.BatchSizesUsed
}

func (m *LoghubMockServer) URL() string {
	return m.Server.URL
}

func (m *LoghubMockServer) Close() {
	m.Server.Close()
}

func FilterEmpty(logs []string) []string {
	filtered := []string{}
	for _, log := range logs {
		if log != "" {
			filtered = append(filtered, log)
		}
	}

	return filtered
}
