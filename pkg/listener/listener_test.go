package listener

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	"github.com/semaphoreci/agent/pkg/eventlogger"
	"github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
	testsupport "github.com/semaphoreci/agent/test/support"
	"github.com/stretchr/testify/assert"
)

func Test__Register(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	config := Config{
		AgentName:          fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		ExitOnShutdown:     false,
		Endpoint:           hubMockServer.Host(),
		Token:              "token",
		RegisterRetryLimit: 5,
		Scheme:             "http",
		EnvVars:            []config.HostEnvVar{},
		FileInjections:     []config.FileInjection{},
		AgentVersion:       "0.0.7",
	}

	listener, err := Start(http.DefaultClient, config)
	assert.Nil(t, err)

	if assert.Nil(t, hubMockServer.WaitUntilRegistered()) {
		registerRequest := hubMockServer.GetRegisterRequest()
		assert.NotEmpty(t, registerRequest.Arch)
		assert.NotEmpty(t, registerRequest.Hostname)
		assert.NotEmpty(t, registerRequest.Name)
		assert.NotEmpty(t, registerRequest.OS)
		assert.NotZero(t, registerRequest.PID)
		assert.Equal(t, registerRequest.Version, "0.0.7")
	}

	listener.Stop()
	hubMockServer.Close()
	loghubMockServer.Close()
}

func Test__RegisterRequestIsRetried(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())
	hubMockServer.RejectRegisterAttempts(3)

	config := Config{
		AgentName:          fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		ExitOnShutdown:     false,
		Endpoint:           hubMockServer.Host(),
		Token:              "token",
		RegisterRetryLimit: 5,
		Scheme:             "http",
		EnvVars:            []config.HostEnvVar{},
		FileInjections:     []config.FileInjection{},
		AgentVersion:       "0.0.7",
	}

	listener, err := Start(http.DefaultClient, config)
	assert.Nil(t, err)

	if assert.Nil(t, hubMockServer.WaitUntilRegistered()) {
		assert.Equal(t, 3, hubMockServer.RegisterAttempts)
		registerRequest := hubMockServer.GetRegisterRequest()
		assert.NotEmpty(t, registerRequest.Arch)
		assert.NotEmpty(t, registerRequest.Hostname)
		assert.NotEmpty(t, registerRequest.Name)
		assert.NotEmpty(t, registerRequest.OS)
		assert.NotZero(t, registerRequest.PID)
		assert.Equal(t, registerRequest.Version, "0.0.7")
	}

	listener.Stop()
	hubMockServer.Close()
	loghubMockServer.Close()
}

func Test__RegistrationFails(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())
	hubMockServer.RejectRegisterAttempts(10)

	config := Config{
		AgentName:          fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		ExitOnShutdown:     false,
		Endpoint:           hubMockServer.Host(),
		Token:              "token",
		RegisterRetryLimit: 5,
		Scheme:             "http",
		EnvVars:            []config.HostEnvVar{},
		FileInjections:     []config.FileInjection{},
		AgentVersion:       "0.0.7",
	}

	_, err := Start(http.DefaultClient, config)
	assert.NotNil(t, err)
	assert.Equal(t, 4, hubMockServer.RegisterAttempts)

	hubMockServer.Close()
	loghubMockServer.Close()
}

func Test__ShutdownHookIsExecuted(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	hook, err := testsupport.TempFileWithExtension()
	assert.Nil(t, err)

	/*
	 * To assert that the shutdown hook was executed,
	 * we make it create a file with the same name + .done suffix.
	 * If that file exists after the listener stopped,
	 * it means the shutdown hook was executed.
	 */
	destination := fmt.Sprintf("%s.done", hook)
	err = ioutil.WriteFile(hook, []byte(testsupport.CopyFile(hook, destination)), 0777)
	assert.Nil(t, err)

	config := Config{
		AgentName:          fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		ExitOnShutdown:     false,
		Endpoint:           hubMockServer.Host(),
		Token:              "token",
		RegisterRetryLimit: 5,
		Scheme:             "http",
		EnvVars:            []config.HostEnvVar{},
		FileInjections:     []config.FileInjection{},
		AgentVersion:       "0.0.7",
		ShutdownHookPath:   hook,
	}

	listener, err := Start(http.DefaultClient, config)
	assert.Nil(t, err)

	// listener has not been stopped yet, so file created by shutdown hook does not exist yet
	assert.NoFileExists(t, destination)

	time.Sleep(time.Second)
	listener.Stop()

	// listener has been stopped, so file created by shutdown hook should exist
	assert.FileExists(t, destination)

	os.Remove(hook)
	os.Remove(destination)
	hubMockServer.Close()
	loghubMockServer.Close()
}

