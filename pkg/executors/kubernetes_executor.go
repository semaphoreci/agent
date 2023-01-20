package executors

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	"github.com/semaphoreci/agent/pkg/kubernetes"
	shell "github.com/semaphoreci/agent/pkg/shell"

	log "github.com/sirupsen/logrus"
)

type KubernetesExecutor struct {
	k8sClient  *kubernetes.KubernetesClient
	jobRequest *api.JobRequest
	podName    string
	secretName string
	logger     *eventlogger.Logger
	Shell      *shell.Shell

	// We need to keep track if the initial environment has already
	// been exposed or not, because ExportEnvVars() gets called twice.
	initialEnvironmentExposed bool
}

func NewKubernetesExecutor(jobRequest *api.JobRequest, logger *eventlogger.Logger) (*KubernetesExecutor, error) {
	clientset, err := kubernetes.NewInClusterClientset()
	if err != nil {
		return nil, err
	}

	// The downwards API allows the namespace to be exposed
	// to the agent container through an environment variable.
	// See: https://kubernetes.io/docs/tasks/inject-data-application/environment-variable-expose-pod-information.
	namespace := os.Getenv("KUBERNETES_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	k8sClient, err := kubernetes.NewKubernetesClient(clientset, kubernetes.Config{
		Namespace:    namespace,
		DefaultImage: os.Getenv("SEMAPHORE_DEFAULT_IMAGE"),
	})

	if err != nil {
		return nil, err
	}

	return &KubernetesExecutor{
		k8sClient:  k8sClient,
		jobRequest: jobRequest,
		logger:     logger,
	}, nil
}

func (e *KubernetesExecutor) Prepare() int {
	e.podName = e.randomPodName()
	e.secretName = fmt.Sprintf("%s-secret", e.podName)

	err := e.k8sClient.CreateSecret(e.secretName, e.jobRequest)
	if err != nil {
		log.Errorf("Error creating secret '%s': %v", e.secretName, err)
		return 1
	}

	err = e.k8sClient.CreatePod(e.podName, e.secretName, e.jobRequest)
	if err != nil {
		log.Errorf("Error creating pod: %v", err)
		return 1
	}

	return 0
}

func (e *KubernetesExecutor) randomPodName() string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

	b := make([]rune, 12)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}

	return string(b)
}

// The idea here is to use the kubectl CLI to exec into the pod
func (e *KubernetesExecutor) Start() int {
	commandStartedAt := int(time.Now().Unix())
	directive := "Starting shell session..."
	exitCode := 0

	e.logger.LogCommandStarted(directive)

	defer func() {
		commandFinishedAt := int(time.Now().Unix())
		e.logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)
	}()

	err := e.k8sClient.WaitForPod(e.podName, func(msg string) {
		e.logger.LogCommandOutput(msg)
		log.Info(msg)
	})

	if err != nil {
		log.Errorf("Failed to create pod: %v", err)
		e.logger.LogCommandOutput(fmt.Sprintf("Failed to create pod: %v\n", err))
		exitCode = 1
		return exitCode
	}

	e.logger.LogCommandOutput("Starting a new bash session in the pod\n")

	// #nosec
	executable := "kubectl"
	args := []string{
		"exec",
		"-it",
		e.podName,
		"-c",
		"main",
		"--",
		"bash",
		"--login",
	}

	shell, err := shell.NewShellFromExecAndArgs(executable, args, os.TempDir())
	if err != nil {
		log.Errorf("Failed to create shell: %v", err)
		e.logger.LogCommandOutput("Failed to create shell in kubernetes container\n")
		e.logger.LogCommandOutput(err.Error())

		exitCode = 1
		return exitCode
	}

	err = shell.Start()
	if err != nil {
		log.Errorf("Failed to start shell err: %+v", err)
		e.logger.LogCommandOutput("Failed to start shell in kubernetes container\n")
		e.logger.LogCommandOutput(err.Error())

		exitCode = 1
		return exitCode
	}

	e.Shell = shell
	return exitCode
}

// This function gets called twice during a job's execution:
// - On the first call, the environment variables come from a secret file injected into the pod.
// - On the second call, the environment variables (currently, just the job result) need to be exported
//   through commands executed through the PTY.
func (e *KubernetesExecutor) ExportEnvVars(envVars []api.EnvVar, hostEnvVars []config.HostEnvVar) int {
	commandStartedAt := int(time.Now().Unix())
	directive := "Exporting environment variables"
	exitCode := 0

	e.logger.LogCommandStarted(directive)

	defer func() {
		commandFinishedAt := int(time.Now().Unix())
		e.logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)
		e.initialEnvironmentExposed = true
	}()

	// Include the environment variables exposed in the job log.
	for _, envVar := range envVars {
		e.logger.LogCommandOutput(fmt.Sprintf("Exporting %s\n", envVar.Name))
	}

	// Second call of this function.
	// Export environment variables through the PTY, one by one, using commands.
	if e.initialEnvironmentExposed {
		env, err := shell.CreateEnvironment(envVars, hostEnvVars)
		if err != nil {
			exitCode = 1
			log.Errorf("Error creating environment: %v", err)
			return exitCode
		}

		for _, command := range env.ToCommands() {
			exitCode = e.RunCommand(command, true, "")
			if exitCode != 0 {
				log.Errorf("Error exporting environment variables")
				return exitCode
			}
		}

		return exitCode
	}

	// First call of this function.
	// In this case, a secret with all the environment variables has been exposed in the pod spec,
	// so all we need to do here is to source that file through the PTY session.
	exitCode = e.RunCommand("source /tmp/injected/.env", true, "")
	if exitCode != 0 {
		log.Errorf("Error sourcing environment file")
		return exitCode
	}

	return exitCode
}

