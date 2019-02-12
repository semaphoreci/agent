package executors

type Executor interface {
	Prepare()
	Start() error
	ExportEnvVars([]EnvVar, EventHandler)
	InjectFiles([]File, EventHandler)
	RunCommand(string, EventHandler)
	Stop()
	Cleanup()
}
