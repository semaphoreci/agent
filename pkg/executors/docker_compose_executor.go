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
)

type DockerComposeExecutor struct {
	Executor

	jobRequest                *api.JobRequest
	tmpDirectory              string
	dockerConfiguration       api.Compose
	dockerComposeManifestPath string
	eventHandler              *EventHandler
	terminal                  *exec.Cmd
	tty                       *os.File
	stdin                     io.Writer
	stdoutScanner             *bufio.Scanner
	mainContainerName         string
}

func NewDockerComposeExecutor(request *api.JobRequest) *DockerComposeExecutor {
	return &DockerComposeExecutor{
		jobRequest:                request,
		dockerConfiguration:       request.Compose,
		dockerComposeManifestPath: "/tmp/docker-compose.yml",
		tmpDirectory:              "/tmp/agent-temp-directory", // make a better random name

		// during testing the name main gets taken up, if we make it random we avoid headaches
		mainContainerName: fmt.Sprintf("%s", request.Compose.Containers[0].Name),
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

	return e.setUpSSHJumpPoint()
}

func (e *DockerComposeExecutor) setUpSSHJumpPoint() int {
	err := InjectEntriesToAuthorizedKeys(e.jobRequest.SSHPublicKeys)

	if err != nil {
		log.Printf("Failed to inject authorized keys: %+v", err)
		return 1
	}

	script := strings.Join([]string{
		`#!/bin/bash`,
		``,
		`cd /tmp`,
		``,
		`echo -n "Waiting for the container to start up"`,
		``,
		`while true; do`,
		`  docker exec -i ` + e.mainContainerName + ` true 2>/dev/null`,
		``,
		`  if [ $? == 0 ]; then`,
		`    echo ""`,
		``,
		`    break`,
		`  else`,
		`    sleep 3`,
		`    echo -n "."`,
		`  fi`,
		`done`,
		``,
		`if [ $# -eq 0 ]; then`,
		`  docker exec -ti ` + e.mainContainerName + ` bash --login`,
		`else`,
		`  docker exec -i ` + e.mainContainerName + ` "$@"`,
		`fi`,
	}, "\n")

	err = SetUpSSHJumpPoint(script)
	if err != nil {
		log.Printf("Failed to set up SSH jump point: %+v", err)
		return 1
	}

	return 0
}

func (e *DockerComposeExecutor) Start(callback EventHandler) int {
	exitCode := e.injectImagePullSecrets(callback)
	if exitCode != 0 {
		log.Printf("[SHELL] Failed to set up image pull secrets")
		return exitCode
	}

	exitCode = e.pullDockerImages(callback)

	if exitCode != 0 {
		log.Printf("Failed to pull images")
		return exitCode
	}

	exitCode = e.startBashSession(callback)

	return exitCode
}

func (e *DockerComposeExecutor) startBashSession(callback EventHandler) int {
	commandStartedAt := int(time.Now().Unix())
	directive := "Starting the docker image..."
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

	callback(NewCommandOutputEvent("Starting a new bash session.\n"))

	log.Printf("Starting stateful shell")

	e.terminal = exec.Command(
		"docker-compose",
		"--no-ansi",
		"-f",
		e.dockerComposeManifestPath,
		"run",
		"--name",
		e.mainContainerName,
		"-v",
		"/var/run/docker.sock:/var/run/docker.sock",
		"-v",
		fmt.Sprintf("%s:%s:ro", e.tmpDirectory, e.tmpDirectory),
		e.mainContainerName,
		"bash",
	)

	tty, err := pty.Start(e.terminal)
	if err != nil {
		log.Printf("Failed to start stateful shell err: %+v", err)
		callback(NewCommandOutputEvent("Failed to start the docker image"))
		exitCode := 1
		return exitCode
	}

	e.stdin = tty
	e.tty = tty

	time.Sleep(1000)

	exitCode = e.silencePromptAndDisablePS1(callback)

	return exitCode
}

func (e *DockerComposeExecutor) injectImagePullSecrets(callback EventHandler) int {
	if len(e.dockerConfiguration.ImagePullCredentials) == 0 {
		return 0 // do nothing if there are no credentials
	}

	directive := "Setting up image pull credentials"
	commandStartedAt := int(time.Now().Unix())
	exitCode := 0
	callback(NewCommandStartedEvent(directive))

	for _, c := range e.dockerConfiguration.ImagePullCredentials {
		s, err := c.Strategy()

		if err != nil {
			callback(NewCommandOutputEvent(fmt.Sprintf("Failed to resolve docker login strategy: %+v\n", err)))

			exitCode = 1
			break
		}

		switch s {
		case api.ImagePullCredentialsStrategyDockerHub:
			exitCode = e.injectImagePullSecretsForDockerHub(callback, c.EnvVars)
		case api.ImagePullCredentialsStrategyECR:
			exitCode = e.injectImagePullSecretsForECR(callback, c.EnvVars)
		case api.ImagePullCredentialsStrategyGenericDocker:
			exitCode = e.injectImagePullSecretsForGenericDocker(callback, c.EnvVars)
		case api.ImagePullCredentialsStrategyGCR:
			exitCode = e.injectImagePullSecretsForGCR(callback, c.EnvVars, c.Files)
		default:
			callback(NewCommandOutputEvent(fmt.Sprintf("Unknown Handler for credential type %s\n", s)))
			exitCode = 1
		}

		if err != nil {
			exitCode = 1
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

func (e *DockerComposeExecutor) injectImagePullSecretsForDockerHub(callback EventHandler, envVars []api.EnvVar) int {
	callback(NewCommandOutputEvent("Setting up credentials for DockerHub\n"))

	env := []string{}

	for _, e := range envVars {
		name := e.Name
		value, err := e.Decode()

		if err != nil {
			callback(NewCommandOutputEvent(fmt.Sprintf("Failed to decode %s\n", name)))
			return 1
		}

		env = append(env, fmt.Sprintf("%s=%s", name, ShellQuote(string(value))))
	}

	loginCmd := `echo $DOCKERHUB_PASSWORD | docker login --username $DOCKERHUB_USERNAME --password-stdin`

	callback(NewCommandOutputEvent(loginCmd + "\n"))

	cmd := exec.Command("bash", "-c", loginCmd)
	cmd.Env = env

	out, err := cmd.CombinedOutput()

	for _, line := range strings.Split(string(out), "\n") {
		callback(NewCommandOutputEvent(line + "\n"))
	}

	if err != nil {
		return 1
	}

	return 0
}

func (e *DockerComposeExecutor) injectImagePullSecretsForGenericDocker(callback EventHandler, envVars []api.EnvVar) int {
	callback(NewCommandOutputEvent("Setting up credentials for Docker\n"))

	env := []string{}

	for _, e := range envVars {
		name := e.Name
		value, err := e.Decode()

		if err != nil {
			callback(NewCommandOutputEvent(fmt.Sprintf("Failed to decode %s\n", name)))
			return 1
		}

		env = append(env, fmt.Sprintf("%s=%s", name, ShellQuote(string(value))))
	}

	loginCmd := `docker login -u "$DOCKER_USERNAME" -p "$DOCKER_PASSWORD" $DOCKER_URL`

	callback(NewCommandOutputEvent(loginCmd + "\n"))

	cmd := exec.Command("bash", "-c", loginCmd)
	cmd.Env = env

	out, err := cmd.CombinedOutput()

	for _, line := range strings.Split(string(out), "\n") {
		callback(NewCommandOutputEvent(line + "\n"))
	}

	if err != nil {
		return 1
	}

	return 0
}

func (e *DockerComposeExecutor) injectImagePullSecretsForECR(callback EventHandler, envVars []api.EnvVar) int {
	callback(NewCommandOutputEvent("Setting up credentials for ECR\n"))

	env := []string{}

	for _, e := range envVars {
		name := e.Name
		value, err := e.Decode()

		if err != nil {
			callback(NewCommandOutputEvent(fmt.Sprintf("Failed to decode %s\n", name)))
			return 1
		}

		env = append(env, fmt.Sprintf("%s=%s", name, ShellQuote(string(value))))
	}

	loginCmd := `$(aws ecr get-login --no-include-email --region $AWS_REGION)`

	callback(NewCommandOutputEvent(loginCmd + "\n"))

	cmd := exec.Command("bash", "-c", loginCmd)
	cmd.Env = env

	out, err := cmd.CombinedOutput()

	for _, line := range strings.Split(string(out), "\n") {
		callback(NewCommandOutputEvent(line + "\n"))
	}

	if err != nil {
		return 1
	}

	return 0
}

func (e *DockerComposeExecutor) injectImagePullSecretsForGCR(callback EventHandler, envVars []api.EnvVar, files []api.File) int {
	callback(NewCommandOutputEvent("Setting up credentials for GCR\n"))

	for _, f := range files {

		content, err := f.Decode()

		if err != nil {
			callback(NewCommandOutputEvent("Failed to decode content of file.\n"))
			return 1
		}

		tmpPath := fmt.Sprintf("%s/file", e.tmpDirectory)

		err = ioutil.WriteFile(tmpPath, []byte(content), 0644)
		if err != nil {
			callback(NewCommandOutputEvent(err.Error() + "\n"))
			return 1
		}

		destPath := ""

		if f.Path[0] == '/' || f.Path[0] == '~' {
			destPath = f.Path
		} else {
			destPath = "~/" + f.Path
		}

		fileCmd := fmt.Sprintf("mkdir -p %s", path.Dir(destPath))
		cmd := exec.Command("bash", "-c", fileCmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			output := fmt.Sprintf("Failed to create destination path %s, cmd: %s, out: %s", destPath, err, out)
			callback(NewCommandOutputEvent(output + "\n"))
			return 1
		}

		fileCmd = fmt.Sprintf("cp %s %s", tmpPath, destPath)
		cmd = exec.Command("bash", "-c", fileCmd)
		out, err = cmd.CombinedOutput()
		if err != nil {
			output := fmt.Sprintf("Failed to move to destination path %s %s, cmd: %s, out: %s", tmpPath, destPath, err, out)
			callback(NewCommandOutputEvent(output + "\n"))
			return 1
		}

		fileCmd = fmt.Sprintf("chmod %s %s", f.Mode, destPath)
		cmd = exec.Command("bash", "-c", fileCmd)
		out, err = cmd.CombinedOutput()
		if err != nil {
			output := fmt.Sprintf("Failed to set file mode to %s, cmd: %s, out: %s", f.Mode, err, out)
			callback(NewCommandOutputEvent(output + "\n"))
			return 1
		}
	}

	env := []string{}

	for _, e := range envVars {
		name := e.Name
		value, err := e.Decode()

		if err != nil {
			callback(NewCommandOutputEvent(fmt.Sprintf("Failed to decode %s\n", name)))
			return 1
		}

		env = append(env, fmt.Sprintf("%s=%s", name, ShellQuote(string(value))))
	}

	loginCmd := `cat /tmp/gcr/keyfile.json | docker login -u _json_key --password-stdin https://$GCR_HOSTNAME`

	callback(NewCommandOutputEvent(loginCmd + "\n"))

	cmd := exec.Command("bash", "-c", loginCmd)
	cmd.Env = env

	out, err := cmd.CombinedOutput()

	for _, line := range strings.Split(string(out), "\n") {
		callback(NewCommandOutputEvent(line + "\n"))
	}

	if err != nil {
		return 1
	}

	return 0
}

func (e *DockerComposeExecutor) pullDockerImages(callback EventHandler) int {
	log.Printf("Pulling docker images")
	directive := "Pulling docker images..."
	commandStartedAt := int(time.Now().Unix())

	callback(NewCommandStartedEvent(directive))

	cmd := exec.Command(
		"docker-compose",
		"--no-ansi",
		"-f",
		e.dockerComposeManifestPath,
		"pull")

	tty, err := pty.Start(cmd)
	if err != nil {
		log.Printf("Failed to initialize docker pull, err: %+v", err)
		return 1
	}

	ScanLines(tty, func(line string) bool {
		log.Printf("(tty) %s\n", line)

		callback(NewCommandOutputEvent(line + "\n"))

		return true
	})

	exitCode := 0

	if err := cmd.Wait(); err != nil {
		exitCode = 1
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

func (e *DockerComposeExecutor) silencePromptAndDisablePS1(callback EventHandler) int {
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

	sessionInitilized := false

	for stdoutScanner.Scan() {
		text := stdoutScanner.Text()
		log.Printf("(tty) %s\n", text)

		// Docker deamon has an issue, no further processing is neaded
		if strings.Contains(text, "Error response from daemon") {
			callback(NewCommandOutputEvent(fmt.Sprintf("%s\n", text)))
			break
		}

		if !strings.Contains(text, "echo") && strings.Contains(text, everythingIsReadyMark) {
			sessionInitilized = true
			break
		}
	}

	if sessionInitilized {
		log.Println("Initialization complete")
		return 0
	} else {
		log.Println("Initialization failed")
		return 1
	}
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

		value, err := e.Decode()

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

		content, err := f.Decode()

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

	log.Printf("Running command: %s", command)

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

	log.Println("Scan started")

	err = ScanLines(e.tty, func(line string) bool {
		log.Printf("(tty) %s\n", line)

		if strings.Contains(line, startMark) {
			log.Printf("Detected command start")
			streamEvents = true

			callback(NewCommandStartedEvent(command))

			return true
		}

		if strings.Contains(line, finishMark) {
			log.Printf("Detected command end")

			finalOutputPart := strings.Split(line, finishMark)

			// if there is anything else other than the command end marker
			// print it to the user
			if finalOutputPart[0] != "" {
				callback(NewCommandOutputEvent(finalOutputPart[0] + "\n"))
			}

			streamEvents = false

			if match := commandEndRegex.FindStringSubmatch(line); len(match) == 2 {
				log.Printf("Parsing exit status succedded")

				exitCode, err = strconv.Atoi(match[1])

				if err != nil {
					log.Printf("Panic while parsing exit status, err: %+v", err)

					callback(NewCommandOutputEvent("Failed to read command exit code\n"))
				}

				log.Printf("Setting exit code to %d", exitCode)
			} else {
				log.Printf("Failed to parse exit status")

				exitCode = 1
				callback(NewCommandOutputEvent("Failed to read command exit code\n"))
			}

			log.Printf("Stopping scanner")
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
	log.Println("Starting the process killing procedure")

	err := e.terminal.Process.Kill()

	if err != nil {
		log.Printf("Process killing procedure returned an erorr %+v\n", err)

		return 0
	}

	log.Printf("Process killing finished without errors")

	return 0
}

func (e *DockerComposeExecutor) Cleanup() int {
	return 0
}
