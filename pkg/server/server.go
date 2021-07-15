package server

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"

	handlers "github.com/gorilla/handlers"
	mux "github.com/gorilla/mux"

	api "github.com/semaphoreci/agent/pkg/api"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	jobs "github.com/semaphoreci/agent/pkg/jobs"
)

type Server struct {
	Host    string
	Port    int
	State   string
	Version string

	TLSKeyPath  string
	TLSCertPath string

	JwtSecret []byte

	Logfile io.Writer

	ActiveJob *jobs.Job
	router    *mux.Router
}

const ServerStateWaitingForJob = "waiting-for-job"
const ServerStateJobReceived = "job-received"

func NewServer(host string, port int, tlsCertPath, tlsKeyPath, version string, logfile io.Writer, jwtSecret []byte) *Server {
	router := mux.NewRouter().StrictSlash(true)

	server := &Server{
		Host:        host,
		Port:        port,
		State:       ServerStateWaitingForJob,
		TLSKeyPath:  tlsKeyPath,
		TLSCertPath: tlsCertPath,
		JwtSecret:   jwtSecret,
		Logfile:     logfile,
		router:      router,
		Version:     version,
	}

	jwtMiddleware := CreateJwtMiddleware(jwtSecret)

	// The path to check if agent is running
	router.HandleFunc("/is_alive", server.isAlive).Methods("GET")

	router.HandleFunc("/status", jwtMiddleware(server.Status)).Methods("GET")
	router.HandleFunc("/jobs", jwtMiddleware(server.Run)).Methods("POST")

	// The path /stop is the new standard, /jobs/terminate is here to support the legacy system.
	router.HandleFunc("/stop", jwtMiddleware(server.Stop)).Methods("POST")
	router.HandleFunc("/jobs/terminate", jwtMiddleware(server.Stop)).Methods("POST")

	// The path /jobs/{job_id}/log is here to support the legacy systems.
	router.HandleFunc("/job_logs", jwtMiddleware(server.JobLogs)).Methods("GET")
	router.HandleFunc("/jobs/{job_id}/log", jwtMiddleware(server.JobLogs)).Methods("GET")

	// Agent Logs
	router.HandleFunc("/agent_logs", jwtMiddleware(server.AgentLogs)).Methods("GET")

	return server
}

func (s *Server) Serve() {
	address := fmt.Sprintf("%s:%d", s.Host, s.Port)

	fmt.Printf("Agent %s listening on https://%s\n", s.Version, address)

	loggedRouter := handlers.LoggingHandler(s.Logfile, s.router)

	log.Fatal(http.ListenAndServeTLS(
		address,
		s.TLSCertPath,
		s.TLSKeyPath,
		loggedRouter,
	))
}

func (s *Server) Status(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	m := make(map[string]interface{})

	m["state"] = s.State
	m["version"] = s.Version

	jsonString, _ := json.Marshal(m)

	fmt.Fprintf(w, string(jsonString))
}

func (s *Server) isAlive(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)

	fmt.Fprintf(w, "yes")
}

func (s *Server) JobLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain")

	startFromLine, err := strconv.Atoi(r.URL.Query().Get("start_from"))
	if err != nil {
		startFromLine = 0
	}

	logFile, ok := s.ActiveJob.Logger.Backend.(*eventlogger.FileBackend)
	if !ok {
		log.Printf("Failed to stream job logs")

		http.Error(w, err.Error(), 500)
		fmt.Fprintf(w, `{"message": "%s"}`, "Failed to open logfile")
	}

	_, err = logFile.Stream(startFromLine, w)
	if err != nil {
		log.Printf("Error while streaming logs")

		http.Error(w, err.Error(), 500)
		fmt.Fprintf(w, `{"message": "%s"}`, err)
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
	log.Printf("New job arrived. Agent version %s.", s.Version)

	log.Printf("Reading content of the request")
	body, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {

		log.Printf("Failed to read the content of the job, returning 500")
		http.Error(w, err.Error(), 500)
		return
	}

	log.Printf("Parsing job request")
	request, err := api.NewRequestFromJSON(body)

	if err != nil {
		log.Printf("Failed to parse job request, returning 422")
		log.Printf("%+v", err)

		http.Error(w, err.Error(), 422)
		fmt.Fprintf(w, `{"message": "%s"}`, err)
		return
	}

	if s.State != ServerStateWaitingForJob {
		if s.ActiveJob != nil && s.ActiveJob.Request.ID == request.ID {
			// idempotent call
			fmt.Fprint(w, `{"message": "ok"}`)
			return
		} else {
			log.Printf("A job is already running, returning 422")

			w.WriteHeader(422)
			fmt.Fprintf(w, `{"message": "a job is already running"}`)
			return
		}
	}

	log.Printf("Creating new job")
	job, err := jobs.NewJob(request)

	if err != nil {
		log.Printf("Failed to create a new job, returning 500")

		http.Error(w, err.Error(), 500)
		fmt.Fprintf(w, `{"message": "%s"}`, err)
		return
	}

	log.Printf("Setting up Active Job context")
	s.ActiveJob = job

	log.Printf("Starting job execution")
	go s.ActiveJob.Run(nil)

	log.Printf("Setting state to '%s'", ServerStateJobReceived)
	s.State = ServerStateJobReceived

	log.Printf("Respongind with OK")
	fmt.Fprint(w, `{"message": "ok"}`)
}

func (s *Server) Stop(w http.ResponseWriter, r *http.Request) {
	go s.ActiveJob.Stop()

	w.WriteHeader(200)
}

func (s *Server) unsuported(w http.ResponseWriter) {
	w.WriteHeader(400)
	fmt.Fprintf(w, `{"message": "not supported"}`)
}
