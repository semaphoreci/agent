package shell

import (
	"bytes"
	"io/ioutil"
	"log"
	"os/exec"
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

	p1 := shell.NewProcess("echo 'Hello' && exit 1")
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

func tempStorageFolder() string {
	dir, err := ioutil.TempDir("", "agent-test")
	if err != nil {
		log.Fatal(err)
	}

	return dir
}

func bashShell() *Shell {
	dir := tempStorageFolder()
	cmd := exec.Command("bash", "--login")

	shell, _ := NewShell(cmd, dir)
	shell.Start()

	return shell
}
