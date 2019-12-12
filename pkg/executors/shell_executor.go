package executors

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	pty "github.com/kr/pty"
	api "github.com/semaphoreci/agent/pkg/api"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
)

type ShellExecutor struct {
	Executor

	Logger        eventlogger.EventLogger
	jobRequest    *api.JobRequest
	terminal      *exec.Cmd
	tty           *os.File
	stdin         io.Writer
	stdoutScanner *bufio.Scanner
	tmpDirectory  string
}

func NewShellExecutor(request *api.JobRequest, logger eventlogger.EventLogger) *ShellExecutor {
	return &ShellExecutor{
		Logger:       logger,
		jobRequest:   request,
		tmpDirectory: "/tmp",
	}
}

func (e *ShellExecutor) Prepare() int {
	e.terminal = exec.Command("bash", "--login")

	return e.setUpSSHJumpPoint()
}

func (e *ShellExecutor) setUpSSHJumpPoint() int {
	err := InjectEntriesToAuthorizedKeys(e.jobRequest.SSHPublicKeys)

	if err != nil {
		log.Printf("Failed to inject authorized keys: %+v", err)
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
		log.Printf("Failed to set up SSH jump point: %+v", err)
		return 1
	}

	return 0
}

func (e *ShellExecutor) Start(EventHandler) int {
	log.Printf("Starting stateful shell")

	tty, err := pty.Start(e.terminal)
	if err != nil {
		log.Printf("Failed to start stateful shell")
		return 1
	}

	e.stdin = tty
	e.tty = tty

	time.Sleep(1000)

	e.silencePromptAndDisablePS1()

	return 0
}

func (e *ShellExecutor) silencePromptAndDisablePS1() {
	everythingIsReadyMark := "87d140552e404df69f6472729d2b2c3"

	e.stdin.Write([]byte("export PS1=''\n"))
	e.stdin.Write([]byte("stty -echo\n"))
	e.stdin.Write([]byte("echo stty `stty -g` > /tmp/restore-tty\n"))
	e.stdin.Write([]byte("cd ~\n"))
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

	log.Println("Waiting for initialization")

	for stdoutScanner.Scan() {
		text := stdoutScanner.Text()

		log.Printf("(tty) %s\n", text)
		if !strings.Contains(text, "echo") && strings.Contains(text, everythingIsReadyMark) {
			break
		}
	}

	log.Println("Initialization complete")
}

func (e *ShellExecutor) ExportEnvVars(envVars []api.EnvVar) int {
	commandStartedAt := int(time.Now().Unix())
	directive := fmt.Sprintf("Exporting environment variables")
	exitCode := 0

	e.Logger.LogCommandStarted(directive)

	defer func() {
		commandFinishedAt := int(time.Now().Unix())

		e.Logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandStartedAt)
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

	err := ioutil.WriteFile("/tmp/.env", []byte(envFile), 0644)

	if err != nil {
		exitCode = 1
		return exitCode
	}

	exitCode = e.RunCommand("source /tmp/.env", true)
	if exitCode != 0 {
		return exitCode
	}

	exitCode = e.RunCommand("echo 'source /tmp/.env' >> ~/.bash_profile", true)
	if exitCode != 0 {
		return exitCode
	}

	return exitCode
}

