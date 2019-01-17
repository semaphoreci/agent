package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"time"

	"github.com/kr/pty"
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
		commandStartRegex:            regexp.MustCompile(separator + " start"),
		commandEndRegex:              regexp.MustCompile(separator + " end " + `(\d)`),
	}
}

func (s *Shell) Run(jobRequest JobRequest, handler ShellStreamHandler) error {
	s.compileCommands(jobRequest)

	cmd := exec.Command("bash", "/tmp/run/semaphore/job.sh")

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
					Command:      jobRequest.Commands[s.currentlyRunningCommandIndex].Directive,
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

	f, err := pty.Start(cmd)
	if err != nil {
		return err
	}

	io.Copy(writter, f)

	return cmd.Wait()
}

func (s *Shell) compileCommands(jobRequest JobRequest) error {
	os.RemoveAll("/tmp/run/semaphore")
	os.MkdirAll("/tmp/run/semaphore/commands", os.ModePerm)
	os.MkdirAll("/tmp/run/semaphore/files", os.ModePerm)

	jobScript := `#!/bin/bash
set -euo pipefail
IFS=$'\n\t'
`

	for _, e := range jobRequest.EnvVars {
		value, _ := base64.StdEncoding.DecodeString(e.Value)

		jobScript += fmt.Sprintf("export %s=%s\n", e.Name, value)
	}

	for i, f := range jobRequest.Files {
		tmpPath := fmt.Sprintf("/tmp/run/semaphore/files/%06d", i)
		content, _ := base64.StdEncoding.DecodeString(f.Content)

		ioutil.WriteFile(tmpPath, []byte(content), 0644)

		jobScript += fmt.Sprintf("mkdir -p %s\n", path.Dir(f.Path))
		jobScript += fmt.Sprintf("cp %s %s\n", tmpPath, f.Path)
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
