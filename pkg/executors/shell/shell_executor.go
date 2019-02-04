package shell

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
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
	e.terminal = exec.Command("bash")
}

func (e *ShellExecutor) Start() error {
	log.Printf("[SHELL] Starting stateful shell")

	tty, err := pty.Start(e.terminal)
	if err != nil {
		log.Printf("[SHELL] Failed to start stateful shell")
		return err
	}

	e.stdin = tty
	e.tty = tty

	time.Sleep(1000)

	e.silencePromptAndDisablePS1()

	return nil
}

func (e *ShellExecutor) silencePromptAndDisablePS1() {
	everythingIsReadyMark := "87d140552e404df69f6472729d2b2c3"

	e.stdin.Write([]byte("export PS1=''\n"))
	e.stdin.Write([]byte("stty -echo\n"))
	e.stdin.Write([]byte("echo '" + everythingIsReadyMark + "'\n"))

	stdoutScanner := bufio.NewScanner(e.tty)

	//
	// At this point, the terminal is still echoing the output back to stdout
	// we ignore the entered command, and look for the magic mark in the output
	//
	// Example content of output before ready mark:
	//
	//   export PS1=''
	//   stty -echo
	//   echo + '87d140552e404df69f6472729d2b2c3'
	//   vagrant@boxbox:~/code/agent/pkg/executors/shell$ export PS1=''
	//   stty -echo
	//   echo '87d140552e404df69f6472729d2b2c3'
	//

	// We wait until marker is displayed in the output

	fmt.Println("[SHELL] Waiting for initialization")

	for stdoutScanner.Scan() {
		text := stdoutScanner.Text()

		fmt.Printf("[SHELL] (tty) %s\n", text)
		if !strings.Contains(text, "echo") && strings.Contains(text, everythingIsReadyMark) {
			break
		}
	}

	fmt.Println("[SHELL] Initialization complete")
}

func (e *ShellExecutor) ExportEnvVar() {

}

func (e *ShellExecutor) InjectFile() {

}

func (e *ShellExecutor) RunCommand(command string, callback executors.EventHandler) {
	e.stdin.Write([]byte(command + "; echo 123" + "\n"))

	reader, writter := io.Pipe()
	stdoutScanner := bufio.NewScanner(reader)

	go func() {
		io.Copy(writter, e.tty)
	}()

	for stdoutScanner.Scan() {
		t := stdoutScanner.Text()
		fmt.Printf("[SHELL] (stdout) %s\n", t)

		if !strings.Contains(t, "123") {
			break
		}
	}
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