func Test__ShutdownHookCanSeeShutdownReason(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	hook, err := testsupport.TempFileWithExtension()
	assert.Nil(t, err)

	/*
	 * To assert that the shutdown hook has access to the SEMAPHORE_AGENT_SHUTDOWN_REASON
	 * variable, we tell the shutdown hook script to write its value on a new file.
	 */
	destination := fmt.Sprintf("%s.done", hook)
	err = ioutil.WriteFile(hook, []byte(testsupport.EchoEnvVarToFile("SEMAPHORE_AGENT_SHUTDOWN_REASON", destination)), 0777)
	assert.Nil(t, err)

	config := Config{
		AgentName:          fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		ExitOnShutdown:     false,
		Endpoint:           hubMockServer.Host(),
		Token:              "token",
		RegisterRetryLimit: 5,
		Scheme:             "http",
		EnvVars:            []config.HostEnvVar{},
		FileInjections:     []config.FileInjection{},
		UploadJobLogs:      config.UploadJobLogsConditionNever,
		AgentVersion:       "0.0.7",
		ShutdownHookPath:   hook,
	}

	listener, err := Start(http.DefaultClient, config)
	assert.Nil(t, err)

	// listener has not been stopped yet, so file created by shutdown hook does not exist yet
	assert.NoFileExists(t, destination)

	time.Sleep(time.Second)
	listener.Stop()

	// listener has been stopped, so file created by shutdown hook should exist
	assert.FileExists(t, destination)

	bytes, err := ioutil.ReadFile(destination)
	assert.Nil(t, err)
	assert.Equal(t, ShutdownReasonRequested.String(), strings.Replace(string(bytes), "\r\n", "", -1))

	os.Remove(hook)
	os.Remove(destination)
	hubMockServer.Close()
	loghubMockServer.Close()
}

func Test__ShutdownAfterJobFinished(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	config := Config{
		AgentName:          fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		ExitOnShutdown:     false,
		DisconnectAfterJob: true,
		Endpoint:           hubMockServer.Host(),
		Token:              "token",
		RegisterRetryLimit: 5,
		Scheme:             "http",
		EnvVars:            []config.HostEnvVar{},
		FileInjections:     []config.FileInjection{},
		UploadJobLogs:      config.UploadJobLogsConditionNever,
		AgentVersion:       "0.0.7",
	}

	listener, err := Start(http.DefaultClient, config)
	assert.Nil(t, err)

	hubMockServer.AssignJob(&api.JobRequest{
		JobID: "Test__ShutdownAfterJobFinished",
		Commands: []api.Command{
			{Directive: testsupport.Output("hello world")},
		},
		Callbacks: api.Callbacks{
			Finished:         "https://httpbin.org/status/200",
			TeardownFinished: "https://httpbin.org/status/200",
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
			URL:    loghubMockServer.URL(),
			Token:  "doesnotmatter",
		},
	})

	assert.Nil(t, hubMockServer.WaitUntilDisconnected(30, 2*time.Second))
	assert.Equal(t, listener.JobProcessor.ShutdownReason, ShutdownReasonJobFinished)

	hubMockServer.Close()
	loghubMockServer.Close()
}

func Test__ShutdownAfterIdleTimeout(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	config := Config{
		AgentName:                  fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		ExitOnShutdown:             false,
		DisconnectAfterIdleSeconds: 15,
		Endpoint:                   hubMockServer.Host(),
		Token:                      "token",
		RegisterRetryLimit:         5,
		Scheme:                     "http",
		EnvVars:                    []config.HostEnvVar{},
		FileInjections:             []config.FileInjection{},
		UploadJobLogs:              config.UploadJobLogsConditionNever,
		AgentVersion:               "0.0.7",
	}

	listener, err := Start(http.DefaultClient, config)
	assert.Nil(t, err)
	assert.Nil(t, hubMockServer.WaitUntilDisconnected(15, 2*time.Second))
	assert.Equal(t, listener.JobProcessor.ShutdownReason, ShutdownReasonIdle)

	hubMockServer.Close()
	loghubMockServer.Close()
}

