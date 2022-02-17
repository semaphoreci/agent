package osinfo

import (
	"fmt"
	"os"
	"runtime"
)

func FormTempDirPath(fileName string) string {
	tempDir := os.TempDir()
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("%s\\%s", tempDir, fileName)
	}

	return fmt.Sprintf("%s/%s", tempDir, fileName)
}
