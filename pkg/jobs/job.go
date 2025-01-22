package jobs

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/compression"
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

// By default, we will only compress the job logs before uploading them as an artifact
// if their size goes above 100MB. However, the SEMAPHORE_AGENT_LOGS_COMPRESSION_SIZE environment
// variable can be used to configure that value, with anything between 1MB and 1GB being possible.
const MinSizeForCompression = 1024 * 1024
const DefaultSizeForCompression = 1024 * 1024 * 100
const MaxSizeForCompression = 1024 * 1024 * 1024

type Job struct {
	Client  *http.Client
	Request *api.JobRequest
	Logger  *eventlogger.Logger

	Executor executors.Executor

	JobLogArchived bool
	Stopped        bool
	Finished       bool
	UploadJobLogs  string
	UserAgent      string
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
	PodSpecDecoratorConfigMap        string
	KubernetesPodStartTimeoutSeconds int
	KubernetesLabels                 map[string]string
	KubernetesImageValidator         *kubernetes.ImageValidator
	KubernetesDefaultImage           string
	UploadJobLogs                    string
	RefreshTokenFn                   func() (string, error)
	UserAgent                        string
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
		l, err := eventlogger.CreateLogger(eventlogger.LoggerOptions{
			Request:        options.Request,
			RefreshTokenFn: options.RefreshTokenFn,
			UserAgent:      options.UserAgent,
		})

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
			Namespace:                 namespace,
			ImageValidator:            jobOptions.KubernetesImageValidator,
			PodSpecDecoratorConfigMap: jobOptions.PodSpecDecoratorConfigMap,
			PodPollingAttempts:        jobOptions.KubernetesPodStartTimeoutSeconds,
			Labels:                    jobOptions.KubernetesLabels,
			PodPollingInterval:        time.Second,
			DefaultImage:              jobOptions.KubernetesDefaultImage,
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
	PreJobHookPath        string
	PostJobHookPath       string
	FailOnPreJobHookError bool
	SourcePreJobHook      bool
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

	/*
	 * This executes the pre-job hook without opening a new shell.
	 * That means that changes to the environment performed in the hook
	 * (current directory, environment variables, export commands)
	 * will be visible by the next job commands.
	 */
	if o.SourcePreJobHook {
		return fmt.Sprintf("source %s", o.PreJobHookPath)
	}

	return fmt.Sprintf("bash %s", o.PreJobHookPath)
}

func (o *RunOptions) GetPostJobHookCommand() string {

	/*
	 * If we are dealing with PowerShell, we make sure to just call the script directly,
	 * without creating a new powershell process. If we did, people would need to set
	 * $ErrorActionPreference to "STOP" in order for errors to propagate properly.
	 */
	if runtime.GOOS == "windows" {
		return o.PostJobHookPath
	}

	return fmt.Sprintf("bash %s", o.PostJobHookPath)
}

func (job *Job) Run() {
	job.RunWithOptions(RunOptions{
		EnvVars:               []config.HostEnvVar{},
		PreJobHookPath:        "",
		PostJobHookPath:       "",
		OnJobFinished:         nil,
		CallbackRetryAttempts: 60,
	})
}

func (job *Job) RunWithOptions(options RunOptions) {
	log.Infof("Running job %s", job.Request.JobID)
	executorRunning := false
	epiloguesExecuted := false
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

		if !job.Stopped {
			log.Debug("Handling epilogues")
			job.handleEpilogues(result)
			epiloguesExecuted = true
		}
	}

	// The post-job hook executes after the job's commands finished,
	// so they do not influence the job's result, just like the epilogues.
	job.runPostJobHook(options)

	result, err := job.Teardown(result, epiloguesExecuted, options.CallbackRetryAttempts)
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

func (job *Job) envVar(name string) string {
	if runtime.GOOS == "windows" {
		return "$env:" + name
	}

	return "$" + name
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

	// Job was stopped from UI or API
	if job.Stopped {
		log.Info("Regular commands were stopped")
		return JobStopped
	}

	// Job was stopped from the job itself.
	// Here, we need to know which job status to report.
	// We use the SEMAPHORE_JOB_RESULT environment variable for that.
	if exitCode == 130 {
		job.Stopped = true
		return job.handleStopExitCode()
	}

	if exitCode == 0 {
		log.Info("Regular commands finished successfully")
		return JobPassed
	}

	log.Info("Regular commands finished with failure")
	return JobFailed
}