func Test__ShutdownAfterInterruption(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	config := Config{
		AgentName:          fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		ExitOnShutdown:     false,
		Endpoint:           hubMockServer.Host(),
		Token:              "token",
		RegisterRetryLimit: 5,
		Scheme:             "http",
		EnvVars:            []config.HostEnvVar{},
		FileInjections:     []config.FileInjection{},
		UploadJobLogs:      config.UploadJobLogsConditionNever,
		AgentVersion:       "0.0.7",
	}

	listener, err := Start(http.DefaultClient, config)
	assert.Nil(t, err)

	listener.Interrupt()
	assert.Nil(t, hubMockServer.WaitUntilDisconnected(15, 2*time.Second))
	assert.Equal(t, listener.JobProcessor.ShutdownReason, ShutdownReasonInterrupted)

	hubMockServer.Close()
	loghubMockServer.Close()
}

func Test__ShutdownAfterInterruptionNoGracePeriod(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	config := Config{
		AgentName:          fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		ExitOnShutdown:     false,
		DisconnectAfterJob: true,
		Endpoint:           hubMockServer.Host(),
		Token:              "token",
		RegisterRetryLimit: 5,
		Scheme:             "http",
		EnvVars:            []config.HostEnvVar{},
		FileInjections:     []config.FileInjection{},
		UploadJobLogs:      config.UploadJobLogsConditionNever,
		AgentVersion:       "0.0.7",
	}

	listener, err := Start(http.DefaultClient, config)
	assert.Nil(t, err)

	// assigns job that sleeps for 60s
	hubMockServer.AssignJob(&api.JobRequest{
		JobID: "Test__ShutdownAfterJobFinished",
		Commands: []api.Command{
			{Directive: "sleep 60"},
		},
		Callbacks: api.Callbacks{
			Finished:         "https://httpbin.org/status/200",
			TeardownFinished: "https://httpbin.org/status/200",
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
			URL:    loghubMockServer.URL(),
			Token:  "doesnotmatter",
		},
	})

	// wait until job is running
	assert.Nil(t, hubMockServer.WaitUntilRunningJob(10, time.Second))

	// send interrupt signal and assert agents disconnected
	// with interrupted reason and job is stopped immediately.
	listener.Interrupt()
	assert.Nil(t, hubMockServer.WaitUntilDisconnected(30, 2*time.Second))
	assert.Equal(t, listener.JobProcessor.ShutdownReason, ShutdownReasonInterrupted)
	assert.Equal(t, selfhostedapi.JobResult(selfhostedapi.JobResultStopped), hubMockServer.GetLastJobResult())

	hubMockServer.Close()
	loghubMockServer.Close()
}

func Test__ShutdownAfterInterruptionWithGracePeriod(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	config := Config{
		AgentName:               fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		ExitOnShutdown:          false,
		DisconnectAfterJob:      true,
		Endpoint:                hubMockServer.Host(),
		Token:                   "token",
		RegisterRetryLimit:      5,
		Scheme:                  "http",
		EnvVars:                 []config.HostEnvVar{},
		FileInjections:          []config.FileInjection{},
		AgentVersion:            "0.0.7",
		UploadJobLogs:           config.UploadJobLogsConditionNever,
		InterruptionGracePeriod: 30,
	}

	listener, err := Start(http.DefaultClient, config)
	assert.Nil(t, err)

	// assigns job that sleeps for 10s
	hubMockServer.AssignJob(&api.JobRequest{
		JobID: "Test__ShutdownAfterJobFinished",
		Commands: []api.Command{
			{Directive: "sleep 15"},
		},
		Callbacks: api.Callbacks{
			Finished:         "https://httpbin.org/status/200",
			TeardownFinished: "https://httpbin.org/status/200",
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
			URL:    loghubMockServer.URL(),
			Token:  "doesnotmatter",
		},
	})

	// wait until job is running
	assert.Nil(t, hubMockServer.WaitUntilRunningJob(10, time.Second))

	// send interrupt signal and assert agents disconnected
	// with interrupted reason and job finishes properly.
	listener.Interrupt()
	assert.Nil(t, hubMockServer.WaitUntilDisconnected(30, time.Second))
	assert.Equal(t, listener.JobProcessor.ShutdownReason, ShutdownReasonInterrupted)
	assert.Equal(t, selfhostedapi.JobResult(selfhostedapi.JobResultPassed), hubMockServer.GetLastJobResult())

	hubMockServer.Close()
	loghubMockServer.Close()
}

