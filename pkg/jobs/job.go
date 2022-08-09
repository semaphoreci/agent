package jobs

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	executors "github.com/semaphoreci/agent/pkg/executors"
	httputils "github.com/semaphoreci/agent/pkg/httputils"
	"github.com/semaphoreci/agent/pkg/listener/selfhostedapi"
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

	JobLogArchived    bool
	Stopped           bool
	Finished          bool
	UploadTrimmedLogs bool
}

type JobOptions struct {
	Request            *api.JobRequest
	Client             *http.Client
	Logger             *eventlogger.Logger
	ExposeKvmDevice    bool
	FileInjections     []config.FileInjection
	FailOnMissingFiles bool
	SelfHosted         bool
	UploadTrimmedLogs  bool
	RefreshTokenFn     func() (string, error)
}

func NewJob(request *api.JobRequest, client *http.Client) (*Job, error) {
	return NewJobWithOptions(&JobOptions{
		Request:            request,
		Client:             client,
		ExposeKvmDevice:    true,
		FileInjections:     []config.FileInjection{},
		FailOnMissingFiles: false,
		SelfHosted:         false,
		RefreshTokenFn:     nil,
	})
}

func NewJobWithOptions(options *JobOptions) (*Job, error) {
	log.Debugf("Job Request %+v", options.Request)

	if options.Request.Executor == "" {
		log.Infof("No executor specified - using %s executor", executors.ExecutorTypeShell)
		options.Request.Executor = executors.ExecutorTypeShell
	}

	if options.Request.Logger.Method == "" {
		log.Infof("No logger method specified - using %s logger method", eventlogger.LoggerMethodPull)
		options.Request.Logger.Method = eventlogger.LoggerMethodPull
	}

	job := &Job{
		Client:            options.Client,
		Request:           options.Request,
		JobLogArchived:    false,
		Stopped:           false,
		UploadTrimmedLogs: options.UploadTrimmedLogs,
	}

	if options.Logger != nil {
		job.Logger = options.Logger
	} else {
		l, err := eventlogger.CreateLogger(options.Request, options.RefreshTokenFn)
		if err != nil {
			return nil, err
		}

		job.Logger = l
	}

	executor, err := CreateExecutor(options.Request, job.Logger, *options)
	if err != nil {
		_ = job.Logger.Close()
		return nil, err
	}

	job.Executor = executor
	return job, nil
}

