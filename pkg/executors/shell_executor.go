package executors

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	"github.com/semaphoreci/agent/pkg/osinfo"
	shell "github.com/semaphoreci/agent/pkg/shell"
	log "github.com/sirupsen/logrus"
)

type ShellExecutor struct {
	Executor
	Logger       *eventlogger.Logger
	Shell        *shell.Shell
	jobRequest   *api.JobRequest
	NoPTY        bool
	tmpDirectory string
}

func NewShellExecutor(request *api.JobRequest, logger *eventlogger.Logger, noPTY bool) *ShellExecutor {
	return &ShellExecutor{
		Logger:       logger,
		jobRequest:   request,
		NoPTY:        noPTY,
		tmpDirectory: os.TempDir(),
	}
}

func (e *ShellExecutor) Prepare() int {
	if runtime.GOOS == "windows" {
		return 0
	}

	return e.setUpSSHJumpPoint()
}

func (e *ShellExecutor) setUpSSHJumpPoint() int {
	err := InjectEntriesToAuthorizedKeys(e.jobRequest.SSHPublicKeys)

	if err != nil {
		log.Errorf("Failed to inject authorized keys: %+v", err)
		return 1
	}

	script := strings.Join([]string{
		"#!/bin/bash",
		"",
		"if [ $# -eq 0 ]; then",
		"  bash --login",
		"else",
		"  exec \"$@\"",
		"fi",
	}, "\n")

	err = SetUpSSHJumpPoint(script)
	if err != nil {
		log.Errorf("Failed to set up SSH jump point: %+v", err)
		return 1
	}

	return 0
}

func (e *ShellExecutor) Start() int {
	cmd := exec.Command("bash", "--login")

	shell, err := shell.NewShell(cmd, e.tmpDirectory, e.NoPTY)
	if err != nil {
		log.Debug(shell)
		return 1
	}

	e.Shell = shell

	err = e.Shell.Start()
	if err != nil {
		log.Error(err)
		return 1
	}

	return 0
}

func (e *ShellExecutor) ExportEnvVars(envVars []api.EnvVar, hostEnvVars []config.HostEnvVar) int {
	commandStartedAt := int(time.Now().Unix())
	directive := "Exporting environment variables"
	exitCode := 0

	e.Logger.LogCommandStarted(directive)

	defer func() {
		commandFinishedAt := int(time.Now().Unix())

		e.Logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)
	}()

	environment, err := shell.EnvFromAPI(envVars)
	if err != nil {
		exitCode = 1
		return exitCode
	}

	environment.Merge(hostEnvVars)

	if e.NoPTY {
		e.Shell.Env.Append(environment, func(name, value string) {
			e.Logger.LogCommandOutput(fmt.Sprintf("Exporting %s\n", name))
		})

		exitCode = 0
		return exitCode
	}

	envFileName := osinfo.FormDirPath(e.tmpDirectory, ".env")
	err = environment.ToFile(envFileName, func(name string) {
		e.Logger.LogCommandOutput(fmt.Sprintf("Exporting %s\n", name))
	})

	if err != nil {
		exitCode = 1
		return exitCode
	}

	exitCode = e.RunCommand(fmt.Sprintf("source %s", envFileName), true, "")
	if exitCode != 0 {
		return exitCode
	}

	exitCode = e.RunCommand(fmt.Sprintf("echo 'source %s' >> ~/.bash_profile", envFileName), true, "")
	if exitCode != 0 {
		return exitCode
	}

	return exitCode
}

func (e *ShellExecutor) InjectFiles(files []api.File) int {
	directive := "Injecting Files"
	commandStartedAt := int(time.Now().Unix())
	exitCode := 0

	e.Logger.LogCommandStarted(directive)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Errorf("Error finding home directory: %v\n", err)
		return 1
	}

	for _, f := range files {
		output := fmt.Sprintf("Injecting %s with file mode %s\n", f.Path, f.Mode)

		e.Logger.LogCommandOutput(output)

		content, err := f.Decode()
		if err != nil {
			e.Logger.LogCommandOutput("Failed to decode the content of the file.\n")
			exitCode = 1
			return exitCode
		}

		tmpPath := osinfo.FormDirPath(e.tmpDirectory, "file")

		// #nosec
		tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			e.Logger.LogCommandOutput(err.Error() + "\n")
			exitCode = 255
			break
		}

		_, err = tmpFile.Write(content)
		if err != nil {
			e.Logger.LogCommandOutput(err.Error() + "\n")
			exitCode = 255
			break
		}

		destPath := ""
		switch p := f.Path; {
		case p[0] == '/':
			destPath = f.Path
		case p[0] == '~':
			destPath = strings.ReplaceAll(p, "~", homeDir)
		default:
			destPath = fmt.Sprintf("%s/%s", homeDir, f.Path)
		}

		destPath = filepath.FromSlash(destPath)
		err = os.MkdirAll(path.Dir(destPath), 0644)
		if err != nil {
			e.Logger.LogCommandOutput(fmt.Sprintf("Failed to create directories for '%s': %v\n", destPath, err))
			exitCode = 1
			break
		}

		// #nosec
		destFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
		if err != nil {
			e.Logger.LogCommandOutput(fmt.Sprintf("Failed to create destination path '%s': %v\n", destPath, err))
			exitCode = 1
			break
		}

		_, err = tmpFile.Seek(0, io.SeekStart)
		if err != nil {
			e.Logger.LogCommandOutput(fmt.Sprintf("Failed to rewind '%s': %v\n", tmpPath, err))
			exitCode = 1
			break
		}

		_, err = io.Copy(destFile, tmpFile)
		if err != nil {
			e.Logger.LogCommandOutput(fmt.Sprintf("Failed to move '%s' to '%s': %v\n", tmpPath, destPath, err))
			exitCode = 1
			break
		}

		fileMode, err := strconv.ParseUint(f.Mode, 8, 32)
		if err != nil {
			e.Logger.LogCommandOutput(fmt.Sprintf("Bad file permission '%s' for '%s': %v\n", f.Mode, f.Path, err))
			exitCode = 1
			break
		}

		err = os.Chmod(destPath, fs.FileMode(fileMode))
		if err != nil {
			e.Logger.LogCommandOutput(fmt.Sprintf("Failed to set file mode '%s' for '%s': %v\n", f.Mode, destPath, err))
			exitCode = 1
			break
		}
	}

	commandFinishedAt := int(time.Now().Unix())

	e.Logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)

	return exitCode
}

func (e *ShellExecutor) RunCommand(command string, silent bool, alias string) int {
	directive := command
	if alias != "" {
		directive = alias
	}

	p := e.Shell.NewProcess(command)
	e.Shell.CurrentProcess = p

	if !silent {
		e.Logger.LogCommandStarted(directive)

		if alias != "" {
			e.Logger.LogCommandOutput(fmt.Sprintf("Running: %s\n", command))
		}
	}

	p.OnStdout(func(output string) {
		if !silent {
			e.Logger.LogCommandOutput(output)
		}
	})

	p.Run()

	if !silent {
		e.Logger.LogCommandFinished(directive, p.ExitCode, p.StartedAt, p.FinishedAt)
	}

	return p.ExitCode
}

func (e *ShellExecutor) Stop() int {
	log.Debug("Starting the process killing procedure")

	err := e.Shell.Close()
	if err != nil {
		log.Error(err)
	}

	err = e.Shell.CurrentProcess.Terminate()
	if err != nil {
		log.Errorf("Error terminating process: %v", err)
		return 1
	}

	log.Debug("Process killing finished without errors")
	return 0
}

func (e *ShellExecutor) Cleanup() int {
	return 0
}
