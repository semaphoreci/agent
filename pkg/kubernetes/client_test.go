package kubernetes

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/semaphoreci/agent/pkg/api"
	assert "github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func Test__CreateSecret(t *testing.T) {
	t.Run("stores .env file in secret", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default"})
		secretName := "mysecret"

		// create secret using job request
		assert.NoError(t, client.CreateSecret(secretName, &api.JobRequest{
			EnvVars: []api.EnvVar{
				{Name: "A", Value: base64.StdEncoding.EncodeToString([]byte("AAA"))},
				{Name: "B", Value: base64.StdEncoding.EncodeToString([]byte("BBB"))},
				{Name: "C", Value: base64.StdEncoding.EncodeToString([]byte("CCC"))},
			},
		}))

		secret, err := clientset.CoreV1().
			Secrets("default").
			Get(context.Background(), secretName, v1.GetOptions{})

		assert.NoError(t, err)
		assert.True(t, *secret.Immutable)
		assert.Equal(t, secret.Name, secretName)
		assert.Empty(t, secret.Labels)
		assert.Equal(t, secret.StringData, map[string]string{
			".env": "export A=AAA\nexport B=BBB\nexport C=CCC\n",
		})
	})

	t.Run("stores files in secret, with base64-encoded keys", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default"})
		secretName := "mysecret"

		// create secret using job request
		assert.NoError(t, client.CreateSecret(secretName, &api.JobRequest{
			EnvVars: []api.EnvVar{
				{Name: "A", Value: base64.StdEncoding.EncodeToString([]byte("AAA"))},
				{Name: "B", Value: base64.StdEncoding.EncodeToString([]byte("BBB"))},
				{Name: "C", Value: base64.StdEncoding.EncodeToString([]byte("CCC"))},
			},
			Files: []api.File{
				{
					Path:    "/tmp/random-file.txt",
					Content: base64.StdEncoding.EncodeToString([]byte("Random content")),
					Mode:    "0600",
				},
				{
					Path:    "/tmp/random-file-2.txt",
					Content: base64.StdEncoding.EncodeToString([]byte("Random content 2")),
					Mode:    "0600",
				},
			},
		}))

		secret, err := clientset.CoreV1().
			Secrets("default").
			Get(context.Background(), secretName, v1.GetOptions{})

		key1 := base64.RawURLEncoding.EncodeToString([]byte("/tmp/random-file.txt"))
		key2 := base64.RawURLEncoding.EncodeToString([]byte("/tmp/random-file-2.txt"))

		assert.NoError(t, err)
		assert.True(t, *secret.Immutable)
		assert.Equal(t, secret.Name, secretName)
		assert.Empty(t, secret.Labels)
		assert.Equal(t, secret.StringData, map[string]string{
			".env": "export A=AAA\nexport B=BBB\nexport C=CCC\n",
			key1:   "Random content",
			key2:   "Random content 2",
		})
	})

	t.Run("uses labels", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		secretName := "mysecret"

		client, _ := NewKubernetesClient(clientset, Config{
			Namespace: "default",
			Labels: map[string]string{
				"app":                        "semaphore-agent",
				"semaphoreci.com/agent-type": "s1-test",
			},
		})

		// create secret using job request
		assert.NoError(t, client.CreateSecret(secretName, &api.JobRequest{
			EnvVars: []api.EnvVar{
				{Name: "A", Value: base64.StdEncoding.EncodeToString([]byte("AAA"))},
				{Name: "B", Value: base64.StdEncoding.EncodeToString([]byte("BBB"))},
				{Name: "C", Value: base64.StdEncoding.EncodeToString([]byte("CCC"))},
			},
		}))

		secret, err := clientset.CoreV1().
			Secrets("default").
			Get(context.Background(), secretName, v1.GetOptions{})

		assert.NoError(t, err)
		assert.True(t, *secret.Immutable)
		assert.Equal(t, secret.Name, secretName)
		assert.Equal(t, secret.Labels, map[string]string{
			"app":                        "semaphore-agent",
			"semaphoreci.com/agent-type": "s1-test",
		})

		assert.Equal(t, secret.StringData, map[string]string{
			".env": "export A=AAA\nexport B=BBB\nexport C=CCC\n",
		})
	})
}

