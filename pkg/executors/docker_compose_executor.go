package executors

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"time"

	watchman "github.com/renderedtext/go-watchman"
	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	"github.com/semaphoreci/agent/pkg/osinfo"
	shell "github.com/semaphoreci/agent/pkg/shell"
	log "github.com/sirupsen/logrus"
)

type DockerComposeExecutor struct {
	Logger     *eventlogger.Logger
	Shell      *shell.Shell
	jobRequest *api.JobRequest

	tmpDirectory              string
	dockerConfiguration       api.Compose
	dockerComposeManifestPath string
	mainContainerName         string
	exposeKvmDevice           bool
	fileInjections            []config.FileInjection
	FailOnMissingFiles        bool
}

type DockerComposeExecutorOptions struct {
	ExposeKvmDevice    bool
	FileInjections     []config.FileInjection
	FailOnMissingFiles bool
}

func NewDockerComposeExecutor(request *api.JobRequest, logger *eventlogger.Logger, options DockerComposeExecutorOptions) *DockerComposeExecutor {
	return &DockerComposeExecutor{
		Logger:                    logger,
		jobRequest:                request,
		dockerConfiguration:       request.Compose,
		exposeKvmDevice:           options.ExposeKvmDevice,
		fileInjections:            options.FileInjections,
		FailOnMissingFiles:        options.FailOnMissingFiles,
		dockerComposeManifestPath: osinfo.FormTempDirPath("docker-compose.yml"),
		tmpDirectory:              osinfo.FormTempDirPath("agent-temp-directory"), // make a better random name

		// during testing the name main gets taken up, if we make it random we avoid headaches
		mainContainerName: request.Compose.Containers[0].Name,
	}
}

func (e *DockerComposeExecutor) Prepare() int {
	err := os.MkdirAll(e.tmpDirectory, os.ModePerm)
	if err != nil {
		return 1
	}

	err = e.executeHostCommands()
	if err != nil {
		return 1
	}

	filesToInject, err := e.findValidFilesToInject()
	if err != nil {
		log.Errorf("Error injecting files: %v", err)
		return 1
	}

	compose := ConstructDockerComposeFile(e.dockerConfiguration, e.exposeKvmDevice, filesToInject)
	log.Debug("Compose File:")
	log.Debug(compose)

	err = ioutil.WriteFile(e.dockerComposeManifestPath, []byte(compose), 0600)
	if err != nil {
		log.Errorf("Error writing docker compose manifest file: %v", err)
		return 1
	}

	return e.setUpSSHJumpPoint()
}

func (e *DockerComposeExecutor) findValidFilesToInject() ([]config.FileInjection, error) {
	filesToInject := []config.FileInjection{}
	for _, fileInjection := range e.fileInjections {
		err := fileInjection.CheckFileExists()
		if err == nil {
			filesToInject = append(filesToInject, fileInjection)
		} else {
			if e.FailOnMissingFiles {
				return nil, err
			}

			log.Warningf("Error injecting file %s - ignoring it: %v", fileInjection.HostPath, err)
		}
	}

	return filesToInject, nil
}

func (e *DockerComposeExecutor) executeHostCommands() error {
	hostCommands := e.jobRequest.Compose.HostSetupCommands

	for _, c := range hostCommands {
		log.Debug("Executing Host Command:", c.Directive)

		// #nosec
		cmd := exec.Command("bash", "-c", c.Directive)

		out, err := cmd.CombinedOutput()
		log.Debug(string(out))

		if err != nil {
			log.Errorf("Error: %v", err)
			return err
		}
	}
	return nil
}

func (e *DockerComposeExecutor) setUpSSHJumpPoint() int {
	err := InjectEntriesToAuthorizedKeys(e.jobRequest.SSHPublicKeys)

	if err != nil {
		log.Errorf("Failed to inject authorized keys: %+v", err)
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
		log.Errorf("Failed to set up SSH jump point: %+v", err)
		return 1
	}

	return 0
}

