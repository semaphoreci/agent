package executors

import (
	"os"
	"path/filepath"

	api "github.com/semaphoreci/agent/pkg/api"
)

func InjectEntriesToAuthorizedKeys(keys []api.PublicKey) error {
	sshDirectory := filepath.Join(UserHomeDir(), ".ssh")

	err := os.MkdirAll(sshDirectory, os.ModePerm)
	if err != nil {
		return err
	}

	authorizedKeysPath := filepath.Join(sshDirectory, "authorized_keys")

	authorizedKeys, err := os.OpenFile(
		authorizedKeysPath,
		os.O_APPEND|os.O_WRONLY|os.O_CREATE,
		0644)

	if err != nil {
		return err
	}

	defer authorizedKeys.Close()

	for _, key := range keys {
		authorizedKeysEntry, err := key.Decode()
		if err != nil {
			return err
		}

		_, err = authorizedKeys.WriteString(string(authorizedKeysEntry) + "\n")
		if err != nil {
			return err
		}
	}

	return nil
}
