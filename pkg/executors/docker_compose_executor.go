package executors

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	pty "github.com/kr/pty"
	api "github.com/semaphoreci/agent/pkg/api"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	shell "github.com/semaphoreci/agent/pkg/shell"
)

type DockerComposeExecutor struct {
	Logger     *eventlogger.Logger
	Shell      *shell.Shell
	jobRequest *api.JobRequest

	tmpDirectory              string
	dockerConfiguration       api.Compose
	dockerComposeManifestPath string
	mainContainerName         string
}

func NewDockerComposeExecutor(request *api.JobRequest, logger *eventlogger.Logger) *DockerComposeExecutor {
	return &DockerComposeExecutor{
		Logger:                    logger,
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

func (e *DockerComposeExecutor) Start() int {
	exitCode := e.injectImagePullSecrets()
	if exitCode != 0 {
		log.Printf("[SHELL] Failed to set up image pull secrets")
		return exitCode
	}

	exitCode = e.pullDockerImages()
	if exitCode != 0 {
		log.Printf("Failed to pull images")
		return exitCode
	}

	exitCode = e.startBashSession()

	return exitCode
}

func (e *DockerComposeExecutor) startBashSession() int {
	commandStartedAt := int(time.Now().Unix())
	directive := "Starting the docker image..."
	exitCode := 0

	e.Logger.LogCommandStarted(directive)

	defer func() {
		commandFinishedAt := int(time.Now().Unix())

		e.Logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)
	}()

	e.Logger.LogCommandOutput("Starting a new bash session.\n")

	log.Printf("Starting stateful shell")

	cmd := exec.Command(
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

	shell, err := shell.NewShell(cmd, e.tmpDirectory)
	if err != nil {
		log.Printf("Failed to start stateful shell err: %+v", err)

		e.Logger.LogCommandOutput("Failed to start the docker image\n")
		e.Logger.LogCommandOutput(err.Error())

		exitCode := 1
		return exitCode
	}

	err = shell.Start()
	if err != nil {
		log.Printf("Failed to start stateful shell err: %+v", err)

		e.Logger.LogCommandOutput("Failed to start the docker image\n")
		e.Logger.LogCommandOutput(err.Error())

		exitCode := 1
		return exitCode
	}

	e.Shell = shell

	return exitCode
}

func (e *DockerComposeExecutor) injectImagePullSecrets() int {
	if len(e.dockerConfiguration.ImagePullCredentials) == 0 {
		return 0 // do nothing if there are no credentials
	}

	directive := "Setting up image pull credentials"
	commandStartedAt := int(time.Now().Unix())
	exitCode := 0
	e.Logger.LogCommandStarted(directive)

	for _, c := range e.dockerConfiguration.ImagePullCredentials {
		s, err := c.Strategy()

		if err != nil {
			e.Logger.LogCommandOutput(fmt.Sprintf("Failed to resolve docker login strategy: %+v\n", err))

			exitCode = 1
			break
		}

		switch s {
		case api.ImagePullCredentialsStrategyDockerHub:
			exitCode = e.injectImagePullSecretsForDockerHub(c.EnvVars)
		case api.ImagePullCredentialsStrategyECR:
			exitCode = e.injectImagePullSecretsForECR(c.EnvVars)
		case api.ImagePullCredentialsStrategyGenericDocker:
			exitCode = e.injectImagePullSecretsForGenericDocker(c.EnvVars)
		case api.ImagePullCredentialsStrategyGCR:
			exitCode = e.injectImagePullSecretsForGCR(c.EnvVars, c.Files)
		default:
			e.Logger.LogCommandOutput(fmt.Sprintf("Unknown Handler for credential type %s\n", s))
			exitCode = 1
		}

		if err != nil {
			exitCode = 1
			break
		}
	}

	commandFinishedAt := int(time.Now().Unix())
	e.Logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)

	return exitCode
}

func (e *DockerComposeExecutor) injectImagePullSecretsForDockerHub(envVars []api.EnvVar) int {
	e.Logger.LogCommandOutput("Setting up credentials for DockerHub\n")

	envs := []string{}

	for _, env := range envVars {
		name := env.Name
		value, err := env.Decode()

		if err != nil {
			e.Logger.LogCommandOutput(fmt.Sprintf("Failed to decode %s\n", name))
			return 1
		}

		envs = append(envs, fmt.Sprintf("%s=%s", name, ShellQuote(string(value))))
	}

	loginCmd := `echo $DOCKERHUB_PASSWORD | docker login --username $DOCKERHUB_USERNAME --password-stdin`

	e.Logger.LogCommandOutput(loginCmd + "\n")

	cmd := exec.Command("bash", "-c", loginCmd)
	cmd.Env = envs

	out, err := cmd.CombinedOutput()

	for _, line := range strings.Split(string(out), "\n") {
		e.Logger.LogCommandOutput(line + "\n")
	}

	if err != nil {
		return 1
	}

	return 0
}

func (e *DockerComposeExecutor) injectImagePullSecretsForGenericDocker(envVars []api.EnvVar) int {
	e.Logger.LogCommandOutput("Setting up credentials for Docker\n")

	envs := []string{}

	for _, env := range envVars {
		name := env.Name
		value, err := env.Decode()

		if err != nil {
			e.Logger.LogCommandOutput(fmt.Sprintf("Failed to decode %s\n", name))
			return 1
		}

		envs = append(envs, fmt.Sprintf("%s=%s", name, ShellQuote(string(value))))
	}

	loginCmd := `docker login -u "$DOCKER_USERNAME" -p "$DOCKER_PASSWORD" $DOCKER_URL`

	e.Logger.LogCommandOutput(loginCmd + "\n")

	cmd := exec.Command("bash", "-c", loginCmd)
	cmd.Env = envs

	out, err := cmd.CombinedOutput()

	for _, line := range strings.Split(string(out), "\n") {
		e.Logger.LogCommandOutput(line + "\n")
	}

	if err != nil {
		return 1
	}

	return 0
}

func (e *DockerComposeExecutor) injectImagePullSecretsForECR(envVars []api.EnvVar) int {
	e.Logger.LogCommandOutput("Setting up credentials for ECR\n")

	envs := []string{}

	for _, env := range envVars {
		name := env.Name
		value, err := env.Decode()

		if err != nil {
			e.Logger.LogCommandOutput(fmt.Sprintf("Failed to decode %s\n", name))
			return 1
		}

		envs = append(envs, fmt.Sprintf("%s=%s", name, ShellQuote(string(value))))
	}

	loginCmd := `$(aws ecr get-login --no-include-email --region $AWS_REGION)`

	e.Logger.LogCommandOutput(loginCmd + "\n")

	cmd := exec.Command("bash", "-c", loginCmd)
	cmd.Env = envs

	out, err := cmd.CombinedOutput()

	for _, line := range strings.Split(string(out), "\n") {
		e.Logger.LogCommandOutput(line + "\n")
	}

	if err != nil {
		return 1
	}

	return 0
}

func (e *DockerComposeExecutor) injectImagePullSecretsForGCR(envVars []api.EnvVar, files []api.File) int {
	e.Logger.LogCommandOutput("Setting up credentials for GCR\n")

	for _, f := range files {

		content, err := f.Decode()

		if err != nil {
			e.Logger.LogCommandOutput("Failed to decode content of file.\n")
			return 1
		}

		tmpPath := fmt.Sprintf("%s/file", e.tmpDirectory)

		err = ioutil.WriteFile(tmpPath, []byte(content), 0644)
		if err != nil {
			e.Logger.LogCommandOutput(err.Error() + "\n")
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
			e.Logger.LogCommandOutput(output + "\n")
			return 1
		}

		fileCmd = fmt.Sprintf("cp %s %s", tmpPath, destPath)
		cmd = exec.Command("bash", "-c", fileCmd)
		out, err = cmd.CombinedOutput()
		if err != nil {
			output := fmt.Sprintf("Failed to move to destination path %s %s, cmd: %s, out: %s", tmpPath, destPath, err, out)
			e.Logger.LogCommandOutput(output + "\n")
			return 1
		}

		fileCmd = fmt.Sprintf("chmod %s %s", f.Mode, destPath)
		cmd = exec.Command("bash", "-c", fileCmd)
		out, err = cmd.CombinedOutput()
		if err != nil {
			output := fmt.Sprintf("Failed to set file mode to %s, cmd: %s, out: %s", f.Mode, err, out)
			e.Logger.LogCommandOutput(output + "\n")
			return 1
		}
	}

	envs := []string{}

	for _, env := range envVars {
		name := env.Name
		value, err := env.Decode()

		if err != nil {
			e.Logger.LogCommandOutput(fmt.Sprintf("Failed to decode %s\n", name))
			return 1
		}

		envs = append(envs, fmt.Sprintf("%s=%s", name, ShellQuote(string(value))))
	}

	loginCmd := `cat /tmp/gcr/keyfile.json | docker login -u _json_key --password-stdin https://$GCR_HOSTNAME`

	e.Logger.LogCommandOutput(loginCmd + "\n")

	cmd := exec.Command("bash", "-c", loginCmd)
	cmd.Env = envs

	out, err := cmd.CombinedOutput()

	for _, line := range strings.Split(string(out), "\n") {
		e.Logger.LogCommandOutput(line + "\n")
	}

	if err != nil {
		return 1
	}

	return 0
}

func (e *DockerComposeExecutor) pullDockerImages() int {
	log.Printf("Pulling docker images")
	directive := "Pulling docker images..."
	commandStartedAt := int(time.Now().Unix())

	e.Logger.LogCommandStarted(directive)

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

	reader := bufio.NewReader(tty)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		log.Println("(tty) ", line)

		e.Logger.LogCommandOutput(line + "\n")
	}

	exitCode := 0

	if err := cmd.Wait(); err != nil {
		log.Println("Docker pull failed", err)
		exitCode = 1
	}

	log.Println("Docker pull finished. Exit Code", exitCode)

	commandFinishedAt := int(time.Now().Unix())

	e.Logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)

	return exitCode
}

