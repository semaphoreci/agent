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
	_, _ = setupShellExecutor(t)

	sshJumpPointPath := filepath.Join(os.TempDir(), "ssh_jump_point")
	os.Remove(sshJumpPointPath)

	// ssh jump point is not set up in windows
	if runtime.GOOS == "windows" {
		assert.NoFileExists(t, sshJumpPointPath)
	} else {
		assert.FileExists(t, sshJumpPointPath)
	}

	os.Remove(sshJumpPointPath)
}

func Test__ShellExecutor__Start(t *testing.T) {
	e, _ := setupShellExecutor(t)

	if runtime.GOOS == "windows" {
		assert.Nil(t, e.Shell.TTY)
	} else {
		assert.NotNil(t, e.Shell.TTY)
	}

	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())
}

func Test__ShellExecutor_EnvVars(t *testing.T) {
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

	assert.Zero(t, e.RunCommand(echoEnvVar("A"), false, ""))
	assert.Zero(t, e.RunCommand(echoEnvVar("B"), false, ""))
	assert.Zero(t, e.RunCommand(echoEnvVar("C"), false, ""))
	assert.Zero(t, e.RunCommand(echoEnvVar("D"), false, ""))
	assert.Zero(t, e.RunCommand(echoEnvVar("E"), false, ""))
	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())

	assert.Equal(t, testLoggerBackend.SimplifiedEvents(true), []string{
		"directive: Exporting environment variables",
		"Exporting A\n",
		"Exporting B\n",
		"Exporting C\n",
		"Exporting D\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", echoEnvVar("A")),
		"AAA",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", echoEnvVar("B")),
		"BBB",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", echoEnvVar("C")),
		"CCC",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", echoEnvVar("D")),
		"DDD",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", echoEnvVar("E")),
		"Exit Code: 0",
	})
}

func Test__ShellExecutor__InjectFiles(t *testing.T) {
	e, testLoggerBackend := setupShellExecutor(t)
	homeDir, _ := os.UserHomeDir()

	absoluteFile := api.File{
		Path:    filepath.Join(os.TempDir(), "somedir", "absolute-file.txt"),
		Content: base64.StdEncoding.EncodeToString([]byte("absolute")),
		Mode:    "0600",
	}

	relativeFile := api.File{
		Path:    filepath.Join("somedir", "relative-file.txt"),
		Content: base64.StdEncoding.EncodeToString([]byte("relative")),
		Mode:    "0644",
	}

	homeFile := api.File{
		Path:    "~/home-file.txt",
		Content: base64.StdEncoding.EncodeToString([]byte("home")),
		Mode:    "0777",
	}

	assert.Zero(t, e.InjectFiles([]api.File{absoluteFile, relativeFile, homeFile}))
	assert.Zero(t, e.RunCommand(catCommand(absoluteFile.NormalizePath(homeDir)), false, ""))
	assert.Zero(t, e.RunCommand(catCommand(relativeFile.NormalizePath(homeDir)), false, ""))
	assert.Zero(t, e.RunCommand(catCommand(homeFile.NormalizePath(homeDir)), false, ""))
	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())

	assert.Equal(t, testLoggerBackend.SimplifiedEvents(true), []string{
		"directive: Injecting Files",
		fmt.Sprintf("Injecting %s with file mode 0600\n", absoluteFile.NormalizePath(homeDir)),
		fmt.Sprintf("Injecting %s with file mode 0644\n", relativeFile.NormalizePath(homeDir)),
		fmt.Sprintf("Injecting %s with file mode 0777\n", homeFile.NormalizePath(homeDir)),
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", catCommand(absoluteFile.NormalizePath(homeDir))),
		"absolute\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", catCommand(relativeFile.NormalizePath(homeDir))),
		"relative\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", catCommand(homeFile.NormalizePath(homeDir))),
		"home\n",
		"Exit Code: 0",
	})

	// Assert file modes
	assertFileMode(t, absoluteFile.NormalizePath(homeDir), fs.FileMode(uint32(0600)))
	assertFileMode(t, relativeFile.NormalizePath(homeDir), fs.FileMode(uint32(0644)))
	assertFileMode(t, homeFile.NormalizePath(homeDir), fs.FileMode(uint32(0777)))

	os.Remove(absoluteFile.NormalizePath(homeDir))
	os.Remove(relativeFile.NormalizePath(homeDir))
	os.Remove(homeFile.NormalizePath(homeDir))
}

func Test__ShellExecutor__MultilineCommand(t *testing.T) {
	e, testLoggerBackend := setupShellExecutor(t)

	assert.Zero(t, e.RunCommand(multilineCmd(), false, ""))
	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())

	assert.Equal(t, testLoggerBackend.SimplifiedEvents(true), []string{
		fmt.Sprintf("directive: %s", multilineCmd()),
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
	assert.NotZero(t, e.RunCommand(catCommand(relativePath), false, ""))

	// works because we are now in the correct directory
	assert.Zero(t, e.RunCommand(changeDirectory(os.TempDir()), false, ""))
	assert.Zero(t, e.RunCommand(catCommand(relativePath), false, ""))

	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())

	assert.Equal(t, testLoggerBackend.SimplifiedEvents(false), []string{
		"directive: Injecting Files",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", catCommand(relativePath)),
		"Exit Code: 1",

		fmt.Sprintf("directive: %s", changeDirectory(os.TempDir())),
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", catCommand(relativePath)),
		"Exit Code: 0",
	})
}

func Test__ShellExecutor__ChangesEnvVars(t *testing.T) {
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
	assert.NotZero(t, e.RunCommand(catCommand(relativePath), false, ""))

	// works because we are now in the correct directory
	assert.Zero(t, e.RunCommand(changeDirectory(os.TempDir()), false, ""))
	assert.Zero(t, e.RunCommand(catCommand(relativePath), false, ""))

	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())

	assert.Equal(t, testLoggerBackend.SimplifiedEvents(false), []string{
		"directive: Injecting Files",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", catCommand(relativePath)),
		"Exit Code: 1",

		fmt.Sprintf("directive: %s", changeDirectory(os.TempDir())),
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", catCommand(relativePath)),
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

	assert.Equal(t, testLoggerBackend.SimplifiedEvents(true)[0:4], []string{
		"directive: echo here",
		"here\n",
		"Exit Code: 0",

		"directive: sleep 20",
	})
}

func Test__ShellExecutor__LargeCommandOutput(t *testing.T) {
	e, testLoggerBackend := setupShellExecutor(t)

	go func() {
		assert.Zero(t, e.RunCommand(largeOutputCommand(), false, ""))
	}()

	time.Sleep(5 * time.Second)

	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())

	time.Sleep(1 * time.Second)

	assert.Equal(t, testLoggerBackend.SimplifiedEvents(true), []string{
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
		return fmt.Sprintf("Get-Content %s", filepath.FromSlash(fileName))
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

	return fmt.Sprintf("echo -n $%s", envVar)
}

func largeOutputCommand() string {
	if runtime.GOOS == "windows" {
		return "foreach ($i in 1..100) { Write-Host \"hello\" -NoNewLine }"
	}

	return "for i in {1..100}; { printf 'hello'; }"
}

func changeDirectory(dirName string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("Set-Location %s", dirName)
	}

	return fmt.Sprintf("cd %s", dirName)
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
