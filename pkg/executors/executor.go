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
	RunCommand(string, bool, string) int
	RunCommandWithOptions(options CommandOptions) int
	Stop() int
	Cleanup() int
}

type CommandOptions struct {
	Command string
	Silent  bool
	Alias   string
	Warning string
}

const ExecutorTypeShell = "shell"
const ExecutorTypeDockerCompose = "dockercompose"
const ExecutorKubernetes = "kubernetes"
