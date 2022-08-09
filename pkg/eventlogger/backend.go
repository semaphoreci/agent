package eventlogger

type Backend interface {
	Open() error
	Write(interface{}) error
	Close() error
	CloseWithOptions(CloseOptions) error
}

type CloseOptions struct {
	OnTrimmedLogs func(string)
}

var _ Backend = (*FileBackend)(nil)
var _ Backend = (*InMemoryBackend)(nil)
