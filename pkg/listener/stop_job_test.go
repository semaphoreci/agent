package listener

import (
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/eventlogger"
	"github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
	testsupport "github.com/semaphoreci/agent/test/support"
	"github.com/stretchr/testify/assert"
)

// A stop-job can arrive while the agent is still fetching the job payload,
// before there is any job object to stop. That used to dereference a nil
// job and panic the whole agent process.
func Test__StopJobBeforeTheJobStartsRunning(t *testing.T) {
	testsupport.SetupTestLogs()

	loghubMockServer := testsupport.NewLoghubMockServer()
	loghubMockServer.Init()

	hubMockServer := testsupport.NewHubMockServer()
	hubMockServer.Init()
	hubMockServer.UseLogsURL(loghubMockServer.URL())

	// hold the payload long enough for a sync to happen in starting-job
	hubMockServer.GetJobDelay = 4 * time.Second
	hubMockServer.StopJobWhenStartingJob = true

	listener, err := Start(http.DefaultClient, newListenerConfig(hubMockServer))
	assert.Nil(t, err)

	hubMockServer.AssignJob(&api.JobRequest{
		JobID: "Test__StopJobBeforeTheJobStartsRunning",
		Commands: []api.Command{
			{Directive: testsupport.Output("this must never run")},
		},
		EnvVars: []api.EnvVar{
			{Name: "A", Value: base64.StdEncoding.EncodeToString([]byte("VALUE_A"))},
		},
		Logger: api.Logger{
			Method: eventlogger.LoggerMethodPush,
			URL:    loghubMockServer.URL(),
			Token:  "doesnotmatter",
		},
	})

	assert.Nil(t, hubMockServer.WaitUntilFinishedJob(20, 2*time.Second))
	assert.Equal(t, selfhostedapi.JobResult(selfhostedapi.JobResultStopped), hubMockServer.GetLastJobResult())

	// the commands must not have run
	eventObjects, err := eventlogger.TransformToObjects(loghubMockServer.GetLogs())
	assert.Nil(t, err)

	simplifiedEvents, err := eventlogger.SimplifyLogEvents(eventObjects, eventlogger.SimplifyOptions{IncludeOutput: true})
	assert.Nil(t, err)
	assert.NotContains(t, simplifiedEvents, "this must never run")

	listener.Stop()
	hubMockServer.Close()
	loghubMockServer.Close()
}
