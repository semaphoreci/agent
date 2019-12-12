package executors

import (
	"fmt"

	api "github.com/semaphoreci/agent/pkg/api"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
)

type Executor interface {
	Prepare() int
	Start() int
	ExportEnvVars([]api.EnvVar) int
	InjectFiles([]api.File) int
	RunCommand(string) int
	Stop() int
	Cleanup() int
}

const ExecutorTypeShell = "shell"
const ExecutorTypeDockerCompose = "dockercompose"

func CreateExecutor(request *api.JobRequest, logger eventlogger.EventLogger) (Executor, error) {
	switch request.Executor {
	case ExecutorTypeShell:
		return NewShellExecutor(request, logger), nil
	case ExecutorTypeDockerCompose:
		return NewDockerComposeExecutor(request, logger), nil
	default:
		return nil, fmt.Errorf("Uknown executor type")
	}
}
