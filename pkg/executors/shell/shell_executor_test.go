package shell

import (
	"fmt"
	"testing"

	api "github.com/semaphoreci/agent/pkg/api"
	executors "github.com/semaphoreci/agent/pkg/executors"
	assert "github.com/stretchr/testify/assert"
)

func Test__ShellExecutor(t *testing.T) {
	events := []string{}

	eventHandler := func(event interface{}) {
		switch e := event.(type) {
		case *executors.CommandStartedEvent:
			events = append(events, e.Directive)
		case *executors.CommandOutputEvent:
			events = append(events, e.Output)
		case *executors.CommandFinishedEvent:
			events = append(events, fmt.Sprintf("Exit Code: %d", e.ExitCode))
		default:
			fmt.Printf("%+v", e)
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

	envVars := []api.EnvVar{
		api.EnvVar{Name: "A", Value: "foo"},
	}

	e.ExportEnvVars(envVars, eventHandler)
	e.RunCommand("echo $A", eventHandler)

	files := []api.File{
		api.File{
			Path:    "/tmp/random-file.txt",
			Content: "aaabbb\n",
			Mode:    "0600",
		},
	}

	e.InjectFiles(files, eventHandler)
	e.RunCommand("cat /tmp/random-file.txt", eventHandler)

	e.RunCommand("echo $?", eventHandler)

	e.Stop()
	e.Cleanup()

	assert.Equal(t, events, []string{
		"echo 'here'",
		"here",
		"Exit Code: 0",

		multilineCmd,
		"etc exists, multiline huzzahh!",
		"Exit Code: 0",

		"Exporting environment variables",
		"Exporting A",
		"Exit Code: 0",

		"echo $A",
		"foo",
		"Exit Code: 0",

		"Injecting File /tmp/random-file.txt with file mode 0600",
		"Exit Code: 0",

		"cat /tmp/random-file.txt",
		"aaabbb",
		"Exit Code: 0",

		"echo $?",
		"0",
		"Exit Code: 0",
	})
}
