package kubernetes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/ghodss/yaml"
	"github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	"github.com/semaphoreci/agent/pkg/docker"
	"github.com/semaphoreci/agent/pkg/retry"
	"github.com/semaphoreci/agent/pkg/shell"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Config struct {
	Namespace          string
	PodSpecConfigMap   string
	PodPollingAttempts int
	PodPollingInterval time.Duration
}

func (c *Config) PollingInterval() time.Duration {
	if c.PodPollingInterval == 0 {
		return time.Second
	}

	return c.PodPollingInterval
}

func (c *Config) PollingAttempts() int {
	if c.PodPollingAttempts == 0 {
		return 60
	}

	return c.PodPollingAttempts
}

func (c *Config) Validate() error {
	if c.Namespace == "" {
		return fmt.Errorf("namespace must be specified")
	}

	return nil
}

type KubernetesClient struct {
	clientset kubernetes.Interface
	config    Config
	podSpec   *corev1.PodSpec
}

func NewKubernetesClient(clientset kubernetes.Interface, config Config) (*KubernetesClient, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config is invalid: %v", err)
	}

	c := &KubernetesClient{
		clientset: clientset,
		config:    config,
	}

	err := c.Init()
	if err != nil {
		return nil, err
	}

	return c, nil
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

func NewClientsetFromConfig() (kubernetes.Interface, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("error getting user home directory: %v", err)
	}

	kubeConfigPath := filepath.Join(homeDir, ".kube", "config")
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("error getting Kubernetes config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating kubernetes clientset from config file: %v", err)
	}

	return clientset, nil
}

func (c *KubernetesClient) Init() error {
	if c.config.PodSpecConfigMap == "" {
		return nil
	}

	configMap, err := c.clientset.CoreV1().
		ConfigMaps(c.config.Namespace).
		Get(context.TODO(), c.config.PodSpecConfigMap, v1.GetOptions{})

	if err != nil {
		return fmt.Errorf("error finding configmap '%s': %v", c.config.PodSpecConfigMap, err)
	}

	podSpecRaw, exists := configMap.Data[".podSpec"]
	if !exists {
		return fmt.Errorf("required key '.podSpec' does not exist in configmap '%s'", c.config.PodSpecConfigMap)
	}

	// We use github.com/ghodss/yaml here
	// because it can deserialise from YAML by using the json
	// struct tags that are defined in the K8s API object structs.
	var podSpec corev1.PodSpec
	err = yaml.Unmarshal([]byte(podSpecRaw), &podSpec)
	if err != nil {
		return fmt.Errorf("error unmarshaling pod spec from configmap '%s': %v", c.config.PodSpecConfigMap, err)
	}

	c.podSpec = &podSpec
	return nil
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
		ObjectMeta: v1.ObjectMeta{Name: name, Namespace: c.config.Namespace},
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

func (c *KubernetesClient) CreateImagePullSecret(secretName string, credentials []api.ImagePullCredentials) error {
	secret, err := c.buildImagePullSecret(secretName, credentials)
	if err != nil {
		return fmt.Errorf("error building image pull secret spec for '%s': %v", secretName, err)
	}

	_, err = c.clientset.CoreV1().
		Secrets(c.config.Namespace).
		Create(context.Background(), secret, v1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("error creating image pull secret '%s': %v", secretName, err)
	}

	return nil
}

func (c *KubernetesClient) buildImagePullSecret(secretName string, credentials []api.ImagePullCredentials) (*corev1.Secret, error) {
	data, err := docker.NewDockerConfig(credentials)
	if err != nil {
		return nil, fmt.Errorf("error creating docker config for '%s': %v", secretName, err)
	}

	json, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("error serializing docker config for '%s': %v", secretName, err)
	}

	immutable := true
	secret := corev1.Secret{
		ObjectMeta: v1.ObjectMeta{Name: secretName, Namespace: c.config.Namespace},
		Type:       corev1.SecretTypeDockerConfigJson,
		Immutable:  &immutable,
		Data:       map[string][]byte{corev1.DockerConfigJsonKey: json},
	}

	return &secret, nil
}

