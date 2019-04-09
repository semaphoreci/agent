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
	log.Printf("Constructing an executor for the job")

	if request.Executor == "" {
		request.Executor = executors.ExecutorTypeShell
	}

	executor, err := executors.CreateExecutor(request)

	if err != nil {
		return nil, err
	}

	log.Printf("Job Request %+v\n", request)

	// TODO: find better place for this
	logfile, err := os.Create("/tmp/job_log.json")
	if err != nil {
		panic(err)
	}

	log.Printf("Constructed job")

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
	log.Printf("Job Started")

	job.SendStartedCallback()
	LogJobStart(job.logfile)

	result := job.RunRegularCommands()

	log.Printf("Regular Commands Finished. Result: %s", result)

	log.Printf("Exporting job result")

	job.RunCommandsUntilFirstFailure([]api.Command{
		api.Command{
			Directive: fmt.Sprintf("export SEMAPHORE_JOB_RESULT=%s", result),
		},
	})

	log.Printf("Starting Epilogue Always Commands.")
	job.RunCommandsUntilFirstFailure(job.Request.EpilogueAlwaysCommands)

	if result == JOB_PASSED {
		log.Printf("Starting Epilogue On Pass Commands.")
		job.RunCommandsUntilFirstFailure(job.Request.EpilogueOnPassCommands)
	} else {
		log.Printf("Starting Epilogue On Fail Commands.")
		job.RunCommandsUntilFirstFailure(job.Request.EpilogueOnFailCommands)
	}

	log.Printf("Sending finished callback.")

	job.SendFinishedCallback(result)

	LogJobFinish(job.logfile, result)

	log.Printf("Waiting for archivator")

	job.WaitForArchivator()

	log.Printf("Archivator finished")

	job.logfile.Sync()
	job.logfile.Close()

	log.Printf("Job Teardown Finished")
}

func (job *Job) RunRegularCommands() string {
	exitCode := job.Executor.Prepare()
	if exitCode != 0 {
		log.Printf("Failed to prepare executor")
		return JOB_FAILED
	}

	exitCode = job.Executor.Start(job.EventHandler)
	if exitCode != 0 {
		log.Printf("Failed to start executor")
		return JOB_FAILED
	}

	exitCode = job.Executor.ExportEnvVars(job.Request.EnvVars, job.EventHandler)
	if exitCode != 0 {
		log.Printf("Failed to export env vars")

		return JOB_FAILED
	}

	exitCode = job.Executor.InjectFiles(job.Request.Files, job.EventHandler)
	if exitCode != 0 {
		log.Printf("Failed to inject files")

		return JOB_FAILED
	}

	exitCode = job.RunCommandsUntilFirstFailure(job.Request.Commands)

	if exitCode == 0 {
		return JOB_PASSED
	} else {
		return JOB_FAILED
	}
}

// returns exit code of last executed command
func (job *Job) RunCommandsUntilFirstFailure(commands []api.Command) int {
	lastExitCode := 1

	for _, c := range commands {
		if job.Stopped {
			return 1
		}

		lastExitCode = job.Executor.RunCommand(c.Directive, job.EventHandler)

		if lastExitCode != 0 {
			break
		}
	}

	return lastExitCode
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

	j.Stopped = true

	j.Executor.Stop()
}

func LogJobStart(logfile *os.File) {
	log.Printf("Logging job start")

	m := make(map[string]interface{})

	m["event"] = "job_started"
	m["timestamp"] = int(time.Now().Unix())

	jsonString, _ := json.Marshal(m)

	logfile.Write([]byte(jsonString))
	logfile.Write([]byte("\n"))

	log.Printf("%s", jsonString)
}

func LogJobFinish(logfile *os.File, result string) {
	log.Printf("Logging job finish")

	m := make(map[string]interface{})

	m["event"] = "job_finished"
	m["timestamp"] = int(time.Now().Unix())
	m["result"] = result

	jsonString, _ := json.Marshal(m)

	logfile.Write([]byte(jsonString))
	logfile.Write([]byte("\n"))

	log.Printf("%s", jsonString)
}

func LogCmdStarted(logfile *os.File, timestamp int, directive string) {
	log.Printf("Logging command started")

	m := make(map[string]interface{})

	m["event"] = "cmd_started"
	m["timestamp"] = timestamp
	m["directive"] = directive

	jsonString, _ := json.Marshal(m)

	logfile.Write([]byte(jsonString))
	logfile.Write([]byte("\n"))

	log.Printf("%s", jsonString)
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
	log.Printf("Logging command finished")

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

	log.Printf("%s", jsonString)
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
	log.Printf("Sending callback: %s with %+v\n", url, payload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer([]byte(payload)))

	log.Printf("%+v\n", resp)

	return err
}
