package kubernetes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/ghodss/yaml"
	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	"github.com/semaphoreci/agent/pkg/docker"
	"github.com/semaphoreci/agent/pkg/retry"
	"github.com/semaphoreci/agent/pkg/shell"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// JobIDLabel is added to every resource created for a job,
// so they can be found by the Semaphore job ID.
const JobIDLabel = "semaphoreci.com/job-id"

const (
	defaultDeleteAttempts = 5
	defaultDeleteInterval = 2 * time.Second
	apiRequestTimeout     = 30 * time.Second

	// Each delete attempt gets a shorter deadline, and all of them are
	// also bounded by the deadline the caller passes in.
	deleteRequestTimeout = 10 * time.Second

	ownerLookupAttempts = 3
)

// A variable so tests don't need to wait for it.
var ownerLookupInterval = time.Second

type Config struct {
	Namespace                 string
	ImageValidator            *ImageValidator
	PodSpecDecoratorConfigMap string
	PodPollingAttempts        int
	Labels                    map[string]string
	PodPollingInterval        time.Duration
	DefaultImage              string

	// The name of the pod the agent itself is running in.
	// If set, and the pod can be found in the namespace, the resources
	// created for the job are owned by it, so Kubernetes garbage-collects
	// them if the agent pod goes away without cleaning them up.
	OwnerPodName string

	// If greater than zero, and the pod spec decorator does not set one,
	// used as the activeDeadlineSeconds for the job pod.
	PodActiveDeadlineSeconds int64

	DeleteAttempts int
	DeleteInterval time.Duration
}

func (c *Config) LabelMap() map[string]string {
	if c.Labels == nil {
		return map[string]string{}
	}

	return c.Labels
}

// Labels for a resource created for a job.
// A copy, so the shared configuration is never modified.
func (c *Config) labelsForJob(jobID string) map[string]string {
	labels := map[string]string{}
	for k, v := range c.LabelMap() {
		labels[k] = v
	}

	if jobID != "" {
		labels[JobIDLabel] = jobID
	}

	return labels
}

func (c *Config) deleteAttempts() int {
	if c.DeleteAttempts <= 0 {
		return defaultDeleteAttempts
	}

	return c.DeleteAttempts
}

func (c *Config) deleteInterval() time.Duration {
	if c.DeleteInterval <= 0 {
		return defaultDeleteInterval
	}

	return c.DeleteInterval
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
	clientset            kubernetes.Interface
	config               Config
	podSpec              *corev1.PodSpec
	mainContainerSpec    *corev1.Container
	sidecarContainerSpec *corev1.Container
	ownerReferences      []v1.OwnerReference

	// Parent of every request made to create or wait for the job's resources.
	// Canceled when the job is stopped, so a slow API server does not delay it.
	// Deletions do not use it: they need to run after the cancelation.
	requestsCtx    context.Context
	cancelRequests context.CancelFunc

	// Set when a create request failed without a definitive answer from
	// the API server, like when it is canceled or times out. The API server
	// may still create the resource after that, even after it was deleted.
	createOutcomeUnknown atomic.Bool
}