func Test__ShutdownFromUpstreamWhileWaiting(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	config := Config{
		AgentName:          fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		ExitOnShutdown:     false,
		Endpoint:           hubMockServer.Host(),
		Token:              "token",
		RegisterRetryLimit: 5,
		Scheme:             "http",
		EnvVars:            []config.HostEnvVar{},
		FileInjections:     []config.FileInjection{},
		UploadJobLogs:      config.UploadJobLogsConditionNever,
		AgentVersion:       "0.0.7",
	}

	listener, err := Start(http.DefaultClient, config)
	assert.Nil(t, err)

	time.Sleep(time.Second)
	hubMockServer.ScheduleShutdown()

	assert.Nil(t, hubMockServer.WaitUntilDisconnected(5, 2*time.Second))
	assert.Equal(t, listener.JobProcessor.ShutdownReason, ShutdownReasonRequested)

	hubMockServer.Close()
	loghubMockServer.Close()
}

func Test__ShutdownFromUpstreamWhileRunningJob(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	config := Config{
		AgentName:          fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		ExitOnShutdown:     false,
		Endpoint:           hubMockServer.Host(),
		Token:              "token",
		RegisterRetryLimit: 5,
		Scheme:             "http",
		EnvVars:            []config.HostEnvVar{},
		FileInjections:     []config.FileInjection{},
		UploadJobLogs:      config.UploadJobLogsConditionNever,
		AgentVersion:       "0.0.7",
	}

	listener, err := Start(http.DefaultClient, config)
	assert.Nil(t, err)

	hubMockServer.AssignJob(&api.JobRequest{
		JobID: "Test__ShutdownFromUpstreamWhileRunningJob",
		Commands: []api.Command{
			{Directive: "sleep 300"},
		},
		Callbacks: api.Callbacks{
			Finished:         "https://httpbin.org/status/200",
			TeardownFinished: "https://httpbin.org/status/200",
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
			URL:    loghubMockServer.URL(),
			Token:  "doesnotmatter",
		},
	})

	assert.Nil(t, hubMockServer.WaitUntilRunningJob(5, 2*time.Second))
	hubMockServer.ScheduleShutdown()

	assert.Nil(t, hubMockServer.WaitUntilDisconnected(10, 2*time.Second))
	assert.Equal(t, listener.JobProcessor.ShutdownReason, ShutdownReasonRequested)

	hubMockServer.Close()
	loghubMockServer.Close()
}

