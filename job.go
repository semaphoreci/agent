package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ghodss/yaml"
)

const JOB_PASSED = "passed"
const JOB_FAILED = "failed"

type Command struct {
	Directive string `json:"directive" yaml:"directive"`
}

type EnvVar struct {
	Name  string `json:"name" yaml:"name"`
	Value string `json:"value" yaml:"value"`
}

type File struct {
	Path    string `json:"path" yaml:"path"`
	Content string `json:"content" yaml:"content"`
	Mode    string `json:"mode" yaml:"mode"`
}

type Callbacks struct {
	Started          string `json:"started" yaml:"started"`
	Finished         string `json:"finished" yaml:"finished"`
	TeardownFinished string `json:"teardown_finished" yaml:"teardown_finished"`
}

type JobRequest struct {
	Commands  []Command `json:"commands" yaml:"commands"`
	EnvVars   []EnvVar  `json:"env_vars" yaml:"env_vars"`
	Files     []File    `json:"files" yaml:"file"`
	Callbacks Callbacks `json:"callbacks" yaml:"callbacks"`
}

type Job struct {
	Request        JobRequest
	JobLogArchived bool
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func NewJobFromYaml(path string) (*Job, error) {
	filename, _ := filepath.Abs(path)
	yamlFile, err := ioutil.ReadFile(filename)

	if err != nil {
		return nil, err
	}

	log.Printf("%s\n", yamlFile)

	var jobRequest JobRequest

	err = yaml.Unmarshal(yamlFile, &jobRequest)
	if err != nil {
		return nil, err
	}

	// don't wait for log archivation
	return &Job{Request: jobRequest, JobLogArchived: true}, nil
}

func (job *Job) Run() {
	result := JOB_FAILED

	log.Printf("Job Request %+v\n", job.Request)

	os.RemoveAll("/tmp/run/semaphore/logs")
	os.MkdirAll("/tmp/run/semaphore/logs", os.ModePerm)

	// TODO: find better place for this
	logfile, err := os.Create("/tmp/job_log.json")
	if err != nil {
		panic(err)
	}

	defer logfile.Close()

	job.SendStartedCallback()

	LogJobStart(logfile)

	shell := NewShell()

	exitStatus := shell.Run(job.Request, func(event interface{}) {
		switch e := event.(type) {
		case CommandStartedShellEvent:
			LogCmdStarted(logfile, e.Timestamp, e.Directive)
		case CommandOutputShellEvent:
			LogCmdOutput(logfile, e.Timestamp, e.Output)
		case CommandFinishedShellEvent:
			LogCmdFinished(logfile, e.Timestamp, e.Directive, e.ExitCode, e.StartedAt, e.FinishedAt)
		default:
			panic("Unknown shell event")
		}
	})

	logfile.Sync()

	if exitStatus == 0 {
		result = JOB_PASSED
	} else {
		result = JOB_FAILED
	}

	job.SendFinishedCallback(result)

	LogJobFinish(logfile, result)

	for {
		if job.JobLogArchived {
			job.SendTeardownFinishedCallback()
			break
		}

		time.Sleep(1000 * time.Millisecond)
	}
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
	log.Printf("Sending callback: %s with %+v\n", url, payload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer([]byte(payload)))

	log.Printf("%+v\n", resp)

	return err
}
