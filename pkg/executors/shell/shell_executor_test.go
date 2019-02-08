package shell

import (
	"fmt"
	"testing"

	executors "github.com/semaphoreci/agent/pkg/executors"
	assert "github.com/stretchr/testify/assert"
)

func TestHelloWorld(t *testing.T) {
	events := []string{}

	eventHandler := func(event interface{}) {
		switch e := event.(type) {
		case executors.CommandStartedEvent:
			events = append(events, e.Directive)
		case executors.CommandOutputEvent:
			events = append(events, e.Output)
		case executors.CommandFinishedEvent:
			events = append(events, fmt.Sprintf("Exit Code: %d", e.ExitCode))
		default:
			panic("Unknown shell event")
		}
	}

	e := NewShellExecutor()

	e.Prepare()
	e.Start()

	e.RunCommand("echo 'here'", eventHandler)

	multilineCmd := `
	  if [ -d /etc ]; then
	    echo 'etc exists, multiline huzzahh!'
	  fi
	`
	e.RunCommand(multilineCmd, eventHandler)

	e.InjectFile("/tmp/random-file.txt", "aaabbb\n", "0600", eventHandler)

	e.RunCommand("cat /tmp/random-file.txt", eventHandler)

	e.RunCommand("echo $?", eventHandler)

	e.Stop()
	e.Cleanup()

	assert.Equal(t, events, []string{
		"here",
		"Exit Status: 0",

		"etc exists, multiline huzzahh!",
		"Exit Status: 0",

		"Injecting File /tmp/random-file.txt with file mode 0600",
		"Exit Status: 0",

		"aaabbb",
		"Exit Status: 0",

		"0",
		"Exit Status: 0",
	})
}
