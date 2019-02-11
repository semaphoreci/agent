package shell

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
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

	log.Println("[SHELL] Waiting for initialization")

	for stdoutScanner.Scan() {
		text := stdoutScanner.Text()

		log.Printf("[SHELL] (tty) %s\n", text)
		if !strings.Contains(text, "echo") && strings.Contains(text, everythingIsReadyMark) {
			break
		}
	}

	log.Println("[SHELL] Initialization complete")
}

func (e *ShellExecutor) ExportEnvVars(envVars []executors.EnvVar, callback executors.EventHandler) {
	commandStartedAt := int(time.Now().Unix())
	directive := fmt.Sprintf("Exporting environment variables")

	callback(executors.NewCommandStartedEvent(directive))

	envFile := ""

	for _, e := range envVars {
		callback(executors.NewCommandOutputEvent(fmt.Sprintf("Exporting %s", e.Name)))

		envFile += fmt.Sprintf("export %s='%s'\n", e.Name, e.Value)
	}

	err := ioutil.WriteFile("/tmp/.env", []byte(envFile), 0644)

	exitCode := 0
	if err != nil {
		exitCode = 1
	}

	e.RunCommand("source /tmp/.env", executors.DevNullEventHandler)

	// TODO: Add source .env from bash profile for SSH sessions

	commandFinishedAt := int(time.Now().Unix())
	callback(executors.NewCommandFinishedEvent(
		directive,
		exitCode,
		commandStartedAt,
		commandFinishedAt,
	))
}

func (e *ShellExecutor) InjectFile(path string, content string, mode string, callback executors.EventHandler) {
	commandStartedAt := int(time.Now().Unix())

	directive := fmt.Sprintf("Injecting File %s with file mode %s", path, mode)

	callback(executors.NewCommandStartedEvent(directive))

	err := ioutil.WriteFile(path, []byte(content), 0644)

	exitCode := 0

	if err != nil {
		exitCode = 1
	}

	commandFinishedAt := int(time.Now().Unix())

	callback(executors.NewCommandFinishedEvent(
		directive,
		exitCode,
		commandStartedAt,
		commandFinishedAt,
	))
}

func (e *ShellExecutor) RunCommand(command string, callback executors.EventHandler) {
	cmdFilePath := "/tmp/current-agent-cmd"
	startMark := "87d140552e404df69f6472729d2b2c1"
	finishMark := "97d140552e404df69f6472729d2b2c2"

	commandEndRegex := regexp.MustCompile(finishMark + " " + `(\d)`)
	streamEvents := false

	//
	// Multiline commands don't work very well with the start/finish marker scheme.
	// To circumvent this, we are storing the command in a file
	//
	ioutil.WriteFile(cmdFilePath, []byte(command), 0644)

	// Constructing command with start and end markers:
	//
	// 1. display START marker
	// 2. execute the command file by sourcing it
	// 3. save the original exit status
	// 4. display the END marker with the exit status
	// 5. return the original exit status to the caller
	//

	commandWithStartAndEndMarkers := strings.Join([]string{
		fmt.Sprintf("echo '%s'", startMark),
		fmt.Sprintf("source %s", cmdFilePath),
		"AGENT_CMD_RESULT=$?",
		fmt.Sprintf(`echo "%s $AGENT_CMD_RESULT"`, finishMark),
		"echo \"exit $AGENT_CMD_RESULT\"|sh\n",
	}, ";")

	e.stdin.Write([]byte(commandWithStartAndEndMarkers))

	commandStartedAt := int(time.Now().Unix())

	stdoutScanner := bufio.NewScanner(e.tty)

	log.Println("[SHELL] Scan started")
	for stdoutScanner.Scan() {
		t := stdoutScanner.Text()
		log.Printf("[SHELL] (tty) %s\n", t)

		if strings.Contains(t, startMark) {
			streamEvents = true

			callback(executors.NewCommandStartedEvent(command))

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

				commandFinishedAt := int(time.Now().Unix())

				callback(executors.NewCommandFinishedEvent(
					command,
					exitStatus,
					commandStartedAt,
					commandFinishedAt,
				))
			} else {
				panic("AAA")
			}

			break
		}

		if streamEvents {
			callback(executors.NewCommandOutputEvent(t))
		}
	}
}

func (e *ShellExecutor) Stop() {

}

func (e *ShellExecutor) Cleanup() {

}
