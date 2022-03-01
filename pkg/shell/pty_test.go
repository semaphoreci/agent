package shell

import (
	"os/exec"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test__PTYIsNotSupportedOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip()
	}

	_, err := StartPTY(exec.Command("powershell"))
	assert.NotNil(t, err)
}

func Test__PTYIsSupportedOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	tty, err := StartPTY(exec.Command("bash", "--login"))
	assert.Nil(t, err)
	tty.Close()
}
