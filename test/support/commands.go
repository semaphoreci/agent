package testsupport

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func AssertJobLogs(t *testing.T, actual, expected []string) {
	actualIndex := 0
	expectedIndex := 0

	for actualIndex < len(actual)-1 && expectedIndex < len(expected)-1 {
		actualLine := actual[actualIndex]
		expectedLine := expected[expectedIndex]

		if expectedLine == "*** OUTPUT ***" {
			if strings.HasPrefix(actualLine, "Exit Code: ") {
				expectedIndex++
			} else {
				actualIndex++
			}
		} else {
			if !assert.Equal(t, actualLine, expectedLine) {
				break
			} else {
				actualIndex++
				expectedIndex++
			}
		}
	}
}

func Cat(fileName string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("Get-Content %s", filepath.FromSlash(fileName))
	}

	return fmt.Sprintf("cat %s", fileName)
}

func Multiline() string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`
			if (Test-Path %s) {
				echo "etc exists, multiline huzzahh!"
			}
		`, os.TempDir())
	}

	return `
		if [ -d /etc ]; then
			echo 'etc exists, multiline huzzahh!'
		fi
	`
}

func Output(line string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("Write-Host \"%s\" -NoNewLine", line)
	}

	return fmt.Sprintf("echo -n '%s'", line)
}

func EchoEnvVar(envVar string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("Write-Host \"$env:%s\" -NoNewLine", envVar)
	}

	return fmt.Sprintf("echo -n $%s", envVar)
}

func SetEnvVar(name, value string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("$env:%s = '%s'", name, value)
	}

	return fmt.Sprintf("export %s=%s", name, value)
}

func UnsetEnvVar(name string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("Remove-Item -Path env:%s", name)
	}

	return fmt.Sprintf("unset %s", name)
}

func LargeOutputCommand() string {
	if runtime.GOOS == "windows" {
		return "foreach ($i in 1..100) { Write-Host \"hello\" -NoNewLine }"
	}

	return "for i in {1..100}; { printf 'hello'; }"
}

func Chdir(dirName string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("Set-Location %s", dirName)
	}

	return fmt.Sprintf("cd %s", dirName)
}

func UnknownCommandExitCode() int {
	if runtime.GOOS == "windows" {
		return 1
	}

	return 127
}