func Test__HostEnvVarsAreExposedToJob(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	config := Config{
		AgentName:          fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		ExitOnShutdown:     false,
		Endpoint:           hubMockServer.Host(),
		Token:              "token",
		RegisterRetryLimit: 5,
		Scheme:             "http",
		EnvVars: []config.HostEnvVar{
			{Name: "IMPORTANT_HOST_VAR_A", Value: "IMPORTANT_HOST_VAR_A_VALUE"},
			{Name: "IMPORTANT_HOST_VAR_B", Value: "IMPORTANT_HOST_VAR_B_VALUE"},
		},
		FileInjections: []config.FileInjection{},
		UploadJobLogs:  config.UploadJobLogsConditionNever,
		AgentVersion:   "0.0.7",
	}

	listener, err := Start(http.DefaultClient, config)
	assert.Nil(t, err)

	hubMockServer.AssignJob(&api.JobRequest{
		JobID: "Test__HostEnvVarsAreExposedToJob",
		Commands: []api.Command{
			{Directive: testsupport.Output("On regular commands")},
			{Directive: testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_A")},
			{Directive: testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_B")},
			{Directive: testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_C")},
		},
		EpilogueAlwaysCommands: []api.Command{
			{Directive: testsupport.Output("On epilogue always")},
			{Directive: testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_A")},
			{Directive: testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_B")},
			{Directive: testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_C")},
		},
		EpilogueOnPassCommands: []api.Command{
			{Directive: testsupport.Output("On epilogue on pass")},
			{Directive: testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_A")},
			{Directive: testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_B")},
			{Directive: testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_C")},
		},
		Callbacks: api.Callbacks{
			Finished:         "https://httpbin.org/status/200",
			TeardownFinished: "https://httpbin.org/status/200",
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
			URL:    loghubMockServer.URL(),
			Token:  "doesnotmatter",
		},
	})

	assert.Nil(t, hubMockServer.WaitUntilFinishedJob(12, 5*time.Second))

	eventObjects, err := eventlogger.TransformToObjects(loghubMockServer.GetLogs())
	assert.Nil(t, err)

	simplifiedEvents, err := eventlogger.SimplifyLogEvents(eventObjects, eventlogger.SimplifyOptions{IncludeOutput: true})
	assert.Nil(t, err)

	assert.Equal(t, []string{
		"job_started",

		"directive: Exporting environment variables",
		"Exporting IMPORTANT_HOST_VAR_A\n",
		"Exporting IMPORTANT_HOST_VAR_B\n",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output("On regular commands")),
		"On regular commands",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_A")),
		"IMPORTANT_HOST_VAR_A_VALUE",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_B")),
		"IMPORTANT_HOST_VAR_B_VALUE",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_C")),
		"Exit Code: 0",

		"directive: Exporting environment variables",
		"Exporting SEMAPHORE_JOB_RESULT\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output("On epilogue always")),
		"On epilogue always",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_A")),
		"IMPORTANT_HOST_VAR_A_VALUE",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_B")),
		"IMPORTANT_HOST_VAR_B_VALUE",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_C")),
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output("On epilogue on pass")),
		"On epilogue on pass",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_A")),
		"IMPORTANT_HOST_VAR_A_VALUE",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_B")),
		"IMPORTANT_HOST_VAR_B_VALUE",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("IMPORTANT_HOST_VAR_C")),
		"Exit Code: 0",

		"job_finished: passed",
	}, simplifiedEvents)

	listener.Stop()
	hubMockServer.Close()
	loghubMockServer.Close()
}

func Test__LogTokenIsRefreshed(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	config := Config{
		AgentName:          fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		ExitOnShutdown:     false,
		Endpoint:           hubMockServer.Host(),
		Token:              "token",
		RegisterRetryLimit: 5,
		Scheme:             "http",
		EnvVars:            []config.HostEnvVar{},
		FileInjections:     []config.FileInjection{},
		UploadJobLogs:      config.UploadJobLogsConditionNever,
		AgentVersion:       "0.0.7",
	}

	listener, err := Start(http.DefaultClient, config)
	assert.Nil(t, err)
	assert.False(t, hubMockServer.TokenIsRefreshed)

	hubMockServer.AssignJob(&api.JobRequest{
		JobID: "Test__LogTokenIsRefreshed",
		Commands: []api.Command{
			{Directive: testsupport.Output("hello")},
		},
		Callbacks: api.Callbacks{
			Finished:         "https://httpbin.org/status/200",
			TeardownFinished: "https://httpbin.org/status/200",
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
			URL:    loghubMockServer.URL(),
			Token:  testsupport.ExpiredLogToken,
		},
	})

	assert.Nil(t, hubMockServer.WaitUntilFinishedJob(12, 5*time.Second))
	assert.True(t, hubMockServer.TokenIsRefreshed)

	eventObjects, err := eventlogger.TransformToObjects(loghubMockServer.GetLogs())
	assert.Nil(t, err)

	simplifiedEvents, err := eventlogger.SimplifyLogEvents(eventObjects, eventlogger.SimplifyOptions{IncludeOutput: true})
	assert.Nil(t, err)

	assert.Equal(t, []string{
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

		"job_finished: passed",
	}, simplifiedEvents)

	listener.Stop()
	hubMockServer.Close()
	loghubMockServer.Close()
}

func Test__GetJobIsRetried(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())
	hubMockServer.RejectGetJobAttempts(5)

	config := Config{
		AgentName:          fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		DisconnectAfterJob: true,
		ExitOnShutdown:     false,
		Endpoint:           hubMockServer.Host(),
		Token:              "token",
		RegisterRetryLimit: 5,
		GetJobRetryLimit:   10,
		Scheme:             "http",
		EnvVars:            []config.HostEnvVar{},
		FileInjections:     []config.FileInjection{},
		UploadJobLogs:      config.UploadJobLogsConditionNever,
		AgentVersion:       "0.0.7",
	}

	listener, err := Start(http.DefaultClient, config)
	assert.Nil(t, err)

	hubMockServer.AssignJob(&api.JobRequest{
		JobID: "Test__GetJobIsRetried",
		Commands: []api.Command{
			{Directive: testsupport.Output("hello")},
		},
		Callbacks: api.Callbacks{
			Finished:         "https://httpbin.org/status/200",
			TeardownFinished: "https://httpbin.org/status/200",
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
			URL:    loghubMockServer.URL(),
			Token:  "doesnotmatter",
		},
	})

	assert.Nil(t, hubMockServer.WaitUntilDisconnected(20, 2*time.Second))
	assert.Equal(t, listener.JobProcessor.ShutdownReason, ShutdownReasonJobFinished)
	assert.Equal(t, hubMockServer.GetJobAttempts, 5)

	hubMockServer.Close()
	loghubMockServer.Close()
}

func Test__ReportsFailedToFetchJob(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())
	hubMockServer.RejectGetJobAttempts(100)

	config := Config{
		AgentName:          fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		DisconnectAfterJob: false,
		ExitOnShutdown:     false,
		Endpoint:           hubMockServer.Host(),
		Token:              "token",
		RegisterRetryLimit: 5,
		GetJobRetryLimit:   2,
		Scheme:             "http",
		EnvVars:            []config.HostEnvVar{},
		FileInjections:     []config.FileInjection{},
		UploadJobLogs:      config.UploadJobLogsConditionNever,
		AgentVersion:       "0.0.7",
	}

	listener, err := Start(http.DefaultClient, config)
	assert.Nil(t, err)

	hubMockServer.AssignJob(&api.JobRequest{
		JobID:    "Test__ReportsFailedToFetchJob",
		Commands: []api.Command{},
		Callbacks: api.Callbacks{
			Finished:         "https://httpbin.org/status/200",
			TeardownFinished: "https://httpbin.org/status/200",
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
			URL:    loghubMockServer.URL(),
			Token:  "doesnotmatter",
		},
	})

	assert.Nil(t, hubMockServer.WaitUntilFinishedJob(12, 5*time.Second))
	assert.Equal(t, selfhostedapi.JobResult(selfhostedapi.JobResultFailed), hubMockServer.GetLastJobResult())

	listener.Stop()
	hubMockServer.Close()
	loghubMockServer.Close()
}

func Test__ReportsFailedToConstructJob(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	config := Config{
		AgentName:          fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		DisconnectAfterJob: false,
		ExitOnShutdown:     false,
		Endpoint:           hubMockServer.Host(),
		Token:              "token",
		RegisterRetryLimit: 5,
		GetJobRetryLimit:   2,
		Scheme:             "http",
		EnvVars:            []config.HostEnvVar{},
		FileInjections:     []config.FileInjection{},
		UploadJobLogs:      config.UploadJobLogsConditionNever,
		AgentVersion:       "0.0.7",
	}

	listener, err := Start(http.DefaultClient, config)
	assert.Nil(t, err)

	hubMockServer.AssignJob(&api.JobRequest{
		JobID:    "Test__ReportsFailedToConstructJob",
		Executor: "doesnotexist",
		Commands: []api.Command{},
		Callbacks: api.Callbacks{
			Finished:         "https://httpbin.org/status/200",
			TeardownFinished: "https://httpbin.org/status/200",
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
			URL:    loghubMockServer.URL(),
			Token:  "doesnotmatter",
		},
	})

	assert.Nil(t, hubMockServer.WaitUntilFinishedJob(10, 2*time.Second))
	assert.Equal(t, selfhostedapi.JobResult(selfhostedapi.JobResultFailed), hubMockServer.GetLastJobResult())

	listener.Stop()
	hubMockServer.Close()
	loghubMockServer.Close()
}
