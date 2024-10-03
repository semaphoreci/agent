package compression

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test__Compress(t *testing.T) {
	// create text file for compression
	raw, err := os.CreateTemp(os.TempDir(), "*.txt")
	require.NoError(t, err)

	defer os.Remove(raw.Name())

	content := ""
	for i := 0; i < 100; i++ {
		l := fmt.Sprintf("[%d] abcdefghijklmnopqrstuvwxyz\n", i)
		raw.WriteString(l)
		content += l
	}

	require.NoError(t, raw.Close())

	// compress file
	compressed, err := Compress(raw.Name())
	require.NoError(t, err)

	defer os.Remove(compressed)

	// decompress and assert its contents are correct
	f, err := os.Open(compressed)
	require.NoError(t, err)
	gzipReader, err := gzip.NewReader(f)
	require.NoError(t, err)
	text, err := io.ReadAll(gzipReader)
	require.NoError(t, err)
	require.Equal(t, content, string(text))
}
