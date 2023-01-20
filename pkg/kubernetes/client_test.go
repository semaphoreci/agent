package kubernetes

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/semaphoreci/agent/pkg/api"
	assert "github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func Test__CreateSecret(t *testing.T) {
	t.Run("stores .env file in secret", func(t *testing.T) {
		clientset := newFakeClientset([]runtime.Object{})
		client, _ := NewKubernetesClient(clientset, "default")
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
		client, _ := NewKubernetesClient(clientset, "default")
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

func newFakeClientset(objects []runtime.Object) kubernetes.Interface {
	return fake.NewSimpleClientset(objects...)
}
