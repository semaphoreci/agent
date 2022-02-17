package shell

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/semaphoreci/agent/pkg/api"
	assert "github.com/stretchr/testify/assert"
)

func Test__Shell__SimpleHelloWorld(t *testing.T) {
	var output bytes.Buffer

	shell := bashShell()

	p1 := shell.NewProcess("echo Hello", []api.EnvVar{})
	p1.OnStdout(func(line string) {
		output.WriteString(line)
	})
	p1.Run()

	assert.Equal(t, output.String(), "Hello\n")
}

func Test__Shell__HandlingBashProcessKill(t *testing.T) {
	var output bytes.Buffer

	shell := bashShell()

	p1 := shell.NewProcess("echo 'Hello' && exit 1", []api.EnvVar{})
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

	shell := bashShell()

	p1 := shell.NewProcess("sleep infinity &", []api.EnvVar{})
	p1.OnStdout(func(line string) {
		output.WriteString(line)
	})
	p1.Run()

	p2 := shell.NewProcess("echo 'Hello' && exit 1", []api.EnvVar{})
	p2.OnStdout(func(line string) {
		output.WriteString(line)
	})
	p2.Run()

	assert.Equal(t, output.String(), "Hello\n")
}

func bashShell() *Shell {
	cmd := exec.Command("bash", "--login")

	shell, _ := NewShell(cmd, false)
	shell.Start()

	return shell
}
