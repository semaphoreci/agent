package listener

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	"github.com/semaphoreci/agent/pkg/eventlogger"
	"github.com/semaphoreci/agent/pkg/executors"
	"github.com/semaphoreci/agent/pkg/jobs"
	"github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
	"github.com/stretchr/testify/assert"
)

// Counts how many times the job stops its executor,
// and optionally blocks the stop until released.
type shutdownTestExecutor struct {
	executors.Executor
	stops          int32
	release        chan struct{}
	commandStarted chan struct{}
	once           sync.Once
}

func (e *shutdownTestExecutor) RunCommand(command string, silent bool, alias string) int {
	if command == "sleep 60" {
		e.once.Do(func() { close(e.commandStarted) })
	}

	return e.Executor.RunCommand(command, silent, alias)
}

func (e *shutdownTestExecutor) Stop() int {
	atomic.AddInt32(&e.stops, 1)
	if e.release != nil {
		<-e.release
	}

	return e.Executor.Stop()
}

func newShutdownTestJob(t *testing.T, commands []api.Command) (*jobs.Job, *shutdownTestExecutor) {
	logger, _ := eventlogger.DefaultTestLogger()
	job, err := jobs.NewJobWithOptions(&jobs.JobOptions{
		Request: &api.JobRequest{
			EnvVars:  []api.EnvVar{},
			Commands: commands,
			Logger:   api.Logger{Method: eventlogger.LoggerMethodPush},
		},
		Client: http.DefaultClient,
		Logger: logger,
	})

	assert.NoError(t, err)
	executor := &shutdownTestExecutor{Executor: job.Executor, commandStarted: make(chan struct{})}
	job.Executor = executor
	return job, executor
}

// A job processor with no sync loop, whose hub only accepts disconnects.
func newShutdownTestProcessor(t *testing.T, job *jobs.Job) (*JobProcessor, *int32) {
	var disconnects int32
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/disconnect") {
			atomic.AddInt32(&disconnects, 1)
		}

		w.WriteHeader(http.StatusOK)
	}))

	t.Cleanup(hub.Close)

	p := &JobProcessor{
		APIClient:               selfhostedapi.New(http.DefaultClient, "http", strings.TrimPrefix(hub.URL, "http://"), "token", "test"),
		DisconnectRetryAttempts: 1,
		ExitOnShutdown:          false,
		State:                   selfhostedapi.AgentStateRunningJob,
		CurrentJob:              job,
	}

	return p, &disconnects
}

// Runs the job like the job processor does, but without reporting to it.
func runInBackground(job *jobs.Job) {
	go job.RunWithOptions(jobs.RunOptions{
		EnvVars:               []config.HostEnvVar{},
		CallbackRetryAttempts: 1,
	})
}

func waitForCommandToStart(t *testing.T, executor *shutdownTestExecutor) {
	select {
	case <-executor.commandStarted:
	case <-time.After(30 * time.Second):
		t.Fatal("job command did not start")
	}

	// Give the shell time to actually run it.
	time.Sleep(time.Second)
}

func shutdownReturnsWithin(t *testing.T, p *JobProcessor, d time.Duration) {
	done := make(chan struct{})
	go func() {
		p.Shutdown(ShutdownReasonUnableToSync, 1)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("Shutdown() did not return in %v", d)
	}
}

