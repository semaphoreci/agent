package listener

import (
	"testing"
	"time"

	"github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
	"github.com/stretchr/testify/assert"
)

/*
 * SyncLoop only drains forceSyncCh between syncs, so anything that nudges it
 * while holding the mutex - which Sync() also needs - deadlocks the agent.
 * That is how a stopped job used to leave the agent reporting stopping-job
 * forever.
 */
func Test__ForcingASyncNeverBlocks(t *testing.T) {
	newProcessor := func() *JobProcessor {
		return &JobProcessor{
			forceSyncCh: make(chan bool, 1),
			State:       selfhostedapi.AgentStateRunningJob,
		}
	}

	assertDoesNotBlock := func(t *testing.T, name string, fn func()) {
		done := make(chan struct{})
		go func() {
			fn()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s blocked with nobody draining forceSyncCh", name)
		}
	}

	t.Run("JobFinished with no sync loop running", func(t *testing.T) {
		p := newProcessor()
		assertDoesNotBlock(t, "JobFinished", func() {
			p.JobFinished(selfhostedapi.JobResultPassed)
		})

		assert.Equal(t, selfhostedapi.AgentState(selfhostedapi.AgentStateFinishedJob), p.State)
		assert.Equal(t, selfhostedapi.JobResult(selfhostedapi.JobResultPassed), p.CurrentJobResult)
	})

	t.Run("repeated nudges with no sync loop running", func(t *testing.T) {
		p := newProcessor()
		assertDoesNotBlock(t, "forceSync", func() {
			for i := 0; i < 10; i++ {
				p.forceSync()
			}
		})
	})

	t.Run("StopJob for a job that has not been constructed yet", func(t *testing.T) {
		p := newProcessor()
		p.State = selfhostedapi.AgentStateStartingJob

		assertDoesNotBlock(t, "StopJob", func() {
			p.StopJob("job-id")
		})

		assert.Equal(t, selfhostedapi.AgentState(selfhostedapi.AgentStateFinishedJob), p.State)
		assert.Equal(t, selfhostedapi.JobResult(selfhostedapi.JobResultStopped), p.CurrentJobResult)
	})
}