func (job *Job) handleStopExitCode() string {
	directive := "Checking job result"
	job.Logger.LogCommandStarted(directive)
	defer func() {
		now := int(time.Now().Unix())
		job.Logger.LogCommandFinished(directive, 0, now, now)
	}()

	output, _ := job.Executor.GetOutputFromCommand("echo " + job.envVar("SEMAPHORE_JOB_RESULT"))
	status := strings.Trim(output, "\n")
	switch status {
	case "passed":
		job.Logger.LogCommandOutput("SEMAPHORE_JOB_RESULT=passed - stopping job and marking it as passed")
		return JobPassed
	case "failed":
		job.Logger.LogCommandOutput("SEMAPHORE_JOB_RESULT=failed - stopping job and marking it as failed")
		return JobFailed
	default:
		job.Logger.LogCommandOutput(fmt.Sprintf(
			"SEMAPHORE_JOB_RESULT is set to '%s' - stopping job and marking it as stopped", status),
		)
		return JobStopped
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

func (job *Job) runPostJobHook(options RunOptions) {
	if options.PostJobHookPath == "" {
		log.Info("No post-job hook configured.")
		return
	}

	log.Infof("Executing post-job hook at %s", options.PostJobHookPath)
	exitCode := job.Executor.RunCommandWithOptions(executors.CommandOptions{
		Command: options.GetPostJobHookCommand(),
		Silent:  false,
		Alias:   "Running the post-job hook configured in the agent",
	})

	if exitCode == 0 {
		log.Info("Post-job hook executed successfully.")
		return
	}

	log.Errorf("Error executing post-job hook - hook return exit code %d", exitCode)
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

func (job *Job) Teardown(result string, epiloguesExecuted bool, callbackRetryAttempts int) (string, error) {
	// if job was stopped during the epilogues, result should be stopped
	if epiloguesExecuted && job.Stopped {
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

func (job *Job) minSizeForCompression() int64 {
	fromEnv := os.Getenv("SEMAPHORE_AGENT_LOGS_COMPRESSION_SIZE")
	if fromEnv == "" {
		return DefaultSizeForCompression
	}

	n, err := strconv.ParseInt(fromEnv, 10, 64)
	if err != nil {
		log.Errorf(
			"Error parsing SEMAPHORE_AGENT_LOGS_COMPRESSION_SIZE: %v - using default of %d",
			err,
			DefaultSizeForCompression,
		)

		return DefaultSizeForCompression
	}

	if n < MinSizeForCompression || n > MaxSizeForCompression {
		log.Errorf(
			"Invalid SEMAPHORE_AGENT_LOGS_COMPRESSION_SIZE %d, not in range %d-%d, using default %d",
			n,
			MinSizeForCompression,
			MaxSizeForCompression,
			DefaultSizeForCompression,
		)

		return DefaultSizeForCompression
	}

	return n
}

// #nosec
func (job *Job) findFileSize(fileName string) (int64, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return 0, fmt.Errorf("error opening %s: %v", fileName, err)
	}

	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("error determining size for file %s: %v", fileName, err)
	}

	return fileInfo.Size(), nil
}

func (job *Job) prepareArtifactForUpload() (string, error) {
	log.Info("Converting job logs to plain-text format...")
	rawFileName, err := job.Logger.GeneratePlainTextFile()
	if err != nil {
		return "", fmt.Errorf("error converting '%s' to plain text: %v", rawFileName, err)
	}

	//
	// If an error happens determing the size of the raw logs file,
	// we should still try to upload the raw logs,
	// so we don't return an error here.
	//
	rawFileSize, err := job.findFileSize(rawFileName)
	if err != nil {
		log.Errorf("Error determining size for %s: %v", rawFileName, err)
		return rawFileName, nil
	}

	// If the size of the file is below our threshold for compression, we upload the raw file.
	minSizeForCompression := job.minSizeForCompression()
	if rawFileSize < minSizeForCompression {
		log.Infof("Logs are below the minimum size for compression - size=%d, minimum=%d", rawFileSize, minSizeForCompression)
		return rawFileName, nil
	}

	log.Info("Compressing job logs")

	//
	// If an error happens compressing the logs,
	// we should still try to upload the raw logs,
	// so we don't return an error here as well.
	//
	compressedFile, err := compression.Compress(rawFileName)
	if err != nil {
		log.Errorf("Error compressing job logs %s: %v - using raw file", rawFileName, err)
		return rawFileName, nil
	}

	// Remove the raw file since we are using the compressed one now.
	if err := os.Remove(rawFileName); err != nil {
		log.Errorf("Error removing file %s: %v", rawFileName, err)
	}

	return compressedFile, nil
}

func (job *Job) uploadLogsAsArtifact(trimmed bool) {
	if job.UploadJobLogs == config.UploadJobLogsConditionNever {
		log.Info("upload-job-logs=never - not uploading job logs as job artifact.")
		return
	}

	if job.UploadJobLogs == config.UploadJobLogsConditionWhenTrimmed && !trimmed {
		log.Info("upload-job-logs=when-trimmed - logs were not trimmed, not uploading job logs as job artifact.")
		return
	}

	token, err := job.Request.FindEnvVar("SEMAPHORE_ARTIFACT_TOKEN")
	if err != nil {
		log.Error("Error uploading job logs as artifact - no SEMAPHORE_ARTIFACT_TOKEN available")
		return
	}

	orgURL, err := job.Request.FindEnvVar("SEMAPHORE_ORGANIZATION_URL")
	if err != nil {
		log.Error("Error uploading job logs as artifact - no SEMAPHORE_ORGANIZATION_URL available")
		return
	}

	path, err := exec.LookPath("artifact")
	if err != nil {
		log.Error("Error uploading job logs as artifact - no artifact CLI available")
		return
	}

	file, err := job.prepareArtifactForUpload()
	if err != nil {
		log.Errorf("Error preparing artifact for upload: %v", err)
		return
	}

	args := []string{"push", "job", file, "-d", "agent/job_logs.txt"}

	// #nosec
	cmd := exec.Command(path, args...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "SEMAPHORE_ARTIFACT_TOKEN", token))
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "SEMAPHORE_JOB_ID", job.Request.JobID))
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "SEMAPHORE_ORGANIZATION_URL", orgURL))

	log.Info("Uploading job logs as artifact...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Error uploading job logs as artifact: %v, %s", err, output)
		return
	}

	log.Info("Successfully uploaded job logs as artifact")
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

	if job.Request.Callbacks.Token != "" {
		request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", job.Request.Callbacks.Token))
	}

	request.Header.Set("User-Agent", job.UserAgent)
	response, err := job.Client.Do(request)
	if err != nil {
		return err
	}

	if !httputils.IsSuccessfulCode(response.StatusCode) {
		return fmt.Errorf("callback to %s got HTTP %d", url, response.StatusCode)
	}

	return nil
}
