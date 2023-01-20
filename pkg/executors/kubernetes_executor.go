package executors

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	"github.com/semaphoreci/agent/pkg/retry"
	shell "github.com/semaphoreci/agent/pkg/shell"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type KubernetesExecutor struct {
	k8sClientset  *kubernetes.Clientset
	k8sRestConfig *rest.Config
	k8sNamespace  string
	jobRequest    *api.JobRequest
	pod           *corev1.Pod
	podName       string
	secret        *corev1.Secret
	secretName    string
	logger        *eventlogger.Logger
	Shell         *shell.Shell
}

func NewKubernetesExecutor(jobRequest *api.JobRequest, logger *eventlogger.Logger) (*KubernetesExecutor, error) {
	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, err
	}

	namespace := os.Getenv("KUBERNETES_NAMESPACE")

	// The downwards API allows the namespace to be exposed
	// to the agent container through an environment variable.
	// See: https://kubernetes.io/docs/tasks/inject-data-application/environment-variable-expose-pod-information.
	return &KubernetesExecutor{
		k8sRestConfig: k8sConfig,
		k8sClientset:  clientset,
		k8sNamespace:  namespace,
		jobRequest:    jobRequest,
		logger:        logger,
	}, nil
}

func (e *KubernetesExecutor) createAuxiliarySecret() error {
	environment, err := shell.CreateEnvironment(e.jobRequest.EnvVars, []config.HostEnvVar{})
	if err != nil {
		return fmt.Errorf("error creating environment: %v", err)
	}

	envFileName := filepath.Join(os.TempDir(), ".env")
	err = environment.ToFile(envFileName, nil)
	if err != nil {
		return fmt.Errorf("error creating temporary environment file: %v", err)
	}

	envFile, err := os.Open(envFileName)
	if err != nil {
		return fmt.Errorf("error opening environment file for reading: %v", err)
	}

	defer envFile.Close()

	env, err := ioutil.ReadAll(envFile)
	if err != nil {
		return fmt.Errorf("error reading environment file: %v", err)
	}

	// We don't allow the secret to be changed after its creation.
	immutable := true

	// We use one key for the environment variables.
	data := map[string]string{".env": string(env)}

	// And one key for each file injected in the job definition.
	// K8s doesn't allow many special characters in a secret's key; it uses [-._a-zA-Z0-9]+ for validation.
	// So, we encode the flle's path (using base64 URL encoding, no padding),
	// and use it as the secret's key.
	// K8s will inject the file at /tmp/injected/<encoded-file-path>
	// On InjectFiles(), we move the file to its proper place.
	for _, file := range e.jobRequest.Files {
		encodedPath := base64.RawURLEncoding.EncodeToString([]byte(file.Path))
		content, err := file.Decode()
		if err != nil {
			return fmt.Errorf("error decoding file '%s': %v", file.Path, err)
		}

		data[encodedPath] = string(content)
	}

	e.secretName = fmt.Sprintf("%s-secret", e.podName)
	secret := corev1.Secret{
		ObjectMeta: v1.ObjectMeta{Name: e.secretName},
		Type:       corev1.SecretTypeOpaque,
		Immutable:  &immutable,
		StringData: data,
	}

	newSecret, err := e.k8sClientset.CoreV1().
		Secrets(e.k8sNamespace).
		Create(context.Background(), &secret, v1.CreateOptions{})

	if err != nil {
		return fmt.Errorf("error creating secret '%s': %v", e.secretName, err)
	}

	e.secret = newSecret
	return nil
}

func (e *KubernetesExecutor) Prepare() int {
	e.podName = e.randomPodName()

	err := e.createAuxiliarySecret()
	if err != nil {
		log.Errorf("Error creating auxiliary secret for pod: %v", err)
		return 1
	}

	pod, err := e.k8sClientset.CoreV1().
		Pods(e.k8sNamespace).
		Create(context.TODO(), e.newPod(), v1.CreateOptions{})

	if err != nil {
		log.Errorf("Error creating k8s pod: %v", err)
		return 1
	}

	e.pod = pod
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

func (e *KubernetesExecutor) newPod() *corev1.Pod {

	return &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Namespace: e.k8sNamespace,

			// TODO: use agent name + job ID here.
			// Note: e.jobRequest.ID is not being sent by API.
			Name: e.podName,

			// TODO: put agent name here too
			Labels: map[string]string{
				"app": "semaphore-agent",
			},
		},

		Spec: corev1.PodSpec{
			Containers:       e.containers(),
			ImagePullSecrets: e.imagePullSecrets(),
			RestartPolicy:    corev1.RestartPolicyNever,
			Volumes: []corev1.Volume{
				{
					Name: "environment",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: e.secretName,
						},
					},
				},
			},
		},
	}
}

func (e *KubernetesExecutor) containers() []corev1.Container {

	// For jobs which do not specify containers, we use the image
	// configured in the SEMAPHORE_DEFAULT_CONTAINER_IMAGE environment variable.
	// TODO: use the same image being used by the agent itself.
	if e.jobRequest.Executor == ExecutorTypeShell {
		return []corev1.Container{
			{
				Name:  "main",
				Image: os.Getenv("SEMAPHORE_DEFAULT_CONTAINER_IMAGE"),
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "environment",
						ReadOnly:  true,
						MountPath: "/tmp/injected",
					},
				},

				// TODO: little hack to make it work with my kubernetes local cluster
				ImagePullPolicy: corev1.PullNever,

				// The k8s pod shouldn't finish, so we sleep infinitely to keep it up.
				Command: []string{"bash", "-c", "sleep infinity"},
			},
		}
	}

	// For jobs which do specify containers, we just relay them to k8s.
	return e.convertContainersFromSemaphore()
}