func Test__CreateImagePullSecret(t *testing.T) {
	t.Run("bad image pull credentials -> error", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default"})
		err := client.CreateImagePullSecret("badsecret", []api.ImagePullCredentials{
			{
				EnvVars: []api.EnvVar{
					{Name: "DOCKER_CREDENTIAL_TYPE", Value: base64.StdEncoding.EncodeToString([]byte("NOT_SUPPORTED"))},
				},
			},
		})

		assert.Error(t, err)
		assert.ErrorContains(t, err, "unknown DOCKER_CREDENTIAL_TYPE")
	})

	t.Run("good image pull credentials -> creates secret", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default"})
		secretName := "mysecretname"

		err := client.CreateImagePullSecret(secretName, []api.ImagePullCredentials{
			{
				EnvVars: []api.EnvVar{
					{Name: "DOCKER_CREDENTIAL_TYPE", Value: base64.StdEncoding.EncodeToString([]byte(api.ImagePullCredentialsStrategyGenericDocker))},
					{Name: "DOCKER_USERNAME", Value: base64.StdEncoding.EncodeToString([]byte("myuser"))},
					{Name: "DOCKER_PASSWORD", Value: base64.StdEncoding.EncodeToString([]byte("mypass"))},
					{Name: "DOCKER_URL", Value: base64.StdEncoding.EncodeToString([]byte("my-custom-registry.com"))},
				},
			},
		})

		assert.NoError(t, err)

		secret, err := clientset.CoreV1().
			Secrets("default").
			Get(context.Background(), secretName, v1.GetOptions{})

		assert.NoError(t, err)
		assert.Equal(t, corev1.SecretTypeDockerConfigJson, secret.Type)
		assert.Empty(t, secret.Labels)
		assert.True(t, *secret.Immutable)
		assert.NotEmpty(t, secret.Data)
	})

	t.Run("uses labels", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		secretName := "mysecretname"
		client, _ := NewKubernetesClient(clientset, Config{
			Namespace: "default",
			Labels: map[string]string{
				"app":                        "semaphore-agent",
				"semaphoreci.com/agent-type": "s1-test",
			},
		})

		err := client.CreateImagePullSecret(secretName, []api.ImagePullCredentials{
			{
				EnvVars: []api.EnvVar{
					{Name: "DOCKER_CREDENTIAL_TYPE", Value: base64.StdEncoding.EncodeToString([]byte(api.ImagePullCredentialsStrategyGenericDocker))},
					{Name: "DOCKER_USERNAME", Value: base64.StdEncoding.EncodeToString([]byte("myuser"))},
					{Name: "DOCKER_PASSWORD", Value: base64.StdEncoding.EncodeToString([]byte("mypass"))},
					{Name: "DOCKER_URL", Value: base64.StdEncoding.EncodeToString([]byte("my-custom-registry.com"))},
				},
			},
		})

		assert.NoError(t, err)

		secret, err := clientset.CoreV1().
			Secrets("default").
			Get(context.Background(), secretName, v1.GetOptions{})

		assert.NoError(t, err)
		assert.Equal(t, corev1.SecretTypeDockerConfigJson, secret.Type)
		assert.Equal(t, secret.Labels, map[string]string{
			"app":                        "semaphore-agent",
			"semaphoreci.com/agent-type": "s1-test",
		})

		assert.True(t, *secret.Immutable)
		assert.NotEmpty(t, secret.Data)
	})
}

