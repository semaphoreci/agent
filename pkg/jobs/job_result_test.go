package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

/*
 * SEMAPHORE_JOB_RESULT is encoded by the agent and decoded by the same code
 * that decodes a job payload, so the two encodings have to agree. They only
 * appeared to agree because "passed" and "failed" are both 6 bytes, which
 * happens to need no padding - "stopped" does.
 */
func Test__JobResultEnvVarRoundTrips(t *testing.T) {
	results := []string{JobPassed, JobFailed, JobStopped}

	for _, result := range results {
		t.Run(result, func(t *testing.T) {
			envVar := jobResultEnvVar(result)
			assert.Equal(t, "SEMAPHORE_JOB_RESULT", envVar.Name)

			decoded, err := envVar.Decode()
			assert.NoError(t, err)
			assert.Equal(t, result, string(decoded))
		})
	}
}