// All the files have already been exposed to the main container
// through a temporary secret created before the k8s pod was created, and used in the pod spec.
// Here, we just need to move the files to their correct location.
func (e *KubernetesExecutor) InjectFiles(files []api.File) int {
	directive := "Injecting Files"
	commandStartedAt := int(time.Now().Unix())
	exitCode := 0

	e.logger.LogCommandStarted(directive)

	defer func() {
		commandFinishedAt := int(time.Now().Unix())
		e.logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)
	}()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Errorf("Error finding home directory: %v\n", err)
		return 1
	}

	for _, file := range files {

		// Find the key used to inject the file in /tmp/injected
		fileNameSecretKey := base64.RawURLEncoding.EncodeToString([]byte(file.Path))

		// Normalize path to properly handle absolute/relative/~ paths
		destPath := file.NormalizePath(homeDir)
		e.logger.LogCommandOutput(fmt.Sprintf("Injecting %s with file mode %s\n", destPath, file.Mode))

		// Create the parent directory
		parentDir := filepath.Dir(file.Path)
		exitCode := e.RunCommand(fmt.Sprintf("mkdir -p %s", parentDir), true, "")
		if exitCode != 0 {
			errMessage := fmt.Sprintf("Error injecting file %s: failed to created parent directory %s\n", destPath, parentDir)
			e.logger.LogCommandOutput(errMessage)
			log.Errorf(errMessage)
			exitCode = 1
			return exitCode
		}

		// Copy the file injected as a secret in the /tmp/injected directory to its proper place
		exitCode = e.RunCommand(fmt.Sprintf("cp /tmp/injected/%s %s", fileNameSecretKey, file.Path), true, "")
		if exitCode != 0 {
			e.logger.LogCommandOutput(fmt.Sprintf("Error injecting file %s\n", file.Path))
			log.Errorf("Error injecting file %s", file.Path)
			exitCode = 1
			return exitCode
		}

		// Adjust the injected file's mode
		exitCode = e.RunCommand(fmt.Sprintf("chmod %s %s", file.Mode, file.Path), true, "")
		if exitCode != 0 {
			errMessage := fmt.Sprintf("Error setting file mode (%s) for %s\n", file.Mode, file.Path)
			e.logger.LogCommandOutput(errMessage)
			log.Errorf(errMessage)
			exitCode = 1
			return exitCode
		}
	}

	return exitCode
}

func (e *KubernetesExecutor) RunCommand(command string, silent bool, alias string) int {
	return e.RunCommandWithOptions(CommandOptions{
		Command: command,
		Silent:  silent,
		Alias:   alias,
		Warning: "",
	})
}

func (e *KubernetesExecutor) RunCommandWithOptions(options CommandOptions) int {
	directive := options.Command
	if options.Alias != "" {
		directive = options.Alias
	}

	/*
	 * Unlike the shell and docker-compose executors,
	 * where a folder can be shared between the agent and the PTY executing the commands,
	 * in here, we don't have that ability. So, we do not use a temporary folder for storing
	 * the command being executed, and instead use base64 encoding to make sure multiline commands
	 * and commands with different types of quote usage are handled properly.
	 */
	p := e.Shell.NewProcessWithConfig(shell.Config{
		UseBase64Encoding: true,
		Command:           options.Command,
		Shell:             e.Shell,
		OnOutput: func(output string) {
			if !options.Silent {
				e.logger.LogCommandOutput(output)
			}
		},
	})

	if !options.Silent {
		e.logger.LogCommandStarted(directive)

		if options.Alias != "" {
			e.logger.LogCommandOutput(fmt.Sprintf("Running: %s\n", options.Command))
		}

		if options.Warning != "" {
			e.logger.LogCommandOutput(fmt.Sprintf("Warning: %s\n", options.Warning))
		}
	}

	p.Run()

	if !options.Silent {
		e.logger.LogCommandFinished(directive, p.ExitCode, p.StartedAt, p.FinishedAt)
	}

	return p.ExitCode
}

func (e *KubernetesExecutor) Stop() int {
	log.Debug("Starting the process killing procedure")

	if e.Shell != nil {
		err := e.Shell.Close()
		if err != nil {
			log.Errorf("Process killing procedure returned an error %+v\n", err)

			return 0
		}
	}

	return e.Cleanup()
}

func (e *KubernetesExecutor) Cleanup() int {
	e.removeK8sResources()
	e.removeLocalResources()
	return 0
}

func (e *KubernetesExecutor) removeK8sResources() {
	err := e.k8sClient.DeletePod(e.podName)
	if err != nil {
		log.Errorf("Error deleting pod '%s': %v\n", e.podName, err)
	}

	err = e.k8sClient.DeleteSecret(e.secretName)
	if err != nil {
		log.Errorf("Error deleting secret '%s': %v\n", e.secretName, err)
	}
}

func (e *KubernetesExecutor) removeLocalResources() {
	envFileName := filepath.Join(os.TempDir(), ".env")
	if err := os.Remove(envFileName); err != nil {
		log.Errorf("Error removing local file '%s': %v", envFileName, err)
	}
}