func Test__CreatePod(t *testing.T) {
	t.Run("no containers from YAML -> error", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		imageValidator, _ := NewImageValidator([]string{})
		client, _ := NewKubernetesClient(clientset, Config{
			Namespace:      "default",
			ImageValidator: imageValidator,
		})

		_ = client.LoadPodSpec()
		podName := "mypod"
		envSecretName := "mysecret"

		assert.ErrorContains(t, client.CreatePod(podName, envSecretName, "", &api.JobRequest{
			Compose: api.Compose{
				Containers: []api.Container{},
			},
		}), "no containers specified in Semaphore YAML")
	})

	t.Run("containers and no pod spec", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		imageValidator, _ := NewImageValidator([]string{})
		client, err := NewKubernetesClient(clientset, Config{
			Namespace:      "default",
			ImageValidator: imageValidator,
		})

		if !assert.NoError(t, err) {
			return
		}

		_ = client.LoadPodSpec()
		podName := "mypod"
		envSecretName := "mysecret"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, envSecretName, "", &api.JobRequest{
			Compose: api.Compose{Containers: []api.Container{{Name: "main", Image: "my-image"}}},
		}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)

		// assert pod spec containers
		if assert.Len(t, pod.Spec.Containers, 1) {
			assert.Equal(t, pod.Spec.Containers[0].Name, "main")
			assert.Equal(t, pod.Spec.Containers[0].Image, "my-image")
			assert.Equal(t, pod.Spec.Containers[0].ImagePullPolicy, corev1.PullPolicy(""))
			assert.Equal(t, pod.Spec.Containers[0].Command, []string{"bash", "-c", "sleep infinity"})
			assert.Empty(t, pod.Spec.Containers[0].Env)
			assert.Equal(t, pod.Spec.Containers[0].VolumeMounts, []corev1.VolumeMount{{Name: "environment", ReadOnly: true, MountPath: "/tmp/injected"}})
		}
	})

	t.Run("1 container", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{
			podSpecWithImagePullPolicy("default", "Always"),
		})

		imageValidator, _ := NewImageValidator([]string{})
		client, err := NewKubernetesClient(clientset, Config{
			Namespace:                 "default",
			PodSpecDecoratorConfigMap: "pod-spec",
			ImageValidator:            imageValidator,
		})

		if !assert.NoError(t, err) {
			return
		}

		_ = client.LoadPodSpec()
		podName := "mypod"
		envSecretName := "mysecret"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, envSecretName, "", &api.JobRequest{
			Compose: api.Compose{
				Containers: []api.Container{
					{
						Name:  "main",
						Image: "custom-image",
					},
				},
			},
		}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)

		// assert pod spec containers
		if assert.Len(t, pod.Spec.Containers, 1) {
			assert.Equal(t, pod.Spec.Containers[0].Name, "main")
			assert.Equal(t, pod.Spec.Containers[0].Image, "custom-image")
			assert.Equal(t, pod.Spec.Containers[0].ImagePullPolicy, corev1.PullAlways)
			assert.Equal(t, pod.Spec.Containers[0].Command, []string{"bash", "-c", "sleep infinity"})
			assert.Empty(t, pod.Spec.Containers[0].Env)
			assert.Equal(t, pod.Spec.Containers[0].VolumeMounts, []corev1.VolumeMount{{Name: "environment", ReadOnly: true, MountPath: "/tmp/injected"}})
		}
	})

	t.Run("container with env vars", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		imageValidator, _ := NewImageValidator([]string{})
		client, _ := NewKubernetesClient(clientset, Config{
			Namespace:      "default",
			ImageValidator: imageValidator,
		})

		_ = client.LoadPodSpec()
		podName := "mypod"
		envSecretName := "mysecret"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, envSecretName, "", &api.JobRequest{
			Compose: api.Compose{
				Containers: []api.Container{
					{
						Name:  "main",
						Image: "custom-image",
						EnvVars: []api.EnvVar{
							{
								Name:  "A",
								Value: base64.StdEncoding.EncodeToString([]byte("AAA")),
							},
						},
					},
				},
			},
		}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)
		if assert.Len(t, pod.Spec.Containers, 1) {
			assert.Equal(t, pod.Spec.Containers[0].Name, "main")
			assert.Equal(t, pod.Spec.Containers[0].Image, "custom-image")
			assert.Equal(t, pod.Spec.Containers[0].ImagePullPolicy, corev1.PullPolicy(""))
			assert.Equal(t, pod.Spec.Containers[0].Command, []string{"bash", "-c", "sleep infinity"})
			assert.Equal(t, pod.Spec.Containers[0].Env, []corev1.EnvVar{{Name: "A", Value: "AAA"}})
			assert.Equal(t, pod.Spec.Containers[0].VolumeMounts, []corev1.VolumeMount{{Name: "environment", ReadOnly: true, MountPath: "/tmp/injected"}})
		}
	})

	t.Run("container with env vars + pod spec with env", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{podSpecWithEnv("default")})
		imageValidator, _ := NewImageValidator([]string{})
		client, _ := NewKubernetesClient(clientset, Config{
			Namespace:                 "default",
			PodSpecDecoratorConfigMap: "pod-spec",
			ImageValidator:            imageValidator,
		})

		_ = client.LoadPodSpec()
		podName := "mypod"
		envSecretName := "mysecret"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, envSecretName, "", &api.JobRequest{
			Compose: api.Compose{
				Containers: []api.Container{
					{
						Name:  "main",
						Image: "custom-image",
						EnvVars: []api.EnvVar{
							{
								Name:  "A",
								Value: base64.StdEncoding.EncodeToString([]byte("AAA")),
							},
						},
					},
				},
			},
		}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)
		if assert.Len(t, pod.Spec.Containers, 1) {
			assert.Equal(t, pod.Spec.Containers[0].Name, "main")
			assert.Equal(t, pod.Spec.Containers[0].Image, "custom-image")
			assert.Equal(t, pod.Spec.Containers[0].ImagePullPolicy, corev1.PullPolicy(""))
			assert.Equal(t, pod.Spec.Containers[0].Command, []string{"bash", "-c", "sleep infinity"})
			assert.Equal(t, pod.Spec.Containers[0].VolumeMounts, []corev1.VolumeMount{{Name: "environment", ReadOnly: true, MountPath: "/tmp/injected"}})
			assert.Equal(t, pod.Spec.Containers[0].Env, []corev1.EnvVar{
				{Name: "FOO_1", Value: "BAR_1"},
				{Name: "FOO_2", Value: "BAR_2"},
				{Name: "A", Value: "AAA"},
			})
		}
	})

	t.Run("multiple containers", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		imageValidator, _ := NewImageValidator([]string{})
		client, _ := NewKubernetesClient(clientset, Config{
			Namespace:      "default",
			ImageValidator: imageValidator,
		})

		_ = client.LoadPodSpec()
		podName := "mypod"
		envSecretName := "mysecret"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, envSecretName, "", &api.JobRequest{
			Compose: api.Compose{
				Containers: []api.Container{
					{
						Name:  "main",
						Image: "custom-image",
					},
					{
						Name:  "db",
						Image: "postgres:9.6",
					},
				},
			},
		}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)

		// assert 2 containers are used and command and volume mounts are only set for the main one
		if assert.Len(t, pod.Spec.Containers, 2) {
			assert.Equal(t, pod.Spec.Containers[0].Name, "main")
			assert.Equal(t, pod.Spec.Containers[0].Image, "custom-image")
			assert.Equal(t, pod.Spec.Containers[0].Command, []string{"bash", "-c", "sleep infinity"})
			assert.Empty(t, pod.Spec.Containers[0].Env)
			assert.Equal(t, pod.Spec.Containers[0].VolumeMounts, []corev1.VolumeMount{{Name: "environment", ReadOnly: true, MountPath: "/tmp/injected"}})
			assert.Equal(t, pod.Spec.Containers[1].Name, "db")
			assert.Equal(t, pod.Spec.Containers[1].Image, "postgres:9.6")
			assert.Empty(t, pod.Spec.Containers[1].Env)
			assert.Empty(t, pod.Spec.Containers[1].Command)
			assert.Empty(t, pod.Spec.Containers[1].VolumeMounts)
		}
	})

	t.Run("no image pull secrets", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		imageValidator, _ := NewImageValidator([]string{})
		client, _ := NewKubernetesClient(clientset, Config{
			Namespace:      "default",
			ImageValidator: imageValidator,
		})

		_ = client.LoadPodSpec()
		podName := "mypod"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, "myenvsecret", "", &api.JobRequest{
			Compose: api.Compose{
				Containers: []api.Container{
					{
						Name:  "main",
						Image: "custom-image",
					},
				},
			},
		}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)
		assert.Len(t, pod.Spec.ImagePullSecrets, 0)
	})

	t.Run("with image pull secrets from pod spec", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{
			podSpecWithImagePullSecret("default", "secret-1"),
		})

		imageValidator, _ := NewImageValidator([]string{})
		client, err := NewKubernetesClient(clientset, Config{
			Namespace:                 "default",
			PodSpecDecoratorConfigMap: "pod-spec",
			ImageValidator:            imageValidator,
		})

		if !assert.NoError(t, err) {
			return
		}

		_ = client.LoadPodSpec()
		podName := "mypod"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, "myenvsecret", "", &api.JobRequest{
			Compose: api.Compose{
				Containers: []api.Container{
					{
						Name:  "main",
						Image: "custom-image",
					},
				},
			},
		}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)
		assert.Equal(t, pod.Spec.ImagePullSecrets, []corev1.LocalObjectReference{{Name: "secret-1"}})
	})

	t.Run("with image pull secret from YAML", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		imageValidator, _ := NewImageValidator([]string{})
		client, _ := NewKubernetesClient(clientset, Config{
			Namespace:      "default",
			ImageValidator: imageValidator,
		})

		_ = client.LoadPodSpec()
		podName := "mypod"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, "myenvsecret", "my-image-pull-secret", &api.JobRequest{
			Compose: api.Compose{
				Containers: []api.Container{
					{
						Name:  "main",
						Image: "custom-image",
					},
				},
			},
		}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)
		assert.Equal(t, pod.Spec.ImagePullSecrets, []corev1.LocalObjectReference{{Name: "my-image-pull-secret"}})
	})

	t.Run("with image pull secret from pod spec and from YAML", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{
			podSpecWithImagePullSecret("default", "secret-1"),
		})

		imageValidator, _ := NewImageValidator([]string{})
		client, _ := NewKubernetesClient(clientset, Config{
			Namespace:                 "default",
			PodSpecDecoratorConfigMap: "pod-spec",
			ImageValidator:            imageValidator,
		})

		_ = client.LoadPodSpec()
		podName := "mypod"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, "myenvsecret", "my-image-pull-secret", &api.JobRequest{
			Compose: api.Compose{
				Containers: []api.Container{
					{
						Name:  "main",
						Image: "custom-image",
					},
				},
			},
		}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)
		assert.Equal(t, pod.Spec.ImagePullSecrets, []corev1.LocalObjectReference{
			{Name: "secret-1"},
			{Name: "my-image-pull-secret"},
		})
	})

	t.Run("with resources", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{podSpecWithResources("default")})
		imageValidator, _ := NewImageValidator([]string{})
		client, _ := NewKubernetesClient(clientset, Config{
			Namespace:                 "default",
			PodSpecDecoratorConfigMap: "pod-spec",
			ImageValidator:            imageValidator,
		})

		_ = client.LoadPodSpec()
		podName := "mypod"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, "myenvsecret", "my-image-pull-secret", &api.JobRequest{
			Compose: api.Compose{
				Containers: []api.Container{
					{
						Name:  "main",
						Image: "custom-image",
					},
					{
						Name:  "db",
						Image: "postgres",
					},
					{
						Name:  "cache",
						Image: "redis",
					},
				},
			},
		}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)
		if assert.Len(t, pod.Spec.Containers, 3) {
			assert.Equal(t, pod.Spec.Containers[0].Resources, corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("200Mi"),
					corev1.ResourceCPU:    resource.MustParse("200m"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("100Mi"),
					corev1.ResourceCPU:    resource.MustParse("100m"),
				},
			})

			assert.Equal(t, pod.Spec.Containers[1].Resources, corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("100Mi"),
					corev1.ResourceCPU:    resource.MustParse("100m"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("50Mi"),
					corev1.ResourceCPU:    resource.MustParse("50m"),
				},
			})

			assert.Equal(t, pod.Spec.Containers[2].Resources, corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("100Mi"),
					corev1.ResourceCPU:    resource.MustParse("100m"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("50Mi"),
					corev1.ResourceCPU:    resource.MustParse("50m"),
				},
			})
		}
	})

	t.Run("no labels", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		imageValidator, _ := NewImageValidator([]string{})
		client, _ := NewKubernetesClient(clientset, Config{
			Namespace:      "default",
			ImageValidator: imageValidator,
		})

		_ = client.LoadPodSpec()
		podName := "mypod"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, "myenvsecret", "", &api.JobRequest{
			Compose: api.Compose{Containers: []api.Container{{Name: "main", Image: "my-image"}}},
		}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)
		assert.Empty(t, pod.Labels)
	})

	t.Run("with labels", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		imageValidator, _ := NewImageValidator([]string{})
		client, _ := NewKubernetesClient(clientset, Config{
			Namespace:      "default",
			ImageValidator: imageValidator,
			Labels: map[string]string{
				"app":                        "semaphore-agent",
				"semaphoreci.com/agent-type": "s1-test",
			},
		})

		_ = client.LoadPodSpec()
		podName := "mypod"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, "myenvsecret", "", &api.JobRequest{
			Compose: api.Compose{Containers: []api.Container{{Name: "main", Image: "my-image"}}},
		}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)
		assert.Equal(t, pod.Labels, map[string]string{
			"app":                        "semaphore-agent",
			"semaphoreci.com/agent-type": "s1-test",
		})
	})
}

