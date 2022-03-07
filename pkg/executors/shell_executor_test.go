package executors

import (
	"encoding/base64"
	"testing"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	testsupport "github.com/semaphoreci/agent/test/support"
	assert "github.com/stretchr/testify/assert"
)

func Test__ShellExecutor(t *testing.T) {
	testsupport.SetupTestLogs()

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()

	request := &api.JobRequest{
		SSHPublicKeys: []api.PublicKey{
			api.PublicKey(base64.StdEncoding.EncodeToString([]byte("ssh-rsa aaaaa"))),
		},
	}

	e := NewShellExecutor(request, testLogger, true)

	e.Prepare()
	e.Start()

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

	assert.Equal(t, testLoggerBackend.SimplifiedEvents(), []string{
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

func Test__ShellExecutor__StopingRunningJob(t *testing.T) {
	testsupport.SetupTestLogs()

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()

	request := &api.JobRequest{
		SSHPublicKeys: []api.PublicKey{
			api.PublicKey(base64.StdEncoding.EncodeToString([]byte("ssh-rsa aaaaa"))),
		},
	}

	e := NewShellExecutor(request, testLogger, true)

	e.Prepare()
	e.Start()

	go func() {
		e.RunCommand("echo 'here'", false, "")
		e.RunCommand("sleep 5", false, "")
	}()

	time.Sleep(1 * time.Second)

	e.Stop()
	e.Cleanup()

	time.Sleep(1 * time.Second)

	assert.Equal(t, testLoggerBackend.SimplifiedEvents()[0:4], []string{
		"directive: echo 'here'",
		"here\n",
		"Exit Code: 0",

		"directive: sleep 5",
	})
}

func Test__ShellExecutor__LargeCommandOutput(t *testing.T) {
	testsupport.SetupTestLogs()

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()

	request := &api.JobRequest{
		SSHPublicKeys: []api.PublicKey{
			api.PublicKey(base64.StdEncoding.EncodeToString([]byte("ssh-rsa aaaaa"))),
		},
	}

	e := NewShellExecutor(request, testLogger, true)

	e.Prepare()
	e.Start()

	go func() {
		e.RunCommand("for i in {1..100}; { printf 'hello'; }", false, "")
	}()

	time.Sleep(5 * time.Second)

	e.Stop()
	e.Cleanup()

	time.Sleep(1 * time.Second)

	assert.Equal(t, testLoggerBackend.SimplifiedEvents(), []string{
		"directive: for i in {1..100}; { printf 'hello'; }",
		"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello",
		"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello",
		"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello",
		"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello",
		"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello",
		"Exit Code: 0",
	})
}
