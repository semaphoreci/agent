package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

type Shell struct {
	currentlyRunningCommandIndex int
	commandSeparator             string
	commandStartRegex            *regexp.Regexp
	commandEndRegex              *regexp.Regexp
}

type ShellStreamHandler func(interface{})

type CommandStartedShellEvent struct {
	Timestamp    int
	CommandIndex int
	Command      string
}

type CommandOutputShellEvent struct {
	Timestamp    int
	CommandIndex int
	Output       string
}

type CommandFinishedShellEvent struct {
	Timestamp    int
	CommandIndex int
	ExitStatus   int
	Duration     int
}

func NewShell() Shell {
	// TODO: generate random separator
	separator := `ae415f5b966d4fb380e2c234ec9300ff`

	return Shell{
		commandSeparator:             separator,
		currentlyRunningCommandIndex: 0,
		commandStartRegex:            regexp.MustCompile(separator + " start$"),
		commandEndRegex:              regexp.MustCompile(separator + " end " + `(\d)` + "$"),
	}
}

func (s *Shell) Run(commands []string, handler ShellStreamHandler) error {
	s.compileCommands(commands)

	cmd := exec.Command("bash", "-c", "docker-compose -f /tmp/dc1 run -v /tmp/run/semaphore:/tmp/run/semaphore main bash /tmp/run/semaphore/job.sh")

	cmdStdoutReader, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	cmdStderrReader, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	stdoutScanner := bufio.NewScanner(cmdStdoutReader)
	stderrScanner := bufio.NewScanner(cmdStderrReader)

	go func() {
		for stdoutScanner.Scan() {
			text := stdoutScanner.Text()

			if s.commandStartRegex.MatchString(text) {
				// command started
				handler(CommandStartedShellEvent{
					Timestamp:    int(time.Now().Unix()),
					CommandIndex: s.currentlyRunningCommandIndex,
					Command:      commands[s.currentlyRunningCommandIndex],
				})
			} else if match := s.commandEndRegex.FindStringSubmatch(text); len(match) == 2 {
				// command finished

				exitStatus, err := strconv.Atoi(match[1])
				if err != nil {
					panic(err)
				}

				handler(CommandFinishedShellEvent{
					Timestamp:    int(time.Now().Unix()),
					CommandIndex: s.currentlyRunningCommandIndex,
					ExitStatus:   exitStatus,
					Duration:     0,
				})

				s.currentlyRunningCommandIndex += 1
			} else {
				handler(CommandOutputShellEvent{
					Timestamp:    int(time.Now().Unix()),
					CommandIndex: s.currentlyRunningCommandIndex,
					Output:       stdoutScanner.Text(),
				})
			}
		}
	}()

	go func() {
		for stderrScanner.Scan() {
			handler(CommandOutputShellEvent{
				Timestamp:    int(time.Now().Unix()),
				CommandIndex: s.currentlyRunningCommandIndex,
				Output:       stderrScanner.Text(),
			})
		}
	}()

	err = cmd.Start()
	if err != nil {
		return err
	}

	return cmd.Wait()
}

func (s *Shell) compileCommands(commands []string) error {
	os.RemoveAll("/tmp/run/semaphore")
	os.MkdirAll("/tmp/run/semaphore/commands", os.ModePerm)

	jobScript := `#!/bin/bash
set -euo pipefail
IFS=$'\n\t'
`

	for i, c := range commands {
		path := fmt.Sprintf("/tmp/run/semaphore/commands/%06d", i)

		err := ioutil.WriteFile(path, []byte(c), 0644)

		jobScript += fmt.Sprintf(`echo "%s start"`+"\n", s.commandSeparator)
		jobScript += fmt.Sprintf("source %s\n", path)
		jobScript += fmt.Sprintf("code=$?" + "\n")
		jobScript += fmt.Sprintf(`echo "%s end $code"`+"\n", s.commandSeparator)

		check(err)
	}

	return ioutil.WriteFile("/tmp/run/semaphore/job.sh", []byte(jobScript), 0644)
}