func (c *KubernetesClient) CreatePod(name string, envSecretName string, imagePullSecret string, jobRequest *api.JobRequest) error {
	pod, err := c.podSpecFromJobRequest(name, envSecretName, imagePullSecret, jobRequest)
	if err != nil {
		return fmt.Errorf("error building pod spec: %v", err)
	}

	_, err = c.clientset.CoreV1().
		Pods(c.config.Namespace).
		Create(context.TODO(), pod, v1.CreateOptions{})

	if err != nil {
		return fmt.Errorf("error creating pod: %v", err)
	}

	return nil
}

func (c *KubernetesClient) podSpecFromJobRequest(podName string, envSecretName string, imagePullSecret string, jobRequest *api.JobRequest) (*corev1.Pod, error) {
	containers, err := c.containers(jobRequest.Compose.Containers)
	if err != nil {
		return nil, fmt.Errorf("error building containers for pod spec: %v", err)
	}

	spec := corev1.PodSpec{
		Containers:       containers,
		ImagePullSecrets: c.imagePullSecrets(imagePullSecret),
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
	}, nil
}

func (c *KubernetesClient) imagePullSecrets(imagePullSecret string) []corev1.LocalObjectReference {
	secrets := []corev1.LocalObjectReference{}

	// Use the secrets previously created, and passed to the agent through its configuration.
	secrets = append(secrets, c.podSpec.ImagePullSecrets...)

	// Use the temporary secret created for the credentials sent in the job definition.
	if imagePullSecret != "" {
		secrets = append(secrets, corev1.LocalObjectReference{Name: imagePullSecret})
	}

	return secrets
}

func (c *KubernetesClient) containers(apiContainers []api.Container) ([]corev1.Container, error) {

	// If the job specifies containers in the YAML, we use them.
	if len(apiContainers) > 0 {
		return c.convertContainersFromSemaphore(apiContainers), nil
	}

	// For jobs which do not specify containers, we require the default image to be specified.
	if c.podSpec == nil || len(c.podSpec.Containers) == 0 || c.podSpec.Containers[0].Image == "" {
		return []corev1.Container{}, fmt.Errorf("no default image specified")
	}

	// We copy the rest of the containers configured.
	containers := []corev1.Container{c.buildMainContainer(nil)}
	return append(containers, c.podSpec.Containers[1:]...), nil
}

func (c *KubernetesClient) buildMainContainer(mainContainerFromAPI *api.Container) corev1.Container {
	mainContainer := c.podSpec.Containers[0]
	mainContainer.Name = "main"

	// We append the volume mount for the environment variables secret,
	// to the list of volume mounts configured.
	mainContainer.VolumeMounts = append(mainContainer.VolumeMounts, corev1.VolumeMount{
		Name:      "environment",
		ReadOnly:  true,
		MountPath: "/tmp/injected",
	})

	// Override the image with the one specified in the job YAML.
	if mainContainerFromAPI != nil {
		mainContainer.Image = mainContainerFromAPI.Image
	}

	// Extend the env vars with the ones that come from the job YAML.
	if mainContainerFromAPI != nil {
		mainContainer.Env = append(mainContainer.Env, c.convertEnvVars(mainContainerFromAPI.EnvVars)...)
	}

	// We don't let people customize the command.
	// since we need the pod to be up.
	mainContainer.Command = []string{"bash", "-c", "sleep infinity"}
	return mainContainer
}

func (c *KubernetesClient) convertContainersFromSemaphore(apiContainers []api.Container) []corev1.Container {
	main, rest := apiContainers[0], apiContainers[1:]

	// The main container needs to be up forever,
	// so we 'sleep infinity' in its command.
	k8sContainers := []corev1.Container{c.buildMainContainer(&main)}

	// The rest of the containers will just follow whatever
	// their images are already configured to do.
	for i, container := range rest {
		c := c.buildSidecarContainer(container, c.podSpec.Containers[i])
		k8sContainers = append(k8sContainers, *c)
	}

	return k8sContainers
}

