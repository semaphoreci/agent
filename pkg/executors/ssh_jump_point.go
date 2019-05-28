package executors

import "os"

const SSHJumpPointPath = "/tmp/ssh_jump_point"

func SetUpSSHJumpPoint(script string) error {
	f, err := os.OpenFile(SSHJumpPointPath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}

	_, err = f.WriteString(script)

	return err
}
