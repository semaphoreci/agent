package jobs

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	testsupport "github.com/semaphoreci/agent/test/support"
	"github.com/stretchr/testify/assert"
)

func Test__EnvVarsAreAvailableToCommands(t *testing.T) {
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
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{
		Request: request,
		Client:  http.DefaultClient,
		Logger:  testLogger,
	})

	assert.Nil(t, err)

	job.Run()
	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
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
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{
		Request: request,
		Client:  http.DefaultClient,
		Logger:  testLogger,
	})

	assert.Nil(t, err)

	job.Run()
	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
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
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{
		Request: request,
		Client:  http.DefaultClient,
		Logger:  testLogger,
	})

	assert.Nil(t, err)

	job.Run()
	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
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
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{
		Request: request,
		Client:  http.DefaultClient,
		Logger:  testLogger,
	})

	assert.Nil(t, err)

	job.Run()
	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
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
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{
		Request: request,
		Client:  http.DefaultClient,
		Logger:  testLogger,
	})

	assert.Nil(t, err)

	job.Run()
	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
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
	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: testsupport.Output("hello world"), Alias: "Display Hello World"},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{
		Request: request,
		Client:  http.DefaultClient,
		Logger:  testLogger,
	})

	assert.Nil(t, err)

	job.Run()
	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
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
	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: "sleep 60"},
			{Directive: testsupport.Output("hello")},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{
		Request: request,
		Client:  http.DefaultClient,
		Logger:  testLogger,
	})

	assert.Nil(t, err)

	go job.Run()

	time.Sleep(10 * time.Second)
	job.Stop()

	assert.True(t, job.Stopped)
	assert.Eventually(t, func() bool { return job.Finished }, 5*time.Second, 1*time.Second)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
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

func Test__StopJobWithExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: testsupport.ReturnExitCodeCommand(130)},
			{Directive: testsupport.Output("hello")},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{
		Request: request,
		Client:  http.DefaultClient,
		Logger:  testLogger,
	})

	assert.Nil(t, err)

	job.Run()

	assert.True(t, job.Stopped)
	assert.Eventually(t, func() bool { return job.Finished }, 5*time.Second, 1*time.Second)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.ReturnExitCodeCommand(130)),
		fmt.Sprintf("Exit Code: %d", testsupport.ManuallyStoppedCommandExitCode()),

		"directive: Checking job result",
		"SEMAPHORE_JOB_RESULT is set to '' - stopping job and marking it as stopped",
		"Exit Code: 0",

		"job_finished: stopped",
	})
}

func Test__StopJobWithExitCodeWithResultSetToPassed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: testsupport.SetEnvVar("SEMAPHORE_JOB_RESULT", "passed")},
			{Directive: testsupport.ReturnExitCodeCommand(130)},
			{Directive: testsupport.Output("hello")},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{
		Request: request,
		Client:  http.DefaultClient,
		Logger:  testLogger,
	})

	assert.Nil(t, err)

	job.Run()

	assert.Eventually(t, func() bool { return job.Finished }, 5*time.Second, 1*time.Second)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.SetEnvVar("SEMAPHORE_JOB_RESULT", "passed")),
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.ReturnExitCodeCommand(130)),
		fmt.Sprintf("Exit Code: %d", testsupport.ManuallyStoppedCommandExitCode()),

		"directive: Checking job result",
		"SEMAPHORE_JOB_RESULT=passed - stopping job and marking it as passed",
		"Exit Code: 0",

		"job_finished: passed",
	})
}

