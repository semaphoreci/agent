package shell

import (
	"fmt"
	"testing"
	"time"
)

func TestHelloWorld(t *testing.T) {
	e := NewShellExecutor()

	e.Prepare()
	e.Start()

	e.RunCommand("echo 'here'", func(event interface{}) {
		fmt.Printf("%+v\n", event)
		time.Sleep(300000000)
	})

	e.Stop()
	e.Cleanup()
}
