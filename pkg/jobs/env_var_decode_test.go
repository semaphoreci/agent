package jobs

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"

	api "github.com/semaphoreci/agent/pkg/api"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	testsupport "github.com/semaphoreci/agent/test/support"
	"github.com/stretchr/testify/assert"
)

// A single env var that is not valid base64 fails the job before file injection,
// the pre-job hook and checkout. The job log has to name the variable and describe
// the shape of its value, otherwise the failure is impossible to triage
// (renderedtext/tasks#10631).
func Test__JobFailsWithNamedEnvVarWhenValueIsNotDecodable(t *testing.T) {
	cases := map[string]struct {
		value           string
		expectedDetails string
	}{
		"bad padding": {
			value:           "AAA",
			expectedDetails: "length 3, not padded to a multiple of 4",
		},
		"illegal character": {
			value:           "abc$def",
			expectedDetails: "length 7, not padded to a multiple of 4",
		},
		"url-safe alphabet": {
			value:           base64.URLEncoding.EncodeToString([]byte{0xfb, 0xff, 0xbf}),
			expectedDetails: "length 4, valid as url-safe base64",
		},
		"unpadded": {
			value:           base64.RawStdEncoding.EncodeToString([]byte("hello")),
			expectedDetails: "length 7, not padded to a multiple of 4",
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
			request := &api.JobRequest{
				Commands: []api.Command{
					{Directive: testsupport.EchoEnvVar("A")},
				},
				EnvVars: []api.EnvVar{
					{Name: "A", Value: base64.StdEncoding.EncodeToString([]byte("VALUE_A"))},
					{Name: "SEMAPHORE_GIT_SHA", Value: test.value},
				},
				Logger: api.Logger{Method: eventlogger.LoggerMethodPush},
			}

			job, err := NewJobWithOptions(&JobOptions{
				Request: request,
				Client:  http.DefaultClient,
				Logger:  testLogger,
			})
			assert.Nil(t, err)

			job.Run()
			assert.True(t, job.Finished)

			events, err := testLoggerBackend.SimplifiedEvents(true, false)
			assert.Nil(t, err)

			assert.Equal(t, []string{
				"job_started",

				"directive: Exporting environment variables",
				fmt.Sprintf(
					"Failed to export environment variables: error decoding 'SEMAPHORE_GIT_SHA' (%s): ",
					test.expectedDetails,
				),
				"Exit Code: 1",

				"directive: Exporting environment variables",
				"Exporting SEMAPHORE_JOB_RESULT\n",
				"Exit Code: 0",

				"job_finished: failed",
			}, withoutBase64ErrorSuffix(events))

			// the value itself never reaches the job log
			assert.NotContains(t, strings.Join(events, "\n"), test.value)
		})
	}
}

// The base64 error tail ("illegal base64 data at input byte N") is an
// implementation detail of the standard library, so it is trimmed before
// comparing the job log events.
func withoutBase64ErrorSuffix(events []string) []string {
	trimmed := []string{}
	for _, event := range events {
		if index := strings.Index(event, "illegal base64 data"); index >= 0 {
			event = event[:index]
		}

		trimmed = append(trimmed, event)
	}

	return trimmed
}
