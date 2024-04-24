package executors

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
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
	k8sClient       *kubernetes.KubernetesClient
	jobRequest      *api.JobRequest
	podName         string
	envSecretName   string
	imagePullSecret string
	logger          *eventlogger.Logger
	Shell           *shell.Shell

	// If the executor is stopped before it even starts, we need to cancel it.
	cancelFunc context.CancelFunc

	// We need to keep track if the initial environment has already
	// been exposed or not, because ExportEnvVars() gets called twice.
	initialEnvironmentExposed bool
}

func NewKubernetesExecutor(jobRequest *api.JobRequest, logger *eventlogger.Logger, k8sConfig kubernetes.Config) (*KubernetesExecutor, error) {
	clientset, err := kubernetes.NewInClusterClientset()
	if err != nil {
		log.Warnf("No in-cluster configuration found - using ~/.kube/config...")

		clientset, err = kubernetes.NewClientsetFromConfig()
		if err != nil {
			return nil, fmt.Errorf("error creating kubernetes clientset: %v", err)
		}
	}

	k8sClient, err := kubernetes.NewKubernetesClient(clientset, k8sConfig)
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
	commandStartedAt := int(time.Now().Unix())
	directive := "Creating Kubernetes resources for job..."
	exitCode := 0

	e.logger.LogCommandStarted(directive)

	defer func() {
		commandFinishedAt := int(time.Now().Unix())
		e.logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)
	}()

	err := e.k8sClient.LoadPodSpec()
	if err != nil {
		log.Errorf("Failed to load pod spec: %v", err)
		e.logger.LogCommandOutput(fmt.Sprintf("Failed to load pod spec: %v\n", err))
		exitCode = 1
		return exitCode
	}

	e.podName = fmt.Sprintf("semaphore-job-%s", e.jobRequest.JobID)
	e.envSecretName = fmt.Sprintf("%s-secret", e.podName)
	err = e.k8sClient.CreateSecret(e.envSecretName, e.jobRequest)
	if err != nil {
		log.Errorf("Failed to create environment secret: %v", err)
		e.logger.LogCommandOutput(fmt.Sprintf("Failed to create environment secret: %v\n", err))
		exitCode = 1
		return exitCode
	}

	// If image pull credentials are specified in the YAML,
	// we create a temporary secret to store them and use it to pull the image.
	if len(e.jobRequest.Compose.ImagePullCredentials) > 0 {
		e.imagePullSecret = fmt.Sprintf("%s-image-pull-secret", e.podName)
		err = e.k8sClient.CreateImagePullSecret(e.imagePullSecret, e.jobRequest.Compose.ImagePullCredentials)
		if err != nil {
			log.Errorf("Failed to create temporary image pull secret: %v", err)
			e.logger.LogCommandOutput(fmt.Sprintf("Failed to create temporary image pull secret: %v\n", err))
			exitCode = 1
			return exitCode
		}
	}

	err = e.k8sClient.CreatePod(e.podName, e.envSecretName, e.imagePullSecret, e.jobRequest)
	if err != nil {
		log.Errorf("Failed to create pod: %v", err)
		e.logger.LogCommandOutput(fmt.Sprintf("Failed to create pod: %v\n", err))
		exitCode = 1
		return exitCode
	}

	return 0
}

func (e *KubernetesExecutor) Start() int {
	commandStartedAt := int(time.Now().Unix())
	directive := "Starting shell session..."
	exitCode := 0

	e.logger.LogCommandStarted(directive)

	defer func() {
		commandFinishedAt := int(time.Now().Unix())
		e.logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)
	}()

	ctx, cancel := context.WithCancel(context.TODO())
	e.cancelFunc = cancel

	err := e.k8sClient.WaitForPod(ctx, e.podName, func(msg string) {
		e.logger.LogCommandOutput(msg + "\n")
		log.Info(msg)
	})

	if err != nil {
		log.Errorf("Failed to create pod: %v", err)
		e.logger.LogCommandOutput(fmt.Sprintf("Failed to create pod: %v\n", err))
		exitCode = 1
		return exitCode
	}

	e.logger.LogCommandOutput("Pod is ready.\n")
	e.logger.LogCommandOutput("Starting a new bash session in the pod...\n")

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

	e.logger.LogCommandOutput("Shell session is ready.\n")
	e.Shell = shell

	// Find the user being used to run the commands.
	// Mostly helpful when troubleshooting issues with permissions on the container.
	output, code := e.GetOutputFromCommand("whoami")
	if code != 0 {
		log.Errorf("Failed to determine user: exit code %d - %s", code, output)
		e.logger.LogCommandOutput("Failed to determine user\n")
		e.logger.LogCommandOutput(fmt.Sprintf("exit code %d - %s", code, output))
		exitCode = code
		return exitCode
	}

	log.Infof("User: %s", output)
	e.logger.LogCommandOutput(fmt.Sprintf("User: %s", output))

	// Find the user identity for the user being used to run the commands.
	// Mostly helpful when troubleshooting issues with permissions on the container.
	output, code = e.GetOutputFromCommand("id")
	if code != 0 {
		log.Errorf("Failed to determine user identity: exit code %d - %s", code, output)
		e.logger.LogCommandOutput("Failed to determine user identity\n")
		e.logger.LogCommandOutput(fmt.Sprintf("exit code %d - %s", code, output))
		exitCode = code
		return exitCode
	}

	log.Infof("User identity: %s", output)
	e.logger.LogCommandOutput(fmt.Sprintf("User identity: %s", output))

	// Find the working directory.
	// Mostly helpful when troubleshooting issues with permissions on the container.
	output, code = e.GetOutputFromCommand("pwd")
	if code != 0 {
		log.Errorf("Failed to determine working directory: exit code %d - %s", code, output)
		e.logger.LogCommandOutput("Failed to determine working directory\n")
		e.logger.LogCommandOutput(fmt.Sprintf("exit code %d - %s", code, output))
		exitCode = code
		return exitCode
	}

	log.Infof("Working directory: %s", output)
	e.logger.LogCommandOutput(fmt.Sprintf("Working directory: %s", output))
	return exitCode
}