func Test__StopJobWithExitCodeWithResultSetToFailed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: testsupport.SetEnvVar("SEMAPHORE_JOB_RESULT", "failed")},
			{Directive: testsupport.ReturnExitCodeCommand(130)},
			{Directive: testsupport.Output("hello")},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{
		Request: request,
		Client:  http.DefaultClient,
		Logger:  testLogger,
	})

	assert.Nil(t, err)

	job.Run()

	assert.Eventually(t, func() bool { return job.Finished }, 5*time.Second, 1*time.Second)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.SetEnvVar("SEMAPHORE_JOB_RESULT", "failed")),
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.ReturnExitCodeCommand(130)),
		fmt.Sprintf("Exit Code: %d", testsupport.ManuallyStoppedCommandExitCode()),

		"directive: Checking job result",
		"SEMAPHORE_JOB_RESULT=failed - stopping job and marking it as failed",
		"Exit Code: 0",

		"job_finished: failed",
	})
}

func Test__StopJobOnEpilogue(t *testing.T) {
	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: testsupport.Output("hello")},
		},
		EpilogueAlwaysCommands: []api.Command{
			{Directive: "sleep 60"},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{
		Request: request,
		Client:  http.DefaultClient,
		Logger:  testLogger,
	})

	assert.Nil(t, err)

	go job.Run()

	time.Sleep(10 * time.Second)
	job.Stop()

	assert.True(t, job.Stopped)
	assert.Eventually(t, func() bool { return job.Finished }, 5*time.Second, 1*time.Second)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
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

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: "stty echo"},
			{Directive: "echo Hello World"},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{Request: request, Client: http.DefaultClient, Logger: testLogger})
	assert.Nil(t, err)

	job.Run()
	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
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

func Test__BackgroundJobIsKilledAfterJobIsDoneInWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip()
	}

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: "Start-Process ping -ArgumentList '-n','300','127.0.0.1'"},
			{Directive: "sleep 5"},
			{Directive: "(Get-Process ping -ErrorAction SilentlyContinue) -and ($true)"},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{Request: request, Client: http.DefaultClient, Logger: testLogger})
	assert.Nil(t, err)

	job.Run()
	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		"directive: Start-Process ping -ArgumentList '-n','300','127.0.0.1'",
		"Exit Code: 0",

		"directive: sleep 5",
		"Exit Code: 0",

		"directive: (Get-Process ping -ErrorAction SilentlyContinue) -and ($true)",
		"True\n",
		"Exit Code: 0",

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 0",

		"job_finished: passed",
	})

	// assert process is not running anymore
	cmd := exec.Command(
		"powershell",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		"(Get-Process ping -ErrorAction SilentlyContinue) -and ($true)",
	)

	output, err := cmd.CombinedOutput()
	assert.Nil(t, err)
	assert.Equal(t, "False\r\n", string(output))
}

func Test__BackgroundJobIsKilledAfterJobIsDoneInNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: "ping -c 300 127.0.0.1 > /dev/null &"},
			{Directive: "sleep 5"},
			{Directive: "pgrep ping > /dev/null && true"},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{Request: request, Client: http.DefaultClient, Logger: testLogger})
	assert.Nil(t, err)

	job.Run()
	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		"directive: ping -c 300 127.0.0.1 > /dev/null &",
		"Exit Code: 0",

		"directive: sleep 5",
		"Exit Code: 0",

		"directive: pgrep ping > /dev/null && true",
		"Exit Code: 0",

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 0",

		"job_finished: passed",
	})

	// assert process is not running anymore
	cmd := exec.Command(
		"bash",
		"-c",
		"'pgrep ping > /dev/null && true'",
	)

	_, err = cmd.CombinedOutput()
	assert.NotNil(t, err)
}

func Test__KillingRootBash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: "sleep infinity &"},
			{Directive: "exit 1"},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{Request: request, Client: http.DefaultClient, Logger: testLogger})
	assert.Nil(t, err)

	job.Run()
	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		"directive: sleep infinity &",
		"Exit Code: 0",

		"directive: exit 1",
		"Exit Code: 1",

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 1",

		"job_finished: failed",
	})
}

func Test__BashSetE(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: "sleep infinity &"},
			{Directive: "set -e"},
			{Directive: "false"},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{Request: request, Client: http.DefaultClient, Logger: testLogger})
	assert.Nil(t, err)

	job.Run()
	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		"directive: sleep infinity &",
		"Exit Code: 0",

		"directive: set -e",
		"Exit Code: 0",

		"directive: false",
		"Exit Code: 1",

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 1",

		"job_finished: failed",
	})
}

