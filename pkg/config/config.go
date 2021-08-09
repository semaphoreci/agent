package config

import "os"

const (
	ConfigFile         = "config-file"
	Endpoint           = "endpoint"
	Token              = "token"
	NoHTTPS            = "no-https"
	ShutdownHookPath   = "shutdown-hook-path"
	DisconnectAfterJob = "disconnect-after-job"
	EnvVars            = "env-vars"
	Files              = "files"
	FailOnMissingFiles = "fail-on-missing-files"
)

var VALID_CONFIG_KEYS = []string{
	ConfigFile,
	Endpoint,
	Token,
	NoHTTPS,
	ShutdownHookPath,
	DisconnectAfterJob,
	EnvVars,
	Files,
	FailOnMissingFiles,
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
	if _, err := os.Stat(f.HostPath); err == nil {
		return nil
	} else {
		return err
	}
}