func (e *DockerComposeExecutor) ExportEnvVars(envVars []api.EnvVar) int {
	commandStartedAt := int(time.Now().Unix())
	directive := fmt.Sprintf("Exporting environment variables")
	exitCode := 0

	e.Logger.LogCommandStarted(directive)

	defer func() {
		commandFinishedAt := int(time.Now().Unix())

		e.Logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)
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

	envPath := fmt.Sprintf("%s/.env", e.tmpDirectory)
	err := ioutil.WriteFile(envPath, []byte(envFile), 0644)

	if err != nil {
		exitCode = 255
		return exitCode
	}

	cmd := fmt.Sprintf("source %s", envPath)
	exitCode = e.RunCommand(cmd, true)
	if exitCode != 0 {
		return exitCode
	}

	cmd = fmt.Sprintf("echo 'source %s' >> ~/.bash_profile", envPath)
	exitCode = e.RunCommand(cmd, true)
	if exitCode != 0 {
		return exitCode
	}

	return exitCode
}

func (e *DockerComposeExecutor) InjectFiles(files []api.File) int {
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

func (e *DockerComposeExecutor) RunCommand(command string, silent bool) int {
	p := e.Shell.NewProcess(command)

	if !silent {
		e.Logger.LogCommandStarted(command)
	}

	p.OnStdout(func(output string) {
		if !silent {
			e.Logger.LogCommandOutput(output)
		}
	})

	p.Run()

	if !silent {
		e.Logger.LogCommandFinished(command, p.ExitCode, p.StartedAt, p.FinishedAt)
	}

	return p.ExitCode
}

func (e *DockerComposeExecutor) Stop() int {
	log.Println("Starting the process killing procedure")

	err := e.Shell.Close()
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
