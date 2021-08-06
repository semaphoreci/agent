package config

import "os"

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
