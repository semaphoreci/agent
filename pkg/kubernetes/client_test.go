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

func Test__CreatePod(t *testing.T) {
	t.Run("no containers specified in job uses default image", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default", DefaultImage: "default-image"})
		podName := "mypod"
		envSecretName := "mysecret"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, envSecretName, &api.JobRequest{
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
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default", DefaultImage: "default-image"})
		podName := "mypod"
		envSecretName := "mysecret"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, envSecretName, &api.JobRequest{
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
			assert.Equal(t, pod.Spec.Containers[0].ImagePullPolicy, corev1.PullNever)
			assert.Equal(t, pod.Spec.Containers[0].Command, []string{"bash", "-c", "sleep infinity"})
			assert.Empty(t, pod.Spec.Containers[0].Env)
			assert.Equal(t, pod.Spec.Containers[0].VolumeMounts, []corev1.VolumeMount{{Name: "environment", ReadOnly: true, MountPath: "/tmp/injected"}})
		}
	})

	t.Run("container with env vars", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default", DefaultImage: "default-image"})
		podName := "mypod"
		envSecretName := "mysecret"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, envSecretName, &api.JobRequest{
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
			assert.Equal(t, pod.Spec.Containers[0].ImagePullPolicy, corev1.PullNever)
			assert.Equal(t, pod.Spec.Containers[0].Command, []string{"bash", "-c", "sleep infinity"})
			assert.Equal(t, pod.Spec.Containers[0].Env, []corev1.EnvVar{{Name: "A", Value: "AAA"}})
			assert.Equal(t, pod.Spec.Containers[0].VolumeMounts, []corev1.VolumeMount{{Name: "environment", ReadOnly: true, MountPath: "/tmp/injected"}})
		}
	})

	t.Run("multiple containers", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, Config{Namespace: "default", DefaultImage: "default-image"})
		podName := "mypod"
		envSecretName := "mysecret"

		// create pod using job request
		assert.NoError(t, client.CreatePod(podName, envSecretName, &api.JobRequest{
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
}

func newFakeClientset(objects []runtime.Object) kubernetes.Interface {
	return fake.NewSimpleClientset(objects...)
}
