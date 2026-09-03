package listener

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	"github.com/semaphoreci/agent/pkg/eventlogger"
	"github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
	testsupport "github.com/semaphoreci/agent/test/support"
	"github.com/stretchr/testify/assert"
)

// A job payload carrying an env var that is not valid base64 is re-fetched,
// since the corruption might have happened in transit (renderedtext/tasks#10631).
func Test__JobPayloadWithUndecodableEnvVarIsRefetched(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	listener, err := Start(http.DefaultClient, newListenerConfig(hubMockServer))
	assert.Nil(t, err)

	hubMockServer.AssignBadJobFor(1, jobRequestWithGitSHA(loghubMockServer.URL(), "abc$def"))
	hubMockServer.AssignJob(jobRequestWithGitSHA(
		loghubMockServer.URL(),
		base64.StdEncoding.EncodeToString([]byte("1234567")),
	))

	assert.Nil(t, hubMockServer.WaitUntilFinishedJob(12, 5*time.Second))
	assert.Equal(t, selfhostedapi.JobResult(selfhostedapi.JobResultPassed), hubMockServer.GetLastJobResult())
	assert.Equal(t, 2, hubMockServer.GetGetJobAttempts())

	listener.Stop()
	hubMockServer.Close()
	loghubMockServer.Close()
}

// The last attempt must decide on a payload it actually validated. Fetching a
// fresh payload on the way out meant the job ran on an unvalidated request
// while the agent log blamed the previous one.
func Test__JobPayloadIsNotRefetchedOnTheFinalAttempt(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	listener, err := Start(http.DefaultClient, newListenerConfig(hubMockServer))
	assert.Nil(t, err)

	// bad for the initial fetch and both re-fetches, good afterwards
	hubMockServer.AssignBadJobFor(3, jobRequestWithGitSHA(loghubMockServer.URL(), "abc$def"))
	hubMockServer.AssignJob(jobRequestWithGitSHA(
		loghubMockServer.URL(),
		base64.StdEncoding.EncodeToString([]byte("1234567")),
	))

	assert.Nil(t, hubMockServer.WaitUntilFinishedJob(12, 5*time.Second))

	// The payload never validated, so the job has to fail - and the agent must
	// not have spent a fourth fetch on a payload it would never look at.
	assert.Equal(t, selfhostedapi.JobResult(selfhostedapi.JobResultFailed), hubMockServer.GetLastJobResult())
	assert.Equal(t, 3, hubMockServer.GetGetJobAttempts())

	listener.Stop()
	hubMockServer.Close()
	loghubMockServer.Close()
}

// When re-fetching does not help, the job still has to run and fail through
// the executor, so that the job log names the offending variable. Failing
// before the job is constructed would leave the customer with an empty job log.
func Test__JobPayloadWithUndecodableEnvVarStillProducesJobLog(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	listener, err := Start(http.DefaultClient, newListenerConfig(hubMockServer))
	assert.Nil(t, err)

	hubMockServer.AssignJob(jobRequestWithGitSHA(loghubMockServer.URL(), "abc$def"))

	assert.Nil(t, hubMockServer.WaitUntilFinishedJob(12, 5*time.Second))
	assert.Equal(t, selfhostedapi.JobResult(selfhostedapi.JobResultFailed), hubMockServer.GetLastJobResult())
	assert.Greater(t, hubMockServer.GetGetJobAttempts(), 1)

	eventObjects, err := eventlogger.TransformToObjects(loghubMockServer.GetLogs())
	assert.Nil(t, err)

	simplifiedEvents, err := eventlogger.SimplifyLogEvents(eventObjects, eventlogger.SimplifyOptions{IncludeOutput: true})
	assert.Nil(t, err)
	assert.NotEmpty(t, simplifiedEvents)

	joinedEvents := strings.Join(simplifiedEvents, "\n")
	assert.Contains(t, joinedEvents, "error decoding 'SEMAPHORE_GIT_SHA'")
	assert.Contains(t, joinedEvents, "job_finished: failed")
	assert.NotContains(t, joinedEvents, "abc$def")

	listener.Stop()
	hubMockServer.Close()
	loghubMockServer.Close()
}

func newListenerConfig(hubMockServer *testsupport.HubMockServer) Config {
	return Config{
		AgentName:          fmt.Sprintf("agent-name-%d", rand.Intn(10000000)),
		ExitOnShutdown:     false,
		Endpoint:           hubMockServer.Host(),
		Token:              "token",
		RegisterRetryLimit: 5,
		GetJobRetryLimit:   5,
		// keep the re-fetch delay out of the test's wall clock
		ValidatePayloadDelay: time.Millisecond,
		Scheme:               "http",
		EnvVars:              []config.HostEnvVar{},
		FileInjections:       []config.FileInjection{},
		UploadJobLogs:        config.UploadJobLogsConditionNever,
		AgentVersion:         testsupport.AgentVersionExpected,
		UserAgent:            fmt.Sprintf("SemaphoreAgent/%s", testsupport.AgentVersionExpected),
	}
}

func jobRequestWithGitSHA(logsURL, gitSHA string) *api.JobRequest {
	return &api.JobRequest{
		JobID: "Test__UndecodableEnvVar",
		Commands: []api.Command{
			{Directive: testsupport.Output("checkout would run here")},
		},
		EnvVars: []api.EnvVar{
			{Name: "SEMAPHORE_GIT_URL", Value: base64.StdEncoding.EncodeToString([]byte("git@github.com:x/y.git"))},
			{Name: "SEMAPHORE_GIT_SHA", Value: gitSHA},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
			URL:    logsURL,
			Token:  "doesnotmatter",
		},
	}
}
