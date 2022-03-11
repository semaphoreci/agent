package executors

import (
	"os"
	"path/filepath"
)

func SetUpSSHJumpPoint(script string) error {
	path := filepath.Join(os.TempDir(), "ssh_jump_point")

	// #nosec
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}

	_, err = f.WriteString(script)
	_ = f.Close()
	return err
}
