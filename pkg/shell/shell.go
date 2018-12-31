package shell

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"time"

	"github.com/semaphoreci/agent/pkg/executor"
)

type Shell struct {
	Executor                     executor.Executor
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

func NewShell(executor executor.Executor) Shell {
	// TODO: generate random separator
	separator := `ae415f5b966d4fb380e2c234ec9300ff`

	return Shell{
		commandSeparator:             separator,
		currentlyRunningCommandIndex: 0,
		commandStartRegex:            regexp.MustCompile(separator + " start"),
		commandEndRegex:              regexp.MustCompile(separator + " end " + `(\d)`),
		Executor:                     executor,
	}
}

func (s *Shell) Run(commands []string, handler ShellStreamHandler) error {
	s.injectJobScript(commands)

	reader, writter := io.Pipe()

	stdoutScanner := bufio.NewScanner(reader)

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

	tty, err := s.Executor.Run("bash /tmp/run/semaphore/job.sh")
	if err != nil {
		return err
	}

	io.Copy(writter, tty)

	return nil
}

func (s *Shell) injectJobScript(commands []string) {
	jobScript := `#!/bin/bash
set -euo pipefail
IFS=$'\n\t'
`
	for i, c := range commands {
		path := fmt.Sprintf("/tmp/run/semaphore/commands/%06d", i)
		s.Executor.AddFile(path, c)

		jobScript += fmt.Sprintf(`echo "%s start"`+"\n", s.commandSeparator)
		jobScript += fmt.Sprintf("source %s\n", path)
		jobScript += fmt.Sprintf("code=$?" + "\n")
		jobScript += fmt.Sprintf(`echo "%s end $code"`+"\n", s.commandSeparator)
	}

	s.Executor.AddFile("/tmp/run/semaphore/job.sh", jobScript)
}
