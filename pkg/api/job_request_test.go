package api

import (
	"path/filepath"
	"testing"

	assert "github.com/stretchr/testify/assert"
)

func Test__JobRequest(t *testing.T) {
	homeDir := filepath.Join("/first", "second", "home")

	t.Run("file path with ~ is normalized", func(t *testing.T) {
		file := File{Path: "~/dir/somefile", Content: "", Mode: "0644"}
		assert.Equal(t, file.NormalizePath(homeDir), "/first/second/home/dir/somefile")
	})

	t.Run("absolute file path remains the same", func(t *testing.T) {
		file := File{Path: "/first/second/home/somefile", Content: "", Mode: "0644"}
		assert.Equal(t, file.NormalizePath(homeDir), "/first/second/home/somefile")
	})

	t.Run("relative file path is put on home directory", func(t *testing.T) {
		file := File{Path: "somefile", Content: "", Mode: "0644"}
		assert.Equal(t, file.NormalizePath(homeDir), "/first/second/home/somefile")
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