func NewKubernetesClient(clientset kubernetes.Interface, config Config) (*KubernetesClient, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config is invalid: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	c := &KubernetesClient{
		clientset:      clientset,
		config:         config,
		requestsCtx:    ctx,
		cancelRequests: cancel,
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

// We use github.com/ghodss/yaml here
// because it can deserialise from YAML by using the json
// struct tags that are defined in the K8s API object structs.
func (c *KubernetesClient) LoadPodSpec() error {
	if c.config.PodSpecDecoratorConfigMap == "" {
		return nil
	}

	ctx, cancel := c.requestContext()
	defer cancel()

	configMap, err := c.clientset.CoreV1().
		ConfigMaps(c.config.Namespace).
		Get(ctx, c.config.PodSpecDecoratorConfigMap, v1.GetOptions{})

	if err != nil {
		return fmt.Errorf("error finding configmap '%s': %v", c.config.PodSpecDecoratorConfigMap, err)
	}

	podSpecRaw, exists := configMap.Data["pod"]
	if !exists {
		log.Infof("No 'pod' key in '%s' - skipping pod decoration", c.config.PodSpecDecoratorConfigMap)
		c.podSpec = nil
	} else {
		var podSpec corev1.PodSpec
		err = yaml.Unmarshal([]byte(podSpecRaw), &podSpec)
		if err != nil {
			return fmt.Errorf("error unmarshaling pod spec from configmap '%s': %v", c.config.PodSpecDecoratorConfigMap, err)
		}

		c.podSpec = &podSpec
	}

	mainContainerSpecRaw, exists := configMap.Data["mainContainer"]
	if !exists {
		log.Infof("No 'mainContainer' key in '%s' - skipping main container decoration", c.config.PodSpecDecoratorConfigMap)
		c.mainContainerSpec = nil
	} else {
		var mainContainer corev1.Container
		err = yaml.Unmarshal([]byte(mainContainerSpecRaw), &mainContainer)
		if err != nil {
			return fmt.Errorf("error unmarshaling main container spec from configmap '%s': %v", c.config.PodSpecDecoratorConfigMap, err)
		}

		c.mainContainerSpec = &mainContainer
	}

	sidecarContainerSpecRaw, exists := configMap.Data["sidecarContainers"]
	if !exists {
		log.Infof("No 'sidecarContainers' key in '%s' - skipping sidecar containers decoration", c.config.PodSpecDecoratorConfigMap)
		c.sidecarContainerSpec = nil
	} else {
		var sidecarContainer corev1.Container
		err = yaml.Unmarshal([]byte(sidecarContainerSpecRaw), &sidecarContainer)
		if err != nil {
			return fmt.Errorf("error unmarshaling sidecar containers spec from configmap '%s': %v", c.config.PodSpecDecoratorConfigMap, err)
		}

		c.sidecarContainerSpec = &sidecarContainer
	}

	return nil
}

// LoadOwnerReference finds the pod the agent is running in, and makes it
// the owner of every resource created for the job from now on. If the agent
// pod is deleted without the agent cleaning up (eviction, OOM kill, node loss),
// Kubernetes garbage-collects the job pod and its secrets.
//
// Failing to find the agent pod is not fatal: the job runs as before,
// with resources that are only removed by the agent itself.
func (c *KubernetesClient) LoadOwnerReference() {
	c.ownerReferences = nil
	if c.config.OwnerPodName == "" {
		log.Info("Agent pod name is unknown - job resources will not have an owner")
		return
	}

	pod, err := c.getOwnerPod()
	if err != nil {
		log.Errorf("Could not find agent pod '%s' - job resources will not have an owner: %v", c.config.OwnerPodName, err)
		return
	}

	// We do not set blockOwnerDeletion:
	// deleting the agent pod should never wait for the job resources,
	// and setting it requires extra permissions on the owner.
	c.ownerReferences = []v1.OwnerReference{
		{
			APIVersion: "v1",
			Kind:       "Pod",
			Name:       pod.Name,
			UID:        pod.UID,
		},
	}

	log.Infof("Job resources will be owned by agent pod '%s'", pod.Name)
}

// A missing pod, or no permission to see it, will not change by retrying.
// Anything else, like a timeout, gets a few more tries, unless the job is stopped.
func (c *KubernetesClient) getOwnerPod() (*corev1.Pod, error) {
	var lastErr error
	for attempt := 1; attempt <= ownerLookupAttempts; attempt++ {
		ctx, cancel := c.requestContext()
		pod, err := c.clientset.CoreV1().
			Pods(c.config.Namespace).
			Get(ctx, c.config.OwnerPodName, v1.GetOptions{})
		cancel()

		if err == nil {
			return pod, nil
		}

		lastErr = err
		if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
			return nil, err
		}

		if attempt < ownerLookupAttempts {
			log.Warnf("Error finding agent pod '%s' (attempt %d) - retrying: %v", c.config.OwnerPodName, attempt, err)
			select {
			case <-time.After(ownerLookupInterval):
			case <-c.requestsCtx.Done():
				return nil, err
			}
		}
	}

	return nil, lastErr
}

func (c *KubernetesClient) requestContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.requestsCtx, apiRequestTimeout)
}

// CancelRequests aborts in-flight and future requests to create
// or wait for the job's resources. Deletions are not affected.
func (c *KubernetesClient) CancelRequests() {
	c.cancelRequests()
}

// CreateOutcomeUnknown reports if a resource may have been created
// even though its create request failed.
func (c *KubernetesClient) CreateOutcomeUnknown() bool {
	return c.createOutcomeUnknown.Load()
}

// A create request that failed with an error from the API server
// rejecting it did not create anything. Anything else, like a canceled
// request, a timeout, or a server error, might still have created it.
func (c *KubernetesClient) recordCreateError(err error) {
	var status apierrors.APIStatus
	if !errors.As(err, &status) ||
		apierrors.IsTimeout(err) ||
		apierrors.IsServerTimeout(err) ||
		apierrors.IsInternalError(err) ||
		apierrors.IsServiceUnavailable(err) ||
		apierrors.IsUnexpectedServerError(err) {
		c.createOutcomeUnknown.Store(true)
	}
}