func Test__BashSetPipefail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: "sleep infinity &"},
			{Directive: "set -eo pipefail"},
			{Directive: "cat non_existant | sort"},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{Request: request, Client: http.DefaultClient, Logger: testLogger})
	assert.Nil(t, err)

	job.Run()
	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		"directive: sleep infinity &",
		"Exit Code: 0",

		"directive: set -eo pipefail",
		"Exit Code: 0",

		"directive: cat non_existant | sort",
		"cat: non_existant: No such file or directory\n",
		"Exit Code: 1",

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 1",

		"job_finished: failed",
	})
}

func Test__UsePreJobHook(t *testing.T) {
	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: testsupport.Output("hello")},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{Request: request, Client: http.DefaultClient, Logger: testLogger})
	assert.Nil(t, err)

	hook, _ := testsupport.TempFileWithExtension()
	_ = ioutil.WriteFile(hook, []byte(testsupport.Output("hello from pre-job hook")), 0777)

	job.RunWithOptions(RunOptions{
		EnvVars:               []config.HostEnvVar{},
		PreJobHookPath:        hook,
		OnJobFinished:         nil,
		CallbackRetryAttempts: 1,
	})

	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
	assert.Nil(t, err)

	testsupport.AssertSimplifiedJobLogs(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		"directive: Running the pre-job hook configured in the agent",
		"*** IGNORE SINGLE LINE ***", // we are using a temp file, it's hard to assert its path, just ignore it
		"Warning: The agent is configured to proceed with the job even if the pre-job hook fails.\n",
		"hello from pre-job hook",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output("hello")),
		"hello",
		"Exit Code: 0",

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 0",

		"job_finished: passed",
	})

	os.Remove(hook)
}

func Test__UsePostJobHook(t *testing.T) {
	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: testsupport.Output("hello")},
		},
		EpilogueAlwaysCommands: []api.Command{
			{Directive: testsupport.Output("On EpilogueAlways")},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{Request: request, Client: http.DefaultClient, Logger: testLogger})
	assert.Nil(t, err)

	hook, _ := testsupport.TempFileWithExtension()
	_ = ioutil.WriteFile(hook, []byte(testsupport.Output("hello from post-job hook")), 0777)

	job.RunWithOptions(RunOptions{
		EnvVars:               []config.HostEnvVar{},
		PostJobHookPath:       hook,
		OnJobFinished:         nil,
		CallbackRetryAttempts: 1,
	})

	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
	assert.Nil(t, err)

	testsupport.AssertSimplifiedJobLogs(t, simplifiedEvents, []string{
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

		fmt.Sprintf("directive: %s", testsupport.Output("On EpilogueAlways")),
		"On EpilogueAlways",
		"Exit Code: 0",

		"directive: Running the post-job hook configured in the agent",
		"*** IGNORE SINGLE LINE ***", // we are using a temp file, it's hard to assert its path, just ignore it
		"hello from post-job hook",
		"Exit Code: 0",

		"job_finished: passed",
	})

	os.Remove(hook)
}

