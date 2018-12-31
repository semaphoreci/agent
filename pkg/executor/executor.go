package executor

import "os"

type Executor interface {
	Build() error
	Setup() error

	AddFile(path string, content string) error
	ExportEnvVar(name string, value string) error

	Run(command string) (*os.File, error)
}
