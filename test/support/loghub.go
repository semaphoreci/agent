package testsupport

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
)

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
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			fmt.Println("[LOGHUB MOCK] Received logs")
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				fmt.Printf("Error reading body: %v\n", err)
			}

			logs := strings.Split(string(body), "\n")
			m.Logs = append(m.Logs, FilterEmpty(logs)...)
			w.WriteHeader(200)
		} else {
			fmt.Println("NOPE")
		}
	}))

	m.Server = mockServer
}

func (m *LoghubMockServer) GetLogs() []string {
	return m.Logs
}

func (m *LoghubMockServer) Url() string {
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
