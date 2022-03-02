package shell

import (
	"bytes"
	"os"
	"runtime"
	"testing"

	assert "github.com/stretchr/testify/assert"
)

func Test__Shell__NewShell(t *testing.T) {
	shell, err := NewShell(os.TempDir())
	assert.Nil(t, err)
	assert.NotNil(t, shell.Cwd)

	if runtime.GOOS == "windows" {
		assert.Equal(t, shell.Executable, "powershell")
	} else {
		assert.Equal(t, shell.Executable, "bash")
	}

	if runtime.GOOS == "windows" {
		assert.Equal(t, shell.Args, []string{"-NoProfile", "-NonInteractive"})
	} else {
		assert.Equal(t, shell.Args, []string{"--login"})
	}
}

func Test__Shell__Start(t *testing.T) {
	shell, err := NewShell(os.TempDir())
	assert.Nil(t, err)

	err = shell.Start()
	assert.Nil(t, err)

	if runtime.GOOS == "windows" {
		assert.Nil(t, shell.BootCommand)
		assert.Nil(t, shell.TTY)
	} else {
		assert.NotNil(t, shell.BootCommand)
		assert.NotNil(t, shell.TTY)
	}
}

func Test__Shell__SimpleHelloWorld(t *testing.T) {
	var output bytes.Buffer

	shell, _ := NewShell(os.TempDir())
	shell.Start()

	p1 := shell.NewProcess("echo Hello")
	p1.OnStdout(func(line string) {
		output.WriteString(line)
	})
	p1.Run()

	assert.Equal(t, output.String(), "Hello\n")
}

func Test__Shell__HandlingBashProcessKill(t *testing.T) {
	var output bytes.Buffer

	shell, _ := NewShell(os.TempDir())
	shell.Start()

	var cmd string
	if runtime.GOOS == "windows" {
		// CMD.exe stupidly outputs the space between the word and the && as well
		cmd = `
			echo Hello
			if ($?) {
				Exit 1
			}
		`
	} else {
		cmd = "echo Hello && exit 1"
	}

	p1 := shell.NewProcess(cmd)
	p1.OnStdout(func(line string) {
		output.WriteString(line)
	})
	p1.Run()

	assert.Equal(t, output.String(), "Hello\n")
}

func Test__Shell__HandlingBashProcessKillThatHasBackgroundJobs(t *testing.T) {
	t.Skip("flaky")

	var output bytes.Buffer

	//
	// If a user starts a background job in the session, for example
	// 'sleep infinity &' and then exists the shell, the Bash session will not
	// be killed properly.
	//
	// It will enter a defunct state until its parent (the agent) reaps it.
	//
	// This test verifies that the reaping process is working properly and that
	// it stops the read procedure.
	//

	shell, _ := NewShell(os.TempDir())
	shell.Start()

	p1 := shell.NewProcess("sleep infinity &")
	p1.OnStdout(func(line string) {
		output.WriteString(line)
	})
	p1.Run()

	p2 := shell.NewProcess("echo 'Hello' && exit 1")
	p2.OnStdout(func(line string) {
		output.WriteString(line)
	})
	p2.Run()

	assert.Equal(t, output.String(), "Hello\n")
}
