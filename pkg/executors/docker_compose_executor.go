package executors

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
	"strconv"
	"strings"
	"time"

	pty "github.com/kr/pty"
	api "github.com/semaphoreci/agent/pkg/api"
)

type DockerComposeExecutor struct {
	Executor

	tmpDirectory              string
	dockerConfiguration       api.Compose
	dockerComposeManifestPath string
	eventHandler              *EventHandler
	terminal                  *exec.Cmd
	tty                       *os.File
	stdin                     io.Writer
	stdoutScanner             *bufio.Scanner
}

func NewDockerComposeExecutor(dockerConfiguration api.Compose) *DockerComposeExecutor {
	return &DockerComposeExecutor{
		dockerConfiguration:       dockerConfiguration,
		dockerComposeManifestPath: "/tmp/docker-compose.yml",
		tmpDirectory:              "/tmp/agent-temp-directory", // make a better random name
	}
}

func (e *DockerComposeExecutor) Prepare() int {
	err := os.MkdirAll(e.tmpDirectory, os.ModePerm)

	if err != nil {
		return 1
	}

	compose := ConstructDockerComposeFile(e.dockerConfiguration)
	log.Println("Compose File:")
	log.Println(compose)

	ioutil.WriteFile(e.dockerComposeManifestPath, []byte(compose), 0644)

	return 0
}

func (e *DockerComposeExecutor) Start(callback EventHandler) int {
	exitCode := e.pullDockerImages(callback)

	if exitCode != 0 {
		log.Printf("[SHELL] Failed to pull images")
		return exitCode
	}

	log.Printf("[SHELL] Starting stateful shell")

	e.terminal = exec.Command(
		"docker-compose",
		"-f",
		e.dockerComposeManifestPath,
		"run",
		"-v",
		"/var/run/docker.sock:/var/run/docker.sock",
		"-v",
		fmt.Sprintf("%s:%s:ro", e.tmpDirectory, e.tmpDirectory),
		"main",
		"bash",
	)

	tty, err := pty.Start(e.terminal)
	if err != nil {
		log.Printf("[SHELL] Failed to start stateful shell err: %+v", err)
		return 1
	}

	e.stdin = tty
	e.tty = tty

	time.Sleep(1000)

	e.silencePromptAndDisablePS1()

	return 0
}

func (e *DockerComposeExecutor) pullDockerImages(callback EventHandler) int {
	log.Printf("[SHELL] Pulling docker images")
	directive := "Pulling docker images..."
	commandStartedAt := int(time.Now().Unix())

	callback(NewCommandStartedEvent(directive))

	cmd := exec.Command(
		"docker-compose",
		"-f",
		e.dockerComposeManifestPath,
		"pull")

	tty, err := pty.Start(cmd)
	if err != nil {
		log.Printf("[SHELL] Failed to initialize docker pull, err: %+v", err)
		return 1
	}

	ScanLines(tty, func(line string) bool {
		log.Printf("[SHELL] (tty) %s\n", line)

		callback(NewCommandOutputEvent(line))

		return true
	})

	commandFinishedAt := int(time.Now().Unix())

	callback(NewCommandFinishedEvent(
		directive,
		0,
		commandStartedAt,
		commandFinishedAt,
	))

	return 0
}

