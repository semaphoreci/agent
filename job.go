package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ghodss/yaml"
)

type Command struct {
	Directive string `yaml:"directive"`
}

type EnvVar struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type File struct {
	Path    string `yaml:"name"`
	Content string `yaml:"content"`
	Mode    string `yaml:"mode"`
}

type Callbacks struct {
	Started  string `yaml:"mode"`
	Finished string `yaml:"mode"`
}

type JobRequest struct {
	Commands  []Command `yaml:"commands"`
	EnvVars   []EnvVar  `yaml:"env_vars"`
	Files     []File    `yaml:"file"`
	Callbacks Callbacks `yaml:"callbacks"`
}

type Job struct {
	Request JobRequest
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

	fmt.Printf("%s\n", yamlFile)

	var jobRequest JobRequest

	err = yaml.Unmarshal(yamlFile, &jobRequest)
	if err != nil {
		return nil, err
	}

	return &Job{Request: jobRequest}, nil
}

func (job *Job) Run() {
	fmt.Printf("%+v\n", job.Request)

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

	shell.Run(job.Request, func(event interface{}) {
		switch e := event.(type) {
		case CommandStartedShellEvent:
			LogCmdStarted(logfile, e.Timestamp, e.Directive)
		case CommandOutputShellEvent:
			LogCmdOutput(logfile, e.Timestamp, e.Output)
		case CommandFinishedShellEvent:
			LogCmdFinished(logfile, e.Timestamp, e.Directive, e.ExitStatus)
		default:
			panic("Unknown shell event")
		}
	})

	logfile.Sync()

	job.SendFinishedCallback("passed")

	LogJobFinish(logfile, "passed")
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

func LogCmdFinished(logfile *os.File, timestamp int, directive string, exitStatus int) {
	m := make(map[string]interface{})

	m["event"] = "cmd_finished"
	m["timestamp"] = timestamp
	m["directive"] = directive
	m["exit_status"] = exitStatus

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

func (job *Job) SendCallback(url string, payload string) error {
	fmt.Printf("Sending started callback: %s with %+v\n", url, payload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer([]byte(payload)))

	fmt.Printf("%+v\n", resp)

	return err
}
