package executors

import (
	"os"

	"github.com/semaphoreci/agent/pkg/osinfo"
)

func SetUpSSHJumpPoint(script string) error {
	// #nosec
	path := osinfo.FormTempDirPath("ssh_jump_point")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}

	_, err = f.WriteString(script)

	return err
}
