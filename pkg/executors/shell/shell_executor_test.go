package shell

import (
	"fmt"
	"testing"
)

func TestHelloWorld(t *testing.T) {
	e := NewShellExecutor()

	e.Prepare()
	e.Start()

	e.RunCommand("echo 'here'", func(event interface{}) {
		fmt.Printf("%+v\n", event)
	})

	e.Stop()
	e.Cleanup()
}
