package listener

import "github.com/semaphoreci/agent/pkg/listener/selfhostedapi"

type ShutdownReason int64

const (

	// When the agent shuts down due to these reasons,
	// the Semaphore API requested it to do so.
	ShutdownReasonIdle ShutdownReason = iota
	ShutdownReasonJobFinished
	ShutdownReasonRequested
	ShutdownReasonUnknown

	// When the agent shuts down due to these reasons,
	// the agent decides to do so.
	ShutdownReasonUnableToSync
	ShutdownReasonInterrupted
)

func ShutdownReasonFromAPI(reasonFromAPI selfhostedapi.ShutdownReason) ShutdownReason {
	switch reasonFromAPI {
	case selfhostedapi.ShutdownReasonIdle:
		return ShutdownReasonIdle
	case selfhostedapi.ShutdownReasonJobFinished:
		return ShutdownReasonJobFinished
	case selfhostedapi.ShutdownReasonRequested:
		return ShutdownReasonRequested
	}

	return ShutdownReasonUnknown
}

func (s ShutdownReason) String() string {
	switch s {
	case ShutdownReasonIdle:
		return "IDLE"
	case ShutdownReasonJobFinished:
		return "JOB_FINISHED"
	case ShutdownReasonUnableToSync:
		return "UNABLE_TO_SYNC"
	case ShutdownReasonRequested:
		return "REQUESTED"
	case ShutdownReasonInterrupted:
		return "INTERRUPTED"
	}
	return "UNKNOWN"
}
