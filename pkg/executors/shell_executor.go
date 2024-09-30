package executors

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	shell "github.com/semaphoreci/agent/pkg/shell"
	log "github.com/sirupsen/logrus"
)

type ShellExecutor struct {
	Executor
	Logger                  *eventlogger.Logger
	Shell                   *shell.Shell
	jobRequest              *api.JobRequest
	tmpDirectory            string
	hasSSHJumpPoint         bool
	shouldUpdateBashProfile bool
	cleanupAfterClose       []string
}

func NewShellExecutor(request *api.JobRequest, logger *eventlogger.Logger, selfHosted bool) *ShellExecutor {
	return &ShellExecutor{
		Logger:                  logger,
		jobRequest:              request,
		tmpDirectory:            os.TempDir(),
		hasSSHJumpPoint:         !selfHosted,
		shouldUpdateBashProfile: !selfHosted,
		cleanupAfterClose:       []string{},
	}
}

func (e *ShellExecutor) Prepare() int {
	if !e.hasSSHJumpPoint {
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
	sh, err := shell.NewShell(e.tmpDirectory)
	if err != nil {
		log.Debug(sh)
		return 1
	}

	e.Shell = sh

	err = e.Shell.Start()
	if err != nil {
		log.Error(err)
		return 1
	}

	return 0
}

func (e *ShellExecutor) envFileName() string {
	//
	// On Windows, we do not use the environment file at all during the job,
	// but we still need it to be created for debug sessions to work properly.
	// And, in order for users to be able to "source" the file in a PowerShell debug session,
	// the file has to be suffixed with the .ps1 suffix.
	//
	if runtime.GOOS == "windows" {
		return filepath.Join(e.tmpDirectory, fmt.Sprintf(".env-%d.ps1", time.Now().UnixNano()))
	}

	return filepath.Join(e.tmpDirectory, fmt.Sprintf(".env-%d", time.Now().UnixNano()))
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

	environment, err := shell.CreateEnvironment(envVars, hostEnvVars)
	if err != nil {
		exitCode = 1
		return exitCode
	}

	/*
	 * Create a temporary file containing all the environment variables.
	 * For Windows agents, we don't use this file for the job itself,
	 * since we keep track of the environment in-memory due to the lack of a PTY,
	 * But we still need the file to be created for debug sessions.
	 */
	envFileName := e.envFileName()
	e.cleanupAfterClose = append(e.cleanupAfterClose, envFileName)
	err = environment.ToFile(envFileName, func(name string) {
		e.Logger.LogCommandOutput(fmt.Sprintf("Exporting %s\n", name))
	})

	if err != nil {
		exitCode = 1
		return exitCode
	}

	/*
	 * In windows, no PTY is used, so we don't source the environment file.
	 * Instead, we keep track of the environment changes in the shell object itself.
	 */
	if runtime.GOOS == "windows" {
		e.Shell.Env.Append(environment, nil)
		exitCode = 0
		return exitCode
	}

	cmd := fmt.Sprintf("source %s", envFileName)
	exitCode = e.RunCommand(cmd, true, "")
	if exitCode != 0 {
		return exitCode
	}

	if e.shouldUpdateBashProfile {
		cmd = fmt.Sprintf("echo 'source %s' >> ~/.bash_profile", envFileName)
		exitCode = e.RunCommand(cmd, true, "")
		if exitCode != 0 {
			return exitCode
		}
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
		destPath := f.NormalizePath(homeDir)

		output := fmt.Sprintf("Injecting %s with file mode %s\n", destPath, f.Mode)
		e.Logger.LogCommandOutput(output)

		content, err := f.Decode()
		if err != nil {
			e.Logger.LogCommandOutput("Failed to decode the content of the file.\n")
			exitCode = 1
			return exitCode
		}

		parentDir := filepath.Dir(destPath)
		err = os.MkdirAll(parentDir, 0750)
		if err != nil {
			e.Logger.LogCommandOutput(fmt.Sprintf("Failed to create directory '%s': %v\n", parentDir, err))
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

		_, err = destFile.Write(content)
		if err != nil {
			e.Logger.LogCommandOutput(err.Error() + "\n")
			exitCode = 255
			break
		}

		fileMode, err := f.ParseMode()
		if err != nil {
			e.Logger.LogCommandOutput(err.Error() + "\n")
			exitCode = 1
			break
		}

		err = os.Chmod(destPath, fileMode)
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

func (e *ShellExecutor) GetOutputFromCommand(command string) (string, int) {
	out := bytes.Buffer{}
	p := e.Shell.NewProcessWithOutput(command, func(output string) {
		out.WriteString(output)
	})

	p.Run()

	return out.String(), p.ExitCode
}

func (e *ShellExecutor) RunCommand(command string, silent bool, alias string) int {
	return e.RunCommandWithOptions(CommandOptions{
		Command: command,
		Silent:  silent,
		Alias:   alias,
		Warning: "",
	})
}

func (e *ShellExecutor) RunCommandWithOptions(options CommandOptions) int {
	directive := options.Command
	if options.Alias != "" {
		directive = options.Alias
	}

	p := e.Shell.NewProcessWithOutput(options.Command, func(output string) {
		if !options.Silent {
			e.Logger.LogCommandOutput(output)
		}
	})

	if !options.Silent {
		e.Logger.LogCommandStarted(directive)

		if options.Alias != "" {
			e.Logger.LogCommandOutput(fmt.Sprintf("Running: %s\n", options.Command))
		}

		if options.Warning != "" {
			e.Logger.LogCommandOutput(fmt.Sprintf("Warning: %s\n", options.Warning))
		}
	}

	p.Run()

	if !options.Silent {
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

	err = e.Shell.Terminate()
	if err != nil {
		log.Errorf("Error terminating shell: %v", err)
		return 1
	}

	exitCode := e.Cleanup()
	if exitCode != 0 {
		log.Errorf("Error cleaning up executor resources: %v", err)
		return exitCode
	}

	log.Debug("Process killing finished without errors")
	return 0
}

func (e *ShellExecutor) Cleanup() int {
	for _, resource := range e.cleanupAfterClose {
		if err := os.Remove(resource); err != nil {
			log.Errorf("Error removing %s: %v\n", resource, err)
		}
	}

	return 0
}
