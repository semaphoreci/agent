package config

import "os"

const (
	ConfigFile                 = "config-file"
	Name                       = "name"
	NameFromEnv                = "name-from-env"
	Endpoint                   = "endpoint"
	Token                      = "token"
	NoHTTPS                    = "no-https"
	ShutdownHookPath           = "shutdown-hook-path"
	PreJobHookPath             = "pre-job-hook-path"
	PostJobHookPath            = "post-job-hook-path"
	DisconnectAfterJob         = "disconnect-after-job"
	JobID                      = "job-id"
	DisconnectAfterIdleTimeout = "disconnect-after-idle-timeout"
	EnvVars                    = "env-vars"
	Files                      = "files"
	ExposeKvmDevice            = "expose-kvm-device"
	FailOnMissingFiles         = "fail-on-missing-files"
	UploadJobLogs              = "upload-job-logs"
	FailOnPreJobHookError      = "fail-on-pre-job-hook-error"
	SourcePreJobHook           = "source-pre-job-hook"
	InterruptionGracePeriod    = "interruption-grace-period"
	KubernetesExecutor         = "kubernetes-executor"
	KubernetesPodSpec          = "kubernetes-pod-spec"
	KubernetesAllowedImages    = "kubernetes-allowed-images"
	KubernetesPodStartTimeout  = "kubernetes-pod-start-timeout"
	KubernetesLabels           = "kubernetes-labels"
	KubernetesDefaultImage     = "kubernetes-default-image"

	KubernetesExecutionStrategy = "kubernetes-execution-strategy"
)

const DefaultKubernetesPodStartTimeout = 300

// The Kubernetes executor can run job commands inside the job pod in two ways:
//
//   - "exec" (default): the agent runs `kubectl exec` to spawn a new shell bound
//     to the streaming connection. If that connection is interrupted (e.g. a
//     Konnectivity tunnel reset during cluster scaling), the exec'd process is
//     killed and the job fails.
//   - "attach": the main container runs a long-lived login shell as PID 1, and
//     the agent `kubectl attach`es to it. The shell (and any running command) is
//     not bound to the connection, so it survives a dropped connection - the
//     prerequisite for re-attaching instead of failing the job.
const (
	KubernetesExecutionStrategyExec   = "exec"
	KubernetesExecutionStrategyAttach = "attach"
)

var ValidKubernetesExecutionStrategies = []string{
	KubernetesExecutionStrategyExec,
	KubernetesExecutionStrategyAttach,
}

type ImagePullPolicy string

const (
	ImagePullPolicyNever        = "Never"
	ImagePullPolicyAlways       = "Always"
	ImagePullPolicyIfNotPresent = "IfNotPresent"
)

var ValidImagePullPolicies = []string{
	ImagePullPolicyNever,
	ImagePullPolicyAlways,
	ImagePullPolicyIfNotPresent,
}

type UploadJobLogsCondition string

const (
	UploadJobLogsConditionNever       = "never"
	UploadJobLogsConditionAlways      = "always"
	UploadJobLogsConditionWhenTrimmed = "when-trimmed"
)

var ValidUploadJobLogsCondition = []string{
	UploadJobLogsConditionNever,
	UploadJobLogsConditionAlways,
	UploadJobLogsConditionWhenTrimmed,
}

var ValidConfigKeys = []string{
	ConfigFile,
	Name,
	NameFromEnv,
	Endpoint,
	Token,
	NoHTTPS,
	ShutdownHookPath,
	PreJobHookPath,
	PostJobHookPath,
	DisconnectAfterJob,
	JobID,
	DisconnectAfterIdleTimeout,
	EnvVars,
	Files,
	FailOnMissingFiles,
	UploadJobLogs,
	FailOnPreJobHookError,
	SourcePreJobHook,
	InterruptionGracePeriod,
	KubernetesExecutor,
	KubernetesPodSpec,
	KubernetesAllowedImages,
	KubernetesPodStartTimeout,
	KubernetesLabels,
	KubernetesDefaultImage,
	KubernetesExecutionStrategy,
}

type HostEnvVar struct {
	Name  string
	Value string
}

type FileInjection struct {
	HostPath    string
	Destination string
}

func (f *FileInjection) CheckFileExists() error {
	_, err := os.Stat(f.HostPath)
	if err != nil {
		return err
	}

	return nil
}
