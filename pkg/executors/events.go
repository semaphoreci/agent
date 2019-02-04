package executors

type EventHandler func(interface{})

type CommandStartedShellEvent struct {
	Timestamp    int
	CommandIndex int
	Directive    string
}

type CommandOutputShellEvent struct {
	Timestamp    int
	CommandIndex int
	Output       string
}

type CommandFinishedShellEvent struct {
	Timestamp    int
	CommandIndex int
	ExitCode     int
	Directive    string
	StartedAt    int
	FinishedAt   int
}
