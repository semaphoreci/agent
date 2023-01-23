package config

import "os"

const (
	ConfigFile                 = "config-file"
	Name                       = "name"
	Endpoint                   = "endpoint"
	Token                      = "token"
	NoHTTPS                    = "no-https"
	ShutdownHookPath           = "shutdown-hook-path"
	PreJobHookPath             = "pre-job-hook-path"
	DisconnectAfterJob         = "disconnect-after-job"
	DisconnectAfterIdleTimeout = "disconnect-after-idle-timeout"
	EnvVars                    = "env-vars"
	Files                      = "files"
	FailOnMissingFiles         = "fail-on-missing-files"
	UploadJobLogs              = "upload-job-logs"
	FailOnPreJobHookError      = "fail-on-pre-job-hook-error"
	KubernetesExecutor         = "kubernetes-executor"
)

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
	Endpoint,
	Token,
	NoHTTPS,
	ShutdownHookPath,
	PreJobHookPath,
	DisconnectAfterJob,
	DisconnectAfterIdleTimeout,
	EnvVars,
	Files,
	FailOnMissingFiles,
	UploadJobLogs,
	FailOnPreJobHookError,
	KubernetesExecutor,
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
