package jobs

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"runtime"
	"testing"
	"time"

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

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
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

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
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

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	testsupport.AssertSimplifiedJobLogs(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exporting A\n",
		"Exporting B\n",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		"directive: badcommand",
		"*** OUTPUT ***",
		fmt.Sprintf("Exit Code: %d", testsupport.UnknownCommandExitCode()),

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

func Test__EpilogueOnPassOnlyExecutesOnSuccessfulJob(t *testing.T) {
	httpClient := http.DefaultClient

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: testsupport.Output("hello")},
		},
		EpilogueAlwaysCommands: []api.Command{
			{Directive: testsupport.Output("On epilogue always")},
		},
		EpilogueOnFailCommands: []api.Command{
			{Directive: testsupport.Output("On epilogue on fail")},
		},
		EpilogueOnPassCommands: []api.Command{
			{Directive: testsupport.Output("On epilogue on pass")},
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

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output("hello")),
		"hello",
		"Exit Code: 0",

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output("On epilogue always")),
		"On epilogue always",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output("On epilogue on pass")),
		"On epilogue on pass",
		"Exit Code: 0",

		"job_finished: passed",
	})
}

func Test__EpilogueOnFailOnlyExecutesOnFailedJob(t *testing.T) {
	httpClient := http.DefaultClient

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: "badcommand"},
		},
		EpilogueAlwaysCommands: []api.Command{
			{Directive: testsupport.Output("On epilogue always")},
		},
		EpilogueOnFailCommands: []api.Command{
			{Directive: testsupport.Output("On epilogue on fail")},
		},
		EpilogueOnPassCommands: []api.Command{
			{Directive: testsupport.Output("On epilogue on pass")},
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

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	testsupport.AssertSimplifiedJobLogs(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		"directive: badcommand",
		"*** OUTPUT ***",
		fmt.Sprintf("Exit Code: %d", testsupport.UnknownCommandExitCode()),

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output("On epilogue always")),
		"On epilogue always",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output("On epilogue on fail")),
		"On epilogue on fail",
		"Exit Code: 0",

		"job_finished: passed",
	})
}

func Test__UsingCommandAliases(t *testing.T) {
	httpClient := http.DefaultClient

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: testsupport.Output("hello world"), Alias: "Display Hello World"},
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

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		"directive: Display Hello World",
		fmt.Sprintf("Running: %s\n", testsupport.Output("hello world")),
		"hello world",
		"Exit Code: 0",

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 0",

		"job_finished: passed",
	})
}

func Test__StopJob(t *testing.T) {
	httpClient := http.DefaultClient

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: "sleep 60"},
			{Directive: testsupport.Output("hello")},
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

	go job.Run()

	time.Sleep(10 * time.Second)
	job.Stop()

	assert.True(t, job.Stopped)
	assert.Eventually(t, func() bool { return job.Finished }, 5*time.Second, 1*time.Second)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		"directive: sleep 60",
		fmt.Sprintf("Exit Code: %d", testsupport.StoppedCommandExitCode()),

		"job_finished: stopped",
	})
}

func Test__StopJobOnEpilogue(t *testing.T) {
	httpClient := http.DefaultClient

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: testsupport.Output("hello")},
		},
		EpilogueAlwaysCommands: []api.Command{
			{Directive: "sleep 60"},
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

	go job.Run()

	time.Sleep(10 * time.Second)
	job.Stop()

	assert.True(t, job.Stopped)
	assert.Eventually(t, func() bool { return job.Finished }, 5*time.Second, 1*time.Second)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output("hello")),
		"hello",
		"Exit Code: 0",

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 0",

		"directive: sleep 60",
		fmt.Sprintf("Exit Code: %d", testsupport.StoppedCommandExitCode()),

		"job_finished: stopped",
	})
}

func Test__STTYRestoration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not support pty")
	}

	httpClient := http.DefaultClient
	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: "stty echo"},
			{Directive: "echo Hello World"},
		},
		Callbacks: api.Callbacks{
			Finished:         "https://httpbin.org/status/200",
			TeardownFinished: "https://httpbin.org/status/200",
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{Request: request, Client: httpClient, Logger: testLogger})
	assert.Nil(t, err)

	job.Run()
	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		"directive: stty echo",
		"Exit Code: 0",

		"directive: echo Hello World",
		"Hello World\n",
		"Exit Code: 0",

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 0",

		"job_finished: passed",
	})
}
