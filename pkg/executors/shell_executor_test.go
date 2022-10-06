package executors

import (
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	testsupport "github.com/semaphoreci/agent/test/support"
	assert "github.com/stretchr/testify/assert"
)

var UnicodeOutput1 = `特定の伝説に拠る物語の由来については諸説存在し。特定の伝説に拠る物語の由来については諸説存在し。特定の伝説に拠る物語の由来については諸説存在し。`
var UnicodeOutput2 = `━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━`

func Test__ShellExecutor__SSHJumpPointIsCreatedForHosted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	sshJumpPointPath := "/tmp/ssh_jump_point"
	os.Remove(sshJumpPointPath)
	_, _ = setupShellExecutor(t, false)
	assert.FileExists(t, sshJumpPointPath)
	os.Remove(sshJumpPointPath)
}

func Test__ShellExecutor__SSHJumpPointIsNotCreatedForWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip()
	}

	sshJumpPointPath := "/tmp/ssh_jump_point"
	os.Remove(sshJumpPointPath)
	_, _ = setupShellExecutor(t, false)
	assert.NoFileExists(t, sshJumpPointPath)
	os.Remove(sshJumpPointPath)
}

func Test__ShellExecutor__SSHJumpPointIsNotCreatedForSelfHosted(t *testing.T) {
	sshJumpPointPath := "/tmp/ssh_jump_point"
	os.Remove(sshJumpPointPath)
	_, _ = setupShellExecutor(t, true)
	assert.NoFileExists(t, sshJumpPointPath)
	os.Remove(sshJumpPointPath)
}

func Test__ShellExecutor__EnvVars(t *testing.T) {
	e, testLoggerBackend := setupShellExecutor(t, true)
	assert.Zero(t, e.ExportEnvVars(
		[]api.EnvVar{
			{Name: "A", Value: base64.StdEncoding.EncodeToString([]byte("AAA"))},
			{Name: "B", Value: base64.StdEncoding.EncodeToString([]byte("BBB"))},
			{Name: "VAR_WITH_QUOTES", Value: base64.StdEncoding.EncodeToString([]byte("quotes ' quotes"))},
			{Name: "VAR_WITH_ENV_VAR", Value: base64.StdEncoding.EncodeToString([]byte(testsupport.NestedEnvVarValue("PATH", ":/etc/a")))},
		},
		[]config.HostEnvVar{
			{Name: "C", Value: "CCC"},
			{Name: "D", Value: "DDD"},
		},
	))

	assert.Zero(t, e.RunCommand(testsupport.EchoEnvVar("A"), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.EchoEnvVar("B"), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.EchoEnvVar("VAR_WITH_QUOTES"), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.EchoEnvVar("VAR_WITH_ENV_VAR"), false, ""))
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
		"Exporting VAR_WITH_ENV_VAR\n",
		"Exporting VAR_WITH_QUOTES\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("A")),
		"AAA",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("B")),
		"BBB",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("VAR_WITH_QUOTES")),
		"quotes ' quotes",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.EchoEnvVar("VAR_WITH_ENV_VAR")),
		testsupport.NestedEnvVarValue("PATH", ":/etc/a"),
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
	e, testLoggerBackend := setupShellExecutor(t, true)
	homeDir, _ := os.UserHomeDir()

	absoluteFile := api.File{
		Path:    filepath.Join(os.TempDir(), "absolute-file.txt"),
		Content: base64.StdEncoding.EncodeToString([]byte("absolute\n")),
		Mode:    "0400",
	}

	absoluteFileInMissingDir := api.File{
		Path:    filepath.Join(os.TempDir(), "somedir", "absolute-file-missing-dir.txt"),
		Content: base64.StdEncoding.EncodeToString([]byte("absolute-in-missing-dir\n")),
		Mode:    "0440",
	}

	relativeFile := api.File{
		Path:    filepath.Join("somedir", "relative-file.txt"),
		Content: base64.StdEncoding.EncodeToString([]byte("relative\n")),
		Mode:    "0600",
	}

	relativeFileInMissingDir := api.File{
		Path:    filepath.Join("somedir", "relative-file-in-missing-dir.txt"),
		Content: base64.StdEncoding.EncodeToString([]byte("relative-in-missing-dir\n")),
		Mode:    "0644",
	}

	homeFile := api.File{
		Path:    "~/home-file.txt",
		Content: base64.StdEncoding.EncodeToString([]byte("home\n")),
		Mode:    "0777",
	}

	assert.Zero(t, e.InjectFiles([]api.File{
		absoluteFile,
		absoluteFileInMissingDir,
		relativeFile,
		relativeFileInMissingDir,
		homeFile,
	}))

	assert.Zero(t, e.RunCommand(testsupport.Cat(absoluteFile.NormalizePath(homeDir)), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.Cat(absoluteFileInMissingDir.NormalizePath(homeDir)), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.Cat(relativeFile.NormalizePath(homeDir)), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.Cat(relativeFileInMissingDir.NormalizePath(homeDir)), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.Cat(homeFile.NormalizePath(homeDir)), false, ""))
	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		"directive: Injecting Files",
		fmt.Sprintf("Injecting %s with file mode 0400\n", absoluteFile.NormalizePath(homeDir)),
		fmt.Sprintf("Injecting %s with file mode 0440\n", absoluteFileInMissingDir.NormalizePath(homeDir)),
		fmt.Sprintf("Injecting %s with file mode 0600\n", relativeFile.NormalizePath(homeDir)),
		fmt.Sprintf("Injecting %s with file mode 0644\n", relativeFileInMissingDir.NormalizePath(homeDir)),
		fmt.Sprintf("Injecting %s with file mode 0777\n", homeFile.NormalizePath(homeDir)),
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Cat(absoluteFile.NormalizePath(homeDir))),
		"absolute\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Cat(absoluteFileInMissingDir.NormalizePath(homeDir))),
		"absolute-in-missing-dir\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Cat(relativeFile.NormalizePath(homeDir))),
		"relative\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Cat(relativeFileInMissingDir.NormalizePath(homeDir))),
		"relative-in-missing-dir\n",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Cat(homeFile.NormalizePath(homeDir))),
		"home\n",
		"Exit Code: 0",
	})

	// Assert file modes
	if runtime.GOOS == "windows" {
		// windows file modes are a bit different, since it uses ACLs and not flags like unix.
		// Therefore, 04xx means everybody can read it, 06xx means everybody can write and read it
		// See: https://pkg.go.dev/os#Chmod
		assertFileMode(t, absoluteFile.NormalizePath(homeDir), fs.FileMode(0444))
		assertFileMode(t, absoluteFileInMissingDir.NormalizePath(homeDir), fs.FileMode(0444))
		assertFileMode(t, relativeFile.NormalizePath(homeDir), fs.FileMode(0666))
		assertFileMode(t, relativeFileInMissingDir.NormalizePath(homeDir), fs.FileMode(0666))
		assertFileMode(t, homeFile.NormalizePath(homeDir), fs.FileMode(0666))
	} else {
		assertFileMode(t, absoluteFile.NormalizePath(homeDir), fs.FileMode(0400))
		assertFileMode(t, absoluteFileInMissingDir.NormalizePath(homeDir), fs.FileMode(0440))
		assertFileMode(t, relativeFile.NormalizePath(homeDir), fs.FileMode(0600))
		assertFileMode(t, relativeFileInMissingDir.NormalizePath(homeDir), fs.FileMode(0644))
		assertFileMode(t, homeFile.NormalizePath(homeDir), fs.FileMode(0777))
	}

	os.Remove(absoluteFile.NormalizePath(homeDir))
	os.Remove(absoluteFileInMissingDir.NormalizePath(homeDir))
	os.Remove(relativeFile.NormalizePath(homeDir))
	os.Remove(relativeFileInMissingDir.NormalizePath(homeDir))
	os.Remove(homeFile.NormalizePath(homeDir))
}

