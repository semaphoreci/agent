package executors

import (
	"encoding/base64"
	"os/exec"
	"testing"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
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

}

func killAllContainers() {
	cmd := exec.Command("/bin/bash", "-c", "docker stop $(docker ps -q) && docker rm $(docker ps -qa)")

	cmd.Run()
}

func Test__DockerComposeExecutor(t *testing.T) {
	killAllContainers()

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()

	e := NewDockerComposeExecutor(request(), testLogger)

	e.Prepare()
	e.Start()

	e.RunCommand("echo 'here'", false)

	multilineCmd := `
	  if [ -d /etc ]; then
	    echo 'etc exists, multiline huzzahh!'
	  fi
	`
	e.RunCommand(multilineCmd, false)

	envVars := []api.EnvVar{
		api.EnvVar{Name: "A", Value: "Zm9vCg=="},
	}

	e.ExportEnvVars(envVars)
	e.RunCommand("echo $A", false)

	files := []api.File{
		api.File{
			Path:    "/tmp/random-file.txt",
			Content: "YWFhYmJiCgo=",
			Mode:    "0600",
		},
	}

	e.InjectFiles(files)
	e.RunCommand("cat /tmp/random-file.txt", false)

	e.RunCommand("echo $?", false)

	e.Stop()
	e.Cleanup()

	assert.Equal(t, testLoggerBackend.SimplifiedEvents(), []string{
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
	killAllContainers()

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()

	e := NewDockerComposeExecutor(request(), testLogger)

	e.Prepare()
	e.Start()

	go func() {
		e.RunCommand("echo 'here'", false)
		e.RunCommand("sleep 5", false)
	}()

	time.Sleep(1 * time.Second)

	e.Stop()
	e.Cleanup()

	time.Sleep(1 * time.Second)

	assert.Equal(t, testLoggerBackend.SimplifiedEvents(), []string{
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
