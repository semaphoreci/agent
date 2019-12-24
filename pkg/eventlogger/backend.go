package eventlogger

type Backend interface {
	Open() error
	Write(interface{}) error
	Close() error
}

var _ Backend = (*FileBackend)(nil)
var _ Backend = (*InMemoryBackend)(nil)
