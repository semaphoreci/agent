package jobs

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"

	"github.com/semaphoreci/agent/pkg/api"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	testsupport "github.com/semaphoreci/agent/test/support"
	"github.com/stretchr/testify/assert"
)

func Test__EnvVarsAreAvailableToCommands(t *testing.T) {
	httpClient := http.DefaultClient

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		Commands: []api.Command{
			{Directive: testsupport.EchoEnvVar("A")},
			{Directive: testsupport.EchoEnvVar("B")},
			{Directive: testsupport.EchoEnvVar("C")},
		},
		EnvVars: []api.EnvVar{
			{Name: "A", Value: base64.StdEncoding.EncodeToString([]byte("VALUE_A"))},
			{Name: "B", Value: base64.StdEncoding.EncodeToString([]byte("VALUE_B"))},
		},
		Callbacks: api.Callbacks{
			Finished:         "https://httpbin.org/status/200",
			TeardownFinished: "https://httpbin.org/status/200",
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{
		Request: request,
		Client:  httpClient,
		Logger:  testLogger,
	})

	assert.Nil(t, err)

	job.Run()
	assert.True(t, job.Finished)
	assert.Equal(t, testLoggerBackend.SimplifiedEvents(true), []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exporting A\n",
		"Exporting B\n",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("A")),
		"VALUE_A",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("B")),
		"VALUE_B",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("C")),
		"Exit Code: 0",

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 0",

		"job_finished: passed",
	})
}

func Test__EnvVarsAreAvailableToEpilogueAlwaysAndOnPass(t *testing.T) {
	httpClient := http.DefaultClient

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		Commands: []api.Command{},
		EnvVars: []api.EnvVar{
			{Name: "A", Value: base64.StdEncoding.EncodeToString([]byte("VALUE_A"))},
			{Name: "B", Value: base64.StdEncoding.EncodeToString([]byte("VALUE_B"))},
		},
		EpilogueAlwaysCommands: []api.Command{
			{Directive: testsupport.Output("On EpilogueAlways")},
			{Directive: testsupport.EchoEnvVar("A")},
			{Directive: testsupport.EchoEnvVar("B")},
			{Directive: testsupport.EchoEnvVar("C")},
			{Directive: testsupport.EchoEnvVar("SEMAPHORE_JOB_RESULT")},
		},
		EpilogueOnPassCommands: []api.Command{
			{Directive: testsupport.Output("On EpilogueOnPass")},
			{Directive: testsupport.EchoEnvVar("A")},
			{Directive: testsupport.EchoEnvVar("B")},
			{Directive: testsupport.EchoEnvVar("C")},
			{Directive: testsupport.EchoEnvVar("SEMAPHORE_JOB_RESULT")},
		},
		Callbacks: api.Callbacks{
			Finished:         "https://httpbin.org/status/200",
			TeardownFinished: "https://httpbin.org/status/200",
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{
		Request: request,
		Client:  httpClient,
		Logger:  testLogger,
	})

	assert.Nil(t, err)

	job.Run()
	assert.True(t, job.Finished)
	assert.Equal(t, testLoggerBackend.SimplifiedEvents(true), []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exporting A\n",
		"Exporting B\n",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output("On EpilogueAlways")),
		"On EpilogueAlways",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("A")),
		"VALUE_A",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("B")),
		"VALUE_B",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("C")),
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("SEMAPHORE_JOB_RESULT")),
		"passed",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output("On EpilogueOnPass")),
		"On EpilogueOnPass",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("A")),
		"VALUE_A",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("B")),
		"VALUE_B",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("C")),
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("SEMAPHORE_JOB_RESULT")),
		"passed",
		"Exit Code: 0",

		"job_finished: passed",
	})
}

func Test__EnvVarsAreAvailableToEpilogueAlwaysAndOnFail(t *testing.T) {
	httpClient := http.DefaultClient

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		Commands: []api.Command{
			{Directive: "badcommand"},
		},
		EnvVars: []api.EnvVar{
			{Name: "A", Value: base64.StdEncoding.EncodeToString([]byte("VALUE_A"))},
			{Name: "B", Value: base64.StdEncoding.EncodeToString([]byte("VALUE_B"))},
		},
		EpilogueAlwaysCommands: []api.Command{
			{Directive: testsupport.Output("On EpilogueAlways")},
			{Directive: testsupport.EchoEnvVar("A")},
			{Directive: testsupport.EchoEnvVar("B")},
			{Directive: testsupport.EchoEnvVar("C")},
			{Directive: testsupport.EchoEnvVar("SEMAPHORE_JOB_RESULT")},
		},
		EpilogueOnFailCommands: []api.Command{
			{Directive: testsupport.Output("On EpilogueOnFail")},
			{Directive: testsupport.EchoEnvVar("A")},
			{Directive: testsupport.EchoEnvVar("B")},
			{Directive: testsupport.EchoEnvVar("C")},
			{Directive: testsupport.EchoEnvVar("SEMAPHORE_JOB_RESULT")},
		},
		Callbacks: api.Callbacks{
			Finished:         "https://httpbin.org/status/200",
			TeardownFinished: "https://httpbin.org/status/200",
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{
		Request: request,
		Client:  httpClient,
		Logger:  testLogger,
	})

	assert.Nil(t, err)

	job.Run()
	assert.True(t, job.Finished)

	testsupport.AssertJobLogs(t, testLoggerBackend.SimplifiedEvents(true), []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exporting A\n",
		"Exporting B\n",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		"directive: badcommand",
		"*** OUTPUT ***",
		"Exit Code: 1",

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output("On EpilogueAlways")),
		"On EpilogueAlways",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("A")),
		"VALUE_A",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("B")),
		"VALUE_B",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("C")),
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("SEMAPHORE_JOB_RESULT")),
		"failed",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output("On EpilogueOnFail")),
		"On EpilogueOnFail",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("A")),
		"VALUE_A",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("B")),
		"VALUE_B",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("C")),
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("SEMAPHORE_JOB_RESULT")),
		"failed",
		"Exit Code: 0",

		"job_finished: failed",
	})
}