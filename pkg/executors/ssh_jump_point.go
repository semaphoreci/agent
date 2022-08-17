package executors

import (
	"os"
	"runtime"

	log "github.com/sirupsen/logrus"
)

func SetUpSSHJumpPoint(script string) error {
	if runtime.GOOS == "windows" {
		log.Warn("Debug sessions are not supported in Windows - skipping")
		return nil
	}

	/*
	 * We can't use os.TempDir() here, because on macOS,
	 * $TMPDIR resolves to something like /var/folders/rg/92ky7bj54xj6pcv5l24g6l_00000gn/T/,
	 * and the sem CLI needs a /tmp/ssh_jump_point file.
	 */
	path := "/tmp/ssh_jump_point"

	// #nosec
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}

	_, err = f.WriteString(script)
	_ = f.Close()
	return err
}
