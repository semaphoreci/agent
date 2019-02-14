package executors

import (
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
