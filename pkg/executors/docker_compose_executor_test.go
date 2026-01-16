package executors

import (
	"encoding/base64"
	"os/exec"
	"runtime"
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
				{
					Name:  "main",
					Image: "ruby:2.6",
				},
				{
					Name:    "db",
					Image:   "postgres:9.6",
					Command: "postgres start",
					EnvVars: []api.EnvVar{
						{
							Name:  "FOO",
							Value: base64.StdEncoding.EncodeToString([]byte("BAR")),
						},
						{
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
	if runtime.GOOS == "windows" {
		t.Skip("docker-compose executor is not yet support in Windows")
	}

	e, _, testLoggerBackend := startComposeExecutor()

	e.RunCommand("echo 'here'", false, "")

	multilineCmd := `
	  if [ -d /etc ]; then
	    echo 'etc exists, multiline huzzahh!'
	  fi
	`
	e.RunCommand(multilineCmd, false, "")

	envVars := []api.EnvVar{
		{Name: "A", Value: "Zm9vCg=="},
	}

	e.ExportEnvVars(envVars, []config.HostEnvVar{})
	e.RunCommand("echo $A", false, "")

	files := []api.File{
		{
			Path:    "/tmp/random-file.txt",
			Content: "YWFhYmJiCgo=",
			Mode:    "0600",
		},
	}

	e.InjectFiles(files)
	e.RunCommand("cat /tmp/random-file.txt", false, "")

	e.RunCommand("echo $?", false, "")

	e.Stop()
	e.Cleanup()

	simplifiedLogEvents, err := testLoggerBackend.SimplifiedEventsWithoutDockerPull()
	assert.Nil(t, err)

	assert.Equal(t, simplifiedLogEvents, []string{
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
	if runtime.GOOS == "windows" {
		t.Skip("docker-compose executor is not yet support in Windows")
	}

	e, _, testLoggerBackend := startComposeExecutor()

	go func() {
		e.RunCommand("echo 'here'", false, "")
		e.RunCommand("sleep 5", false, "")
	}()

	time.Sleep(1 * time.Second)

	e.Stop()
	e.Cleanup()

	time.Sleep(1 * time.Second)

	simplifiedLogEvents, err := testLoggerBackend.SimplifiedEventsWithoutDockerPull()
	assert.Nil(t, err)

	assert.Equal(t, simplifiedLogEvents, []string{
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

func Test__DockerComposeExecutor__composeExecutableAndArgs(t *testing.T) {
	testCases := []struct {
		name         string
		version      string
		expectedExec string
		expectedArgs []string
	}{
		{
			name:         "v1 legacy standalone",
			version:      "1.29.2",
			expectedExec: "docker-compose",
			expectedArgs: []string{},
		},
		{
			name:         "v2 plugin",
			version:      "v2.20.0",
			expectedExec: "docker",
			expectedArgs: []string{"compose"},
		},
		{
			name:         "v2 plugin with build metadata",
			version:      "v2.21.0-desktop.1",
			expectedExec: "docker",
			expectedArgs: []string{"compose"},
		},
		{
			name:         "v3 plugin",
			version:      "v3.0.0",
			expectedExec: "docker",
			expectedArgs: []string{"compose"},
		},
		{
			name:         "v4 plugin",
			version:      "v4.0.0",
			expectedExec: "docker",
			expectedArgs: []string{"compose"},
		},
		{
			name:         "v5 plugin",
			version:      "v5.0.0",
			expectedExec: "docker",
			expectedArgs: []string{"compose"},
		},
		{
			name:         "v5 plugin with build metadata",
			version:      "v5.1.0-desktop.1",
			expectedExec: "docker",
			expectedArgs: []string{"compose"},
		},
		{
			name:         "future v10 plugin",
			version:      "v10.0.0",
			expectedExec: "docker",
			expectedArgs: []string{"compose"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e := &DockerComposeExecutor{
				dockerComposeVersion: tc.version,
			}

			exec, args := e.composeExecutableAndArgs()
			assert.Equal(t, tc.expectedExec, exec)
			assert.Equal(t, tc.expectedArgs, args)
		})
	}
}
