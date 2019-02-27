package executors

import (
	"fmt"

	api "github.com/semaphoreci/agent/pkg/api"
)

type Executor interface {
	Prepare() int
	Start() int
	ExportEnvVars([]api.EnvVar, EventHandler) int
	InjectFiles([]api.File, EventHandler) int
	RunCommand(string, EventHandler) int
	Stop() int
	Cleanup() int
}

const ExecutorTypeShell = "shell"
const ExecutorTypeDockerCompose = "dockercompose"

func CreateExecutor(request *api.JobRequest) (Executor, error) {
	switch request.Executor {
	case ExecutorTypeShell:
		return NewShellExecutor(), nil
	case ExecutorTypeDockerCompose:
		return NewDockerComposeExecutor(request.Compose), nil
	default:
		return nil, fmt.Errorf("Uknown executor type")
	}
}
