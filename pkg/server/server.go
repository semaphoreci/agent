package server

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	handlers "github.com/gorilla/handlers"
	mux "github.com/gorilla/mux"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	jobs "github.com/semaphoreci/agent/pkg/jobs"
	slices "github.com/semaphoreci/agent/pkg/slices"
	log "github.com/sirupsen/logrus"
)

const DefaultCallbackRetryAttempts = 300

type Server struct {
	Logfile    io.Writer
	ActiveJob  *jobs.Job
	router     *mux.Router
	HTTPClient *http.Client
	Config     ServerConfig

	activeJobLock sync.Mutex
}

type ServerConfig struct {
	Host                  string
	Port                  int
	UserAgent             string
	TLSCertPath           string
	TLSKeyPath            string
	Version               string
	LogFile               io.Writer
	JWTSecret             []byte
	HTTPClient            *http.Client
	PreJobHookPath        string
	FileInjections        []config.FileInjection
	CallbackRetryAttempts int
	ExposeKvmDevice       bool

	// A way to execute some code before handling a POST /jobs request.
	// Currently, only used to make tests that assert race condition scenarios more reproducible.
	BeforeRunJobFn func()
}

func (c *ServerConfig) GetCallbackRetryAttempts() int {
	if c.CallbackRetryAttempts == 0 {
		return DefaultCallbackRetryAttempts
	}

	return c.CallbackRetryAttempts
}

const ServerStateWaitingForJob = "waiting-for-job"
const ServerStateJobReceived = "job-received"

func NewServer(config ServerConfig) *Server {
	router := mux.NewRouter().StrictSlash(true)

	server := &Server{
		Config:     config,
		Logfile:    config.LogFile,
		router:     router,
		HTTPClient: config.HTTPClient,
	}

	jwtMiddleware := CreateJwtMiddleware(config.JWTSecret)

	// The path to check if agent is running
	router.HandleFunc("/is_alive", server.isAlive).Methods("GET")

	router.HandleFunc("/status", jwtMiddleware(server.Status)).Methods("GET")
	router.HandleFunc("/jobs", jwtMiddleware(server.Run)).Methods("POST")
	router.HandleFunc("/jobs/{job_id}/log", jwtMiddleware(server.JobLogs)).Methods("GET")

	// The path /stop is the new standard, /jobs/terminate is here to support the legacy system.
	router.HandleFunc("/stop", jwtMiddleware(server.Stop)).Methods("POST")
	router.HandleFunc("/jobs/terminate", jwtMiddleware(server.Stop)).Methods("POST")

	// Agent Logs
	router.HandleFunc("/agent_logs", jwtMiddleware(server.AgentLogs)).Methods("GET")

	return server
}

func (s *Server) Serve() {
	address := fmt.Sprintf("%s:%d", s.Config.Host, s.Config.Port)

	log.Infof("Agent %s listening on https://%s\n", s.Config.Version, address)

	loggedRouter := handlers.LoggingHandler(s.Logfile, s.router)

	server := &http.Server{
		Addr:              address,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       30 * time.Second,
		Handler:           loggedRouter,
	}

	err := server.ListenAndServeTLS(s.Config.TLSCertPath, s.Config.TLSKeyPath)
	if err != nil {
		panic(err)
	}
}

func (s *Server) Status(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	m := make(map[string]interface{})

	state := ServerStateWaitingForJob
	if s.ActiveJob != nil {
		state = ServerStateJobReceived
	}

	m["state"] = state
	m["version"] = s.Config.Version

	jsonString, _ := json.Marshal(m)

	fmt.Fprint(w, string(jsonString))
}

func (s *Server) isAlive(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)

	fmt.Fprintf(w, "yes")
}

func (s *Server) JobLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain")

	jobID := mux.Vars(r)["job_id"]

	// If no jobs have been received yet, we have no logs.
	if s.ActiveJob == nil {
		log.Warnf("Attempt to fetch logs for '%s' before any job is received", jobID)
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"message": "job %s is not running"}`, jobID)
		return
	}

	// Here, we know that a job was scheduled.
	// We need to ensure the ID in the request matches the one executing.
	runningJobID := s.ActiveJob.Request.JobID
	if runningJobID != jobID {
		log.Warnf("Attempt to fetch logs for '%s', but job '%s' is the one running", jobID, runningJobID)
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"message": "job %s is not running"}`, jobID)
		return
	}

	startFromLine, err := strconv.Atoi(r.URL.Query().Get("start_from"))
	if err != nil {
		startFromLine = 0
	}

	_, err = s.ActiveJob.Logger.Backend.Read(startFromLine, math.MaxInt32, w)
	if err != nil {
		log.Errorf("Error while streaming logs: %v", err)

		http.Error(w, err.Error(), 500)
		fmt.Fprintf(w, `{"message": "%s"}`, err)
	}

	if r.Header.Get("X-Client-Name") == "archivator" {
		s.ActiveJob.JobLogArchived = true
	}
}