func (e *DockerComposeExecutor) silencePromptAndDisablePS1() {
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

func (e *DockerComposeExecutor) ExportEnvVars(envVars []api.EnvVar, callback EventHandler) int {
	commandStartedAt := int(time.Now().Unix())
	directive := fmt.Sprintf("Exporting environment variables")
	exitCode := 0

	callback(NewCommandStartedEvent(directive))

	defer func() {
		commandFinishedAt := int(time.Now().Unix())

		callback(NewCommandFinishedEvent(
			directive,
			exitCode,
			commandStartedAt,
			commandFinishedAt,
		))
	}()

	envFile := ""

	for _, e := range envVars {
		callback(NewCommandOutputEvent(fmt.Sprintf("Exporting %s\n", e.Name)))

		value, err := base64.StdEncoding.DecodeString(e.Value)

		if err != nil {
			exitCode = 1
			return exitCode
		}

		envFile += fmt.Sprintf("export %s=%s\n", e.Name, ShellQuote(string(value)))
	}

	envPath := fmt.Sprintf("%s/.env", e.tmpDirectory)
	err := ioutil.WriteFile(envPath, []byte(envFile), 0644)

	if err != nil {
		exitCode = 255
		return exitCode
	}

	cmd := fmt.Sprintf("source %s", envPath)
	exitCode = e.RunCommand(cmd, DevNullEventHandler)
	if exitCode != 0 {
		return exitCode
	}

	cmd = fmt.Sprintf("echo 'source %s' >> ~/.bash_profile", envPath)
	exitCode = e.RunCommand(cmd, DevNullEventHandler)
	if exitCode != 0 {
		return exitCode
	}

	return exitCode
}

func (e *DockerComposeExecutor) InjectFiles(files []api.File, callback EventHandler) int {
	directive := fmt.Sprintf("Injecting Files")
	commandStartedAt := int(time.Now().Unix())
	exitCode := 0

	callback(NewCommandStartedEvent(directive))

	for _, f := range files {
		output := fmt.Sprintf("Injecting %s with file mode %s\n", f.Path, f.Mode)

		callback(NewCommandOutputEvent(output))

		content, err := base64.StdEncoding.DecodeString(f.Content)

		if err != nil {
			callback(NewCommandOutputEvent("Failed to decode content of file.\n"))
			exitCode = 1
			return exitCode
		}

		tmpPath := fmt.Sprintf("%s/file", e.tmpDirectory)

		err = ioutil.WriteFile(tmpPath, []byte(content), 0644)
		if err != nil {
			callback(NewCommandOutputEvent(err.Error() + "\n"))
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
		exitCode = e.RunCommand(cmd, DevNullEventHandler)
		if exitCode != 0 {
			output := fmt.Sprintf("Failed to create destination path %s", destPath)
			callback(NewCommandOutputEvent(output + "\n"))
			break
		}

		cmd = fmt.Sprintf("cp %s %s", tmpPath, destPath)
		exitCode = e.RunCommand(cmd, DevNullEventHandler)
		if exitCode != 0 {
			output := fmt.Sprintf("Failed to move to destination path %s %s", tmpPath, destPath)
			callback(NewCommandOutputEvent(output + "\n"))
			break
		}

		cmd = fmt.Sprintf("chmod %s %s", f.Mode, destPath)
		exitCode = e.RunCommand(cmd, DevNullEventHandler)
		if exitCode != 0 {
			output := fmt.Sprintf("Failed to set file mode to %s", f.Mode)
			callback(NewCommandOutputEvent(output + "\n"))
			break
		}
	}

	commandFinishedAt := int(time.Now().Unix())

	callback(NewCommandFinishedEvent(
		directive,
		exitCode,
		commandStartedAt,
		commandFinishedAt,
	))

	return exitCode
}

func (e *DockerComposeExecutor) RunCommand(command string, callback EventHandler) int {
	var err error

	log.Printf("[SHELL] Running command: %s", command)

	cmdFilePath := fmt.Sprintf("%s/current-agent-cmd", e.tmpDirectory)
	restoreTtyMark := "97d140552e404df69f6472729d2b2c1"
	startMark := "87d140552e404df69f6472729d2b2c1"
	finishMark := "97d140552e404df69f6472729d2b2c2"

	commandStartedAt := int(time.Now().Unix())
	exitCode := 1

	defer func() {
		commandFinishedAt := int(time.Now().Unix())

		callback(NewCommandFinishedEvent(
			command,
			exitCode,
			commandStartedAt,
			commandFinishedAt,
		))
	}()

	commandEndRegex := regexp.MustCompile(finishMark + " " + `(\d)`)
	streamEvents := false

	restoreTtyCmd := "source /tmp/restore-tty; echo " + restoreTtyMark + "\n"

	// restore a sane STTY interface
	ioutil.WriteFile(cmdFilePath, []byte(restoreTtyCmd), 0644)
	e.stdin.Write([]byte("source " + cmdFilePath + "\n"))

	ScanLines(e.tty, func(line string) bool {
		log.Printf("[SHELL] (tty-restore) %s\n", line)

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
		callback(NewCommandStartedEvent(command))
		callback(NewCommandOutputEvent(fmt.Sprintf("Failed to run command: %+v\n", err)))

		return 1
	}

	// Constructing command with start and end markers:
	//
	// 0. Restore stty options to sanity
	// 1. display START marker
	// 2. execute the command file by sourcing it
	// 3. save the original exit status
	// 4. display the END marker with the exit status
	// 5. return the original exit status to the caller
	//

	commandWithStartAndEndMarkers := strings.Join([]string{
		"/tmp/restore-tty",
		fmt.Sprintf("echo '%s'", startMark),
		fmt.Sprintf("source %s", cmdFilePath),
		"AGENT_CMD_RESULT=$?",
		fmt.Sprintf(`echo "%s $AGENT_CMD_RESULT"`, finishMark),
		"echo \"exit $AGENT_CMD_RESULT\"|sh\n",
	}, ";")

	e.stdin.Write([]byte(commandWithStartAndEndMarkers))

	log.Println("[SHELL] Scan started")

	err = ScanLines(e.tty, func(line string) bool {
		log.Printf("[SHELL] (tty) %s\n", line)

		if strings.Contains(line, startMark) {
			log.Printf("[SHELL] Detected command start")
			streamEvents = true

			callback(NewCommandStartedEvent(command))

			return true
		}

		if strings.Contains(line, finishMark) {
			log.Printf("[SHELL] Detected command end")

			finalOutputPart := strings.Split(line, finishMark)

			// if there is anything else other than the command end marker
			// print it to the user
			if finalOutputPart[0] != "" {
				callback(NewCommandOutputEvent(finalOutputPart[0] + "\n"))
			}

			streamEvents = false

			if match := commandEndRegex.FindStringSubmatch(line); len(match) == 2 {
				log.Printf("[SHELL] Parsing exit status succedded")

				exitCode, err = strconv.Atoi(match[1])

				if err != nil {
					log.Printf("[SHELL] Panic while parsing exit status, err: %+v", err)

					callback(NewCommandOutputEvent("Failed to read command exit code\n"))
				}

				log.Printf("[SHELL] Setting exit code to %d", exitCode)
			} else {
				log.Printf("[SHELL] Failed to parse exit status")

				exitCode = 1
				callback(NewCommandOutputEvent("Failed to read command exit code\n"))
			}

			log.Printf("[SHELL] Stopping scanner")
			return false
		}

		if streamEvents {
			callback(NewCommandOutputEvent(line + "\n"))
		}

		return true
	})

	return exitCode
}

func (e *DockerComposeExecutor) Stop() int {
	err := e.terminal.Process.Kill()

	if err != nil {
		return 0
	}

	return 0
}

func (e *DockerComposeExecutor) Cleanup() int {
	return 0
}