// This function gets called twice during a job's execution:
//   - On the first call, the environment variables come from a secret file injected into the pod.
//   - On the second call, the environment variables (currently, just the job result) need to be exported
//     through commands executed through the PTY.
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
	output := ""

	e.logger.LogCommandStarted(directive)

	defer func() {
		commandFinishedAt := int(time.Now().Unix())
		e.logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)
	}()

	for _, file := range files {

		// Find the key used to inject the file in /tmp/injected
		fileNameSecretKey := base64.RawURLEncoding.EncodeToString([]byte(file.Path))

		// Normalize path to properly handle absolute/relative/~ paths
		destPath := ""
		if file.Path[0] == '/' || file.Path[0] == '~' {
			destPath = file.Path
		} else {
			destPath = "~/" + file.Path
		}

		e.logger.LogCommandOutput(fmt.Sprintf("Injecting %s with file mode %s\n", file.Path, file.Mode))

		// Create the parent directory
		parentDir := filepath.Dir(destPath)
		output, exitCode = e.GetOutputFromCommand(fmt.Sprintf("mkdir -p %s", parentDir))
		if exitCode != 0 {
			errMessage := fmt.Sprintf("Error injecting file %s: failed to created parent directory %s: %s\n", destPath, parentDir, output)
			e.logger.LogCommandOutput(errMessage)
			log.Errorf(errMessage)
			return exitCode
		}

		// Copy the file injected as a secret in the /tmp/injected directory to its proper place
		output, exitCode = e.GetOutputFromCommand(fmt.Sprintf("cp /tmp/injected/%s %s", fileNameSecretKey, destPath))
		if exitCode != 0 {
			errMessage := fmt.Sprintf("Error injecting file %s: %s\n", destPath, output)
			e.logger.LogCommandOutput(errMessage)
			log.Errorf(errMessage)
			return exitCode
		}

		// Adjust the injected file's mode
		output, exitCode = e.GetOutputFromCommand(fmt.Sprintf("chmod %s %s", file.Mode, destPath))
		if exitCode != 0 {
			errMessage := fmt.Sprintf("Error injecting file %s: error setting file mode %s: %s\n", destPath, file.Mode, output)
			e.logger.LogCommandOutput(errMessage)
			log.Errorf(errMessage)
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

// Similar to RunCommand(), but instead of displaying the output
// of the commands in the job log, we return them to the caller.
func (e *KubernetesExecutor) GetOutputFromCommand(command string) (string, int) {
	out := bytes.Buffer{}
	p := e.Shell.NewProcessWithConfig(shell.Config{
		UseBase64Encoding: true,
		Command:           command,
		Shell:             e.Shell,
		OnOutput: func(output string) {
			out.WriteString(output)
		},
	})

	p.Run()

	return out.String(), p.ExitCode
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

	if e.cancelFunc != nil {
		e.cancelFunc()
	}

	if e.Shell != nil {
		err := e.Shell.Close()
		if err != nil {
			log.Errorf("Process killing procedure returned an error %+v\n", err)
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
	if e.podName != "" {
		if err := e.k8sClient.DeletePod(e.podName); err != nil {
			log.Errorf("Error deleting pod '%s': %v\n", e.podName, err)
		}
	}

	if e.envSecretName != "" {
		if err := e.k8sClient.DeleteSecret(e.envSecretName); err != nil {
			log.Errorf("Error deleting secret '%s': %v\n", e.envSecretName, err)
		}
	}

	// Not all jobs create this temporary secret,
	// just the ones that send credentials to pull images
	// in the job definition, so we only delete it if it was previously created.
	if e.imagePullSecret != "" {
		if err := e.k8sClient.DeleteSecret(e.imagePullSecret); err != nil {
			log.Errorf("Error deleting secret '%s': %v\n", e.imagePullSecret, err)
		}
	}
}

func (e *KubernetesExecutor) removeLocalResources() {
	envFileName := filepath.Join(os.TempDir(), ".env")
	if err := os.Remove(envFileName); err != nil {
		log.Errorf("Error removing local file '%s': %v", envFileName, err)
	}
}
