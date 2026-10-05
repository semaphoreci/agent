package kubernetes

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	log "github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	assert "github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const testJobID = "11111111-2222-3333-4444-555555555555"

func newTestClient(t *testing.T, clientset *fake.Clientset, config Config) *KubernetesClient {
	validator, err := NewImageValidator([]string{})
	assert.NoError(t, err)
	config.ImageValidator = validator

	client, err := NewKubernetesClient(clientset, config)
	assert.NoError(t, err)
	return client
}

func agentPod(name string, uid types.UID) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{Name: name, Namespace: "default", UID: uid},
	}
}

func dockerCredentials() []api.ImagePullCredentials {
	return []api.ImagePullCredentials{
		{
			EnvVars: []api.EnvVar{
				{Name: "DOCKER_CREDENTIAL_TYPE", Value: base64.StdEncoding.EncodeToString([]byte(api.ImagePullCredentialsStrategyGenericDocker))},
				{Name: "DOCKER_USERNAME", Value: base64.StdEncoding.EncodeToString([]byte("user"))},
				{Name: "DOCKER_PASSWORD", Value: base64.StdEncoding.EncodeToString([]byte("pass"))},
				{Name: "DOCKER_URL", Value: base64.StdEncoding.EncodeToString([]byte("registry.example.com"))},
			},
		},
	}
}

// Creates the pod and both secrets for a job, the way the executor does.
func createJobResources(t *testing.T, client *KubernetesClient, jobID string) {
	request := &api.JobRequest{
		JobID:   jobID,
		Compose: api.Compose{Containers: []api.Container{{Name: "main", Image: "some-image"}}},
	}

	assert.NoError(t, client.CreateSecret("job-secret", request))
	assert.NoError(t, client.CreateImagePullSecret("job-pull-secret", jobID, dockerCredentials()))
	assert.NoError(t, client.CreatePod("job-pod", "job-secret", "job-pull-secret", request))
}

func jobObjectMetas(t *testing.T, clientset *fake.Clientset) []v1.ObjectMeta {
	pod, err := clientset.CoreV1().Pods("default").Get(context.Background(), "job-pod", v1.GetOptions{})
	assert.NoError(t, err)
	secret, err := clientset.CoreV1().Secrets("default").Get(context.Background(), "job-secret", v1.GetOptions{})
	assert.NoError(t, err)
	pullSecret, err := clientset.CoreV1().Secrets("default").Get(context.Background(), "job-pull-secret", v1.GetOptions{})
	assert.NoError(t, err)

	return []v1.ObjectMeta{pod.ObjectMeta, secret.ObjectMeta, pullSecret.ObjectMeta}
}

func Test__OwnerReferences(t *testing.T) {
	t.Run("agent pod found -> pod and secrets are owned by it", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(agentPod("agent-pod", "agent-uid"))
		client := newTestClient(t, clientset, Config{Namespace: "default", OwnerPodName: "agent-pod"})
		client.LoadOwnerReference()
		createJobResources(t, client, testJobID)

		for _, meta := range jobObjectMetas(t, clientset) {
			if assert.Len(t, meta.OwnerReferences, 1, meta.Name) {
				ref := meta.OwnerReferences[0]
				assert.Equal(t, "v1", ref.APIVersion)
				assert.Equal(t, "Pod", ref.Kind)
				assert.Equal(t, "agent-pod", ref.Name)
				assert.Equal(t, types.UID("agent-uid"), ref.UID)
				assert.Nil(t, ref.BlockOwnerDeletion)
				assert.Nil(t, ref.Controller)
			}
		}
	})

	t.Run("no agent pod name -> resources are created without owner", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(agentPod("agent-pod", "agent-uid"))
		client := newTestClient(t, clientset, Config{Namespace: "default"})
		client.LoadOwnerReference()
		createJobResources(t, client, testJobID)

		for _, meta := range jobObjectMetas(t, clientset) {
			assert.Empty(t, meta.OwnerReferences, meta.Name)
		}
	})

	t.Run("agent pod not found -> resources are created without owner", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		client := newTestClient(t, clientset, Config{Namespace: "default", OwnerPodName: "agent-pod"})
		client.LoadOwnerReference()
		createJobResources(t, client, testJobID)

		for _, meta := range jobObjectMetas(t, clientset) {
			assert.Empty(t, meta.OwnerReferences, meta.Name)
		}
	})

	t.Run("agent pod in another namespace -> not used as owner", func(t *testing.T) {
		other := agentPod("agent-pod", "agent-uid")
		other.Namespace = "other"
		clientset := fake.NewSimpleClientset(other)
		client := newTestClient(t, clientset, Config{Namespace: "default", OwnerPodName: "agent-pod"})
		client.LoadOwnerReference()
		createJobResources(t, client, testJobID)

		for _, meta := range jobObjectMetas(t, clientset) {
			assert.Empty(t, meta.OwnerReferences, meta.Name)
		}
	})

	t.Run("not allowed to get the agent pod -> resources are created without owner", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(agentPod("agent-pod", "agent-uid"))
		clientset.PrependReactor("get", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if action.(k8stesting.GetAction).GetName() != "agent-pod" {
				return false, nil, nil
			}

			return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "agent-pod", errors.New("no"))
		})

		client := newTestClient(t, clientset, Config{Namespace: "default", OwnerPodName: "agent-pod"})
		client.LoadOwnerReference()
		createJobResources(t, client, testJobID)

		for _, meta := range jobObjectMetas(t, clientset) {
			assert.Empty(t, meta.OwnerReferences, meta.Name)
		}
	})
}