func (e *ShellExecutor) InjectFiles(files []api.File) int {
	directive := fmt.Sprintf("Injecting Files")
	commandStartedAt := int(time.Now().Unix())
	exitCode := 0

	e.Logger.LogCommandStarted(directive)

	for _, f := range files {
		output := fmt.Sprintf("Injecting %s with file mode %s\n", f.Path, f.Mode)

		e.Logger.LogCommandOutput(output)

		content, err := f.Decode()

		if err != nil {
			e.Logger.LogCommandOutput("Failed to decode content of file.\n")
			exitCode = 1
			return exitCode
		}

		tmpPath := fmt.Sprintf("%s/file", e.tmpDirectory)

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
		exitCode = e.RunCommand(cmd, true)
		if exitCode != 0 {
			output := fmt.Sprintf("Failed to create destination path %s", destPath)
			e.Logger.LogCommandOutput(output + "\n")
			break
		}

		cmd = fmt.Sprintf("cp %s %s", tmpPath, destPath)
		exitCode = e.RunCommand(cmd, true)
		if exitCode != 0 {
			output := fmt.Sprintf("Failed to move to destination path %s %s", tmpPath, destPath)
			e.Logger.LogCommandOutput(output + "\n")
			break
		}

		cmd = fmt.Sprintf("chmod %s %s", f.Mode, destPath)
		exitCode = e.RunCommand(cmd, true)
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

func (e *ShellExecutor) RunCommand(command string, silent bool) int {
	var err error

	log.Printf("Running command: %s", command)

	cmdFilePath := "/tmp/current-agent-cmd"
	restoreTtyMark := "97d140552e404df69f6472729d2b2c1"
	startMark := "87d140552e404df69f6472729d2b2c1"
	finishMark := "97d140552e404df69f6472729d2b2c2"

	commandStartedAt := int(time.Now().Unix())
	exitCode := 1

	defer func() {
		commandFinishedAt := int(time.Now().Unix())

		if !silent {
			e.Logger.LogCommandFinished(command, exitCode, commandStartedAt, commandFinishedAt)
		}
	}()

	commandEndRegex := regexp.MustCompile(finishMark + " " + `(\d)`)
	streamEvents := false

	restoreTtyCmd := "source /tmp/restore-tty; echo " + restoreTtyMark + "\n"

	// restore a sane STTY interface
	ioutil.WriteFile(cmdFilePath, []byte(restoreTtyCmd), 0644)
	e.stdin.Write([]byte("source " + cmdFilePath + "\n"))

	ScanLines(e.tty, func(line string) bool {
		log.Printf("(tty-restore) %s\n", line)

		if strings.Contains(line, restoreTtyMark) {
			return false
		}

		return true
	})

	//
	// Multiline commands don't work very well with the start/finish marker scheme.
	// To circumvent this, we are storing the command in a file
	//
	err = ioutil.WriteFile(cmdFilePath, []byte(command), 0644)

	if err != nil {
		if !silent {
			e.Logger.LogCommandStarted(command)
			e.Logger.LogCommandOutput(fmt.Sprintf("Failed to run command: %+v\n", err))
		}

		return 1
	}

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

	log.Println("Scan started")

	err = ScanLines(e.tty, func(line string) bool {
		log.Printf("(tty) %s\n", line)

		if strings.Contains(line, startMark) {
			log.Printf("Detected command start")
			streamEvents = true

			if !silent {
				e.Logger.LogCommandStarted(command)
			}

			return true
		}

		if strings.Contains(line, finishMark) {
			log.Printf("Detected command end")

			finalOutputPart := strings.Split(line, finishMark)

			// if there is anything else other than the command end marker
			// print it to the user
			if finalOutputPart[0] != "" {
				if !silent {
					e.Logger.LogCommandOutput(finalOutputPart[0] + "\n")
				}
			}

			streamEvents = false

			if match := commandEndRegex.FindStringSubmatch(line); len(match) == 2 {
				log.Printf("Parsing exit status succedded")

				exitCode, err = strconv.Atoi(match[1])

				if err != nil {
					log.Printf("Panic while parsing exit status, err: %+v", err)

					e.Logger.LogCommandOutput("Failed to read command exit code\n")
				}

				log.Printf("Setting exit code to %d", exitCode)
			} else {
				log.Printf("Failed to parse exit status")

				exitCode = 1
				e.Logger.LogCommandOutput("Failed to read command exit code\n")
			}

			log.Printf("Stopping scanner")
			return false
		}

		if streamEvents {
			e.Logger.LogCommandOutput(line + "\n")
		}

		return true
	})

	return exitCode
}

func (e *ShellExecutor) Stop() int {
	log.Println("Starting the process killing procedure")

	err := e.terminal.Process.Kill()

	if err != nil {
		log.Printf("Process killing procedure returned an erorr %+v\n", err)
		return 0
	}

	log.Printf("Process killing finished without errors")

	return 0
}

func (e *ShellExecutor) Cleanup() int {
	return 0
}
