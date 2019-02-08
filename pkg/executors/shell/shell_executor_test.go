package shell

import (
	"testing"

	assert "github.com/stretchr/testify/assert"
)

func TestHelloWorld(t *testing.T) {
	events := []string{}

	eventHandler := func(event interface{}) {
		events = append(events, event.(string))
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