func Test__PreJobHookHasAccessToEnvVars(t *testing.T) {
	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{
			{Name: "A", Value: base64.StdEncoding.EncodeToString([]byte("VALUE_A"))},
			{Name: "B", Value: base64.StdEncoding.EncodeToString([]byte("VALUE_B"))},
		},
		Commands: []api.Command{
			{Directive: testsupport.Output("hello")},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{Request: request, Client: http.DefaultClient, Logger: testLogger})
	assert.Nil(t, err)

	hook, _ := testsupport.TempFileWithExtension()
	hookContent := []string{
		testsupport.EchoEnvVar("A"),
		testsupport.Output(" - "),
		testsupport.EchoEnvVar("B"),
	}

	_ = ioutil.WriteFile(hook, []byte(strings.Join(hookContent, "\n")), 0777)
	job.RunWithOptions(RunOptions{
		EnvVars:               []config.HostEnvVar{},
		PreJobHookPath:        hook,
		OnJobFinished:         nil,
		CallbackRetryAttempts: 1,
	})

	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
	assert.Nil(t, err)

	testsupport.AssertSimplifiedJobLogs(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exporting A\n",
		"Exporting B\n",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		"directive: Running the pre-job hook configured in the agent",
		"*** IGNORE SINGLE LINE ***", // we are using a temp file, it's hard to assert its path, just ignore it
		"Warning: The agent is configured to proceed with the job even if the pre-job hook fails.\n",
		"VALUE_A - VALUE_B",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output("hello")),
		"hello",
		"Exit Code: 0",

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 0",

		"job_finished: passed",
	})

	os.Remove(hook)
}

func Test__PostJobHookHasAccessToEnvVars(t *testing.T) {
	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{
			{Name: "A", Value: base64.StdEncoding.EncodeToString([]byte("VALUE_A"))},
			{Name: "B", Value: base64.StdEncoding.EncodeToString([]byte("VALUE_B"))},
		},
		Commands: []api.Command{
			{Directive: testsupport.Output("hello")},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{Request: request, Client: http.DefaultClient, Logger: testLogger})
	assert.Nil(t, err)

	hook, _ := testsupport.TempFileWithExtension()
	hookContent := []string{
		testsupport.EchoEnvVar("A"),
		testsupport.Output(" - "),
		testsupport.EchoEnvVar("B"),
	}

	_ = ioutil.WriteFile(hook, []byte(strings.Join(hookContent, "\n")), 0777)
	job.RunWithOptions(RunOptions{
		EnvVars:               []config.HostEnvVar{},
		PostJobHookPath:       hook,
		OnJobFinished:         nil,
		CallbackRetryAttempts: 1,
	})

	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
	assert.Nil(t, err)

	testsupport.AssertSimplifiedJobLogs(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exporting A\n",
		"Exporting B\n",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output("hello")),
		"hello",
		"Exit Code: 0",

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 0",

		"directive: Running the post-job hook configured in the agent",
		"*** IGNORE SINGLE LINE ***", // we are using a temp file, it's hard to assert its path, just ignore it
		"VALUE_A - VALUE_B",
		"Exit Code: 0",

		"job_finished: passed",
	})

	os.Remove(hook)
}

func Test__UsePreJobHookAndFailOnError(t *testing.T) {
	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	request := &api.JobRequest{
		EnvVars: []api.EnvVar{},
		Commands: []api.Command{
			{Directive: testsupport.Output("hello")},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
		},
	}

	job, err := NewJobWithOptions(&JobOptions{Request: request, Client: http.DefaultClient, Logger: testLogger})
	assert.Nil(t, err)

	hook, _ := testsupport.TempFileWithExtension()
	_ = ioutil.WriteFile(hook, []byte("badcommand"), 0777)

	job.RunWithOptions(RunOptions{
		EnvVars:               []config.HostEnvVar{},
		PreJobHookPath:        hook,
		FailOnPreJobHookError: true,
		OnJobFinished:         nil,
		CallbackRetryAttempts: 1,
	})

	assert.True(t, job.Finished)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true, false)
	assert.Nil(t, err)

	testsupport.AssertSimplifiedJobLogs(t, simplifiedEvents, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		"directive: Running the pre-job hook configured in the agent",
		"*** IGNORE SINGLE LINE ***", // we are using a temp file, it's hard to assert its path, just ignore it
		"Warning: The agent is configured to fail the job if the pre-job hook fails.\n",
		"*** IGNORE LINES UNTIL EXIT CODE ***", // also hard to assert the actual error message, just ignore it
		fmt.Sprintf("Exit Code: %d", testsupport.UnknownCommandExitCode()),

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 0",

		"job_finished: failed",
	})

	os.Remove(hook)
}
