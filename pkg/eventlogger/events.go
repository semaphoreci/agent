package eventlogger

type JobStartedEvent struct {
	Event     string
	Timestamp int
}

type JobFinishedEvent struct {
	Event     string
	Timestamp int
	Result    string
}

type CommandStartedEvent struct {
	Event     string
	Timestamp int
	Directive string
}

type CommandOutputEvent struct {
	Event     string
	Timestamp int
	Output    string
}

type CommandFinishedEvent struct {
	Event      string
	Timestamp  int
	Directive  string
	ExitCode   int
	StartedAt  int
	FinishedAt int
}