func Test__JobIDLabel(t *testing.T) {
	t.Run("pod and secrets carry the job ID and the configured labels", func(t *testing.T) {
		labels := map[string]string{"app": "semaphore"}
		clientset := fake.NewSimpleClientset()
		client := newTestClient(t, clientset, Config{Namespace: "default", Labels: labels})
		createJobResources(t, client, testJobID)

		for _, meta := range jobObjectMetas(t, clientset) {
			assert.Equal(t, map[string]string{"app": "semaphore", JobIDLabel: testJobID}, meta.Labels, meta.Name)
		}

		// the configured labels are shared between jobs, so they are never modified
		assert.Equal(t, map[string]string{"app": "semaphore"}, labels)
	})

	t.Run("no job ID -> no job ID label", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		client := newTestClient(t, clientset, Config{Namespace: "default"})
		createJobResources(t, client, "")

		for _, meta := range jobObjectMetas(t, clientset) {
			assert.Empty(t, meta.Labels, meta.Name)
		}
	})
}

func Test__PodActiveDeadline(t *testing.T) {
	request := &api.JobRequest{
		JobID:   testJobID,
		Compose: api.Compose{Containers: []api.Container{{Name: "main", Image: "some-image"}}},
	}

	getPod := func(clientset *fake.Clientset) *corev1.Pod {
		pod, err := clientset.CoreV1().Pods("default").Get(context.Background(), "job-pod", v1.GetOptions{})
		assert.NoError(t, err)
		return pod
	}

	t.Run("not configured -> no deadline", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		client := newTestClient(t, clientset, Config{Namespace: "default"})
		assert.NoError(t, client.CreatePod("job-pod", "job-secret", "", request))
		assert.Nil(t, getPod(clientset).Spec.ActiveDeadlineSeconds)
	})

	t.Run("configured -> used as the pod deadline", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		client := newTestClient(t, clientset, Config{Namespace: "default", PodActiveDeadlineSeconds: 90000})
		assert.NoError(t, client.CreatePod("job-pod", "job-secret", "", request))
		deadline := getPod(clientset).Spec.ActiveDeadlineSeconds
		if assert.NotNil(t, deadline) {
			assert.Equal(t, int64(90000), *deadline)
		}
	})

	t.Run("pod spec decorator sets one -> decorator wins", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(&corev1.ConfigMap{
			ObjectMeta: v1.ObjectMeta{Name: "pod-spec", Namespace: "default"},
			Data:       map[string]string{"pod": "activeDeadlineSeconds: 3600\n"},
		})

		client := newTestClient(t, clientset, Config{
			Namespace:                 "default",
			PodSpecDecoratorConfigMap: "pod-spec",
			PodActiveDeadlineSeconds:  90000,
		})

		assert.NoError(t, client.LoadPodSpec())
		assert.NoError(t, client.CreatePod("job-pod", "job-secret", "", request))
		deadline := getPod(clientset).Spec.ActiveDeadlineSeconds
		if assert.NotNil(t, deadline) {
			assert.Equal(t, int64(3600), *deadline)
		}
	})
}

