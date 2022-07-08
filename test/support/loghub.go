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
	Logs    []string
	Server  *httptest.Server
	Handler http.Handler
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

func (m *LoghubMockServer) handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// just an easy way to mock the expired token scenario
	token, err := m.findToken(r)
	if err != nil || token == ExpiredLogToken {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	fmt.Println("[LOGHUB MOCK] Received logs")
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("Error reading body: %v\n", err)
	}

	logs := strings.Split(string(body), "\n")
	m.Logs = append(m.Logs, FilterEmpty(logs)...)
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
