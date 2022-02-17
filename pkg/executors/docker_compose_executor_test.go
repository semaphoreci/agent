package executors

import (
	"encoding/base64"
	"os/exec"
	"testing"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	assert "github.com/stretchr/testify/assert"
)

func startComposeExecutor() (*DockerComposeExecutor, *eventlogger.Logger, *eventlogger.InMemoryBackend) {
	request := &api.JobRequest{
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
							Value: base64.StdEncoding.EncodeToString([]byte("BAR")),
						},
						api.EnvVar{
							Name:  "FAZ",
							Value: base64.StdEncoding.EncodeToString([]byte("ZEZ")),
						},
					},
				},
			},
		},

		SSHPublicKeys: []api.PublicKey{
			api.PublicKey(base64.StdEncoding.EncodeToString([]byte("ssh-rsa aaaaa"))),
		},
	}

	// kill all existing container
	cmd := exec.Command("/bin/bash", "-c", "docker stop $(docker ps -q); docker rm $(docker ps -qa)")
	cmd.Run()

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()

	e := NewDockerComposeExecutor(request, testLogger, DockerComposeExecutorOptions{
		ExposeKvmDevice: true,
		FileInjections:  []config.FileInjection{},
	})

	if code := e.Prepare(); code != 0 {
		panic("Prapare failed")
	}

	if code := e.Start(); code != 0 {
		panic("Start failed")
	}

	return e, testLogger, testLoggerBackend
}

func Test__DockerComposeExecutor(t *testing.T) {
	e, _, testLoggerBackend := startComposeExecutor()

	e.RunCommand("echo 'here'", false, "", []api.EnvVar{})

	multilineCmd := `
	  if [ -d /etc ]; then
	    echo 'etc exists, multiline huzzahh!'
	  fi
	`
	e.RunCommand(multilineCmd, false, "", []api.EnvVar{})

	envVars := []api.EnvVar{
		api.EnvVar{Name: "A", Value: "Zm9vCg=="},
	}

	e.ExportEnvVars(envVars, []config.HostEnvVar{})
	e.RunCommand("echo $A", false, "", []api.EnvVar{})

	files := []api.File{
		api.File{
			Path:    "/tmp/random-file.txt",
			Content: "YWFhYmJiCgo=",
			Mode:    "0600",
		},
	}

	e.InjectFiles(files)
	e.RunCommand("cat /tmp/random-file.txt", false, "", []api.EnvVar{})

	e.RunCommand("echo $?", false, "", []api.EnvVar{})

	e.Stop()
	e.Cleanup()

	assert.Equal(t, testLoggerBackend.SimplifiedEventsWithoutDockerPull(), []string{
		"directive: Pulling docker images...",
		"Exit Code: 0",

		"directive: Starting the docker image...",
		"Starting a new bash session.\n",
		"Exit Code: 0",

		"directive: echo 'here'",
		"here\n",
		"Exit Code: 0",

		"directive: " + multilineCmd,
		"etc exists, multiline huzzahh!\n",
		"Exit Code: 0",

		"directive: Exporting environment variables",
		"Exporting A\n",
		"Exit Code: 0",

		"directive: echo $A",
		"foo\n",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Injecting /tmp/random-file.txt with file mode 0600\n",
		"Exit Code: 0",

		"directive: cat /tmp/random-file.txt",
		"aaabbb\n\n",
		"Exit Code: 0",

		"directive: echo $?",
		"0\n",
		"Exit Code: 0",
	})
}

func Test__DockerComposeExecutor__StopingRunningJob(t *testing.T) {
	e, _, testLoggerBackend := startComposeExecutor()

	go func() {
		e.RunCommand("echo 'here'", false, "", []api.EnvVar{})
		e.RunCommand("sleep 5", false, "", []api.EnvVar{})
	}()

	time.Sleep(1 * time.Second)

	e.Stop()
	e.Cleanup()

	time.Sleep(1 * time.Second)

	assert.Equal(t, testLoggerBackend.SimplifiedEventsWithoutDockerPull(), []string{
		"directive: Pulling docker images...",
		"Exit Code: 0",

		"directive: Starting the docker image...",
		"Starting a new bash session.\n",
		"Exit Code: 0",

		"directive: echo 'here'",
		"here\n",
		"Exit Code: 0",

		"directive: sleep 5",
		"Exit Code: 1",
	})
}
