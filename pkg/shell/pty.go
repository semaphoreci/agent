// +build !windows

package shell

import (
	"os"
	"os/exec"

	pty "github.com/creack/pty"
)

func StartPTY(command *exec.Cmd) (*os.File, error) {
	return pty.Start(command)
}
