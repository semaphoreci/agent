package agentapi

import (
	"encoding/base64"
	"path/filepath"
	"runtime"
	"testing"

	assert "github.com/stretchr/testify/assert"
)

func Test__JobRequest(t *testing.T) {
	homeDir := filepath.Join("/first", "second", "home")

	t.Run("file path with ~ is normalized", func(t *testing.T) {
		file := File{Path: "~/dir/somefile", Content: "", Mode: "0644"}
		if runtime.GOOS == "windows" {
			assert.Equal(t, file.NormalizePath(homeDir), "\\first\\second\\home\\dir\\somefile")
		} else {
			assert.Equal(t, file.NormalizePath(homeDir), "/first/second/home/dir/somefile")
		}
	})

	t.Run("absolute file path remains the same", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			file := File{Path: "C:\\first\\second\\home\\somefile", Content: "", Mode: "0644"}
			assert.Equal(t, file.NormalizePath(homeDir), "C:\\first\\second\\home\\somefile")
		} else {
			file := File{Path: "/first/second/home/somefile", Content: "", Mode: "0644"}
			assert.Equal(t, file.NormalizePath(homeDir), "/first/second/home/somefile")
		}
	})

	t.Run("relative file path is put on home directory", func(t *testing.T) {
		file := File{Path: "somefile", Content: "", Mode: "0644"}
		if runtime.GOOS == "windows" {
			assert.Equal(t, file.NormalizePath(homeDir), "\\first\\second\\home\\somefile")
		} else {
			assert.Equal(t, file.NormalizePath(homeDir), "/first/second/home/somefile")
		}
	})

	t.Run("accepted file modes", func(t *testing.T) {
		fileModes := []string{"0600", "0644", "0777"}
		for _, fileMode := range fileModes {
			file := File{Path: "somefile", Content: "", Mode: fileMode}
			_, err := file.ParseMode()
			assert.Nil(t, err)
		}
	})

	t.Run("bad file modes", func(t *testing.T) {
		fileModes := []string{"+x", "+r", "+w", "+rw"}
		for _, fileMode := range fileModes {
			file := File{Path: "somefile", Content: "", Mode: fileMode}
			_, err := file.ParseMode()
			assert.NotNil(t, err)
		}
	})
}

func Test__ImagePullCredentials(t *testing.T) {
	t.Run("ToCmdEnvVars()", func(t *testing.T) {
		// returns slice of key-value env vars
		c := ImagePullCredentials{EnvVars: []EnvVar{
			{Name: "FOO", Value: base64.StdEncoding.EncodeToString([]byte("FOO_VALUE"))},
			{Name: "BAR", Value: base64.StdEncoding.EncodeToString([]byte("BAR_VALUE"))},
		}}

		envs, err := c.ToCmdEnvVars()
		assert.NoError(t, err)
		assert.Equal(t, envs, []string{"FOO=FOO_VALUE", "BAR=BAR_VALUE"})

		// returns error
		c = ImagePullCredentials{EnvVars: []EnvVar{
			{Name: "FOO", Value: base64.StdEncoding.EncodeToString([]byte("FOO_VALUE"))},
			{Name: "BAR", Value: "NOT_PROPERLY_ENCODED"},
		}}

		_, err = c.ToCmdEnvVars()
		assert.ErrorContains(t, err, "error decoding 'BAR'")
	})

	t.Run("FindEnvVar()", func(t *testing.T) {
		c := ImagePullCredentials{EnvVars: []EnvVar{
			{Name: "FOO", Value: base64.StdEncoding.EncodeToString([]byte("FOO_VALUE"))},
			{Name: "BAR", Value: "not-encoded-value"},
		}}

		// env var that exists returns no error
		v, err := c.FindEnvVar("FOO")
		assert.NoError(t, err)
		assert.Equal(t, "FOO_VALUE", v)

		// env var that exists, but is not properly encoded returns error
		_, err = c.FindEnvVar("BAR")
		assert.ErrorContains(t, err, "error decoding 'BAR'")

		// env var that does not exist returns error
		_, err = c.FindEnvVar("DOES_NOT_EXIST")
		assert.ErrorContains(t, err, "no env var 'DOES_NOT_EXIST' found")
	})

	t.Run("FindFile()", func(t *testing.T) {
		c := ImagePullCredentials{Files: []File{
			{Path: "a/b/c", Content: base64.StdEncoding.EncodeToString([]byte("VALUE_1"))},
			{Path: "d/e/f", Content: "not-encoded-value"},
		}}

		// file that exists returns no error
		v, err := c.FindFile("a/b/c")
		assert.NoError(t, err)
		assert.Equal(t, "VALUE_1", v)

		// file that exists, but is not properly encoded returns error
		_, err = c.FindFile("d/e/f")
		assert.ErrorContains(t, err, "error decoding 'd/e/f'")

		// file that does not exist returns error
		_, err = c.FindFile("does/not/exist")
		assert.ErrorContains(t, err, "no file 'does/not/exist' found")
	})
}