func (s *Server) AgentLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain")

	logsPath := filepath.Join(os.TempDir(), "agent_log")

	// #nosec
	logfile, err := os.Open(logsPath)

	if err != nil {
		w.WriteHeader(404)
		return
	}

	_, err = io.Copy(w, logfile)
	if err != nil {
		log.Errorf("Error writing agent logs: %v", err)
	}

	err = logfile.Close()
	if err != nil {
		log.Errorf("Error closing agent logs file: %v", err)
	}
}

func (s *Server) Run(w http.ResponseWriter, r *http.Request) {
	s.activeJobLock.Lock()
	defer s.activeJobLock.Unlock()

	log.Infof("New job arrived. Agent version %s.", s.Config.Version)

	if s.Config.BeforeRunJobFn != nil {
		s.Config.BeforeRunJobFn()
	}

	log.Debug("Reading content of the request")
	body, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {

		log.Errorf("Failed to read the content of the job, returning 500: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}

	log.Debug("Parsing job request")
	request, err := api.NewRequestFromJSON(body)

	if err != nil {
		log.Errorf("Failed to parse job request, returning 422: %v", err)

		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		fmt.Fprintf(w, `{"message": "%s"}`, err)
		return
	}

	// If there's an active job already, we check if the IDs match.
	// If they do, we return a 200, but don't do anything (idempotency).
	// If they don't, we return a 422, since only one job should be run at a time.
	if s.ActiveJob != nil {
		if s.ActiveJob.Request.JobID == request.JobID {
			log.Infof("Job %s is already running - no need to run again", s.ActiveJob.Request.JobID)
			fmt.Fprint(w, `{"message": "job is already running"}`)
			return
		}

		log.Warnf("Another job %s is already running - rejecting %s", s.ActiveJob.Request.JobID, request.JobID)
		w.WriteHeader(422)
		fmt.Fprintf(w, `{"message": "another job is already running"}`)
		return
	}

	log.Infof("Creating new job for %s", request.JobID)
	job, err := jobs.NewJobWithOptions(&jobs.JobOptions{
		Request:         request,
		Client:          s.HTTPClient,
		ExposeKvmDevice: s.Config.ExposeKvmDevice,
		FileInjections:  s.Config.FileInjections,
		SelfHosted:      false,
		RefreshTokenFn:  nil,
		UploadJobLogs:   s.resolveUploadJobsConfig(request),
		UserAgent:       s.Config.UserAgent,
	})

	if err != nil {
		log.Errorf("Failed to create a new job, returning 500: %v", err)

		http.Error(w, err.Error(), 500)
		fmt.Fprintf(w, `{"message": "%s"}`, err)
		return
	}

	log.Debug("Setting up Active Job context")
	s.ActiveJob = job

	log.Debug("Starting job execution")
	go s.ActiveJob.RunWithOptions(jobs.RunOptions{
		EnvVars:               []config.HostEnvVar{},
		PreJobHookPath:        s.Config.PreJobHookPath,
		FailOnPreJobHookError: true, // cloud jobs should always fail if the pre-job hook fails
		SourcePreJobHook:      true, // cloud jobs should always source the pre-job hook
		PostJobHookPath:       "",
		OnJobFinished:         nil,
		CallbackRetryAttempts: s.Config.GetCallbackRetryAttempts(),
	})

	fmt.Fprint(w, `{"message": "ok"}`)
}

func (s *Server) Stop(w http.ResponseWriter, r *http.Request) {
	go s.ActiveJob.Stop()

	w.WriteHeader(200)
}

func (s *Server) resolveUploadJobsConfig(jobRequest *api.JobRequest) string {
	value, err := jobRequest.FindEnvVar("SEMAPHORE_AGENT_UPLOAD_JOB_LOGS")

	// We use config.UploadJobLogsConditionNever, by default.
	if err != nil {
		return config.UploadJobLogsConditionNever
	}

	// If the value specified is not a valid one, use the default.
	if !slices.Contains(config.ValidUploadJobLogsCondition, value) {
		log.Debugf(
			"The value '%s' is not acceptable as SEMAPHORE_AGENT_UPLOAD_JOB_LOGS - using '%s'",
			value, config.UploadJobLogsConditionNever,
		)

		return config.UploadJobLogsConditionNever
	}

	// Otherwise, use the value specified by job definition.
	return value
}