func CreateExecutor(request *api.JobRequest, logger *eventlogger.Logger, jobOptions JobOptions) (executors.Executor, error) {
	switch request.Executor {
	case executors.ExecutorTypeShell:
		return executors.NewShellExecutor(request, logger, jobOptions.SelfHosted), nil
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
	EnvVars               []config.HostEnvVar
	FileInjections        []config.FileInjection
	OnJobFinished         func(selfhostedapi.JobResult)
	CallbackRetryAttempts int
}

func (job *Job) Run() {
	job.RunWithOptions(RunOptions{
		EnvVars:               []config.HostEnvVar{},
		FileInjections:        []config.FileInjection{},
		OnJobFinished:         nil,
		CallbackRetryAttempts: 60,
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

		if result != JobStopped {
			log.Debug("Handling epilogues")
			job.handleEpilogues(result)
		}
	}

	result, err := job.Teardown(result, options.CallbackRetryAttempts)
	if err != nil {
		log.Errorf("Error tearing down job: %v", err)
	}

	// the executor is already stopped when the job is stopped, so there's no need to stop it again
	if !job.Stopped {
		job.Executor.Stop()
	}

	job.Finished = true
	if options.OnJobFinished != nil {
		options.OnJobFinished(selfhostedapi.JobResult(result))
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

	if len(job.Request.Commands) == 0 {
		exitCode = 0
	} else {
		exitCode = job.RunCommandsUntilFirstFailure(job.Request.Commands)
	}

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
	envVars := []api.EnvVar{
		{Name: "SEMAPHORE_JOB_RESULT", Value: base64.RawStdEncoding.EncodeToString([]byte(result))},
	}

	exitCode := job.Executor.ExportEnvVars(envVars, []config.HostEnvVar{})
	if exitCode != 0 {
		log.Errorf("Error setting SEMAPHORE_JOB_RESULT: exit code %d", exitCode)
	}

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

func (job *Job) Teardown(result string, callbackRetryAttempts int) (string, error) {
	// if job was stopped during the epilogues, result should be stopped
	if job.Stopped {
		result = JobStopped
	}

	if job.Request.Logger.Method == eventlogger.LoggerMethodPull {
		return result, job.teardownWithCallbacks(result, callbackRetryAttempts)
	}

	return result, job.teardownWithNoCallbacks(result)
}

/*
 * For hosted jobs, we use callbacks:
 * 1. Send finished callback and log job_finished event
 * 2. Wait for archivator to collect all the logs
 * 3. Send teardown_finished callback and close the logger
 */
func (job *Job) teardownWithCallbacks(result string, callbackRetryAttempts int) error {
	err := job.SendFinishedCallback(result, callbackRetryAttempts)
	if err != nil {
		log.Errorf("Could not send finished callback: %v", err)
		return err
	}

	job.Logger.LogJobFinished(result)
	log.Debug("Waiting for archivator")

	for {
		if job.JobLogArchived {
			break
		} else {
			time.Sleep(1000 * time.Millisecond)
		}
	}

	log.Debug("Archivator finished")
	err = job.Logger.Close()
	if err != nil {
		log.Errorf("Error closing logger: %+v", err)
	}

	err = job.SendTeardownFinishedCallback(callbackRetryAttempts)
	if err != nil {
		log.Errorf("Could not send teardown finished callback: %v", err)
		return err
	}

	log.Info("Job teardown finished")
	return nil
}

/*
 * For self-hosted jobs, we don't use callbacks.
 * The only thing we need to do is log the job_finished event and close the logger.
 */
func (job *Job) teardownWithNoCallbacks(result string) error {
	job.Logger.LogJobFinished(result)

	// The job already finished, but executor is still open.
	// We use the open executor to upload the job logs as an artifact,
	// in case it has been trimmed during streaming.
	err := job.Logger.CloseWithOptions(eventlogger.CloseOptions{
		OnTrimmedLogs: job.uploadLogsAsArtifact,
	})

	if err != nil {
		log.Errorf("Error closing logger: %+v", err)
	}

	log.Info("Job teardown finished")
	return nil
}

func (job *Job) uploadLogsAsArtifact(filePath string) {
	if !job.UploadTrimmedLogs {
		log.Infof("Logs were trimmed, but agent is not configured to upload them as artifact - skipping.")
		return
	}

	log.Infof("Uploading job logs as artifact...")
	cmd := []string{"artifact", "push", "job", filePath, "-d", "logs.json"}
	exitCode := job.Executor.RunCommand(strings.Join(cmd, " "), true, "")
	if exitCode != 0 {
		log.Errorf("Error uploading job logs as job artifact")
		return
	}

	log.Info("Successfully uploaded job logs as a job artifact.")
}

func (job *Job) Stop() {
	log.Info("Stopping job")

	job.Stopped = true

	log.Debug("Invoking process stopping")

	PreventPanicPropagation(func() {
		job.Executor.Stop()
	})
}

func (job *Job) SendFinishedCallback(result string, retries int) error {
	payload := fmt.Sprintf(`{"result": "%s"}`, result)
	log.Infof("Sending finished callback: %+v", payload)
	return retry.RetryWithConstantWait(retry.RetryOptions{
		Task:                 "Send finished callback",
		MaxAttempts:          retries,
		DelayBetweenAttempts: time.Second,
		Fn: func() error {
			return job.SendCallback(job.Request.Callbacks.Finished, payload)
		},
	})
}

func (job *Job) SendTeardownFinishedCallback(retries int) error {
	log.Info("Sending teardown finished callback")
	return retry.RetryWithConstantWait(retry.RetryOptions{
		Task:                 "Send teardown finished callback",
		MaxAttempts:          retries,
		DelayBetweenAttempts: time.Second,
		Fn: func() error {
			return job.SendCallback(job.Request.Callbacks.TeardownFinished, "{}")
		},
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

	if !httputils.IsSuccessfulCode(response.StatusCode) {
		return fmt.Errorf("callback to %s got HTTP %d", url, response.StatusCode)
	}

	return nil
}
