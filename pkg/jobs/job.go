package jobs

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	executors "github.com/semaphoreci/agent/pkg/executors"
	"github.com/semaphoreci/agent/pkg/retry"
	log "github.com/sirupsen/logrus"
)

const JobPassed = "passed"
const JobFailed = "failed"
const JobStopped = "stopped"

type Job struct {
	Client  *http.Client
	Request *api.JobRequest
	Logger  *eventlogger.Logger

	Executor executors.Executor

	JobLogArchived bool
	Stopped        bool
	Finished       bool
}

type JobOptions struct {
	Request            *api.JobRequest
	Client             *http.Client
	ExposeKvmDevice    bool
	FileInjections     []config.FileInjection
	FailOnMissingFiles bool
}

func NewJob(request *api.JobRequest, client *http.Client) (*Job, error) {
	return NewJobWithOptions(&JobOptions{
		Request:            request,
		Client:             client,
		ExposeKvmDevice:    true,
		FileInjections:     []config.FileInjection{},
		FailOnMissingFiles: false,
	})
}

func NewJobWithOptions(options *JobOptions) (*Job, error) {
	if options.Request.Executor == "" {
		log.Infof("No executor specified - using %s executor", executors.ExecutorTypeShell)
		options.Request.Executor = executors.ExecutorTypeShell
	}

	if options.Request.Logger.Method == "" {
		log.Infof("No logger method specified - using %s logger method", eventlogger.LoggerMethodPull)
		options.Request.Logger.Method = eventlogger.LoggerMethodPull
	}

	logger, err := eventlogger.CreateLogger(options.Request)
	if err != nil {
		return nil, err
	}

	executor, err := CreateExecutor(options.Request, logger, *options)
	if err != nil {
		return nil, err
	}

	log.Debugf("Job Request %+v", options.Request)

	return &Job{
		Client:         options.Client,
		Request:        options.Request,
		Executor:       executor,
		JobLogArchived: false,
		Stopped:        false,
		Logger:         logger,
	}, nil
}

func CreateExecutor(request *api.JobRequest, logger *eventlogger.Logger, jobOptions JobOptions) (executors.Executor, error) {
	switch request.Executor {
	case executors.ExecutorTypeShell:
		return executors.NewShellExecutor(request, logger), nil
	case executors.ExecutorTypeDockerCompose:
		executorOptions := executors.DockerComposeExecutorOptions{
			ExposeKvmDevice:    jobOptions.ExposeKvmDevice,
			FileInjections:     jobOptions.FileInjections,
			FailOnMissingFiles: jobOptions.FailOnMissingFiles,
		}

		return executors.NewDockerComposeExecutor(request, logger, executorOptions), nil
	default:
		return nil, fmt.Errorf("unknown executor type")
	}
}

type RunOptions struct {
	EnvVars              []config.HostEnvVar
	FileInjections       []config.FileInjection
	OnSuccessfulTeardown func()
	OnFailedTeardown     func()
}

func (job *Job) Run() {
	job.RunWithOptions(RunOptions{
		EnvVars:              []config.HostEnvVar{},
		FileInjections:       []config.FileInjection{},
		OnSuccessfulTeardown: nil,
		OnFailedTeardown:     nil,
	})
}

func (job *Job) RunWithOptions(options RunOptions) {
	log.Infof("Running job %s", job.Request.ID)
	executorRunning := false
	result := JobFailed

	job.Logger.LogJobStarted()

	exitCode := job.PrepareEnvironment()
	if exitCode == 0 {
		executorRunning = true
	} else {
		log.Error("Executor failed to boot up")
	}

	if executorRunning {
		result = job.RunRegularCommands(options.EnvVars)
		log.Debug("Exporting job result")
		job.RunCommandsUntilFirstFailure([]api.Command{
			{
				Directive: fmt.Sprintf("export SEMAPHORE_JOB_RESULT=%s", result),
			},
		})

		if result != JobStopped {
			job.handleEpilogues(result)
		}
	}

	err := job.Teardown(result)
	if err != nil {
		callFuncIfNotNull(options.OnFailedTeardown)
	} else {
		callFuncIfNotNull(options.OnSuccessfulTeardown)
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

func (job *Job) RunRegularCommands(hostEnvVars []config.HostEnvVar) string {
	exitCode := job.Executor.ExportEnvVars(job.Request.EnvVars, hostEnvVars)
	if exitCode != 0 {
		log.Error("Failed to export env vars")

		return JobFailed
	}

	exitCode = job.Executor.InjectFiles(job.Request.Files)
	if exitCode != 0 {
		log.Error("Failed to inject files")

		return JobFailed
	}

	exitCode = job.RunCommandsUntilFirstFailure(job.Request.Commands)

	if job.Stopped {
		log.Info("Regular commands were stopped")
		return JobStopped
	} else if exitCode == 0 {
		log.Info("Regular commands finished successfully")
		return JobPassed
	} else {
		log.Info("Regular commands finished with failure")
		return JobFailed
	}
}

func (job *Job) handleEpilogues(result string) {
	job.executeIfNotStopped(func() {
		log.Info("Starting epilogue always commands")
		job.RunCommandsUntilFirstFailure(job.Request.EpilogueAlwaysCommands)
	})

	job.executeIfNotStopped(func() {
		if result == JobPassed {
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
		result = JobStopped
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

func (job *Job) Stop() {
	log.Info("Stopping job")

	job.Stopped = true

	log.Debug("Invoking process stopping")

	PreventPanicPropagation(func() {
		job.Executor.Stop()
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
