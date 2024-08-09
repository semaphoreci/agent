package eventlogger

import "io"

type Backend interface {
	Open() error
	Write(interface{}) error
	Read(startFrom, maxLines int, writer io.Writer) (int, error)
	Iterate(fn func(event []byte) error) error
	Close() error
	CloseWithOptions(CloseOptions) error
}

type CloseOptions struct {
	OnClose func(bool)
}

var _ Backend = (*FileBackend)(nil)
var _ Backend = (*HTTPBackend)(nil)
var _ Backend = (*InMemoryBackend)(nil)