func Test__WaitForPod(t *testing.T) {
	t.Run("pod exist and is ready - no error", func(t *testing.T) {
		podName := "mypod"
		clientset := newFakeClientset([]runtime.Object{
			&corev1.Pod{
				ObjectMeta: v1.ObjectMeta{Name: podName, Namespace: "default"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Image: "whatever"}},
				},
			},
		})

		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default"})
		assert.NoError(t, client.WaitForPod(context.TODO(), podName, func(s string) {}))
	})

	t.Run("pod does not exist - error", func(t *testing.T) {
		podName := "mypod"
		clientset := newFakeClientset([]runtime.Object{
			&corev1.Pod{
				ObjectMeta: v1.ObjectMeta{Name: podName, Namespace: "default"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Image: "whatever"}},
				},
			},
		})

		client, _ := NewKubernetesClient(clientset, Config{
			Namespace:          "default",
			PodPollingAttempts: 2,
		})

		assert.Error(t, client.WaitForPod(context.TODO(), "somepodthatdoesnotexist", func(s string) {}))
	})

	t.Run("pod does not exist and context is cancelled - error", func(t *testing.T) {
		podName := "mypod"
		clientset := newFakeClientset([]runtime.Object{
			&corev1.Pod{
				ObjectMeta: v1.ObjectMeta{Name: podName, Namespace: "default"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main", Image: "whatever"}},
				},
			},
		})

		client, _ := NewKubernetesClient(clientset, Config{
			Namespace:          "default",
			PodPollingAttempts: 120,
		})

		ctx, cancel := context.WithCancel(context.TODO())

		// Wait a little bit before cancelling
		go func() {
			time.Sleep(time.Second)
			cancel()
		}()

		assert.ErrorContains(t, client.WaitForPod(ctx, "somepodthatdoesnotexist", func(s string) {}), "context canceled")
	})
}

