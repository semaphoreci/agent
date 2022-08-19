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
	FailOnPreJobHookError      = "fail-on-pre-job-hook-error"
)

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
	FailOnPreJobHookError,
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
