package shell

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

type Shell struct {
	BootCommand    *exec.Cmd
	TTY            *os.File
	ExitSignal     chan string
	NoPTY          bool
	Env            *Environment
	Cwd            string
	CurrentProcess *Process
}

func NewShell(bootCommand *exec.Cmd, noPTY bool) (*Shell, error) {
	exitChannel := make(chan string, 1)

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("error finding current working directory: %v", err)
	}

	return &Shell{
		BootCommand: bootCommand,
		ExitSignal:  exitChannel,
		NoPTY:       noPTY,
		Env:         &Environment{},
		Cwd:         cwd,
	}, nil
}

func (s *Shell) Start() error {
	if s.NoPTY {
		return nil
	}

	log.Debug("Starting stateful shell")

	tty, err := StartPTY(s.BootCommand)
	if err != nil {
		log.Errorf("Failed to start stateful shell: %v", err)
		return err
	}

	s.TTY = tty

	s.handleAbruptShellCloses()

	time.Sleep(1000)

	return s.silencePromptAndDisablePS1()
}

func (s *Shell) handleAbruptShellCloses() {
	//
	// If the Shell is abrupty closed, we are cleaning up, and sending out an
	// exit signal.
	//
	// Abrupt closes can be caused by:
	//
	//  - running exit 1 command
	//  - setting up set -e
	//  - setting up set -pipefail
	//  - killing the shell with kill <pid>
	//
	go func() {
		err := s.BootCommand.Wait()

		msg := "no exit message"
		if err != nil {
			msg = err.Error()
		}

		log.Debugf("Shell closed with %s. Closing associated TTY", msg)
		_ = s.TTY.Close()

		log.Debugf("Publishing an exit signal: %s", msg)
		s.ExitSignal <- msg
	}()
}

func (s *Shell) Read(buffer *([]byte)) (int, error) {
	done := make(chan bool, 1)

	var n int
	var err error

	go func() {
		n, err = s.TTY.Read(*buffer)
		done <- true
	}()

	select {
	case <-done:
		return n, err
	case <-s.ExitSignal:
		return 0, fmt.Errorf("Shell Closed")
	}
}

func (s *Shell) Write(instruction string) (int, error) {
	log.Debugf("Sending Instruction: %s", instruction)

	done := make(chan bool, 1)

	var n int
	var err error

	go func() {
		n, err = s.TTY.Write([]byte(instruction + "\n"))
		done <- true
	}()

	select {
	case <-done:
		return n, err
	case <-s.ExitSignal:
		return 0, fmt.Errorf("Shell Closed")
	}
}

func (s *Shell) silencePromptAndDisablePS1() error {
	everythingIsReadyMark := "87d140552e404df69f6472729d2b2c3"

	_, err := s.TTY.Write([]byte("export PS1=''\n"))
	if err != nil {
		return err
	}

	_, err = s.TTY.Write([]byte("stty -echo\n"))
	if err != nil {
		return err
	}

	_, err = s.TTY.Write([]byte("echo stty `stty -g` > /tmp/restore-tty\n"))
	if err != nil {
		return err
	}

	_, err = s.TTY.Write([]byte("cd ~\n"))
	if err != nil {
		return err
	}

	_, err = s.TTY.Write([]byte("echo '" + everythingIsReadyMark + "'\n"))
	if err != nil {
		return err
	}

	stdoutScanner := bufio.NewScanner(s.TTY)

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

	log.Debug("Waiting for initialization")

	for stdoutScanner.Scan() {
		text := stdoutScanner.Text()

		log.Debugf("(tty) %s\n", text)

		if strings.Contains(text, "executable file not found") {
			return fmt.Errorf(text)
		}

		if !strings.Contains(text, "echo") && strings.Contains(text, everythingIsReadyMark) {
			break
		}
	}

	return nil
}

func (s *Shell) NewProcess(command string) *Process {
	return NewProcess(
		Config{
			Command: command,
			Shell:   s,
			noPTY:   s.NoPTY,
		})
}

func (s *Shell) Close() error {
	if s.TTY != nil {
		err := s.TTY.Close()
		if err != nil {
			log.Errorf("Closing the TTY returned an error: %v", err)
			return err
		}
	}

	if s.BootCommand.Process != nil {
		err := s.BootCommand.Process.Kill()
		if err != nil && !errors.Is(err, os.ErrProcessDone) {
			log.Errorf("Process killing procedure returned an error %+v", err)
			return err
		}
	}

	return nil
}

func (s *Shell) Chdir(newCwd string) {
	if newCwd != s.Cwd {
		s.Cwd = newCwd
	}
}

func (s *Shell) UpdateEnvironment(newEnvironment *Environment) {
	s.Env = newEnvironment
}