func (e *KubernetesExecutor) convertContainersFromSemaphore() []corev1.Container {
	semaphoreContainers := e.jobRequest.Compose.Containers
	main, rest := semaphoreContainers[0], semaphoreContainers[1:]

	// The main container needs to be up forever,
	// so we 'sleep infinity' in its command.
	k8sContainers := []corev1.Container{
		{
			Name:    main.Name,
			Image:   main.Image,
			Env:     e.convertEnvVars(main.EnvVars),
			Command: []string{"bash", "-c", "sleep infinity"},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "environment",
					ReadOnly:  true,
					MountPath: "/tmp/injected",
				},
			},
			// TODO: little hack to make it work with my kubernetes local cluster
			ImagePullPolicy: corev1.PullNever,
		},
	}

	// The rest of the containers will just follow whatever
	// their images are already configured to do.
	for _, container := range rest {
		k8sContainers = append(k8sContainers, corev1.Container{
			Name:  container.Name,
			Image: container.Image,
			Env:   e.convertEnvVars(container.EnvVars),
		})
	}

	return k8sContainers
}

func (e *KubernetesExecutor) convertEnvVars(envVarsFromSemaphore []api.EnvVar) []corev1.EnvVar {
	k8sEnvVars := []corev1.EnvVar{}

	for _, envVar := range envVarsFromSemaphore {
		v, _ := base64.StdEncoding.DecodeString(envVar.Value)
		k8sEnvVars = append(k8sEnvVars, corev1.EnvVar{
			Name:  envVar.Name,
			Value: string(v),
		})
	}

	return k8sEnvVars
}

// TODO: support for private images
func (e *KubernetesExecutor) imagePullSecrets() []corev1.LocalObjectReference {
	return []corev1.LocalObjectReference{}
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

	var pod corev1.Pod
	err := retry.RetryWithConstantWait(retry.RetryOptions{
		Task:                 "Waiting for pod to be ready",
		MaxAttempts:          60,
		DelayBetweenAttempts: time.Second,
		Fn: func() error {
			p, err := e.findPod()
			if err != nil {
				return err
			}

			pod = *p
			return nil
		},
	})

	if err != nil {
		log.Errorf("Failed to create pod: %v", err)
		e.logger.LogCommandOutput("Failed to find pod\n")
		e.logger.LogCommandOutput(err.Error())

		exitCode = 1
		return exitCode
	}

	e.logger.LogCommandOutput("Starting a new bash session in the pod\n")

	// #nosec
	executable := "kubectl"
	args := []string{
		"exec",
		"-it",
		pod.Name,
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

func (e *KubernetesExecutor) findPod() (*corev1.Pod, error) {
	pod, err := e.k8sClientset.CoreV1().
		Pods(e.k8sNamespace).
		Get(context.Background(), e.podName, v1.GetOptions{})

	if err != nil {
		return nil, err
	}

	// If the pod already finished, something went wrong.
	if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
		e.logger.LogCommandOutput(fmt.Sprintf("Pod ended up in %s state...\n", pod.Status.Phase))
		return nil, fmt.Errorf("pod '%s' already finished with status %s", pod.Name, pod.Status.Phase)
	}

	if pod.Status.Phase == corev1.PodPending {
		e.logger.LogCommandOutput("Pod is still pending - waiting...\n")
		log.Infof("Pod '%s', is still pending - waiting...", pod.Name)
		return nil, fmt.Errorf("pod in pending state")
	}

	for _, container := range pod.Status.ContainerStatuses {
		if !container.Ready {
			log.Info("container '%s' is not ready yet - waiting...", container.Name)
			return nil, fmt.Errorf("container '%s' is not ready yet", container.Name)
		}
	}

	log.Info("Pod is ready.")
	e.logger.LogCommandOutput("Pod is ready.\n")
	return pod, nil
}

// All the environment has already been exposed to the main container
// through a temporary secret created before the k8s pod was created, and used in the pod spec.
// Here, we just need to source that file through the PTY session.
func (e *KubernetesExecutor) ExportEnvVars(envVars []api.EnvVar, hostEnvVars []config.HostEnvVar) int {
	commandStartedAt := int(time.Now().Unix())
	directive := "Exporting environment variables"
	exitCode := 0

	e.logger.LogCommandStarted(directive)

	defer func() {
		commandFinishedAt := int(time.Now().Unix())
		e.logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)
	}()

	// All the environment variables were already put into a secret,
	// and injected into /tmp/injected/.env, and will be sourced below.
	// Here, we just need to include them in the job's output.
	for _, envVar := range envVars {
		e.logger.LogCommandOutput(fmt.Sprintf("Exporting %s\n", envVar.Name))
	}

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
	if e.pod != nil {
		err := e.k8sClientset.CoreV1().
			Pods(e.k8sNamespace).
			Delete(context.Background(), e.podName, v1.DeleteOptions{})

		if err != nil {
			log.Errorf("Error deleting pod '%s': %v\n", e.podName, err)
		}
	}

	if e.secret != nil {
		err := e.k8sClientset.CoreV1().
			Secrets(e.k8sNamespace).
			Delete(context.Background(), e.secretName, v1.DeleteOptions{})

		if err != nil {
			log.Errorf("Error deleting k8s secret '%s': %v\n", e.secretName, err)
		}
	}
}

func (e *KubernetesExecutor) removeLocalResources() {
	envFileName := filepath.Join(os.TempDir(), ".env")
	if err := os.Remove(envFileName); err != nil {
		log.Errorf("Error removing local file '%s': %v", envFileName)
	}
}
