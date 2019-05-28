package executors

import (
	"encoding/base64"
	"fmt"
	"log"
	"testing"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	assert "github.com/stretchr/testify/assert"
)

func request() *api.JobRequest {
	return &api.JobRequest{
		Compose: api.Compose{
			Containers: []api.Container{
				api.Container{
					Name:  "main",
					Image: "ruby:2.6",
				},
				api.Container{
					Name:    "db",
					Image:   "postgres:9.6",
					Command: "postgres start",
					EnvVars: []api.EnvVar{
						api.EnvVar{
							Name:  "FOO",
							Value: "BAR",
						},
						api.EnvVar{
							Name:  "FAZ",
							Value: "ZEZ",
						},
					},
				},
			},
		},

		SSHPublicKeys: []api.PublicKey{
			api.PublicKey(base64.StdEncoding.EncodeToString([]byte("ssh-rsa aaaaa"))),
		},
	}

}

func Test__DockerComposeExecutor(t *testing.T) {
	events := []string{}

	eventHandler := func(event interface{}) {
		log.Printf("%+v", event)

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

	e := NewDockerComposeExecutor(request())

	e.Prepare()
	e.Start(DevNullEventHandler)

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

func Test__DockerComposeExecutor__StopingRunningJob(t *testing.T) {
	events := []string{}

	eventHandler := func(event interface{}) {
		log.Printf("%+v", event)

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

	e := NewDockerComposeExecutor(request())

	e.Prepare()
	e.Start(DevNullEventHandler)

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
