package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	jobs "github.com/semaphoreci/agent/pkg/jobs"
)

type Server struct {
	Host      string
	Port      int
	State     string
	ActiveJob *jobs.Job

	router mux.Router
}

func NewServer(host string, port int) *Server {
	router := mux.NewRouter().StrictSlash(true)

	router.HandleFunc("/status", s.Status).Methods("GET")
	router.HandleFunc("/jobs", s.Run).Methods("POST")

	// The path /stop is the new standard, /jobs/terminate is here to support the legacy system.
	router.HandleFunc("/stop", s.Stop).Methods("POST")
	router.HandleFunc("/jobs/terminate", s.Stop).Methods("POST")

	// The path /jobs/{job_id}/log is here to support the legacy systems.
	router.HandleFunc("/job_logs", s.JobLogs).Methods("GET")
	router.HandleFunc("/jobs/{job_id}/log", s.JobLogs).Methods("GET")

	// Agent Logs
	router.HandleFunc("/agent_logs", s.AgentLogs).Methods("GET")

	server := &Server{
		Host:   host,
		Port:   port,
		State:  "waiting for job",
		router: router,
	}

	return server
}

func (s *Server) Serve() {
	address := fmt.Sprintf("%s:%d", s.Host, s.Port)

	fmt.Printf("Agent %s listening on https://%s\n", VERSION, address)

	f, err := os.OpenFile("/tmp/agent_log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)

	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	log.SetOutput(f)

	loggedRouter := handlers.LoggingHandler(f, r)

	log.Fatal(http.ListenAndServeTLS(address, "server.crt", "server.key", loggedRouter))
}

func (s *Server) Status(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(400)
	m := make(map[string]interface{})

	m["state"] = s.State
	m["version"] = VERSION

	jsonString, _ := json.Marshal(m)

	fmt.Fprintf(w, string(jsonString))
}

func (s *Server) JobLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain")

	startFromLine, err := strconv.Atoi(r.URL.Query().Get("start_from"))
	if err != nil {
		startFromLine = 0
	}

	logfile, err := os.Open("/tmp/job_log.json")

	if err != nil {
		w.WriteHeader(404)
		return
	}
	defer logfile.Close()

	logLine := 0
	scanner := bufio.NewScanner(logfile)
	for scanner.Scan() {
		if logLine >= startFromLine {
			fmt.Fprintln(w, scanner.Text())
		}

		logLine += 1
	}

	if r.Header.Get("X-Client-Name") == "archivator" {
		s.ActiveJob.JobLogArchived = true
	}
}

func (s *Server) AgentLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain")

	logfile, err := os.Open("/tmp/agent_log")

	if err != nil {
		w.WriteHeader(404)
		return
	}
	defer logfile.Close()

	io.Copy(w, logfile)
}

func (s *Server) Run(w http.ResponseWriter, r *http.Request) {
	if s.State != "waiting for job" {
		w.WriteHeader(422)
		fmt.Fprintf(w, `{"message": "a job is already running"}`)
		return
	}

	s.State = "received-job"

	request, err := api.NewRequestFromJSON(r.Body)

	if err != nil {
		fmt.Fprintf(w, `{"message": "%s"}`, err)
		return
	}

	job, err := &jobs.NewJob(request)

	if err != nil {
		fmt.Fprintf(w, `{"message": "%s"}`, err)
		return
	}

	s.ActiveJob = job
	go s.ActiveJob.Run()

	s.State = "job-started"
}

func (s *Server) Stop(w http.ResponseWriter, r *http.Request) {
	go s.ActiveJob.Stop()

	w.WriteHeader(200)
}

func (s *Server) unsuported(w http.ResponseWriter) {
	w.WriteHeader(400)
	fmt.Fprintf(w, `{"message": "not supported"}`)
}
