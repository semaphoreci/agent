package executors

import (
	"encoding/base64"
	"fmt"
	"os"
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

	fileName := filepath.Join(os.TempDir(), "random-file.txt")
	files := []api.File{
		{
			Path:    fileName,
			Content: "YWFhYmJiCgo=",
			Mode:    "0600",
		},
	}

	e.InjectFiles(files)
	e.RunCommand(catCommand(fileName), false, "")

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
		fmt.Sprintf("Injecting %s with file mode 0600\n", fileName),
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", catCommand(fileName)),
		"aaabbb\n\n",
		"Exit Code: 0",
	})
}

func Test__ShellExecutor__StoppingRunningJob(t *testing.T) {
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
		e.RunCommand("sleep 20", false, "")
	}()

	time.Sleep(10 * time.Second)

	e.Stop()
	e.Cleanup()

	time.Sleep(1 * time.Second)

	assert.Equal(t, testLoggerBackend.SimplifiedEvents()[0:4], []string{
		"directive: echo here",
		"here\n",
		"Exit Code: 0",

		"directive: sleep 20",
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
		return fmt.Sprintf(`
			if (Test-Path %s) {
				echo "etc exists, multiline huzzahh!"
			}
		`, os.TempDir())
	}

	return `
		if [ -d /etc ]; then
			echo 'etc exists, multiline huzzahh!'
		fi
	`
}

func echoEnvVar(envVar string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("Write-Host \"$env:%s\" -NoNewLine", envVar)
	}

	return fmt.Sprintf("echo $%s", envVar)
}

func largeOutputCommand() string {
	if runtime.GOOS == "windows" {
		return "foreach ($i in 1..100) { Write-Host \"hello\" -NoNewLine }"
	}

	return "for i in {1..100}; { printf 'hello'; }"
}
