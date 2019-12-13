package eventlogger

type JobStartedEvent struct {
	Event     string `json:"event"`
	Timestamp int    `json:"timestamp"`
}

type JobFinishedEvent struct {
	Event     string `json:"event"`
	Timestamp int    `json:"timestamp"`
	Result    string `json:"result"`
}

type CommandStartedEvent struct {
	Event     string `json:"event"`
	Timestamp int    `json:"timestamp"`
	Directive string `json:"directive"`
}

type CommandOutputEvent struct {
	Event     string `json:"event"`
	Timestamp int    `json:"timestamp"`
	Output    string `json:"output"`
}

type CommandFinishedEvent struct {
	Event     string `json:"event"`
	Timestamp int    `json:"timestamp"`

	Directive  string `json:"directive"`
	ExitCode   int    `json:"exit_code"`
	StartedAt  int    `json:"started_at"`
	FinishedAt int    `json:"finished_at"`
}