func Test_DeletePod(t *testing.T) {
	podName := "mypod"
	clientset := newFakeClientset([]runtime.Object{
		&corev1.Pod{
			ObjectMeta: v1.ObjectMeta{Name: podName, Namespace: "default"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "main", Image: "whatever"}},
			},
		},
	})

	client, _ := NewKubernetesClient(clientset, Config{Namespace: "default"})
	assert.NoError(t, client.DeletePod(podName))

	// pod does not exist anymore
	pod, err := clientset.CoreV1().
		Pods("default").
		Get(context.Background(), podName, v1.GetOptions{})
	assert.Error(t, err)
	assert.Nil(t, pod)
}

func Test_DeleteSecret(t *testing.T) {
	secretName := "mysecret"
	clientset := newFakeClientset([]runtime.Object{
		&corev1.Secret{
			ObjectMeta: v1.ObjectMeta{Name: secretName, Namespace: "default"},
			Type:       corev1.SecretTypeOpaque,
			StringData: map[string]string{},
		},
	})

	client, _ := NewKubernetesClient(clientset, Config{Namespace: "default"})
	assert.NoError(t, client.DeleteSecret(secretName))

	// secret does not exist anymore
	pod, err := clientset.CoreV1().
		Secrets("default").
		Get(context.Background(), secretName, v1.GetOptions{})
	assert.Error(t, err)
	assert.Nil(t, pod)
}

