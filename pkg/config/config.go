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
	RedactRegexes              = "redact-regexes"
	RedactEnvVars              = "redact-env-vars"
	KubernetesExecutor         = "kubernetes-executor"
	KubernetesPodSpec          = "kubernetes-pod-spec"
	KubernetesAllowedImages    = "kubernetes-allowed-images"
	KubernetesPodStartTimeout  = "kubernetes-pod-start-timeout"
	KubernetesLabels           = "kubernetes-labels"
	KubernetesDefaultImage     = "kubernetes-default-image"
)

const DefaultKubernetesPodStartTimeout = 300

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
	RedactRegexes,
	RedactEnvVars,
	KubernetesExecutor,
	KubernetesPodSpec,
	KubernetesAllowedImages,
	KubernetesPodStartTimeout,
	KubernetesLabels,
	KubernetesDefaultImage,
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
