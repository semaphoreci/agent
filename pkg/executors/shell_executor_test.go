package executors

import (
	"encoding/base64"
	"fmt"
	"io/fs"
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

func Test__ShellExecutor__SSHJumpPoint(t *testing.T) {
	sshJumpPointPath := filepath.Join(os.TempDir(), "ssh_jump_point")
	os.Remove(sshJumpPointPath)

	_, _ = setupShellExecutor(t)

	// ssh jump point is not set up in windows
	if runtime.GOOS == "windows" {
		assert.NoFileExists(t, sshJumpPointPath)
	} else {
		assert.FileExists(t, sshJumpPointPath)
	}

	os.Remove(sshJumpPointPath)
}

func Test__ShellExecutor__EnvVars(t *testing.T) {
	e, testLoggerBackend := setupShellExecutor(t)
	assert.Zero(t, e.ExportEnvVars(
		[]api.EnvVar{
			{Name: "A", Value: base64.StdEncoding.EncodeToString([]byte("AAA"))},
			{Name: "B", Value: base64.StdEncoding.EncodeToString([]byte("BBB"))},
		},
		[]config.HostEnvVar{
			{Name: "C", Value: "CCC"},
			{Name: "D", Value: "DDD"},
		},
	))

	assert.Zero(t, e.RunCommand(testsupport.EchoEnvVar("A"), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.EchoEnvVar("B"), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.EchoEnvVar("C"), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.EchoEnvVar("D"), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.EchoEnvVar("E"), false, ""))
	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"directive: Exporting environment variables",
		"Exporting A\n",
		"Exporting B\n",
		"Exporting C\n",
		"Exporting D\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("A")),
		"AAA",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("B")),
		"BBB",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("C")),
		"CCC",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("D")),
		"DDD",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("E")),
		"Exit Code: 0",
	})
}

func Test__ShellExecutor__InjectFiles(t *testing.T) {
	e, testLoggerBackend := setupShellExecutor(t)
	homeDir, _ := os.UserHomeDir()

	absoluteFile := api.File{
		Path:    filepath.Join(os.TempDir(), "somedir", "absolute-file.txt"),
		Content: base64.StdEncoding.EncodeToString([]byte("absolute\n")),
		Mode:    "0600",
	}

	relativeFile := api.File{
		Path:    filepath.Join("somedir", "relative-file.txt"),
		Content: base64.StdEncoding.EncodeToString([]byte("relative\n")),
		Mode:    "0644",
	}

	homeFile := api.File{
		Path:    "~/home-file.txt",
		Content: base64.StdEncoding.EncodeToString([]byte("home\n")),
		Mode:    "0777",
	}

	assert.Zero(t, e.InjectFiles([]api.File{absoluteFile, relativeFile, homeFile}))
	assert.Zero(t, e.RunCommand(testsupport.Cat(absoluteFile.NormalizePath(homeDir)), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.Cat(relativeFile.NormalizePath(homeDir)), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.Cat(homeFile.NormalizePath(homeDir)), false, ""))
	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"directive: Injecting Files",
		fmt.Sprintf("Injecting %s with file mode 0600\n", absoluteFile.NormalizePath(homeDir)),
		fmt.Sprintf("Injecting %s with file mode 0644\n", relativeFile.NormalizePath(homeDir)),
		fmt.Sprintf("Injecting %s with file mode 0777\n", homeFile.NormalizePath(homeDir)),
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Cat(absoluteFile.NormalizePath(homeDir))),
		"absolute\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Cat(relativeFile.NormalizePath(homeDir))),
		"relative\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Cat(homeFile.NormalizePath(homeDir))),
		"home\n",
		"Exit Code: 0",
	})

	// Assert file modes
	if runtime.GOOS == "windows" {
		// TODO: figure out how to set and assert file modes in windows
	} else {
		assertFileMode(t, absoluteFile.NormalizePath(homeDir), fs.FileMode(uint32(0600)))
		assertFileMode(t, relativeFile.NormalizePath(homeDir), fs.FileMode(uint32(0644)))
		assertFileMode(t, homeFile.NormalizePath(homeDir), fs.FileMode(uint32(0777)))
	}

	os.Remove(absoluteFile.NormalizePath(homeDir))
	os.Remove(relativeFile.NormalizePath(homeDir))
	os.Remove(homeFile.NormalizePath(homeDir))
}