func (c *KubernetesClient) objectMeta(name, jobID string) v1.ObjectMeta {
	return v1.ObjectMeta{
		Name:            name,
		Namespace:       c.config.Namespace,
		Labels:          c.config.labelsForJob(jobID),
		OwnerReferences: append([]v1.OwnerReference{}, c.ownerReferences...),
	}
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

	// #nosec
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
		ObjectMeta: c.objectMeta(name, jobRequest.JobID),
		Type:       corev1.SecretTypeOpaque,
		Immutable:  &immutable,
		StringData: data,
	}

	ctx, cancel := c.requestContext()
	defer cancel()

	_, err = c.clientset.CoreV1().
		Secrets(c.config.Namespace).
		Create(ctx, &secret, v1.CreateOptions{})

	if err != nil {
		c.recordCreateError(err)
		return fmt.Errorf("error creating secret '%s': %v", name, err)
	}

	return nil
}

func (c *KubernetesClient) CreateImagePullSecret(secretName, jobID string, credentials []api.ImagePullCredentials) error {
	secret, err := c.buildImagePullSecret(secretName, jobID, credentials)
	if err != nil {
		return fmt.Errorf("error building image pull secret spec for '%s': %v", secretName, err)
	}

	ctx, cancel := c.requestContext()
	defer cancel()

	_, err = c.clientset.CoreV1().
		Secrets(c.config.Namespace).
		Create(ctx, secret, v1.CreateOptions{})
	if err != nil {
		c.recordCreateError(err)
		return fmt.Errorf("error creating image pull secret '%s': %v", secretName, err)
	}

	return nil
}

func (c *KubernetesClient) buildImagePullSecret(secretName, jobID string, credentials []api.ImagePullCredentials) (*corev1.Secret, error) {
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
		ObjectMeta: c.objectMeta(secretName, jobID),
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

	ctx, cancel := c.requestContext()
	defer cancel()

	_, err = c.clientset.CoreV1().
		Pods(c.config.Namespace).
		Create(ctx, pod, v1.CreateOptions{})

	if err != nil {
		c.recordCreateError(err)
		return fmt.Errorf("error creating pod: %v", err)
	}

	return nil
}

func (c *KubernetesClient) podSpecFromJobRequest(podName string, envSecretName string, imagePullSecret string, jobRequest *api.JobRequest) (*corev1.Pod, error) {
	containers, err := c.containers(jobRequest.Compose.Containers)
	if err != nil {
		return nil, fmt.Errorf("error building containers for pod spec: %v", err)
	}

	var spec *corev1.PodSpec
	if c.podSpec != nil {
		spec = c.podSpec.DeepCopy()
	} else {
		spec = &corev1.PodSpec{}
	}

	spec.Containers = containers
	spec.HostAliases = c.hostAliases(containers)
	spec.ImagePullSecrets = append(spec.ImagePullSecrets, c.imagePullSecrets(imagePullSecret)...)
	spec.RestartPolicy = corev1.RestartPolicyNever

	// A safety net for pods left behind by an agent that could not clean up.
	// A deadline set by the pod spec decorator always takes precedence.
	if spec.ActiveDeadlineSeconds == nil && c.config.PodActiveDeadlineSeconds > 0 {
		deadline := c.config.PodActiveDeadlineSeconds
		spec.ActiveDeadlineSeconds = &deadline
	}

	spec.Volumes = append(spec.Volumes, corev1.Volume{
		Name: "environment",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: envSecretName,
			},
		},
	})

	return &corev1.Pod{
		Spec:       *spec,
		ObjectMeta: c.objectMeta(podName, jobRequest.JobID),
	}, nil
}

func (c *KubernetesClient) hostAliases(containers []corev1.Container) []corev1.HostAlias {
	// If only 1 container is used for the job, there's no need for host aliases.
	// We only use the ones used by the pod spec, or none if no pod spec is specified.
	if len(containers) == 1 {
		if c.podSpec != nil {
			return c.podSpec.HostAliases
		}

		return []corev1.HostAlias{}
	}

	// Otherwise, we add a hostAlias for each sidecar container,
	// so they can also be addressable via their names and not only localhost.
	alias := corev1.HostAlias{IP: "127.0.0.1", Hostnames: []string{}}
	for _, c := range containers[1:] {
		alias.Hostnames = append(alias.Hostnames, c.Name)
	}

	return []corev1.HostAlias{alias}
}

func (c *KubernetesClient) imagePullSecrets(imagePullSecret string) []corev1.LocalObjectReference {
	secrets := []corev1.LocalObjectReference{}

	// Use the temporary secret created for the credentials sent in the job definition.
	if imagePullSecret != "" {
		secrets = append(secrets, corev1.LocalObjectReference{Name: imagePullSecret})
	}

	return secrets
}

