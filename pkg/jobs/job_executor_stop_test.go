package jobs

import (
	"net/http"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	executors "github.com/semaphoreci/agent/pkg/executors"
	selfhostedapi "github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
	testsupport "github.com/semaphoreci/agent/test/support"
	"github.com/stretchr/testify/assert"
)

// Counts how many times the job stops its executor.
// For the Kubernetes executor, stopping is what removes the job pod.
type stopCountingExecutor struct {
	executors.Executor
	stops int32
}

func (e *stopCountingExecutor) Stop() int {
	atomic.AddInt32(&e.stops, 1)
	return e.Executor.Stop()
}

func newJobWithStopCounter(t *testing.T, commands []api.Command) (*Job, *stopCountingExecutor) {
	testLogger, _ := eventlogger.DefaultTestLogger()
	job, err := NewJobWithOptions(&JobOptions{
		Request: &api.JobRequest{
			EnvVars:  []api.EnvVar{},
			Commands: commands,
			Logger:   api.Logger{Method: eventlogger.LoggerMethodPush},
		},
		Client: http.DefaultClient,
		Logger: testLogger,
	})

	assert.NoError(t, err)
	counter := &stopCountingExecutor{Executor: job.Executor}
	job.Executor = counter
	return job, counter
}

func Test__ExecutorIsStoppedOnce(t *testing.T) {
	t.Run("job passes -> executor stopped once", func(t *testing.T) {
		job, counter := newJobWithStopCounter(t, []api.Command{{Directive: testsupport.Output("hello")}})
		job.Run()
		assert.True(t, job.Finished)
		assert.False(t, job.Stopped)
		assert.Equal(t, int32(1), atomic.LoadInt32(&counter.stops))
	})

	t.Run("job fails -> executor stopped once", func(t *testing.T) {
		job, counter := newJobWithStopCounter(t, []api.Command{{Directive: "false"}})
		job.Run()
		assert.True(t, job.Finished)
		assert.Equal(t, int32(1), atomic.LoadInt32(&counter.stops))
	})

	t.Run("commands exit with 130 -> executor still stopped once", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip()
		}

		job, counter := newJobWithStopCounter(t, []api.Command{{Directive: testsupport.ReturnExitCodeCommand(130)}})
		job.Run()
		assert.True(t, job.Finished)
		assert.True(t, job.Stopped)
		assert.Equal(t, int32(1), atomic.LoadInt32(&counter.stops))
	})

	t.Run("job stopped while running -> executor stopped once, not again at teardown", func(t *testing.T) {
		job, counter := newJobWithStopCounter(t, []api.Command{{Directive: "sleep 60"}})
		go job.Run()

		time.Sleep(3 * time.Second)
		job.Stop()

		assert.Eventually(t, func() bool { return job.Finished }, 10*time.Second, 100*time.Millisecond)
		assert.True(t, job.Stopped)
		assert.Equal(t, int32(1), atomic.LoadInt32(&counter.stops))
	})

	t.Run("job stopped after it finished -> executor not stopped again", func(t *testing.T) {
		job, counter := newJobWithStopCounter(t, []api.Command{{Directive: testsupport.Output("hello")}})
		job.Run()
		job.Stop()
		assert.Equal(t, int32(1), atomic.LoadInt32(&counter.stops))
	})
}

// Lets a test decide what Prepare() and Start() do, like the Kubernetes
// executor giving up on creating the job pod when the job is stopped.
type bootExecutor struct {
	executors.Executor
	prepare func() int
	start   func() int
	stopped chan struct{}
	once    sync.Once
}

func (e *bootExecutor) Prepare() int {
	if e.prepare != nil {
		return e.prepare()
	}

	return e.Executor.Prepare()
}

func (e *bootExecutor) Start() int {
	if e.start != nil {
		return e.start()
	}

	return e.Executor.Start()
}

func (e *bootExecutor) Stop() int {
	e.once.Do(func() { close(e.stopped) })
	return e.Executor.Stop()
}

// Returns the exit code once the job is stopped.
func (e *bootExecutor) untilStopped() int {
	<-e.stopped
	return 1
}

func newJobWithBootExecutor(t *testing.T, commands []api.Command) (*Job, *bootExecutor) {
	testLogger, _ := eventlogger.DefaultTestLogger()
	job, err := NewJobWithOptions(&JobOptions{
		Request: &api.JobRequest{
			EnvVars:  []api.EnvVar{},
			Commands: commands,
			Logger:   api.Logger{Method: eventlogger.LoggerMethodPush},
		},
		Client: http.DefaultClient,
		Logger: testLogger,
	})

	assert.NoError(t, err)
	executor := &bootExecutor{Executor: job.Executor, stopped: make(chan struct{})}
	job.Executor = executor
	return job, executor
}