func Test__DeleteRetries(t *testing.T) {
	failing := func(clientset *fake.Clientset, verb, resource string, failures int) *int {
		calls := 0
		clientset.PrependReactor(verb, resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
			calls++
			if calls <= failures {
				return true, nil, apierrors.NewServiceUnavailable("try again")
			}

			return false, nil, nil
		})

		return &calls
	}

	config := Config{Namespace: "default", DeleteAttempts: 3, DeleteInterval: time.Millisecond}

	t.Run("pod already gone -> success, not retried", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		calls := failing(clientset, "delete", "pods", 0)
		client := newTestClient(t, clientset, config)
		assert.NoError(t, client.DeletePod(context.Background(), "does-not-exist"))
		assert.Equal(t, 1, *calls)
	})

	t.Run("secret already gone -> success, not retried", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		calls := failing(clientset, "delete", "secrets", 0)
		client := newTestClient(t, clientset, config)
		assert.NoError(t, client.DeleteSecret(context.Background(), "does-not-exist"))
		assert.Equal(t, 1, *calls)
	})

	t.Run("transient error -> retried until deleted", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(agentPod("job-pod", "uid"))
		calls := failing(clientset, "delete", "pods", 2)
		client := newTestClient(t, clientset, config)
		assert.NoError(t, client.DeletePod(context.Background(), "job-pod"))
		assert.Equal(t, 3, *calls)

		_, err := clientset.CoreV1().Pods("default").Get(context.Background(), "job-pod", v1.GetOptions{})
		assert.True(t, apierrors.IsNotFound(err))
	})

	t.Run("persistent error -> gives up after the configured attempts", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(&corev1.Secret{ObjectMeta: v1.ObjectMeta{Name: "job-secret", Namespace: "default"}})
		calls := failing(clientset, "delete", "secrets", 100)
		client := newTestClient(t, clientset, config)
		assert.Error(t, client.DeleteSecret(context.Background(), "job-secret"))
		assert.Equal(t, 3, *calls)
	})
}

func Test__OwnerReferenceLookupRetries(t *testing.T) {
	previous := ownerLookupInterval
	ownerLookupInterval = time.Millisecond
	defer func() { ownerLookupInterval = previous }()

	// Fails the first `failures` gets of the agent pod with err.
	failGets := func(clientset *fake.Clientset, failures int, err error) *int {
		calls := 0
		clientset.PrependReactor("get", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if action.(k8stesting.GetAction).GetName() != "agent-pod" {
				return false, nil, nil
			}

			calls++
			if calls <= failures {
				return true, nil, err
			}

			return false, nil, nil
		})

		return &calls
	}

	unavailable := apierrors.NewServiceUnavailable("try again")
	ownerNames := func(t *testing.T, clientset *fake.Clientset) []string {
		names := []string{}
		for _, meta := range jobObjectMetas(t, clientset) {
			for _, ref := range meta.OwnerReferences {
				names = append(names, ref.Name)
			}
		}

		return names
	}

	t.Run("transient errors, then found -> owned by agent pod", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(agentPod("agent-pod", "agent-uid"))
		calls := failGets(clientset, 2, unavailable)
		client := newTestClient(t, clientset, Config{Namespace: "default", OwnerPodName: "agent-pod"})
		client.LoadOwnerReference()
		createJobResources(t, client, testJobID)

		assert.Equal(t, 3, *calls)
		assert.Equal(t, []string{"agent-pod", "agent-pod", "agent-pod"}, ownerNames(t, clientset))
	})

	t.Run("transient errors on every attempt -> gives up, no owner", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(agentPod("agent-pod", "agent-uid"))
		calls := failGets(clientset, 100, unavailable)
		client := newTestClient(t, clientset, Config{Namespace: "default", OwnerPodName: "agent-pod"})
		client.LoadOwnerReference()
		createJobResources(t, client, testJobID)

		assert.Equal(t, ownerLookupAttempts, *calls)
		assert.Empty(t, ownerNames(t, clientset))
	})

	t.Run("not found -> no retries", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		calls := failGets(clientset, 0, nil)
		client := newTestClient(t, clientset, Config{Namespace: "default", OwnerPodName: "agent-pod"})
		client.LoadOwnerReference()
		assert.Equal(t, 1, *calls)
	})

	t.Run("forbidden -> no retries", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(agentPod("agent-pod", "agent-uid"))
		calls := failGets(clientset, 100, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "agent-pod", errors.New("no")))
		client := newTestClient(t, clientset, Config{Namespace: "default", OwnerPodName: "agent-pod"})
		client.LoadOwnerReference()
		assert.Equal(t, 1, *calls)
	})

	t.Run("job stopped -> no retries", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(agentPod("agent-pod", "agent-uid"))
		calls := failGets(clientset, 100, unavailable)
		client := newTestClient(t, clientset, Config{Namespace: "default", OwnerPodName: "agent-pod"})
		client.CancelRequests()
		client.LoadOwnerReference()
		assert.Equal(t, 1, *calls)
	})
}

