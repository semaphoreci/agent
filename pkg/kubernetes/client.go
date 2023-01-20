package kubernetes

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	"github.com/semaphoreci/agent/pkg/retry"
	"github.com/semaphoreci/agent/pkg/shell"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Config struct {
	Namespace    string
	DefaultImage string
}

func (c *Config) Validate() error {
	if c.Namespace == "" {
		return fmt.Errorf("namespace must be specified")
	}

	if c.DefaultImage == "" {
		return fmt.Errorf("default image must be specified")
	}

	return nil
}

type KubernetesClient struct {
	clientset kubernetes.Interface
	config    Config
}

func NewKubernetesClient(clientset kubernetes.Interface, config Config) (*KubernetesClient, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config is invalid: %v", err)
	}

	return &KubernetesClient{
		clientset: clientset,
		config:    config,
	}, nil
}

func NewInClusterClientset() (kubernetes.Interface, error) {
	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

func (c *KubernetesClient) CreateSecret(name string, jobRequest *api.JobRequest) error {
	environment, err := shell.CreateEnvironment(jobRequest.EnvVars, []config.HostEnvVar{})
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
	for _, file := range jobRequest.Files {
		encodedPath := base64.RawURLEncoding.EncodeToString([]byte(file.Path))
		content, err := file.Decode()
		if err != nil {
			return fmt.Errorf("error decoding file '%s': %v", file.Path, err)
		}

		data[encodedPath] = string(content)
	}

	secret := corev1.Secret{
		ObjectMeta: v1.ObjectMeta{Name: name},
		Type:       corev1.SecretTypeOpaque,
		Immutable:  &immutable,
		StringData: data,
	}

	_, err = c.clientset.CoreV1().
		Secrets(c.config.Namespace).
		Create(context.Background(), &secret, v1.CreateOptions{})

	if err != nil {
		return fmt.Errorf("error creating secret '%s': %v", name, err)
	}

	return nil
}

func (c *KubernetesClient) CreatePod(name string, envSecretName string, jobRequest *api.JobRequest) error {
	_, err := c.clientset.CoreV1().
		Pods(c.config.Namespace).
		Create(context.TODO(), c.podSpecFromJobRequest(name, envSecretName, jobRequest), v1.CreateOptions{})

	if err != nil {
		return fmt.Errorf("error creating pod: %v", err)
	}

	return nil
}

func (c *KubernetesClient) podSpecFromJobRequest(podName string, envSecretName string, jobRequest *api.JobRequest) *corev1.Pod {
	spec := corev1.PodSpec{
		Containers:       c.containers(jobRequest.Compose.Containers),
		ImagePullSecrets: c.imagePullSecrets(),
		RestartPolicy:    corev1.RestartPolicyNever,
		Volumes: []corev1.Volume{
			{
				Name: "environment",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: envSecretName,
					},
				},
			},
		},
	}

	return &corev1.Pod{
		Spec: spec,
		ObjectMeta: v1.ObjectMeta{
			Namespace: c.config.Namespace,
			Name:      podName,
			Labels: map[string]string{
				"app": "semaphore-agent",
			},
		},
	}
}

func (c *KubernetesClient) containers(containers []api.Container) []corev1.Container {

	// For jobs which do not specify containers (shell jobs), we use the default image.
	if len(containers) == 0 {
		return []corev1.Container{
			{
				Name:  "main",
				Image: c.config.DefaultImage,
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
	return c.convertContainersFromSemaphore(containers)
}

func (c *KubernetesClient) convertContainersFromSemaphore(containers []api.Container) []corev1.Container {
	main, rest := containers[0], containers[1:]

	// The main container needs to be up forever,
	// so we 'sleep infinity' in its command.
	k8sContainers := []corev1.Container{
		{
			Name:    main.Name,
			Image:   main.Image,
			Env:     c.convertEnvVars(main.EnvVars),
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
			Env:   c.convertEnvVars(container.EnvVars),
		})
	}

	return k8sContainers
}

func (c *KubernetesClient) convertEnvVars(envVarsFromSemaphore []api.EnvVar) []corev1.EnvVar {
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
func (c *KubernetesClient) imagePullSecrets() []corev1.LocalObjectReference {
	return []corev1.LocalObjectReference{}
}

func (c *KubernetesClient) WaitForPod(name string, logFn func(string)) error {
	return retry.RetryWithConstantWait(retry.RetryOptions{
		Task:                 "Waiting for pod to be ready",
		MaxAttempts:          60,
		DelayBetweenAttempts: time.Second,
		HideError:            true,
		Fn: func() error {
			_, err := c.findPod(name)
			if err != nil {
				logFn(fmt.Sprintf("Pod is not ready yet: %v\n", err))
				return err
			}

			logFn("Pod is ready.\n")
			return nil
		},
	})
}

func (c *KubernetesClient) findPod(name string) (*corev1.Pod, error) {
	pod, err := c.clientset.CoreV1().
		Pods(c.config.Namespace).
		Get(context.Background(), name, v1.GetOptions{})

	if err != nil {
		return nil, err
	}

	// If the pod already finished, something went wrong.
	if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
		return nil, fmt.Errorf("pod '%s' already finished with status %s", pod.Name, pod.Status.Phase)
	}

	if pod.Status.Phase == corev1.PodPending {
		return nil, fmt.Errorf("pod in pending state")
	}

	for _, container := range pod.Status.ContainerStatuses {
		if !container.Ready {
			return nil, fmt.Errorf("container '%s' is not ready yet", container.Name)
		}
	}

	return pod, nil
}

func (c *KubernetesClient) DeletePod(name string) error {
	return c.clientset.CoreV1().
		Pods(c.config.Namespace).
		Delete(context.Background(), name, v1.DeleteOptions{})
}

func (c *KubernetesClient) DeleteSecret(name string) error {
	return c.clientset.CoreV1().
		Secrets(c.config.Namespace).
		Delete(context.Background(), name, v1.DeleteOptions{})
}