// Runs the job in the background and returns the result it reports.
func runForResult(job *Job) chan selfhostedapi.JobResult {
	results := make(chan selfhostedapi.JobResult, 1)
	go job.RunWithOptions(RunOptions{
		EnvVars:               []config.HostEnvVar{},
		CallbackRetryAttempts: 1,
		OnJobFinished:         func(result selfhostedapi.JobResult) { results <- result },
	})

	return results
}

func waitForResult(t *testing.T, results chan selfhostedapi.JobResult) selfhostedapi.JobResult {
	select {
	case result := <-results:
		return result
	case <-time.After(30 * time.Second):
		t.Fatal("job did not finish")
		return ""
	}
}

func Test__JobResultWhenExecutorDoesNotBoot(t *testing.T) {
	t.Run("stopped during Prepare -> stopped", func(t *testing.T) {
		job, executor := newJobWithBootExecutor(t, []api.Command{{Directive: testsupport.Output("hello")}})
		prepareStarted := make(chan struct{})
		executor.prepare = func() int {
			close(prepareStarted)
			return executor.untilStopped()
		}

		results := runForResult(job)
		<-prepareStarted
		job.Stop()

		assert.Equal(t, selfhostedapi.JobResult(JobStopped), waitForResult(t, results))
		assert.True(t, job.Finished)
	})

	t.Run("stopped during Start -> stopped", func(t *testing.T) {
		job, executor := newJobWithBootExecutor(t, []api.Command{{Directive: testsupport.Output("hello")}})
		startStarted := make(chan struct{})
		executor.start = func() int {
			close(startStarted)
			return executor.untilStopped()
		}

		results := runForResult(job)
		<-startStarted
		job.Stop()

		assert.Equal(t, selfhostedapi.JobResult(JobStopped), waitForResult(t, results))
	})

	t.Run("Prepare fails without a stop -> failed", func(t *testing.T) {
		job, executor := newJobWithBootExecutor(t, []api.Command{{Directive: testsupport.Output("hello")}})
		executor.prepare = func() int { return 1 }

		assert.Equal(t, selfhostedapi.JobResult(JobFailed), waitForResult(t, runForResult(job)))
		assert.False(t, job.Stopped)
	})

	t.Run("Start fails without a stop -> failed", func(t *testing.T) {
		job, executor := newJobWithBootExecutor(t, []api.Command{{Directive: testsupport.Output("hello")}})
		executor.start = func() int { return 1 }

		assert.Equal(t, selfhostedapi.JobResult(JobFailed), waitForResult(t, runForResult(job)))
		assert.False(t, job.Stopped)
	})

	t.Run("executor boots, commands pass -> passed", func(t *testing.T) {
		job, _ := newJobWithBootExecutor(t, []api.Command{{Directive: testsupport.Output("hello")}})
		assert.Equal(t, selfhostedapi.JobResult(JobPassed), waitForResult(t, runForResult(job)))
	})

	t.Run("executor boots, commands fail -> failed", func(t *testing.T) {
		job, _ := newJobWithBootExecutor(t, []api.Command{{Directive: "false"}})
		assert.Equal(t, selfhostedapi.JobResult(JobFailed), waitForResult(t, runForResult(job)))
	})
}

func Test__AgentPodName(t *testing.T) {
	t.Run("uses KUBERNETES_POD_NAME when set", func(t *testing.T) {
		t.Setenv("KUBERNETES_POD_NAME", "agent-pod-from-env")
		assert.Equal(t, "agent-pod-from-env", agentPodName())
	})

	t.Run("inside the cluster -> falls back to the hostname", func(t *testing.T) {
		t.Setenv("KUBERNETES_POD_NAME", "")
		t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
		hostname, err := os.Hostname()
		assert.NoError(t, err)
		assert.Equal(t, hostname, agentPodName())
	})

	t.Run("outside the cluster -> no pod name", func(t *testing.T) {
		t.Setenv("KUBERNETES_POD_NAME", "")
		t.Setenv("KUBERNETES_SERVICE_HOST", "")
		assert.Equal(t, "", agentPodName())
	})

	t.Run("outside the cluster, KUBERNETES_POD_NAME set -> uses it", func(t *testing.T) {
		t.Setenv("KUBERNETES_POD_NAME", "agent-pod-from-env")
		t.Setenv("KUBERNETES_SERVICE_HOST", "")
		assert.Equal(t, "agent-pod-from-env", agentPodName())
	})
}