func Test__ShutdownStopsRunningJob(t *testing.T) {
	t.Run("running job -> stopped exactly once before disconnecting", func(t *testing.T) {
		job, executor := newShutdownTestJob(t, []api.Command{{Directive: "sleep 60"}})
		p, disconnects := newShutdownTestProcessor(t, job)

		runInBackground(job)
		waitForCommandToStart(t, executor)

		shutdownReturnsWithin(t, p, 30*time.Second)
		assert.Equal(t, int32(1), atomic.LoadInt32(&executor.stops))
		assert.Equal(t, int32(1), atomic.LoadInt32(disconnects))
		assert.True(t, job.Stopped)

		// The end of the job does not stop the executor again.
		assert.Eventually(t, func() bool { return job.IsFinished() }, 30*time.Second, 100*time.Millisecond)
		assert.Equal(t, int32(1), atomic.LoadInt32(&executor.stops))
	})

	t.Run("stop-job and shutdown at the same time -> stopped exactly once", func(t *testing.T) {
		job, executor := newShutdownTestJob(t, []api.Command{{Directive: "sleep 60"}})
		p, _ := newShutdownTestProcessor(t, job)

		runInBackground(job)
		waitForCommandToStart(t, executor)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.StopJob("job-id")
		}()

		shutdownReturnsWithin(t, p, 30*time.Second)
		wg.Wait()
		assert.Equal(t, int32(1), atomic.LoadInt32(&executor.stops))
	})

	t.Run("job stop hangs -> shutdown still returns and disconnects", func(t *testing.T) {
		previous := jobStopOnShutdownTimeout
		jobStopOnShutdownTimeout = 200 * time.Millisecond
		t.Cleanup(func() { jobStopOnShutdownTimeout = previous })

		job, executor := newShutdownTestJob(t, []api.Command{{Directive: "sleep 60"}})
		executor.release = make(chan struct{})
		t.Cleanup(func() { close(executor.release) })
		p, disconnects := newShutdownTestProcessor(t, job)

		runInBackground(job)
		waitForCommandToStart(t, executor)

		shutdownReturnsWithin(t, p, 10*time.Second)
		assert.Equal(t, int32(1), atomic.LoadInt32(&executor.stops))
		assert.Equal(t, int32(1), atomic.LoadInt32(disconnects))
	})

	t.Run("job already finished -> executor not stopped again", func(t *testing.T) {
		job, executor := newShutdownTestJob(t, []api.Command{{Directive: "echo hello"}})
		job.Run()
		assert.True(t, job.Finished)

		p, disconnects := newShutdownTestProcessor(t, job)
		p.State = selfhostedapi.AgentStateFinishedJob

		shutdownReturnsWithin(t, p, 10*time.Second)
		assert.Equal(t, int32(1), atomic.LoadInt32(&executor.stops))
		assert.False(t, job.Stopped)
		assert.Equal(t, int32(1), atomic.LoadInt32(disconnects))
	})

	t.Run("no job -> shutdown disconnects", func(t *testing.T) {
		p, disconnects := newShutdownTestProcessor(t, nil)
		p.State = selfhostedapi.AgentStateWaitingForJobs

		shutdownReturnsWithin(t, p, 10*time.Second)
		assert.Equal(t, int32(1), atomic.LoadInt32(disconnects))
	})
}

// A hub that holds the job description until released,
// like a slow API while the agent is starting a job.
func newSlowJobHub(t *testing.T, release chan struct{}) (*selfhostedapi.API, chan struct{}) {
	requested := make(chan struct{}, 1)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/jobs/") {
			requested <- struct{}{}
			<-release
			_, _ = w.Write([]byte(`{"job_id": "slow-job", "commands": [{"directive": "echo hello"}], "logger": {"method": "pull"}}`))
			return
		}

		w.WriteHeader(http.StatusOK)
	}))

	t.Cleanup(hub.Close)
	return selfhostedapi.New(http.DefaultClient, "http", strings.TrimPrefix(hub.URL, "http://"), "token", "test"), requested
}

func newRunJobTestProcessor(apiClient *selfhostedapi.API) *JobProcessor {
	return &JobProcessor{
		APIClient:               apiClient,
		HTTPClient:              http.DefaultClient,
		DisconnectRetryAttempts: 1,
		GetJobRetryAttempts:     1,
		CallbackRetryAttempts:   1,
		ExitOnShutdown:          false,
		State:                   selfhostedapi.AgentStateWaitingForJobs,
		forceSyncCh:             make(chan bool, 1),
	}
}

func Test__ShutdownWhileStartingJob(t *testing.T) {
	t.Run("shutdown while the job is being fetched -> job is not run", func(t *testing.T) {
		release := make(chan struct{})
		apiClient, requested := newSlowJobHub(t, release)
		p := newRunJobTestProcessor(apiClient)

		runJobDone := make(chan struct{})
		go func() {
			p.RunJob("slow-job")
			close(runJobDone)
		}()

		<-requested
		shutdownReturnsWithin(t, p, 10*time.Second)
		close(release)

		select {
		case <-runJobDone:
		case <-time.After(10 * time.Second):
			t.Fatal("RunJob() did not return")
		}

		p.jobMu.Lock()
		defer p.jobMu.Unlock()
		assert.Nil(t, p.CurrentJob, "a job was started after the shutdown")
		assert.NotEqual(t, selfhostedapi.AgentStateRunningJob, p.State)
	})

	t.Run("no shutdown -> job is run", func(t *testing.T) {
		release := make(chan struct{})
		close(release)
		apiClient, _ := newSlowJobHub(t, release)
		p := newRunJobTestProcessor(apiClient)

		p.RunJob("slow-job")

		p.jobMu.Lock()
		job := p.CurrentJob
		p.jobMu.Unlock()

		if assert.NotNil(t, job) {
			assert.Eventually(t, job.IsFinished, 30*time.Second, 100*time.Millisecond)
		}
	})
}