func newFakeClientset(objects []runtime.Object) kubernetes.Interface {
	return fake.NewSimpleClientset(objects...)
}

func podSpecWithEnv(namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{Name: "pod-spec", Namespace: namespace},
		Data: map[string]string{
			"mainContainer": `
        env:
          - name: FOO_1
            value: BAR_1
          - name: FOO_2
            value: BAR_2
      `,
		},
	}
}

func podSpecWithImagePullPolicy(namespace, pullPolicy string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{Name: "pod-spec", Namespace: namespace},
		Data: map[string]string{
			"mainContainer": fmt.Sprintf(`
        imagePullPolicy: %s
      `, pullPolicy),
		},
	}
}

func podSpecWithImagePullSecret(namespace, secretName string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{Name: "pod-spec", Namespace: namespace},
		Data: map[string]string{
			"mainContainer": `
        imagePullPolicy: Never
      `,
			"pod": fmt.Sprintf(`
        imagePullSecrets:
          - name: %s
      `, secretName),
		},
	}
}

func podSpecWithResources(namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{Name: "pod-spec", Namespace: namespace},
		Data: map[string]string{
			"mainContainer": `
        resources:
          limits:
            cpu: 200m
            memory: 200Mi
          requests:
            cpu: 100m
            memory: 100Mi
      `,
			"sidecarContainers": `
        resources:
          limits:
            cpu: 100m
            memory: 100Mi
          requests:
            cpu: 50m
            memory: 50Mi
       `,
		},
	}
}
