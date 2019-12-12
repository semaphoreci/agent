package eventlogger

type Backend interface {
	Open() error
	Write(interface{}) error
	Read(from, to int) []string
	Close() error
}
