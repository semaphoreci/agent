package executors

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	shell "github.com/semaphoreci/agent/pkg/shell"

	log "github.com/sirupsen/logrus"
	apibatchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	batchv1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	"k8s.io/client-go/rest"
)

type KubernetesExecutor struct {
	k8sClientset     *kubernetes.Clientset
	k8sRestConfig    *rest.Config
	k8sJobsInterface batchv1.JobInterface
	k8sNamespace     string
	jobRequest       *api.JobRequest
	job              *apibatchv1.Job
	logger           *eventlogger.Logger
	Shell            *shell.Shell
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
		k8sRestConfig:    k8sConfig,
		k8sClientset:     clientset,
		k8sJobsInterface: clientset.BatchV1().Jobs(namespace),
		k8sNamespace:     namespace,
		jobRequest:       jobRequest,
		logger:           logger,
	}, nil
}

func (e *KubernetesExecutor) Prepare() int {
	job, err := e.k8sJobsInterface.Create(context.TODO(), e.newJob(), v1.CreateOptions{})
	if err != nil {
		log.Errorf("Error creating k8s job: %v", err)
		return 1
	}

	e.job = job
	return 0
}

func (e *KubernetesExecutor) randomJobName() string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

	b := make([]rune, 12)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}

	return string(b)
}

func (e *KubernetesExecutor) newJob() *apibatchv1.Job {

	// This k8s job will have a single pod, so we set parallelism to 1.
	parallelism := int32(1)

	// The k8s job will not finish by itself
	// We are deleting it once all the commands in the Semaphore are executed.
	// But, let's set spec.completions=1 just in case.
	completions := int32(1)

	// We don't new pods to be created if the initial one fails.
	backoffLimit := int32(0)

	jobName := e.randomJobName()

	return &apibatchv1.Job{
		ObjectMeta: v1.ObjectMeta{
			Namespace: e.k8sNamespace,

			// TODO: use agent name + job ID here.
			// Note: e.jobRequest.ID is not being sent by API.
			Name: jobName,

			// TODO: put agent name here too
			Labels: map[string]string{
				"app": "semaphore-agent",
			},
		},

		Spec: apibatchv1.JobSpec{
			Parallelism:  &parallelism,
			Completions:  &completions,
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers:       e.containers(),
					ImagePullSecrets: e.imagePullSecrets(),
					RestartPolicy:    corev1.RestartPolicyNever,
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
	directive := "Starting containers..."
	exitCode := 0

	e.logger.LogCommandStarted(directive)

	defer func() {
		commandFinishedAt := int(time.Now().Unix())
		e.logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)
	}()

	// TODO: find pod for the job
	// TODO: check if container is ready before trying to create a session there.

	e.logger.LogCommandOutput("Starting a new bash session in the container\n")

	// #nosec
	executable := "kubectl"
	args := []string{
		"exec",
		"-it",
		"<POD_NAME????????>",
		"-c",
		"main",
		"--",
		"bash",
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

func (e *KubernetesExecutor) ExportEnvVars(envVars []api.EnvVar, hostEnvVars []config.HostEnvVar) int {
	commandStartedAt := int(time.Now().Unix())
	directive := "Exporting environment variables"
	exitCode := 0

	e.logger.LogCommandStarted(directive)

	defer func() {
		commandFinishedAt := int(time.Now().Unix())

		e.logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)
	}()

	environment, err := shell.CreateEnvironment(envVars, hostEnvVars)
	if err != nil {
		log.Errorf("Error creating environment: %v", err)
		exitCode = 1
		return exitCode
	}

	envFileName := filepath.Join(os.TempDir(), ".env")
	err = environment.ToFile(envFileName, func(name string) {
		e.logger.LogCommandOutput(fmt.Sprintf("Exporting %s\n", name))
	})

	if err != nil {
		exitCode = 255
		return exitCode
	}

	cmd := fmt.Sprintf("source %s", envFileName)
	exitCode = e.RunCommand(cmd, true, "")
	if exitCode != 0 {
		return exitCode
	}

	cmd = fmt.Sprintf("echo 'source %s' >> ~/.bash_profile", envFileName)
	exitCode = e.RunCommand(cmd, true, "")
	if exitCode != 0 {
		return exitCode
	}

	return exitCode
}

func (e *KubernetesExecutor) InjectFiles(files []api.File) int {
	directive := "Injecting Files"
	commandStartedAt := int(time.Now().Unix())
	exitCode := 0

	e.logger.LogCommandStarted(directive)

	for _, f := range files {
		output := fmt.Sprintf("Injecting %s with file mode %s\n", f.Path, f.Mode)

		e.logger.LogCommandOutput(output)

		content, err := f.Decode()

		if err != nil {
			e.logger.LogCommandOutput("Failed to decode the content of the file.\n")
			exitCode = 1
			return exitCode
		}

		tmpPath := fmt.Sprintf("%s/file", os.TempDir())

		// #nosec
		err = ioutil.WriteFile(tmpPath, []byte(content), 0644)
		if err != nil {
			e.logger.LogCommandOutput(err.Error() + "\n")
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
			e.logger.LogCommandOutput(output + "\n")
			break
		}

		cmd = fmt.Sprintf("cp %s %s", tmpPath, destPath)
		exitCode = e.RunCommand(cmd, true, "")
		if exitCode != 0 {
			output := fmt.Sprintf("Failed to move to destination path %s %s", tmpPath, destPath)
			e.logger.LogCommandOutput(output + "\n")
			break
		}

		cmd = fmt.Sprintf("chmod %s %s", f.Mode, destPath)
		exitCode = e.RunCommand(cmd, true, "")
		if exitCode != 0 {
			output := fmt.Sprintf("Failed to set file mode to %s", f.Mode)
			e.logger.LogCommandOutput(output + "\n")
			break
		}
	}

	commandFinishedAt := int(time.Now().Unix())
	e.logger.LogCommandFinished(directive, exitCode, commandStartedAt, commandFinishedAt)

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

	p := e.Shell.NewProcessWithOutput(options.Command, func(output string) {
		if !options.Silent {
			e.logger.LogCommandOutput(output)
		}
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

	if e.job != nil {
		err := e.k8sJobsInterface.Delete(context.Background(), e.job.Name, v1.DeleteOptions{})
		if err != nil {
			log.Errorf("Error deleting k8s job '%s': %v\n", e.job.Name, err)
		}
	}

	return 0
}

func (e *KubernetesExecutor) Cleanup() int {
	// TODO: not really sure what to do here yet.
	return 0
}
