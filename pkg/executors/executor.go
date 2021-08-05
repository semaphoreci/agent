package executors

import (
	"fmt"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
)

type Executor interface {
	Prepare() int
	Start() int
	ExportEnvVars([]api.EnvVar, []config.HostEnvVar) int
	InjectFiles([]api.File) int
	RunCommand(string, bool, string) int
	Stop() int
	Cleanup() int
}

const ExecutorTypeShell = "shell"
const ExecutorTypeDockerCompose = "dockercompose"

func CreateExecutor(request *api.JobRequest, logger *eventlogger.Logger, exposeKvmDevice bool, fileInjections []config.FileInjection) (Executor, error) {
	switch request.Executor {
	case ExecutorTypeShell:
		return NewShellExecutor(request, logger), nil
	case ExecutorTypeDockerCompose:
		return NewDockerComposeExecutor(request, logger, exposeKvmDevice, fileInjections), nil
	default:
		return nil, fmt.Errorf("unknown executor type")
	}
}
