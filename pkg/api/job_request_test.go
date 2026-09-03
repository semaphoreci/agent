package agentapi

import (
	"encoding/base64"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
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

func Test__DecodeNamesTheOffendingValue(t *testing.T) {
	t.Run("env var decode error names the variable", func(t *testing.T) {
		envVar := EnvVar{Name: "SEMAPHORE_GIT_SHA", Value: "abc$def"}

		_, err := envVar.Decode()
		assert.ErrorContains(t, err, "error decoding 'SEMAPHORE_GIT_SHA'")
		assert.ErrorContains(t, err, "illegal base64 data")
	})

	t.Run("env var decode error does not leak the value", func(t *testing.T) {
		envVar := EnvVar{Name: "SEMAPHORE_GIT_SHA", Value: "super-secret-value"}

		_, err := envVar.Decode()
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "super-secret-value")
		assert.NotContains(t, err.Error(), "secret")
	})

	t.Run("file decode error names the path", func(t *testing.T) {
		file := File{Path: "/home/semaphore/.ssh/id_rsa", Content: "abc$def"}

		_, err := file.Decode()
		assert.ErrorContains(t, err, "error decoding '/home/semaphore/.ssh/id_rsa'")
	})

	t.Run("file decode error does not leak the content", func(t *testing.T) {
		file := File{Path: "a/b/c", Content: "private-key-material"}

		_, err := file.Decode()
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "private-key-material")
	})

	t.Run("valid values decode without error", func(t *testing.T) {
		envVar := EnvVar{Name: "A", Value: base64.StdEncoding.EncodeToString([]byte("VALUE_A"))}
		v, err := envVar.Decode()
		assert.NoError(t, err)
		assert.Equal(t, "VALUE_A", string(v))

		file := File{Path: "a/b/c", Content: base64.StdEncoding.EncodeToString([]byte("CONTENT"))}
		c, err := file.Decode()
		assert.NoError(t, err)
		assert.Equal(t, "CONTENT", string(c))
	})
}

func Test__PublicKeyDecodeNamesThePosition(t *testing.T) {
	t.Run("decode error identifies the key by position", func(t *testing.T) {
		key := PublicKey("abc$def")

		_, err := key.DecodeAt(2)
		assert.ErrorContains(t, err, "error decoding SSH public key #2")
		assert.ErrorContains(t, err, "length 7, not padded to a multiple of 4")
		assert.ErrorContains(t, err, "illegal base64 data")
	})

	t.Run("decode error does not leak the key material", func(t *testing.T) {
		key := PublicKey("ssh-rsa AAAAB3NzaC1yc2EAAAA-not-base64")

		_, err := key.DecodeAt(0)
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "ssh-rsa")
		assert.NotContains(t, err.Error(), "AAAAB3")
	})

	t.Run("valid key decodes", func(t *testing.T) {
		key := PublicKey(base64.StdEncoding.EncodeToString([]byte("ssh-rsa AAAA")))

		v, err := key.DecodeAt(0)
		assert.NoError(t, err)
		assert.Equal(t, "ssh-rsa AAAA", string(v))
	})
}

func Test__DescribeBase64Value(t *testing.T) {
	t.Run("reports the shape, never the content", func(t *testing.T) {
		assert.Equal(t, "length 0", describeBase64Value(""))
		assert.Equal(t, "length 8", describeBase64Value("aGVsbG8="))

		// url-safe alphabet: '-' and '_' are illegal in std encoding
		assert.Equal(t, "length 4, valid as url-safe base64", describeBase64Value("-_-_"))

		// plaintext that merely contains a hyphen is not url-safe base64,
		// and saying so would point at the wrong culprit
		assert.Equal(
			t,
			"length 18, not padded to a multiple of 4",
			describeBase64Value("super-secret-value"),
		)

		// unpadded values are not a multiple of 4 characters long
		assert.Equal(t, "length 7, not padded to a multiple of 4", describeBase64Value("aGVsbG8"))

		assert.Equal(
			t,
			"length 9, not padded to a multiple of 4, contains whitespace",
			describeBase64Value("aGVs bG8="),
		)

		// \r and \n are ignored by the decoder, so they are not padding problems
		assert.Equal(t, "length 9, contains whitespace", describeBase64Value("aGVsbG8=\n"))

		assert.Equal(
			t,
			"length 6, not padded to a multiple of 4",
			describeBase64Value("aGV-b8"),
		)
	})

	t.Run("does not include the value itself", func(t *testing.T) {
		assert.NotContains(t, describeBase64Value("super-secret-value"), "secret")
	})
}

