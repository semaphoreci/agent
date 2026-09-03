package executors

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	api "github.com/semaphoreci/agent/pkg/api"
	assert "github.com/stretchr/testify/assert"
)

func Test__InjectEntriesToAuthorizedKeys(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	t.Run("no keys => nothing to do", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		assert.NoError(t, InjectEntriesToAuthorizedKeys([]api.PublicKey{}))
	})

	t.Run("valid keys are appended", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("HOME", homeDir)

		err := InjectEntriesToAuthorizedKeys([]api.PublicKey{
			api.PublicKey(base64.StdEncoding.EncodeToString([]byte("ssh-rsa AAAA"))),
		})
		assert.NoError(t, err)

		contents, err := os.ReadFile(filepath.Join(homeDir, ".ssh", "authorized_keys"))
		assert.NoError(t, err)
		assert.Equal(t, "ssh-rsa AAAA\n", string(contents))
	})

	// An undecodable key used to fail the job with nothing but
	// "Failed to inject authorized keys" and a bare base64 offset.
	t.Run("undecodable key => error names its position", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())

		err := InjectEntriesToAuthorizedKeys([]api.PublicKey{
			api.PublicKey(base64.StdEncoding.EncodeToString([]byte("ssh-rsa AAAA"))),
			api.PublicKey("abc$def"),
		})

		assert.ErrorContains(t, err, "error decoding SSH public key #1")
		assert.ErrorContains(t, err, "length 7, not padded to a multiple of 4")
		assert.NotContains(t, err.Error(), "abc$def")
	})
}
