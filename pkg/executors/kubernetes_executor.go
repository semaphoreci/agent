package executors

import (
	"context"
	"encoding/base64"
	"os"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"

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
	k8sJobsInterface batchv1.JobInterface
	k8sNamespace     string
	jobRequest       *api.JobRequest
	job              *apibatchv1.Job
	logger           *eventlogger.Logger
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

func (e *KubernetesExecutor) newJob() *apibatchv1.Job {

	// This k8s job will have a single pod, so we set parallelism to 1.
	parallelism := int32(1)

	// The k8s job will not finish by itself
	// We are deleting it once all the commands in the Semaphore are executed.
	// But, let's set spec.completions=1 just in case.
	completions := int32(1)

	// TODO: not really sure what this does, but pretty sure we want it as 1 too.
	backoffLimit := int32(1)

	return &apibatchv1.Job{
		ObjectMeta: v1.ObjectMeta{
			Namespace: e.k8sNamespace,

			// TODO: put agent name here too
			Name: e.jobRequest.ID,

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
				Name:  "semaphore-job",
				Image: os.Getenv("SEMAPHORE_DEFAULT_CONTAINER_IMAGE"),

				// The k8s pod shouldn't finish, so we sleep infinitely to keep it up.
				Command: []string{"sleep infinity"},
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
			Command: []string{"sleep infinity"},
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

func (e *KubernetesExecutor) Start() int {
	// TODO: create PTY session on k8s job main container
	return 0
}

func (e *KubernetesExecutor) ExportEnvVars([]api.EnvVar, []config.HostEnvVar) int {
	// TODO: this will probably look the same as in the other executors
	// TODO: consider moving everything to a shared common function
	return 0
}

func (e *KubernetesExecutor) InjectFiles([]api.File) int {
	// TODO: this will probably look the same as in the other executors
	// TODO: consider moving everything to a shared common function
	return 0
}

func (e *KubernetesExecutor) RunCommand(string, bool, string) int {
	// TODO: this will probably look the same as in the other executors
	// TODO: consider moving everything to a shared common function
	return 0
}

func (e *KubernetesExecutor) RunCommandWithOptions(options CommandOptions) int {
	// TODO: this will probably look the same as in the other executors
	// TODO: consider moving everything to a shared common function
	return 0
}

func (e *KubernetesExecutor) Stop() int {
	// TODO: stop commands, if running, and delete k8s job
	return 0
}

func (e *KubernetesExecutor) Cleanup() int {
	// TODO: not really sure what to do here yet.
	return 0
}