func Test__ValidateEncoding(t *testing.T) {
	validValue := base64.StdEncoding.EncodeToString([]byte("VALUE"))

	t.Run("no error when everything decodes", func(t *testing.T) {
		request := JobRequest{
			EnvVars: []EnvVar{{Name: "A", Value: validValue}},
			Files:   []File{{Path: "a/b/c", Content: validValue}},
		}

		assert.NoError(t, request.ValidateEncoding())
	})

	t.Run("no error when there is nothing to decode", func(t *testing.T) {
		assert.NoError(t, (&JobRequest{}).ValidateEncoding())
	})

	t.Run("collects every offender", func(t *testing.T) {
		request := JobRequest{
			EnvVars: []EnvVar{
				{Name: "GOOD", Value: validValue},
				{Name: "BAD_ONE", Value: "abc$def"},
				{Name: "BAD_TWO", Value: "-_-_"},
			},
			Files: []File{{Path: "d/e/f", Content: "abc$def"}},
		}

		err := request.ValidateEncoding()
		assert.ErrorContains(t, err, "error decoding 'BAD_ONE'")
		assert.ErrorContains(t, err, "error decoding 'BAD_TWO'")
		assert.ErrorContains(t, err, "error decoding 'd/e/f'")
		assert.NotContains(t, err.Error(), "GOOD")
	})

	t.Run("every offender stays individually reachable", func(t *testing.T) {
		request := JobRequest{
			EnvVars: []EnvVar{
				{Name: "BAD_ONE", Value: "abc$def"},
				{Name: "BAD_TWO", Value: "-_-_"},
			},
			Files: []File{{Path: "d/e/f", Content: "abc$def"}},
		}

		err := request.ValidateEncoding()
		joined, ok := err.(interface{ Unwrap() []error })
		assert.True(t, ok, "expected the errors to be joined, not flattened")
		assert.Len(t, joined.Unwrap(), 3)

		// the underlying base64 error survives the wrapping
		var corrupt base64.CorruptInputError
		assert.True(t, errors.As(err, &corrupt))
	})

	t.Run("env vars are reported before files", func(t *testing.T) {
		request := JobRequest{
			EnvVars: []EnvVar{{Name: "BAD_ENV_VAR", Value: "abc$def"}},
			Files:   []File{{Path: "bad/file", Content: "abc$def"}},
		}

		err := request.ValidateEncoding()
		assert.Error(t, err)
		assert.Less(
			t,
			strings.Index(err.Error(), "BAD_ENV_VAR"),
			strings.Index(err.Error(), "bad/file"),
		)
	})

	// SSH public keys are a known gap: a bad key is fatal on the docker-compose
	// executor, but it is not validated here and PublicKey.Decode carries no name.
	t.Run("container env vars and image pull credentials are out of scope", func(t *testing.T) {
		request := JobRequest{
			SSHPublicKeys: []PublicKey{"abc$def"},
			Compose: Compose{
				Containers: []Container{
					{Name: "main", EnvVars: []EnvVar{{Name: "CONTAINER_VAR", Value: "abc$def"}}},
				},
				ImagePullCredentials: []ImagePullCredentials{
					{EnvVars: []EnvVar{{Name: "CREDENTIAL_VAR", Value: "abc$def"}}},
				},
			},
		}

		assert.NoError(t, request.ValidateEncoding())
	})
}
