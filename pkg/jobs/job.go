package jobs

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	executors "github.com/semaphoreci/agent/pkg/executors"
	"github.com/semaphoreci/agent/pkg/retry"
	log "github.com/sirupsen/logrus"
)

const JOB_PASSED = "passed"
const JOB_FAILED = "failed"
const JOB_STOPPED = "stopped"

type Job struct {
	Client  *http.Client
	Request *api.JobRequest
	Logger  *eventlogger.Logger

	Executor executors.Executor

	JobLogArchived bool
	Stopped        bool
	Finished       bool
}

func NewJob(request *api.JobRequest, client *http.Client) (*Job, error) {
	if request.Executor == "" {
		log.Infof("No executor specified - using %s executor", executors.ExecutorTypeShell)
		request.Executor = executors.ExecutorTypeShell
	}

	if request.Logger.Method == "" {
		log.Infof("No logger method specified - using %s logger method", eventlogger.LoggerMethodPull)
		request.Logger.Method = eventlogger.LoggerMethodPull
	}

	logger, err := eventlogger.CreateLogger(request)
	if err != nil {
		return nil, err
	}

	executor, err := executors.CreateExecutor(request, logger)
	if err != nil {
		return nil, err
	}

	log.Debugf("Job Request %+v", request)

	return &Job{
		Client:         client,
		Request:        request,
		Executor:       executor,
		JobLogArchived: false,
		Stopped:        false,
		Logger:         logger,
	}, nil
}

func (job *Job) Run() {
	job.RunWithCallbacks(nil, nil)
}

func (job *Job) RunWithCallbacks(onSuccessfulTeardown func(), onFailedTeardown func()) {
	log.Infof("Running job %s", job.Request.ID)
	executorRunning := false
	result := JOB_FAILED

	job.Logger.LogJobStarted()

	exitCode := job.PrepareEnvironment()
	if exitCode == 0 {
		executorRunning = true
	} else {
		log.Error("Executor failed to boot up")
	}

	if executorRunning {
		result = job.RunRegularCommands()
		log.Debug("Exporting job result")
		job.RunCommandsUntilFirstFailure([]api.Command{
			{
				Directive: fmt.Sprintf("export SEMAPHORE_JOB_RESULT=%s", result),
			},
		})

		if result != JOB_STOPPED {
			job.handleEpilogues(result)
		}
	}

	err := job.Teardown(result)
	if err != nil {
		callFuncIfNotNull(onFailedTeardown)
	} else {
		callFuncIfNotNull(onSuccessfulTeardown)
	}

	job.Finished = true

	// the executor is already stopped when the job is stopped, so there's no need to stop it again
	if !job.Stopped {
		job.Executor.Stop()
	}
}

func (job *Job) PrepareEnvironment() int {
	exitCode := job.Executor.Prepare()
	if exitCode != 0 {
		log.Error("Failed to prepare executor")
		return exitCode
	}

	exitCode = job.Executor.Start()
	if exitCode != 0 {
		log.Error("Failed to start executor")
		return exitCode
	}

	return 0
}

func (job *Job) RunRegularCommands() string {
	exitCode := job.Executor.ExportEnvVars(job.Request.EnvVars)
	if exitCode != 0 {
		log.Error("Failed to export env vars")

		return JOB_FAILED
	}

	exitCode = job.Executor.InjectFiles(job.Request.Files)
	if exitCode != 0 {
		log.Error("Failed to inject files")

		return JOB_FAILED
	}

	exitCode = job.RunCommandsUntilFirstFailure(job.Request.Commands)

	if job.Stopped {
		log.Info("Regular commands were stopped")
		return JOB_STOPPED
	} else if exitCode == 0 {
		log.Info("Regular commands finished successfully")
		return JOB_PASSED
	} else {
		log.Info("Regular commands finished with failure")
		return JOB_FAILED
	}
}

func (job *Job) handleEpilogues(result string) {
	job.executeIfNotStopped(func() {
		log.Info("Starting epilogue always commands")
		job.RunCommandsUntilFirstFailure(job.Request.EpilogueAlwaysCommands)
	})

	job.executeIfNotStopped(func() {
		if result == JOB_PASSED {
			log.Info("Starting epilogue on pass commands")
			job.RunCommandsUntilFirstFailure(job.Request.EpilogueOnPassCommands)
		} else {
			log.Info("Starting epilogue on fail commands")
			job.RunCommandsUntilFirstFailure(job.Request.EpilogueOnFailCommands)
		}
	})
}

func (job *Job) executeIfNotStopped(callback func()) {
	if !job.Stopped && callback != nil {
		callback()
	}
}

// returns exit code of last executed command
func (job *Job) RunCommandsUntilFirstFailure(commands []api.Command) int {
	lastExitCode := 1

	for _, c := range commands {
		if job.Stopped {
			return 1
		}

		lastExitCode = job.Executor.RunCommand(c.Directive, false, c.Alias)

		if lastExitCode != 0 {
			break
		}
	}

	return lastExitCode
}

func (job *Job) Teardown(result string) error {
	// if job was stopped during the epilogues, result should be stopped
	if job.Stopped {
		result = JOB_STOPPED
	}

	err := job.SendFinishedCallback(result)
	if err != nil {
		log.Errorf("Could not send finished callback: %v", err)
		return err
	}

	job.Logger.LogJobFinished(result)

	if job.Request.Logger.Method == eventlogger.LoggerMethodPull {
		log.Debug("Waiting for archivator")

		for {
			if job.JobLogArchived {
				break
			} else {
				time.Sleep(1000 * time.Millisecond)
			}
		}

		log.Debug("Archivator finished")
	}

	err = job.Logger.Close()
	if err != nil {
		log.Errorf("Error closing logger: %+v", err)
	}

	err = job.SendTeardownFinishedCallback()
	if err != nil {
		log.Errorf("Could not send teardown finished callback: %v", err)
		return err
	}

	log.Info("Job teardown finished")
	return nil
}

func (j *Job) Stop() {
	log.Info("Stopping job")

	j.Stopped = true

	log.Debug("Invoking process stopping")

	PreventPanicPropagation(func() {
		j.Executor.Stop()
	})
}

func (job *Job) SendFinishedCallback(result string) error {
	payload := fmt.Sprintf(`{"result": "%s"}`, result)
	log.Infof("Sending finished callback: %+v", payload)
	return retry.RetryWithConstantWait("Send finished callback", 60, time.Second, func() error {
		return job.SendCallback(job.Request.Callbacks.Finished, payload)
	})
}

func (job *Job) SendTeardownFinishedCallback() error {
	log.Info("Sending teardown finished callback")
	return retry.RetryWithConstantWait("Send teardown finished callback", 60, time.Second, func() error {
		return job.SendCallback(job.Request.Callbacks.TeardownFinished, "{}")
	})
}

func (job *Job) SendCallback(url string, payload string) error {
	log.Debugf("Sending callback to %s: %+v", url, payload)
	request, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(payload)))
	if err != nil {
		return err
	}

	response, err := job.Client.Do(request)
	if err != nil {
		return err
	}

	if !isSuccessfulCode(response.StatusCode) {
		return fmt.Errorf("callback to %s got HTTP %d", url, response.StatusCode)
	}

	return nil
}

func callFuncIfNotNull(function func()) {
	if function != nil {
		function()
	}
}

func isSuccessfulCode(code int) bool {
	return code >= 200 && code < 300
}
