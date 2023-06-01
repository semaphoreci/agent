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

	handlers "github.com/gorilla/handlers"
	mux "github.com/gorilla/mux"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	jobs "github.com/semaphoreci/agent/pkg/jobs"
	slices "github.com/semaphoreci/agent/pkg/slices"
	log "github.com/sirupsen/logrus"
)

type Server struct {
	State      string
	Logfile    io.Writer
	ActiveJob  *jobs.Job
	router     *mux.Router
	HTTPClient *http.Client
	Config     ServerConfig
}

type ServerConfig struct {
	Host           string
	Port           int
	TLSCertPath    string
	TLSKeyPath     string
	Version        string
	LogFile        io.Writer
	JWTSecret      []byte
	HTTPClient     *http.Client
	PreJobHookPath string
	FileInjections []config.FileInjection
}

const ServerStateWaitingForJob = "waiting-for-job"
const ServerStateJobReceived = "job-received"

func NewServer(config ServerConfig) *Server {
	router := mux.NewRouter().StrictSlash(true)

	server := &Server{
		Config:     config,
		State:      ServerStateWaitingForJob,
		Logfile:    config.LogFile,
		router:     router,
		HTTPClient: config.HTTPClient,
	}

	jwtMiddleware := CreateJwtMiddleware(config.JWTSecret)

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
	address := fmt.Sprintf("%s:%d", s.Config.Host, s.Config.Port)

	log.Infof("Agent %s listening on https://%s\n", s.Config.Version, address)

	loggedRouter := handlers.LoggingHandler(s.Logfile, s.router)

	log.Fatal(http.ListenAndServeTLS(
		address,
		s.Config.TLSCertPath,
		s.Config.TLSKeyPath,
		loggedRouter,
	))
}

func (s *Server) Status(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	m := make(map[string]interface{})

	m["state"] = s.State
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
	log.Infof("New job arrived. Agent version %s.", s.Config.Version)

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

	if s.State != ServerStateWaitingForJob {
		if s.ActiveJob != nil && s.ActiveJob.Request.ID == request.ID {
			// idempotent call
			fmt.Fprint(w, `{"message": "ok"}`)
			return
		}

		log.Warn("A job is already running, returning 422")

		w.WriteHeader(422)
		fmt.Fprintf(w, `{"message": "a job is already running"}`)
		return
	}

	log.Debug("Creating new job")
	job, err := jobs.NewJobWithOptions(&jobs.JobOptions{
		Request:         request,
		Client:          s.HTTPClient,
		ExposeKvmDevice: true,
		FileInjections:  s.Config.FileInjections,
		SelfHosted:      false,
		RefreshTokenFn:  nil,
		UploadJobLogs:   s.resolveUploadJobsConfig(request),
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
		PostJobHookPath:       "",
		OnJobFinished:         nil,
		CallbackRetryAttempts: 60,
	})

	log.Debugf("Setting state to '%s'", ServerStateJobReceived)
	s.State = ServerStateJobReceived

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