func (c *KubernetesClient) containers(apiContainers []api.Container) ([]corev1.Container, error) {
	if len(apiContainers) > 0 {
		if err := c.config.ImageValidator.Validate(apiContainers); err != nil {
			return []corev1.Container{}, fmt.Errorf("error validating images: %v", err)
		}

		return c.convertContainersFromSemaphore(apiContainers), nil
	}

	if c.config.DefaultImage != "" {
		containers := []api.Container{}

		containers = append(containers, api.Container{Image: c.config.DefaultImage})

		return c.convertContainersFromSemaphore(containers), nil
	}

	return []corev1.Container{}, fmt.Errorf(
		"no containers specified in Semaphore YAML, and no default container is provided",
		)
}

func (c *KubernetesClient) buildMainContainer(mainContainerFromAPI *api.Container) corev1.Container {
	var mainContainer *corev1.Container
	if c.mainContainerSpec != nil {
		mainContainer = c.mainContainerSpec.DeepCopy()
	} else {
		mainContainer = &corev1.Container{}
	}

	mainContainer.Name = "main"
	mainContainer.Image = mainContainerFromAPI.Image
	mainContainer.Env = append(mainContainer.Env, c.convertEnvVars(mainContainerFromAPI.EnvVars)...)
	mainContainer.Command = []string{"bash", "-c", "sleep infinity"}

	// We append the volume mount for the environment variables secret,
	// to the list of volume mounts configured.
	mainContainer.VolumeMounts = append(mainContainer.VolumeMounts, corev1.VolumeMount{
		Name:      "environment",
		ReadOnly:  true,
		MountPath: "/tmp/injected",
	})

	return *mainContainer
}

func (c *KubernetesClient) convertContainersFromSemaphore(apiContainers []api.Container) []corev1.Container {
	main, rest := apiContainers[0], apiContainers[1:]

	// The main container needs to be up forever,
	// so we 'sleep infinity' in its command.
	k8sContainers := []corev1.Container{c.buildMainContainer(&main)}

	// The rest of the containers will just follow whatever
	// their images are already configured to do.
	for _, container := range rest {
		c := c.buildSidecarContainer(container)
		k8sContainers = append(k8sContainers, *c)
	}

	return k8sContainers
}

func (c *KubernetesClient) buildSidecarContainer(apiContainer api.Container) *corev1.Container {
	var sidecarContainer *corev1.Container
	if c.sidecarContainerSpec != nil {
		sidecarContainer = c.sidecarContainerSpec.DeepCopy()
	} else {
		sidecarContainer = &corev1.Container{}
	}

	sidecarContainer.Name = apiContainer.Name
	sidecarContainer.Image = apiContainer.Image
	sidecarContainer.Env = append(sidecarContainer.Env, c.convertEnvVars(apiContainer.EnvVars)...)
	return sidecarContainer
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
			r = c.findPod(ctx, name)
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

func (c *KubernetesClient) findPod(ctx context.Context, name string) findPodResult {
	ctx, cancel := context.WithTimeout(ctx, apiRequestTimeout)
	defer cancel()

	pod, err := c.clientset.CoreV1().
		Pods(c.config.Namespace).
		Get(ctx, name, v1.GetOptions{})

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

func (c *KubernetesClient) DeletePod(ctx context.Context, name string) error {
	return c.deleteWithRetries(ctx, fmt.Sprintf("Delete pod %s", name), func(ctx context.Context) error {
		return c.clientset.CoreV1().
			Pods(c.config.Namespace).
			Delete(ctx, name, v1.DeleteOptions{})
	})
}

func (c *KubernetesClient) DeleteSecret(ctx context.Context, name string) error {
	return c.deleteWithRetries(ctx, fmt.Sprintf("Delete secret %s", name), func(ctx context.Context) error {
		return c.clientset.CoreV1().
			Secrets(c.config.Namespace).
			Delete(ctx, name, v1.DeleteOptions{})
	})
}

// A resource that is already gone is what we want, so it is not an error.
// Anything else is retried a bounded number of times, within ctx's deadline.
func (c *KubernetesClient) deleteWithRetries(ctx context.Context, task string, fn func(ctx context.Context) error) error {
	return retry.RetryWithConstantWaitAndContext(ctx, retry.RetryOptions{
		Task:                 task,
		MaxAttempts:          c.config.deleteAttempts(),
		DelayBetweenAttempts: c.config.deleteInterval(),
		Fn: func() error {
			attemptCtx, cancel := context.WithTimeout(ctx, deleteRequestTimeout)
			defer cancel()

			err := fn(attemptCtx)
			if err == nil || apierrors.IsNotFound(err) {
				return nil
			}

			return err
		},
	})
}
