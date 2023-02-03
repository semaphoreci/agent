package kubernetes

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/semaphoreci/agent/pkg/api"
	assert "github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func Test__CreateSecret(t *testing.T) {
	t.Run("stores .env file in secret", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default", DefaultImage: "default-image"})
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
		assert.Equal(t, secret.StringData, map[string]string{
			".env": "export A=AAA\nexport B=BBB\nexport C=CCC\n",
		})
	})

	t.Run("stores files in secret, with base64-encoded keys", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default", DefaultImage: "default-image"})
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
		assert.Equal(t, secret.StringData, map[string]string{
			".env": "export A=AAA\nexport B=BBB\nexport C=CCC\n",
			key1:   "Random content",
			key2:   "Random content 2",
		})
	})
}

func Test__CreateImagePullSecret(t *testing.T) {
	t.Run("bad image pull credentials -> error", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default", ImagePullPolicy: "Never"})
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
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default", ImagePullPolicy: "Never"})
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
		assert.True(t, *secret.Immutable)
		assert.NotEmpty(t, secret.Data)
	})
}

func Test__CreatePod(t *testing.T) {
	t.Run("no containers and no default image specified -> error", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default", ImagePullPolicy: "Never"})
		podName := "mypod"
		envSecretName := "mysecret"

		assert.ErrorContains(t, client.CreatePod(podName, envSecretName, "", &api.JobRequest{
			Compose: api.Compose{
				Containers: []api.Container{},
			},
		}), "no default image specified")
	})

	t.Run("no containers specified in job uses default image", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default", DefaultImage: "default-image", ImagePullPolicy: "Never"})
		podName := "mypod"
		envSecretName := "mysecret"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, envSecretName, "", &api.JobRequest{
			Compose: api.Compose{
				Containers: []api.Container{},
			},
		}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)

		// assert pod metadata
		assert.Equal(t, pod.ObjectMeta.Name, podName)
		assert.Equal(t, pod.ObjectMeta.Namespace, "default")
		assert.Equal(t, pod.ObjectMeta.Labels, map[string]string{"app": "semaphore-agent"})

		// assert pod spec
		assert.Equal(t, pod.Spec.RestartPolicy, corev1.RestartPolicyNever)
		assert.Empty(t, pod.Spec.ImagePullSecrets)

		// assert pod spec containers
		if assert.Len(t, pod.Spec.Containers, 1) {
			assert.Equal(t, pod.Spec.Containers[0].Name, "main")
			assert.Equal(t, pod.Spec.Containers[0].Image, "default-image")
			assert.Equal(t, pod.Spec.Containers[0].ImagePullPolicy, corev1.PullNever)
			assert.Equal(t, pod.Spec.Containers[0].Command, []string{"bash", "-c", "sleep infinity"})
			assert.Empty(t, pod.Spec.Containers[0].Env)
			assert.Equal(t, pod.Spec.Containers[0].VolumeMounts, []corev1.VolumeMount{{Name: "environment", ReadOnly: true, MountPath: "/tmp/injected"}})
		}

		// assert pod spec volumes
		if assert.Len(t, pod.Spec.Volumes, 1) {
			assert.Equal(t, pod.Spec.Volumes[0].Name, "environment")
			assert.Equal(t, pod.Spec.Volumes[0].VolumeSource, corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: envSecretName,
				},
			})
		}
	})

	t.Run("1 container", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default", DefaultImage: "default-image", ImagePullPolicy: "Always"})
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
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default", DefaultImage: "default-image", ImagePullPolicy: "Always"})
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
			assert.Equal(t, pod.Spec.Containers[0].ImagePullPolicy, corev1.PullAlways)
			assert.Equal(t, pod.Spec.Containers[0].Command, []string{"bash", "-c", "sleep infinity"})
			assert.Equal(t, pod.Spec.Containers[0].Env, []corev1.EnvVar{{Name: "A", Value: "AAA"}})
			assert.Equal(t, pod.Spec.Containers[0].VolumeMounts, []corev1.VolumeMount{{Name: "environment", ReadOnly: true, MountPath: "/tmp/injected"}})
		}
	})

	t.Run("multiple containers", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default", DefaultImage: "default-image", ImagePullPolicy: "Always"})
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
		client, _ := NewKubernetesClient(clientset, Config{
			Namespace:       "default",
			DefaultImage:    "default-image",
			ImagePullPolicy: "Always",
		})

		podName := "mypod"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, "myenvsecret", "", &api.JobRequest{}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)
		assert.Len(t, pod.Spec.ImagePullSecrets, 0)
	})

	t.Run("with image pull secrets from config", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{
			Namespace:        "default",
			DefaultImage:     "default-image",
			ImagePullPolicy:  "Always",
			ImagePullSecrets: []string{"secret-1"},
		})

		podName := "mypod"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, "myenvsecret", "", &api.JobRequest{}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)
		assert.Equal(t, pod.Spec.ImagePullSecrets, []corev1.LocalObjectReference{{Name: "secret-1"}})
	})

	t.Run("with image pull secret - ephemeral", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{
			Namespace:       "default",
			DefaultImage:    "default-image",
			ImagePullPolicy: "Always",
		})

		podName := "mypod"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, "myenvsecret", "my-image-pull-secret", &api.JobRequest{}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)
		assert.Equal(t, pod.Spec.ImagePullSecrets, []corev1.LocalObjectReference{{Name: "my-image-pull-secret"}})
	})

	t.Run("with image pull secret from config + ephemeral", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{
			Namespace:        "default",
			DefaultImage:     "default-image",
			ImagePullPolicy:  "Always",
			ImagePullSecrets: []string{"secret-1"},
		})

		podName := "mypod"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, "myenvsecret", "my-image-pull-secret", &api.JobRequest{}))

		pod, err := clientset.CoreV1().
			Pods("default").
			Get(context.Background(), podName, v1.GetOptions{})

		assert.NoError(t, err)
		assert.Equal(t, pod.Spec.ImagePullSecrets, []corev1.LocalObjectReference{
			{Name: "secret-1"},
			{Name: "my-image-pull-secret"},
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

		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default", DefaultImage: "default-image"})
		assert.NoError(t, client.WaitForPod(podName, func(s string) {}))
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
			DefaultImage:       "default-image",
			PodPollingAttempts: 2,
		})

		assert.Error(t, client.WaitForPod("somepodthatdoesnotexist", func(s string) {}))
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

	client, _ := NewKubernetesClient(clientset, Config{Namespace: "default", DefaultImage: "default-image"})
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

	client, _ := NewKubernetesClient(clientset, Config{Namespace: "default", DefaultImage: "default-image"})
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