func Test__CreateOutcomeUnknown(t *testing.T) {
	request := &api.JobRequest{
		JobID:   testJobID,
		Compose: api.Compose{Containers: []api.Container{{Name: "main", Image: "some-image"}}},
	}

	failCreates := func(resource string, err error) *fake.Clientset {
		clientset := fake.NewSimpleClientset()
		clientset.PrependReactor("create", resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, err
		})

		return clientset
	}

	unknown := map[string]error{
		"canceled request":    context.Canceled,
		"timed out request":   context.DeadlineExceeded,
		"connection error":    errors.New("connection reset by peer"),
		"server timeout":      apierrors.NewServerTimeout(schema.GroupResource{Resource: "pods"}, "create", 1),
		"timeout":             apierrors.NewTimeoutError("timeout", 1),
		"internal error":      apierrors.NewInternalError(errors.New("etcd")),
		"service unavailable": apierrors.NewServiceUnavailable("unavailable"),
	}

	for name, err := range unknown {
		t.Run(name+" -> pod may exist", func(t *testing.T) {
			client := newTestClient(t, failCreates("pods", err), Config{Namespace: "default"})
			assert.Error(t, client.CreatePod("job-pod", "job-secret", "", request))
			assert.True(t, client.CreateOutcomeUnknown())
		})

		t.Run(name+" -> secret may exist", func(t *testing.T) {
			client := newTestClient(t, failCreates("secrets", err), Config{Namespace: "default"})
			assert.Error(t, client.CreateSecret("job-secret", request))
			assert.True(t, client.CreateOutcomeUnknown())
		})

		t.Run(name+" -> image pull secret may exist", func(t *testing.T) {
			client := newTestClient(t, failCreates("secrets", err), Config{Namespace: "default"})
			assert.Error(t, client.CreateImagePullSecret("job-pull-secret", testJobID, dockerCredentials()))
			assert.True(t, client.CreateOutcomeUnknown())
		})
	}

	rejected := map[string]error{
		"forbidden":      apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "job-pod", errors.New("rbac")),
		"invalid":        apierrors.NewBadRequest("invalid pod spec"),
		"already exists": apierrors.NewAlreadyExists(schema.GroupResource{Resource: "pods"}, "job-pod"),
	}

	for name, err := range rejected {
		t.Run(name+" -> nothing was created", func(t *testing.T) {
			client := newTestClient(t, failCreates("pods", err), Config{Namespace: "default"})
			assert.Error(t, client.CreatePod("job-pod", "job-secret", "", request))
			assert.False(t, client.CreateOutcomeUnknown())
		})
	}

	t.Run("creates succeed -> outcome known", func(t *testing.T) {
		client := newTestClient(t, fake.NewSimpleClientset(), Config{Namespace: "default"})
		createJobResources(t, client, testJobID)
		assert.False(t, client.CreateOutcomeUnknown())
	})
}

func Test__OwnerReferenceLookupLogLevel(t *testing.T) {
	levels := func(fn func()) []log.Level {
		previous := log.StandardLogger().ReplaceHooks(make(log.LevelHooks))
		defer log.StandardLogger().ReplaceHooks(previous)
		hook := logtest.NewLocal(log.StandardLogger())

		fn()
		result := []log.Level{}
		for _, entry := range hook.AllEntries() {
			result = append(result, entry.Level)
		}

		return result
	}

	t.Run("no agent pod name -> info, no warning or error", func(t *testing.T) {
		client := newTestClient(t, fake.NewSimpleClientset(), Config{Namespace: "default"})
		got := levels(client.LoadOwnerReference)
		assert.Equal(t, []log.Level{log.InfoLevel}, got)
	})

	t.Run("agent pod lookup fails -> error", func(t *testing.T) {
		client := newTestClient(t, fake.NewSimpleClientset(), Config{Namespace: "default", OwnerPodName: "agent-pod"})
		got := levels(client.LoadOwnerReference)
		assert.Equal(t, []log.Level{log.ErrorLevel}, got)
	})

	t.Run("agent pod found -> info", func(t *testing.T) {
		client := newTestClient(t, fake.NewSimpleClientset(agentPod("agent-pod", "agent-uid")), Config{Namespace: "default", OwnerPodName: "agent-pod"})
		got := levels(client.LoadOwnerReference)
		assert.Equal(t, []log.Level{log.InfoLevel}, got)
	})
}
