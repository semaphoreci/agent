package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"regexp"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/kr/pty"
)

type Shell struct {
	currentlyRunningCommandIndex int
	commandSeparator             string
	commandStartRegex            *regexp.Regexp
	commandEndRegex              *regexp.Regexp
	terminal                     *exec.Cmd
}

type ShellStreamHandler func(interface{})

type CommandStartedShellEvent struct {
	Timestamp    int
	CommandIndex int
	Directive    string
}

type CommandOutputShellEvent struct {
	Timestamp    int
	CommandIndex int
	Output       string
}

type CommandFinishedShellEvent struct {
	Timestamp    int
	CommandIndex int
	ExitCode     int
	Directive    string
	StartedAt    int
	FinishedAt   int
}

func NewShell() Shell {
	// TODO: generate random separator
	separator := `ae415f5b966d4fb380e2c234ec9300ff`

	return Shell{
		commandSeparator:             separator,
		currentlyRunningCommandIndex: 0,
		commandStartRegex:            regexp.MustCompile(separator + " start"),
		commandEndRegex:              regexp.MustCompile(separator + " end " + `(\d)`),
	}
}

func (s *Shell) Run(jobRequest JobRequest, handler ShellStreamHandler) int {
	s.compileCommands(jobRequest)

	lastExecutedCommandReturnedExitCode := false
	lastCommandStartedAt := 0

	s.terminal = exec.Command("bash", "/tmp/run/semaphore/job.sh")
	reader, writter := io.Pipe()

	stdoutScanner := bufio.NewScanner(reader)

	go func() {
		for stdoutScanner.Scan() {
			text := stdoutScanner.Text()

			log.Printf("[SHELL] %s", text)

			if s.commandStartRegex.MatchString(text) {
				lastExecutedCommandReturnedExitCode = false
				lastCommandStartedAt = int(time.Now().Unix())

				log.Printf("[SHELL] Command Started Event")

				// command started
				handler(CommandStartedShellEvent{
					Timestamp:    int(time.Now().Unix()),
					CommandIndex: s.currentlyRunningCommandIndex,
					Directive:    jobRequest.Commands[s.currentlyRunningCommandIndex].Directive,
				})
			} else if match := s.commandEndRegex.FindStringSubmatch(text); len(match) == 2 {
				// command finished

				log.Printf("[SHELL] Command Finished Event")

				exitStatus, err := strconv.Atoi(match[1])
				if err != nil {
					log.Printf("[SHELL] Panic while parsing exit status, err: %+v", err)
					panic(err)
				}

				handler(CommandFinishedShellEvent{
					Timestamp:    int(time.Now().Unix()),
					CommandIndex: s.currentlyRunningCommandIndex,
					ExitCode:     exitStatus,
					StartedAt:    lastCommandStartedAt,
					FinishedAt:   int(time.Now().Unix()),
					Directive:    jobRequest.Commands[s.currentlyRunningCommandIndex].Directive,
				})

				s.currentlyRunningCommandIndex += 1
			} else {
				lastExecutedCommandReturnedExitCode = true

				log.Printf("[SHELL] Command Output Event")

				handler(CommandOutputShellEvent{
					Timestamp:    int(time.Now().Unix()),
					CommandIndex: s.currentlyRunningCommandIndex,
					Output:       stdoutScanner.Text() + "\n",
				})
			}
		}
	}()

	log.Printf("[SHELL] Starting stateful shell")
	f, err := pty.Start(s.terminal)
	if err != nil {
		log.Printf("[SHELL] Failed to start stateful shell")
		return 1
	}
	io.Copy(writter, f)

	exitCode := 1

	log.Printf("[SHELL] Waiting for stateful shell to close")
	if err := s.terminal.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			// The program has exited with an exit code != 0

			// This works on both Unix and Windows. Although package
			// syscall is generally platform dependent, WaitStatus is
			// defined for both Unix and Windows and in both cases has
			// an ExitStatus() method with the same signature.
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			} else {
				exitCode = 1
			}
		} else {
			// unknown error happened, setting exit code to 1
			exitCode = 1
		}
	} else {
		// program exited with exit code 0
		exitCode = 0
	}

	log.Printf("[SHELL] Final exit code is %d", exitCode)

	if lastExecutedCommandReturnedExitCode == false {
		handler(CommandFinishedShellEvent{
			Timestamp:    int(time.Now().Unix()),
			CommandIndex: s.currentlyRunningCommandIndex,
			ExitCode:     exitCode,
			StartedAt:    lastCommandStartedAt,
			FinishedAt:   int(time.Now().Unix()),
			Directive:    jobRequest.Commands[s.currentlyRunningCommandIndex].Directive,
		})
	}

	return exitCode
}

func (s *Shell) Stop() error {
	return s.terminal.Process.Kill()
}

func (s *Shell) compileCommands(jobRequest JobRequest) error {
	os.RemoveAll("/tmp/run/semaphore")
	os.MkdirAll("/tmp/run/semaphore/commands", os.ModePerm)
	os.MkdirAll("/tmp/run/semaphore/files", os.ModePerm)

	env := ""

	for _, e := range jobRequest.EnvVars {
		value, _ := base64.StdEncoding.DecodeString(e.Value)

		env += fmt.Sprintf("export %s='%s'\n", e.Name, value)
	}

	ioutil.WriteFile("/tmp/run/semaphore/.env", []byte(env), 0644)

	jobScript := `#!/bin/bash
set -eo pipefail
IFS=$'\n\t'

cd ~

# source env vars into current session
source /tmp/run/semaphore/.env

# make sure that env vars are also exported in new sessions (for example ssh sessions)
echo 'source /tmp/run/semaphore/.env' >> ~/.bash_profile
`

	for i, f := range jobRequest.Files {
		tmpPath := fmt.Sprintf("/tmp/run/semaphore/files/%06d", i)
		content, _ := base64.StdEncoding.DecodeString(f.Content)

		ioutil.WriteFile(tmpPath, []byte(content), 0644)

		destPath := f.Path

		// if the path is not-absolute, it will be relative to home path
		if destPath[0] == '/' {
			destPath = UserHomeDir() + "/" + destPath
		}

		jobScript += fmt.Sprintf("mkdir -p %s\n", path.Dir(destPath))
		jobScript += fmt.Sprintf("cp %s %s\n", tmpPath, destPath)
	}

	for i, c := range jobRequest.Commands {
		path := fmt.Sprintf("/tmp/run/semaphore/commands/%06d", i)

		err := ioutil.WriteFile(path, []byte(c.Directive), 0644)

		jobScript += fmt.Sprintf(`echo "%s start"`+"\n", s.commandSeparator)
		jobScript += fmt.Sprintf("source %s\n", path)
		jobScript += fmt.Sprintf("code=$?" + "\n")
		jobScript += fmt.Sprintf(`echo "%s end $code"`+"\n", s.commandSeparator)

		check(err)
	}

	return ioutil.WriteFile("/tmp/run/semaphore/job.sh", []byte(jobScript), 0644)
}

func UserHomeDir() string {
	if runtime.GOOS == "windows" {
		home := os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		return home
	}
	return os.Getenv("HOME")
}
