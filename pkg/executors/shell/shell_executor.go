package shell

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
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
	startMark := "87d140552e404df69f6472729d2b2c1"
	finishMark := "97d140552e404df69f6472729d2b2c2"

	commandEndRegex := regexp.MustCompile(finishMark + " " + `(\d)`)
	streamEvents := false

	e.stdin.Write([]byte("echo " + startMark + "; " + command + "; AGENT_CMD_RESULT=$?; echo \"" + finishMark + " $AGENT_CMD_RESULT\"; echo \"exit $AGENT_CMD_RESULT\"|sh\n"))

	stdoutScanner := bufio.NewScanner(e.tty)

	fmt.Println("[SHELL] Scan started")
	for stdoutScanner.Scan() {
		t := stdoutScanner.Text()
		fmt.Printf("[SHELL] (tty) %s\n", t)

		if strings.Contains(t, startMark) {
			streamEvents = true
			continue
		}

		if strings.Contains(t, finishMark) {
			streamEvents = false

			if match := commandEndRegex.FindStringSubmatch(t); len(match) == 2 {
				exitStatus, err := strconv.Atoi(match[1])
				if err != nil {
					log.Printf("[SHELL] Panic while parsing exit status, err: %+v", err)
					panic(err)
				}
				callback(fmt.Sprintf("Exit Status: %d", exitStatus))
			} else {
				panic("AAA")
			}

			break
		}

		if streamEvents {
			callback(t)
		}
	}
}

func (e *ShellExecutor) Stop() {

}

func (e *ShellExecutor) Cleanup() {

}
