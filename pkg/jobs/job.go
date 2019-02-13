package jobs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	executors "github.com/semaphoreci/agent/pkg/executors"
	shellexecutor "github.com/semaphoreci/agent/pkg/executors/shell"
)

const JOB_PASSED = "passed"
const JOB_FAILED = "failed"

type Job struct {
	Request  *api.JobRequest
	Executor executors.Executor

	JobLogArchived bool
	Stopped        bool

	logfile *os.File // TODO: extract me
}

func NewJob(request *api.JobRequest) (*Job, error) {
	executor := shellexecutor.NewShellExecutor()

	log.Printf("Job Request %+v\n", request)

	// TODO: find better place for this
	logfile, err := os.Create("/tmp/job_log.json")
	if err != nil {
		panic(err)
	}

	return &Job{
		Request:        request,
		Executor:       executor,
		JobLogArchived: false,
		Stopped:        false,
		logfile:        logfile,
	}, nil
}

func (job *Job) EventHandler(event interface{}) {
	switch e := event.(type) {
	case *executors.CommandStartedEvent:
		LogCmdStarted(job.logfile, e.Timestamp, e.Directive)
	case *executors.CommandOutputEvent:
		LogCmdOutput(job.logfile, e.Timestamp, e.Output)
	case *executors.CommandFinishedEvent:
		LogCmdFinished(job.logfile, e.Timestamp, e.Directive, e.ExitCode, e.StartedAt, e.FinishedAt)
	default:
		log.Printf("(err) Unknown executor event event: %+v", e)
	}
}

func (job *Job) Run() {
	job.SendStartedCallback()
	LogJobStart(job.logfile)

	result := job.RunRegularCommands()

	job.RunEpilogueCommands(result)

	job.SendFinishedCallback(result)
	LogJobFinish(job.logfile, result)

	job.WaitForArchivator()

	job.logfile.Sync()
	job.logfile.Close()
}

func (job *Job) RunRegularCommands() string {
	exitCode := job.Executor.Prepare()
	if exitCode != 0 {
		return JOB_FAILED
	}

	exitCode = job.Executor.Start()
	if exitCode != 0 {
		return JOB_FAILED
	}

	exitCode = job.Executor.ExportEnvVars(job.Request.EnvVars, job.EventHandler)
	if exitCode != 0 {
		return JOB_FAILED
	}

	exitCode = job.Executor.InjectFiles(job.Request.Files, job.EventHandler)
	if exitCode != 0 {
		return JOB_FAILED
	}

	for _, c := range job.Request.Commands {
		exitCode = job.Executor.RunCommand(c.Directive, job.EventHandler)

		if exitCode != 0 {
			return JOB_FAILED
		}
	}

	return JOB_PASSED
}

func (job *Job) RunEpilogueCommands(result string) {
	log.Printf("[JOB] Epilogue Commands Started")

	cmd := fmt.Sprintf("export SEMAPHORE_JOB_RESULT=%s", result)
	job.Executor.RunCommand(cmd, job.EventHandler)

	for _, c := range job.Request.EpilogueCommands {
		// exit code is ignored in epilogue commands
		job.Executor.RunCommand(c.Directive, job.EventHandler)
	}
}

func (job *Job) WaitForArchivator() {
	for {
		if job.JobLogArchived {
			job.SendTeardownFinishedCallback()
			break
		}

		time.Sleep(1000 * time.Millisecond)
	}
}

func (j *Job) Stop() {
	log.Printf("Stopping job")

	j.Executor.Stop()

	j.Stopped = true
}

func LogJobStart(logfile *os.File) {
	m := make(map[string]interface{})

	m["event"] = "job_started"
	m["timestamp"] = int(time.Now().Unix())

	jsonString, _ := json.Marshal(m)

	logfile.Write([]byte(jsonString))
	logfile.Write([]byte("\n"))
}

func LogJobFinish(logfile *os.File, result string) {
	m := make(map[string]interface{})

	m["event"] = "job_finished"
	m["timestamp"] = int(time.Now().Unix())
	m["result"] = result

	jsonString, _ := json.Marshal(m)

	logfile.Write([]byte(jsonString))
	logfile.Write([]byte("\n"))
}

func LogCmdStarted(logfile *os.File, timestamp int, directive string) {
	m := make(map[string]interface{})

	m["event"] = "cmd_started"
	m["timestamp"] = timestamp
	m["directive"] = directive

	jsonString, _ := json.Marshal(m)

	logfile.Write([]byte(jsonString))
	logfile.Write([]byte("\n"))
}

func LogCmdOutput(logfile *os.File, timestamp int, output string) {
	m := make(map[string]interface{})

	m["event"] = "cmd_output"
	m["timestamp"] = timestamp
	m["output"] = output

	jsonString, _ := json.Marshal(m)

	logfile.Write([]byte(jsonString))
	logfile.Write([]byte("\n"))
}

func LogCmdFinished(logfile *os.File, timestamp int, directive string, exitCode int, startedAt int, finishedAt int) {
	m := make(map[string]interface{})

	m["event"] = "cmd_finished"
	m["timestamp"] = timestamp
	m["directive"] = directive
	m["exit_code"] = exitCode
	m["started_at"] = startedAt
	m["finished_at"] = finishedAt

	jsonString, _ := json.Marshal(m)

	logfile.Write([]byte(jsonString))
	logfile.Write([]byte("\n"))
}

func (job *Job) SendStartedCallback() error {
	payload := `{"port": 22}`

	return job.SendCallback(job.Request.Callbacks.Started, payload)
}

func (job *Job) SendFinishedCallback(result string) error {
	payload := fmt.Sprintf(`{"result": "%s"}`, result)

	return job.SendCallback(job.Request.Callbacks.Finished, payload)
}

func (job *Job) SendTeardownFinishedCallback() error {
	return job.SendCallback(job.Request.Callbacks.TeardownFinished, "{}")
}

func (job *Job) SendCallback(url string, payload string) error {
	log.Printf("[JOB] Sending callback: %s with %+v\n", url, payload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer([]byte(payload)))

	log.Printf("%+v\n", resp)

	return err
}
