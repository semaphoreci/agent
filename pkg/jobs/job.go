package jobs

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	executors "github.com/semaphoreci/agent/pkg/executors"
	httputils "github.com/semaphoreci/agent/pkg/httputils"
	"github.com/semaphoreci/agent/pkg/kubernetes"
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

	JobLogArchived bool
	Stopped        bool
	Finished       bool
	UploadJobLogs  string
}

type JobOptions struct {
	Request                          *api.JobRequest
	Client                           *http.Client
	Logger                           *eventlogger.Logger
	ExposeKvmDevice                  bool
	FileInjections                   []config.FileInjection
	FailOnMissingFiles               bool
	SelfHosted                       bool
	UseKubernetesExecutor            bool
	KubernetesDefaultImage           string
	KubernetesImagePullPolicy        string
	KubernetesImagePullSecrets       []string
	KubernetesPodStartTimeoutSeconds int
	UploadJobLogs                    string
	RefreshTokenFn                   func() (string, error)
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
		Client:         options.Client,
		Request:        options.Request,
		JobLogArchived: false,
		Stopped:        false,
		UploadJobLogs:  options.UploadJobLogs,
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
	if jobOptions.UseKubernetesExecutor {
		// The downwards API allows the namespace to be exposed
		// to the agent container through an environment variable.
		// See: https://kubernetes.io/docs/tasks/inject-data-application/environment-variable-expose-pod-information.
		namespace := os.Getenv("KUBERNETES_NAMESPACE")
		if namespace == "" {
			namespace = "default"
		}

		return executors.NewKubernetesExecutor(request, logger, kubernetes.Config{
			Namespace:          namespace,
			DefaultImage:       jobOptions.KubernetesDefaultImage,
			ImagePullPolicy:    jobOptions.KubernetesImagePullPolicy,
			ImagePullSecrets:   jobOptions.KubernetesImagePullSecrets,
			PodPollingAttempts: jobOptions.KubernetesPodStartTimeoutSeconds,
			PodPollingInterval: time.Second,
		})
	}

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
	PreJobHookPath        string
	FailOnPreJobHookError bool
	OnJobFinished         func(selfhostedapi.JobResult)
	CallbackRetryAttempts int
}

func (o *RunOptions) GetPreJobHookWarning() string {
	if o.PreJobHookPath == "" {
		return ""
	}

	if o.FailOnPreJobHookError {
		return `The agent is configured to fail the job if the pre-job hook fails.`
	}

	return `The agent is configured to proceed with the job even if the pre-job hook fails.`
}

func (o *RunOptions) GetPreJobHookCommand() string {

	/*
	 * If we are dealing with PowerShell, we make sure to just call the script directly,
	 * without creating a new powershell process. If we did, people would need to set
	 * $ErrorActionPreference to "STOP" in order for errors to propagate properly.
	 */
	if runtime.GOOS == "windows" {
		return o.PreJobHookPath
	}

	return fmt.Sprintf("bash %s", o.PreJobHookPath)
}

func (job *Job) Run() {
	job.RunWithOptions(RunOptions{
		EnvVars:               []config.HostEnvVar{},
		FileInjections:        []config.FileInjection{},
		PreJobHookPath:        "",
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
		result = job.RunRegularCommands(options)
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

func (job *Job) RunRegularCommands(options RunOptions) string {
	exitCode := job.Executor.ExportEnvVars(job.Request.EnvVars, options.EnvVars)
	if exitCode != 0 {
		log.Error("Failed to export env vars")

		return JobFailed
	}

	exitCode = job.Executor.InjectFiles(job.Request.Files)
	if exitCode != 0 {
		log.Error("Failed to inject files")

		return JobFailed
	}

	shouldProceed := job.runPreJobHook(options)
	if !shouldProceed {
		return JobFailed
	}

	if len(job.Request.Commands) == 0 {
		exitCode = 0
	} else {
		exitCode = job.RunCommandsUntilFirstFailure(job.Request.Commands)
	}

	if job.Stopped || exitCode == 130 {
		job.Stopped = true
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

func (job *Job) runPreJobHook(options RunOptions) bool {
	if options.PreJobHookPath == "" {
		log.Info("No pre-job hook configured.")
		return true
	}

	log.Infof("Executing pre-job hook at %s", options.PreJobHookPath)
	exitCode := job.Executor.RunCommandWithOptions(executors.CommandOptions{
		Command: options.GetPreJobHookCommand(),
		Silent:  false,
		Alias:   "Running the pre-job hook configured in the agent",
		Warning: options.GetPreJobHookWarning(),
	})

	if exitCode == 0 {
		log.Info("Pre-job hook executed successfully.")
		return true
	}

	if options.FailOnPreJobHookError {
		log.Error("Error executing pre-job hook - failing job")
		return false
	}

	log.Error("Error executing pre-job hook - proceeding")
	return true
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

	// The job already finished, but executor is still open.
	// We use the open executor to upload the job logs as
	// an artifact, in case it is above the acceptable limit.
	err = job.Logger.CloseWithOptions(eventlogger.CloseOptions{
		OnClose: job.uploadLogsAsArtifact,
	})

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
		OnClose: job.uploadLogsAsArtifact,
	})

	if err != nil {
		log.Errorf("Error closing logger: %+v", err)
	}

	log.Info("Job teardown finished")
	return nil
}

func (job *Job) uploadLogsAsArtifact(trimmed bool) {
	if job.UploadJobLogs == config.UploadJobLogsConditionNever {
		log.Infof("upload-job-logs=never - not uploading job logs as job artifact.")
		return
	}

	if job.UploadJobLogs == config.UploadJobLogsConditionWhenTrimmed && !trimmed {
		log.Infof("upload-job-logs=when-trimmed - logs were not trimmed, not uploading job logs as job artifact.")
		return
	}

	log.Infof("Uploading job logs as job artifact...")
	file, err := job.Logger.GeneratePlainTextFile()
	if err != nil {
		log.Errorf("Error converting '%s' to plain text: %v", file, err)
		return
	}

	defer os.Remove(file)

	cmd := []string{"artifact", "push", "job", file, "-d", "agent/job_logs.txt"}
	exitCode := job.Executor.RunCommand(strings.Join(cmd, " "), true, "")
	if exitCode != 0 {
		log.Errorf("Error uploading job logs as artifact")
		return
	}

	log.Info("Successfully uploaded job logs as artifact.")
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
