package testsupport

import (
	"fmt"
	"io/ioutil"
	"runtime"
)

func TempFileWithExtension() (string, error) {
	tmpFile, err := ioutil.TempFile("", fmt.Sprintf("file*.%s", extension()))
	if err != nil {
		return "", err
	}

	tmpFile.Close()
	return tmpFile.Name(), nil
}

func extension() string {
	if runtime.GOOS == "windows" {
		return "ps1"
	}

	return "sh"
}
