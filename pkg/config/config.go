package config

import "os"

const (
	CONFIG_FILE           = "config-file"
	ENDPOINT              = "endpoint"
	TOKEN                 = "token"
	NO_HTTPS              = "no-https"
	SHUTDOWN_HOOK_PATH    = "shutdown-hook-path"
	DISCONNECT_AFTER_JOB  = "disconnect-after-job"
	ENV_VARS              = "env-vars"
	FILES                 = "files"
	FAIL_ON_MISSING_FILES = "fail-on-missing-files"
)

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
