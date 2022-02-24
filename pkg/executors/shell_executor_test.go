package executors

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"runtime"
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

	e := NewShellExecutor(request, testLogger)

	e.Prepare()
	e.Start()

	e.RunCommand("echo here", false, "")
	e.RunCommand(multilineCmd(), false, "")

	envVars := []api.EnvVar{
		{Name: "A", Value: "Zm9vCg=="},
	}

	e.ExportEnvVars(envVars, []config.HostEnvVar{})
	e.RunCommand(echoEnvVar("A"), false, "")

	files := []api.File{
		{
			Path:    "/tmp/random-file.txt",
			Content: "YWFhYmJiCgo=",
			Mode:    "0600",
		},
	}

	e.InjectFiles(files)
	e.RunCommand(catCommand("/tmp/random-file.txt"), false, "")

	e.RunCommand(echoExitCode(), false, "")

	e.Stop()
	e.Cleanup()

	assert.Equal(t, testLoggerBackend.SimplifiedEvents(), []string{
		"directive: echo here",
		"here\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", multilineCmd()),
		"etc exists, multiline huzzahh!\n",
		"Exit Code: 0",

		"directive: Exporting environment variables",
		"Exporting A\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", echoEnvVar("A")),
		"foo\n",
		"Exit Code: 0",

		"directive: Injecting Files",
		"Injecting /tmp/random-file.txt with file mode 0600\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", catCommand("/tmp/random-file.txt")),
		"aaabbb\n\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", echoExitCode()),
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

	e := NewShellExecutor(request, testLogger)

	e.Prepare()
	e.Start()

	go func() {
		e.RunCommand("echo here", false, "")
		e.RunCommand("sleep 5", false, "")
	}()

	time.Sleep(1 * time.Second)

	e.Stop()
	e.Cleanup()

	time.Sleep(1 * time.Second)

	assert.Equal(t, testLoggerBackend.SimplifiedEvents()[0:4], []string{
		"directive: echo here",
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

	e := NewShellExecutor(request, testLogger)

	e.Prepare()
	e.Start()

	go func() {
		e.RunCommand(largeOutputCommand(), false, "")
	}()

	time.Sleep(5 * time.Second)

	e.Stop()
	e.Cleanup()

	time.Sleep(1 * time.Second)

	assert.Equal(t, testLoggerBackend.SimplifiedEvents(), []string{
		fmt.Sprintf("directive: %s", largeOutputCommand()),
		"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello",
		"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello",
		"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello",
		"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello",
		"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello",
		"Exit Code: 0",
	})
}

func catCommand(fileName string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("type %s", filepath.FromSlash(fileName))
	}

	return fmt.Sprintf("cat %s", fileName)
}

func multilineCmd() string {
	if runtime.GOOS == "windows" {
		return `
			if exist \ProgramData (
				echo etc exists, multiline huzzahh!
			)
		`
	}

	return `
		if [ -d /etc ]; then
			echo 'etc exists, multiline huzzahh!'
		fi
	`
}

func echoEnvVar(envVar string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("echo %%%s%%", envVar)
	}

	return fmt.Sprintf("$%s", envVar)
}

func echoExitCode() string {
	if runtime.GOOS == "windows" {
		return "echo %errorlevel%"
	}

	return "echo $?"
}

func largeOutputCommand() string {
	if runtime.GOOS == "windows" {
		// 'set /p=' without specifying a variable name will set the ERRORLEVEL to 1, so we just use a dummy name here
		return "for /L %%I in (1,1,100) do echo|set /p dummy=hello"
	}

	return "for i in {1..100}; { printf 'hello'; }"
}
