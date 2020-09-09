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