func Test__ShellExecutor__MultilineCommand(t *testing.T) {
	e, testLoggerBackend := setupShellExecutor(t, true)

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
	e, testLoggerBackend := setupShellExecutor(t, true)

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
	e, testLoggerBackend := setupShellExecutor(t, true)

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
	e, testLoggerBackend := setupShellExecutor(t, true)

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
	e, testLoggerBackend := setupShellExecutor(t, true)

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
		strings.Repeat("hello", 100),
		"Exit Code: 0",
	})
}

func Test__ShellExecutor__Unicode(t *testing.T) {
	e, testLoggerBackend := setupShellExecutor(t, true)
	assert.Zero(t, e.RunCommand(testsupport.Output(UnicodeOutput1), false, ""))
	assert.Zero(t, e.RunCommand(testsupport.Output(UnicodeOutput2), false, ""))
	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())

	time.Sleep(1 * time.Second)
	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		fmt.Sprintf("directive: %s", testsupport.Output(UnicodeOutput1)),
		"特定の伝説に拠る物語の由来については諸説存在し。特定の伝説に拠る物語の由来については諸説存在し。特定の伝説に拠る物語の由来については諸説存在し。",
		"Exit Code: 0",

		fmt.Sprintf("directive: %s", testsupport.Output(UnicodeOutput2)),
		"━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
		"Exit Code: 0",
	})
}

func Test__ShellExecutor__BrokenUnicode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	e, testLoggerBackend := setupShellExecutor(t, true)

	go func() {
		assert.Zero(t, e.RunCommand(testsupport.EchoBrokenUnicode(), false, ""))
	}()

	time.Sleep(5 * time.Second)

	assert.Zero(t, e.Stop())
	assert.Zero(t, e.Cleanup())

	time.Sleep(1 * time.Second)

	simplifiedEvents, err := testLoggerBackend.SimplifiedEvents(true)
	assert.Nil(t, err)

	assert.Equal(t, simplifiedEvents, []string{
		fmt.Sprintf("directive: %s", testsupport.EchoBrokenUnicode()),
		"\x96\x96\x96\x96\x96",
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

func setupShellExecutor(t *testing.T, selfHosted bool) (*ShellExecutor, *eventlogger.InMemoryBackend) {
	testsupport.SetupTestLogs()

	testLogger, testLoggerBackend := eventlogger.DefaultTestLogger()
	e := NewShellExecutor(basicRequest(), testLogger, selfHosted)

	assert.Zero(t, e.Prepare())
	assert.Zero(t, e.Start())

	return e, testLoggerBackend
}