func (e *DockerComposeExecutor) Start() int {
	exitCode := e.injectImagePullSecrets()
	if exitCode != 0 {
		log.Error("[SHELL] Failed to set up image pull secrets")
		return exitCode
	}

	exitCode = e.pullDockerImages()
	if exitCode != 0 {
		log.Error("Failed to pull images")
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

	log.Debug("Starting stateful shell")

	// #nosec
	cmd := exec.Command(
		"docker-compose",
		"--ansi",
		"never",
		"-f",
		e.dockerComposeManifestPath,
		"run",
		"--rm",
		"--name",
		e.mainContainerName,
		"-v",
		"/var/run/docker.sock:/var/run/docker.sock",
		"-v",
		fmt.Sprintf("%s:%s:ro", os.TempDir(), os.TempDir()),
		e.mainContainerName,
		"bash",
	)

	shell, err := shell.NewShell(cmd, false)
	if err != nil {
		log.Errorf("Failed to start stateful shell err: %+v", err)

		e.Logger.LogCommandOutput("Failed to start the docker image\n")
		e.Logger.LogCommandOutput(err.Error())

		exitCode = 1
		return exitCode
	}

	err = shell.Start()
	if err != nil {
		log.Errorf("Failed to start stateful shell err: %+v", err)

		e.Logger.LogCommandOutput("Failed to start the docker image\n")
		e.Logger.LogCommandOutput(err.Error())

		exitCode = 1
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

		envs = append(envs, fmt.Sprintf("%s=%s", name, string(value)))
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

		envs = append(envs, fmt.Sprintf("%s=%s", name, string(value)))
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

		envs = append(envs, fmt.Sprintf("%s=%s", name, string(value)))
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
			e.Logger.LogCommandOutput("Failed to decode the content of the file.\n")
			return 1
		}

		var tmpPath string
		if runtime.GOOS == "windows" {
			tmpPath = fmt.Sprintf("%s\\file", e.tmpDirectory)
		} else {
			tmpPath = fmt.Sprintf("%s/file", e.tmpDirectory)
		}

		// #nosec
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

		// #nosec
		cmd := exec.Command("bash", "-c", fileCmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			output := fmt.Sprintf("Failed to create destination path %s, cmd: %s, out: %s", destPath, err, out)
			e.Logger.LogCommandOutput(output + "\n")
			return 1
		}

		fileCmd = fmt.Sprintf("cp %s %s", tmpPath, destPath)

		// #nosec
		cmd = exec.Command("bash", "-c", fileCmd)
		out, err = cmd.CombinedOutput()
		if err != nil {
			output := fmt.Sprintf("Failed to move to destination path %s %s, cmd: %s, out: %s", tmpPath, destPath, err, out)
			e.Logger.LogCommandOutput(output + "\n")
			return 1
		}

		fileCmd = fmt.Sprintf("chmod %s %s", f.Mode, destPath)

		// #nosec
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

		envs = append(envs, fmt.Sprintf("%s=%s", name, string(value)))
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
	log.Debug("Pulling docker images")
	directive := "Pulling docker images..."
	commandStartedAt := int(time.Now().Unix())
	e.SubmitDockerStats("compose.docker.pull.rate")
	e.Logger.LogCommandStarted(directive)

	//
	// Docker-Compose doens't have the equivalent of image_pull_policy: IfNotPresent
	// "docker-compose pull" pulls always.
	//
	// This is sub-optimal. We want to enable our customers to use cached images.
	// However, we also need to make sure that everything is pulled down before
	// we start the job.
	//
	// As a workaround, we are not using "docker-compose pull", but:
	//
	//    docker-compose run main true
	//
	// The "run" command will first pull the images, and only pull the ones that
	// are not present locally.
	//

	// #nosec
	cmd := exec.Command(
		"docker-compose",
		"--ansi",
		"never",
		"-f",
		e.dockerComposeManifestPath,
		"run",
		"--rm",
		e.mainContainerName,
		"true")

	tty, err := shell.StartPTY(cmd)
	if err != nil {
		log.Errorf("Failed to initialize docker pull, err: %+v", err)
		return 1
	}

	reader := bufio.NewReader(tty)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		log.Debug("(tty) ", line)

		e.Logger.LogCommandOutput(line + "\n")
	}

	exitCode := 0

	if err := cmd.Wait(); err != nil {
		log.Errorf("Docker pull failed: %v", err)
		e.SubmitDockerStats("compose.docker.error.rate")
		exitCode = 1
	}

	log.Infof("Docker pull finished. Exit Code: %d", exitCode)

	commandFinishedAt := int(time.Now().Unix())
	e.SubmitDockerPullTime(commandFinishedAt - commandStartedAt)
	e.Logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)

	return exitCode
}