func (c *KubernetesClient) buildSidecarContainer(apiContainer api.Container, baseContainer corev1.Container) *corev1.Container {
	new := baseContainer.DeepCopy()
	new.Name = apiContainer.Name
	new.Image = apiContainer.Image
	new.Env = append(baseContainer.Env, c.convertEnvVars(apiContainer.EnvVars)...)
	return new
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

func (c *KubernetesClient) WaitForPod(ctx context.Context, name string, logFn func(string)) error {
	var r findPodResult

	err := retry.RetryWithConstantWaitAndContext(ctx, retry.RetryOptions{
		Task:                 "Waiting for pod to be ready",
		MaxAttempts:          c.config.PollingAttempts(),
		DelayBetweenAttempts: c.config.PollingInterval(),
		HideError:            true,
		Fn: func() error {
			r = c.findPod(name)
			if r.continueWaiting {
				if r.err != nil {
					logFn(r.err.Error())
				}

				return r.err
			}

			return nil
		},
	})

	// If we stopped the retrying,
	// but still an error occurred, we need to report that
	if !r.continueWaiting && r.err != nil {
		return r.err
	}

	return err
}

type findPodResult struct {
	continueWaiting bool
	err             error
}

func (c *KubernetesClient) findPod(name string) findPodResult {
	pod, err := c.clientset.CoreV1().
		Pods(c.config.Namespace).
		Get(context.Background(), name, v1.GetOptions{})

	if err != nil {
		return findPodResult{continueWaiting: true, err: err}
	}

	// If the pod already finished, something went wrong.
	if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
		return findPodResult{
			continueWaiting: false,
			err: fmt.Errorf(
				"pod '%s' already finished with status %s - reason: '%v', message: '%v', statuses: %v",
				pod.Name,
				pod.Status.Phase,
				pod.Status.Reason,
				pod.Status.Message,
				c.getContainerStatuses(pod.Status.ContainerStatuses),
			),
		}
	}

	// if one of the pod's containers isn't ready, we need to wait
	for _, container := range pod.Status.ContainerStatuses {

		// If the reason for a container to be in the waiting state
		// is Kubernetes not being able to pull its image,
		// we should not wait for the whole pod start timeout until the job fails.
		if c.failedToPullImage(container.State.Waiting) {
			return findPodResult{
				continueWaiting: false,
				err: fmt.Errorf(
					"failed to pull image for '%s': %v",
					container.Name,
					c.getContainerStatuses(pod.Status.ContainerStatuses),
				),
			}
		}

		// If the container is just not ready yet, we wait.
		if !container.Ready {
			return findPodResult{
				continueWaiting: true,
				err: fmt.Errorf(
					"container '%s' is not ready yet - statuses: %v",
					container.Name,
					c.getContainerStatuses(pod.Status.ContainerStatuses),
				),
			}
		}
	}

	// if we get here, all the containers are ready
	// but the pod is still pending, so we need to wait too.
	if pod.Status.Phase == corev1.PodPending {
		return findPodResult{
			continueWaiting: true,
			err: fmt.Errorf(
				"pod in pending state - statuses: %v",
				c.getContainerStatuses(pod.Status.ContainerStatuses),
			),
		}
	}

	// If we get here, everything is ready, we can start the job.
	return findPodResult{continueWaiting: false, err: nil}
}

func (c *KubernetesClient) failedToPullImage(state *corev1.ContainerStateWaiting) bool {
	if state == nil {
		return false
	}

	if state.Reason == "ErrImagePull" || state.Reason == "ImagePullBackOff" {
		return true
	}

	return false
}

func (c *KubernetesClient) getContainerStatuses(statuses []corev1.ContainerStatus) []string {
	messages := []string{}
	for _, s := range statuses {
		if s.State.Terminated != nil {
			messages = append(
				messages,
				fmt.Sprintf(
					"container '%s' terminated - reason='%s', message='%s'",
					s.Image,
					s.State.Terminated.Reason,
					s.State.Terminated.Message,
				),
			)
		}

		if s.State.Waiting != nil {
			messages = append(
				messages,
				fmt.Sprintf(
					"container '%s' waiting - reason='%s', message='%s'",
					s.Image,
					s.State.Waiting.Reason,
					s.State.Waiting.Message,
				),
			)
		}

		if s.State.Running != nil {
			messages = append(
				messages,
				fmt.Sprintf(
					"container '%s' is running since %v",
					s.Image,
					s.State.Running.StartedAt,
				),
			)
		}
	}

	return messages
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
