package executors

import (
	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
)

type Executor interface {
	Prepare() int
	Start() int
	ExportEnvVars([]api.EnvVar, []config.HostEnvVar) int
	InjectFiles([]api.File) int
	RunCommand(string, bool, string, []api.EnvVar) int
	Stop() int
	Cleanup() int
}

const ExecutorTypeShell = "shell"
const ExecutorTypeDockerCompose = "dockercompose"
