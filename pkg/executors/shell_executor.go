package executors

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"path"
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
	Logger       *eventlogger.Logger
	Shell        *shell.Shell
	jobRequest   *api.JobRequest
	tmpDirectory string
	NoPTY        bool
}

func NewShellExecutor(request *api.JobRequest, logger *eventlogger.Logger, noPTY bool) *ShellExecutor {
	return &ShellExecutor{
		Logger:       logger,
		jobRequest:   request,
		tmpDirectory: "/tmp",
		NoPTY:        noPTY,
	}
}

func (e *ShellExecutor) Prepare() int {
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

	envFile := ""

	for _, env := range envVars {
		e.Logger.LogCommandOutput(fmt.Sprintf("Exporting %s\n", env.Name))

		value, err := env.Decode()

		if err != nil {
			exitCode = 1
			return exitCode
		}

		envFile += fmt.Sprintf("export %s=%s\n", env.Name, ShellQuote(string(value)))
	}

	for _, env := range hostEnvVars {
		e.Logger.LogCommandOutput(fmt.Sprintf("Exporting %s\n", env.Name))
		envFile += fmt.Sprintf("export %s=%s\n", env.Name, ShellQuote(env.Value))
	}

	// #nosec
	err := ioutil.WriteFile("/tmp/.env", []byte(envFile), 0644)

	if err != nil {
		exitCode = 1
		return exitCode
	}

	exitCode = e.RunCommand("source /tmp/.env", true, "")
	if exitCode != 0 {
		return exitCode
	}

	exitCode = e.RunCommand("echo 'source /tmp/.env' >> ~/.bash_profile", true, "")
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

	for _, f := range files {
		output := fmt.Sprintf("Injecting %s with file mode %s\n", f.Path, f.Mode)

		e.Logger.LogCommandOutput(output)

		content, err := f.Decode()
		if err != nil {
			e.Logger.LogCommandOutput("Failed to decode the content of the file.\n")
			exitCode = 1
			return exitCode
		}

		tmpPath := fmt.Sprintf("%s/file", e.tmpDirectory)

		// #nosec
		err = ioutil.WriteFile(tmpPath, []byte(content), 0644)
		if err != nil {
			e.Logger.LogCommandOutput(err.Error() + "\n")
			exitCode = 255
			break
		}

		destPath := ""

		if f.Path[0] == '/' || f.Path[0] == '~' {
			destPath = f.Path
		} else {
			destPath = "~/" + f.Path
		}

		cmd := fmt.Sprintf("mkdir -p %s", path.Dir(destPath))
		exitCode = e.RunCommand(cmd, true, "")
		if exitCode != 0 {
			output := fmt.Sprintf("Failed to create destination path %s", destPath)
			e.Logger.LogCommandOutput(output + "\n")
			break
		}

		cmd = fmt.Sprintf("cp %s %s", tmpPath, destPath)
		exitCode = e.RunCommand(cmd, true, "")
		if exitCode != 0 {
			output := fmt.Sprintf("Failed to move to destination path %s %s", tmpPath, destPath)
			e.Logger.LogCommandOutput(output + "\n")
			break
		}

		cmd = fmt.Sprintf("chmod %s %s", f.Mode, destPath)
		exitCode = e.RunCommand(cmd, true, "")
		if exitCode != 0 {
			output := fmt.Sprintf("Failed to set file mode to %s", f.Mode)
			e.Logger.LogCommandOutput(output + "\n")
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

	log.Debug("Process killing finished without errors")

	return 0
}

func (e *ShellExecutor) Cleanup() int {
	return 0
}
