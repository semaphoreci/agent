package listener

import (
	"net/http"
	"testing"

	"github.com/semaphoreci/agent/pkg/config"
	testsupport "github.com/semaphoreci/agent/test/support"
	"github.com/stretchr/testify/assert"
)

func Test__Register(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.Url())

	config := Config{
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
	hubMockServer.UseLogsURL(loghubMockServer.Url())
	hubMockServer.RejectRegisterAttempts(3)

	config := Config{
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
