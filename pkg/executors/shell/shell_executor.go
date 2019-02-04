package shell

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"

	pty "github.com/kr/pty"
	executors "github.com/semaphoreci/agent/pkg/executors"
)

type ShellExecutor struct {
	eventHandler  *executors.EventHandler
	terminal      *exec.Cmd
	tty           *os.File
	stdin         io.Writer
	stdoutScanner *bufio.Scanner
}

func NewShellExecutor() *ShellExecutor {
	return &ShellExecutor{}
}

func (e *ShellExecutor) Prepare() {

}

func (e *ShellExecutor) Start() error {
	// e.stdoutScanner = bufio.NewScanner(reader)

	log.Printf("[SHELL] Starting stateful shell")

	e.terminal = exec.Command("bash")

	tty, err := pty.Start(e.terminal)
	if err != nil {
		log.Printf("[SHELL] Failed to start stateful shell")
		return err
	}

	e.stdin = tty
	e.tty = tty

	return nil
}

func (e *ShellExecutor) ExportEnvVar() {

}

func (e *ShellExecutor) InjectFile() {

}

func (e *ShellExecutor) RunCommand(command string, callback executors.EventHandler) {
	e.stdin.Write([]byte(command + "\n"))

	reader, writter := io.Pipe()
	stdoutScanner := bufio.NewScanner(reader)

	go func() {
		io.Copy(writter, e.tty)
	}()

	go func() {
		for stdoutScanner.Scan() {
			fmt.Printf("[SHELL] %s\n", stdoutScanner.Text())
			time.Sleep(1000)
		}
	}()

	callback("a")
}

// func (s *Shell) compileCommands(jobRequest JobRequest) error {
// 	os.RemoveAll("/tmp/run/semaphore")
// 	os.MkdirAll("/tmp/run/semaphore/commands", os.ModePerm)
// 	os.MkdirAll("/tmp/run/semaphore/files", os.ModePerm)

// 	jobScript := `#!/bin/bash
// set -euo pipefail
// IFS=$'\n\t'
// `

// 	for _, e := range jobRequest.EnvVars {
// 		value, _ := base64.StdEncoding.DecodeString(e.Value)

// 		jobScript += fmt.Sprintf("export %s=%s\n", e.Name, value)
// 	}

// 	for i, f := range jobRequest.Files {
// 		tmpPath := fmt.Sprintf("/tmp/run/semaphore/files/%06d", i)
// 		content, _ := base64.StdEncoding.DecodeString(f.Content)

// 		ioutil.WriteFile(tmpPath, []byte(content), 0644)

// 		jobScript += fmt.Sprintf("mkdir -p %s\n", path.Dir(f.Path))
// 		jobScript += fmt.Sprintf("cp %s %s\n", tmpPath, f.Path)
// 	}

// 	for i, c := range jobRequest.Commands {
// 		path := fmt.Sprintf("/tmp/run/semaphore/commands/%06d", i)

// 		err := ioutil.WriteFile(path, []byte(c.Directive), 0644)

// 		jobScript += fmt.Sprintf(`echo "%s start"`+"\n", s.commandSeparator)
// 		jobScript += fmt.Sprintf("source %s\n", path)
// 		jobScript += fmt.Sprintf("code=$?" + "\n")
// 		jobScript += fmt.Sprintf(`echo "%s end $code"`+"\n", s.commandSeparator)

// 		check(err)
// 	}

// 	return ioutil.WriteFile("/tmp/run/semaphore/job.sh", []byte(jobScript), 0644)
// }

func (e *ShellExecutor) Stop() {

}

func (e *ShellExecutor) Cleanup() {

}
