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

func AssertSimplifiedJobLogs(t *testing.T, actual, expected []string) {
	actualIndex := 0
	expectedIndex := 0

	for actualIndex < len(actual)-1 && expectedIndex < len(expected)-1 {
		actualLine := actual[actualIndex]
		expectedLine := expected[expectedIndex]

		if expectedLine == "*** OUTPUT ***" || expectedLine == "*** IGNORE LINES UNTIL EXIT CODE ***" {
			if strings.HasPrefix(actualLine, "Exit Code: ") {
				expectedIndex++
			} else {
				actualIndex++
			}
		} else if expectedLine == "*** IGNORE SINGLE LINE ***" {
			actualIndex++
			expectedIndex++
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

func EchoEnvVarToFile(envVar, fileName string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("Set-Content -Path %s -Value \"$env:%s\"", fileName, envVar)
	}

	return fmt.Sprintf("echo -n $%s > %s", envVar, fileName)
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

func StoppedCommandExitCode() int {
	if runtime.GOOS == "windows" {
		return 0
	}

	return 1
}

func ReturnExitCodeCommand(exitCode int) string {
	if runtime.GOOS == "windows" {
		return "powershell -Command 'exit 130'"
	}

	return "echo 'exit 130' | sh"
}

func ManuallyStoppedCommandExitCode() int {
	return 130
}

func CopyFile(src, dest string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("Copy-Item %s -Destination %s", src, dest)
	}

	return fmt.Sprintf("cp %s %s", src, dest)
}

func NestedEnvVarValue(name, rest string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("$env:%s%s", name, rest)
	}

	return fmt.Sprintf("$%s%s", name, rest)
}

func EchoBrokenUnicode() string {
	return "echo | awk '{ printf(\"%c%c%c%c%c\", 150, 150, 150, 150, 150) }'"
}
