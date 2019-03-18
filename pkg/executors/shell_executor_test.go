package executors

import (
	"fmt"
	"log"
	"testing"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	assert "github.com/stretchr/testify/assert"
)

func Test__ShellExecutor(t *testing.T) {
	events := []string{}

	eventHandler := func(event interface{}) {
		log.Printf("[TEST] %+v", event)

		switch e := event.(type) {
		case *CommandStartedEvent:
			events = append(events, e.Directive)
		case *CommandOutputEvent:
			events = append(events, e.Output)
		case *CommandFinishedEvent:
			events = append(events, fmt.Sprintf("Exit Code: %d", e.ExitCode))
		default:
			fmt.Printf("Shell Event %+v\n", e)
			panic("Unknown shell event")
		}
	}

	e := NewShellExecutor()

	e.Prepare()
	e.Start(eventHandler)

	e.RunCommand("echo 'here'", eventHandler)

	multilineCmd := `
	  if [ -d /etc ]; then
	    echo 'etc exists, multiline huzzahh!'
	  fi
	`
	e.RunCommand(multilineCmd, eventHandler)

	envVars := []api.EnvVar{
		api.EnvVar{Name: "A", Value: "Zm9vCg=="},
	}

	e.ExportEnvVars(envVars, eventHandler)
	e.RunCommand("echo $A", eventHandler)

	files := []api.File{
		api.File{
			Path:    "/tmp/random-file.txt",
			Content: "YWFhYmJiCgo=",
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
		"here\n",
		"Exit Code: 0",

		multilineCmd,
		"etc exists, multiline huzzahh!\n",
		"Exit Code: 0",

		"Exporting environment variables",
		"Exporting A\n",
		"Exit Code: 0",

		"echo $A",
		"foo\n",
		"Exit Code: 0",

		"Injecting Files",
		"Injecting /tmp/random-file.txt with file mode 0600\n",
		"Exit Code: 0",

		"cat /tmp/random-file.txt",
		"aaabbb\n",
		"\n",
		"Exit Code: 0",

		"echo $?",
		"0\n",
		"Exit Code: 0",
	})
}

func Test__ShellExecutor__StopingRunningJob(t *testing.T) {
	events := []string{}

	eventHandler := func(event interface{}) {
		log.Printf("[TEST] %+v", event)

		switch e := event.(type) {
		case *CommandStartedEvent:
			events = append(events, e.Directive)
		case *CommandOutputEvent:
			events = append(events, e.Output)
		case *CommandFinishedEvent:
			events = append(events, fmt.Sprintf("Exit Code: %d", e.ExitCode))
		default:
			fmt.Printf("Shell Event %+v\n", e)
			panic("Unknown shell event")
		}
	}

	e := NewShellExecutor()

	e.Prepare()
	e.Start(eventHandler)

	go func() {
		e.RunCommand("echo 'here'", eventHandler)
		e.RunCommand("sleep 5", eventHandler)
	}()

	time.Sleep(1 * time.Second)

	e.Stop()
	e.Cleanup()

	time.Sleep(1 * time.Second)

	assert.Equal(t, events, []string{
		"echo 'here'",
		"here\n",
		"Exit Code: 0",

		"sleep 5",
		"Exit Code: 1",
	})
}

func Test__ShellExecutor__LargeCommandOutput(t *testing.T) {
	events := []string{}

	eventHandler := func(event interface{}) {
		log.Printf("[TEST] %+v", event)

		switch e := event.(type) {
		case *CommandStartedEvent:
			events = append(events, e.Directive)
		case *CommandOutputEvent:
			events = append(events, e.Output)
		case *CommandFinishedEvent:
			events = append(events, fmt.Sprintf("Exit Code: %d", e.ExitCode))
		default:
			fmt.Printf("Shell Event %+v\n", e)
			panic("Unknown shell event")
		}
	}

	e := NewShellExecutor()

	e.Prepare()
	e.Start(eventHandler)

	go func() {
		e.RunCommand("for i in {1..100}; { printf 'hello'; }", eventHandler)
		e.RunCommand("sleep 5", eventHandler)
	}()

	time.Sleep(3 * time.Second)

	e.Stop()
	e.Cleanup()

	time.Sleep(1 * time.Second)

	assert.Equal(t, events, []string{
		"for i in {1..100}; { printf 'hello'; }",
		"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello\n",
		"Exit Code: 0",

		"sleep 5",
		"Exit Code: 1",
	})
}