func (e *DockerComposeExecutor) ExportEnvVars(envVars []api.EnvVar, hostEnvVars []config.HostEnvVar) int {
	commandStartedAt := int(time.Now().Unix())
	directive := "Exporting environment variables"
	exitCode := 0

	e.Logger.LogCommandStarted(directive)

	defer func() {
		commandFinishedAt := int(time.Now().Unix())

		e.Logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)
	}()

	environment, err := shell.EnvFromApi(envVars)
	if err != nil {
		exitCode = 1
		return exitCode
	}

	environment.Merge(hostEnvVars)

	envFileName := osinfo.FormTempDirPath(".env")
	err = environment.ToFile(envFileName, func(name string) {
		e.Logger.LogCommandOutput(fmt.Sprintf("Exporting %s\n", name))
	})

	if err != nil {
		exitCode = 1
		return exitCode
	}

	exitCode = e.RunCommand(fmt.Sprintf("source %s", envFileName), true, "")
	if exitCode != 0 {
		return exitCode
	}

	exitCode = e.RunCommand(fmt.Sprintf("echo 'source %s' >> ~/.bash_profile", envFileName), true, "")
	if exitCode != 0 {
		return exitCode
	}

	return exitCode
}

func (e *DockerComposeExecutor) InjectFiles(files []api.File) int {
	directive := "Injecting Files"
	commandStartedAt := int(time.Now().Unix())
	exitCode := 0

	e.Logger.LogCommandStarted(directive)

	for _, f := range files {
		output := fmt.Sprintf("Injecting %s with file mode %s\n", f.Path, f.Mode)

		e.Logger.LogCommandOutput(output)

		content, err := f.Decode()

		if err != nil {
			e.Logger.LogCommandOutput("Failed to decode the content of the file.\n")
			exitCode = 1
			return exitCode
		}

		var tmpPath string
		if runtime.GOOS == "windows" {
			tmpPath = fmt.Sprintf("%s\\file", e.tmpDirectory)
		} else {
			tmpPath = fmt.Sprintf("%s/file", e.tmpDirectory)
		}

		// #nosec
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
		exitCode = e.RunCommand(cmd, true, "")
		if exitCode != 0 {
			output := fmt.Sprintf("Failed to create destination path %s", destPath)
			e.Logger.LogCommandOutput(output + "\n")
			break
		}

		cmd = fmt.Sprintf("cp %s %s", tmpPath, destPath)
		exitCode = e.RunCommand(cmd, true, "")
		if exitCode != 0 {
			output := fmt.Sprintf("Failed to move to destination path %s %s", tmpPath, destPath)
			e.Logger.LogCommandOutput(output + "\n")
			break
		}

		cmd = fmt.Sprintf("chmod %s %s", f.Mode, destPath)
		exitCode = e.RunCommand(cmd, true, "")
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

func (e *DockerComposeExecutor) RunCommand(command string, silent bool, alias string) int {
	directive := command
	if alias != "" {
		directive = alias
	}

	p := e.Shell.NewProcess(command)

	if !silent {
		e.Logger.LogCommandStarted(directive)

		if alias != "" {
			e.Logger.LogCommandOutput(fmt.Sprintf("Running: %s\n", command))
		}
	}

	p.OnStdout(func(output string) {
		if !silent {
			e.Logger.LogCommandOutput(output)
		}
	})

	p.Run()

	if !silent {
		e.Logger.LogCommandFinished(directive, p.ExitCode, p.StartedAt, p.FinishedAt)
	}

	return p.ExitCode
}

func (e *DockerComposeExecutor) Stop() int {
	log.Debug("Starting the process killing procedure")

	if e.Shell != nil {
		err := e.Shell.Close()
		if err != nil {
			log.Errorf("Process killing procedure returned an error %+v\n", err)

			return 0
		}
	}

	log.Debug("Process killing finished without errors")

	return 0
}

func (e *DockerComposeExecutor) Cleanup() int {
	return 0
}

func (e *DockerComposeExecutor) SubmitDockerStats(metricName string) {
	e.SubmitStats("semaphoreci/android", metricName, []string{"semaphoreci/android"}, 1)
}

func (e *DockerComposeExecutor) SubmitStats(imageName, metricName string, tags []string, value int) {
	if strings.Contains(e.jobRequest.Compose.Containers[0].Image, imageName) {
		err := watchman.SubmitWithTags(metricName, tags, value)
		if err != nil {
			log.Errorf("Error submiting metrics: %v", err)
		}
	}
}

func (e *DockerComposeExecutor) SubmitDockerPullTime(duration int) {
	if strings.Contains(e.jobRequest.Compose.Containers[0].Image, "semaphoreci/android") {
		// only submiting android metrics.
		err := watchman.SubmitWithTags("compose.docker.pull.duration", []string{"semaphoreci/android"}, duration)
		if err != nil {
			log.Errorf("Error submiting metrics: %v", err)
		}
	}
}
