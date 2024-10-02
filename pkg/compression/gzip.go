package compression

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
)

func Compress(rawFileName string) (string, error) {
	rawFile, err := os.Open(rawFileName)
	if err != nil {
		return "", fmt.Errorf("error opening raw file %s: %v", rawFileName, err)
	}

	gzippedFileName := rawFileName + ".gz"
	gzippedFile, err := os.Create(gzippedFileName)
	if err != nil {
		return "", fmt.Errorf("error creating file %s for compression: %v", gzippedFileName, err)
	}

	defer gzippedFile.Close()
	gzipWriter := gzip.NewWriter(gzippedFile)
	defer gzipWriter.Close()

	_, err = io.Copy(gzipWriter, rawFile)
	if err != nil {
		return "", fmt.Errorf("error writing data into %s: %v", gzippedFileName, err)
	}

	err = gzipWriter.Flush()
	if err != nil {
		return "", fmt.Errorf("error flushing compressed data into %s: %v", gzippedFileName, err)
	}

	return gzippedFileName, nil
}
