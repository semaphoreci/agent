package shell

import (
	"bytes"
	"runtime"
	"testing"

	assert "github.com/stretchr/testify/assert"
)

func Test__Shell__SimpleHelloWorld(t *testing.T) {
	var output bytes.Buffer

	shell := bashShell()

	p1 := shell.NewProcess("echo Hello")
	p1.OnStdout(func(line string) {
		output.WriteString(line)
	})
	p1.Run()

	assert.Equal(t, output.String(), "Hello\n")
}

func Test__Shell__HandlingBashProcessKill(t *testing.T) {
	var output bytes.Buffer

	shell := bashShell()

	var cmd string
	if runtime.GOOS == "windows" {
		// CMD.exe stupidly outputs the space between the word and the && as well
		cmd = "echo Hello&& exit 1"
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

	if runtime.GOOS == "windows" {
		t.Skip("figure out this later")
	}

	shell := bashShell()

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

func bashShell() *Shell {
	shell, _ := NewShell("/tmp")
	shell.Start()

	return shell
}