func Test__ShellExecutor__MultilineCommand(t *testing.T) {
	e, testLoggerBackend := setupShellExecutor(t)

	assert.Zero(t, e.RunCommand(testsupport.Multiline(), false, ""))
	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		fmt.Sprintf("directive: %s", testsupport.Multiline()),
		"etc exists, multiline huzzahh!\n",
		"Exit Code: 0",
	})
}

func Test__ShellExecutor__ChangesCurrentDirectory(t *testing.T) {
	e, testLoggerBackend := setupShellExecutor(t)

	dirName := "somedir"
	absolutePath := filepath.Join(os.TempDir(), dirName, "some-file.txt")
	relativePath := filepath.Join(dirName, "some-file.txt")

	fileInDir := api.File{
		Path:    absolutePath,
		Content: base64.StdEncoding.EncodeToString([]byte("content")),
		Mode:    "0644",
	}

	assert.Zero(t, e.InjectFiles([]api.File{fileInDir}))

	// fails because current directory is not 'dirName'
	assert.NotZero(t, e.RunCommand(testsupport.Cat(relativePath), false, ""))

	// works because we are now in the correct directory
	assert.Zero(t, e.RunCommand(testsupport.Chdir(os.TempDir()), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.Cat(relativePath), false, ""))

	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(false)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"directive: Injecting Files",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Cat(relativePath)),
		"Exit Code: 1",

		fmt.Sprintf("directive: %s", testsupport.Chdir(os.TempDir())),
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Cat(relativePath)),
		"Exit Code: 0",
	})
}

func Test__ShellExecutor__ChangesEnvVars(t *testing.T) {
	e, testLoggerBackend := setupShellExecutor(t)

	varName := "IMPORTANT_VAR"
	assert.Zero(t, e.RunCommand(testsupport.EchoEnvVar(varName), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.SetEnvVar(varName, "IMPORTANT_VAR_VALUE"), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.EchoEnvVar(varName), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.UnsetEnvVar(varName), false, ""))

	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar(varName)),
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.SetEnvVar(varName, "IMPORTANT_VAR_VALUE")),
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar(varName)),
		"IMPORTANT_VAR_VALUE",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.UnsetEnvVar(varName)),
		"Exit Code: 0",
	})
}

func Test__ShellExecutor__StoppingRunningJob(t *testing.T) {
	e, testLoggerBackend := setupShellExecutor(t)

	go func() {
		e.RunCommand("echo here", false, "")
		e.RunCommand("sleep 20", false, "")
	}()

	time.Sleep(10 * time.Second)

	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())

	time.Sleep(1 * time.Second)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents[0:4], []string{
		"directive: echo here",
		"here\n",
		"Exit Code: 0",

		"directive: sleep 20",
	})
}

func Test__ShellExecutor__LargeCommandOutput(t *testing.T) {
	e, testLoggerBackend := setupShellExecutor(t)

	go func() {
		assert.Zero(t, e.RunCommand(testsupport.LargeOutputCommand(), false, ""))
	}()

	time.Sleep(5 * time.Second)

	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())

	time.Sleep(1 * time.Second)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		fmt.Sprintf("directive: %s", testsupport.LargeOutputCommand()),
		"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello",
		"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello",
		"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello",
		"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello",
		"hellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohellohello",
		"Exit Code: 0",
	})
}

func basicRequest() *api.JobRequest {
	return &api.JobRequest{
		SSHPublicKeys: []api.PublicKey{
			api.PublicKey(base64.StdEncoding.EncodeToString([]byte("ssh-rsa aaaaa"))),
		},
	}
}

func assertFileMode(t *testing.T, fileName string, fileMode fs.FileMode) {
	stat, err := os.Stat(fileName)
	if assert.Nil(t, err) {
		assert.Equal(t, stat.Mode(), fileMode)
	}
}

func setupShellExecutor(t *testing.T) (*ShellExecutor, *eventlogger.InMemoryBackend) {
	testsupport.SetupTestLogs()
	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	e := NewShellExecutor(basicRequest(), testLogger)

	assert.Zero(t, e.Prepare())
	assert.Zero(t, e.Start())

	return e, testLoggerBackend
}
