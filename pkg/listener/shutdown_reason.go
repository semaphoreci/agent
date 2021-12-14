package listener

type ShutdownReason int64

const (
	ShutdownReasonIdle ShutdownReason = iota
	ShutdownReasonJobFinished
	ShutdownReasonUnableToSync
	ShutdownReasonRequested
	ShutdownReasonInterrupted
)

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
