package executors

import (
	"context"
	"os"
	goruntime "runtime"
	"sync"
	"testing"
	"time"

	api "github.com/semaphoreci/agent/pkg/api"
	eventlogger "github.com/semaphoreci/agent/pkg/eventlogger"
	"github.com/semaphoreci/agent/pkg/kubernetes"
	shell "github.com/semaphoreci/agent/pkg/shell"
	assert "github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	k8stesting "k8s.io/client-go/testing"
)

const k8sTestJobID = "11111111-2222-3333-4444-555555555555"

// Wraps the fake clientset so pod creation can be held before it reaches
// the fake. Blocking inside a fake reactor would not work: the fake holds
// a lock while reactors run, which would serialize every other call too.
type gatedClientset struct {
	*fake.Clientset
	createStarted chan struct{}
	release       chan struct{}
}

func (c *gatedClientset) CoreV1() typedcorev1.CoreV1Interface {
	return &gatedCoreV1{CoreV1Interface: c.Clientset.CoreV1(), c: c}
}

type gatedCoreV1 struct {
	typedcorev1.CoreV1Interface
	c *gatedClientset
}

func (g *gatedCoreV1) Pods(namespace string) typedcorev1.PodInterface {
	return &gatedPods{PodInterface: g.CoreV1Interface.Pods(namespace), c: g.c}
}

type gatedPods struct {
	typedcorev1.PodInterface
	c *gatedClientset
}

func (p *gatedPods) Create(ctx context.Context, pod *corev1.Pod, opts v1.CreateOptions) (*corev1.Pod, error) {
	close(p.c.createStarted)
	<-p.c.release
	return p.PodInterface.Create(ctx, pod, opts)
}

func k8sJobRequest() *api.JobRequest {
	return &api.JobRequest{
		JobID:   k8sTestJobID,
		Compose: api.Compose{Containers: []api.Container{{Name: "main", Image: "some-image"}}},
	}
}

func newK8sExecutor(t *testing.T, clientset k8sclient.Interface) *KubernetesExecutor {
	validator, err := kubernetes.NewImageValidator([]string{})
	assert.NoError(t, err)

	client, err := kubernetes.NewKubernetesClient(clientset, kubernetes.Config{
		Namespace:          "default",
		ImageValidator:     validator,
		OwnerPodName:       "agent-pod",
		PodPollingAttempts: 2,
		PodPollingInterval: 10 * time.Millisecond,
		DeleteAttempts:     2,
		DeleteInterval:     time.Millisecond,
	})
	assert.NoError(t, err)

	logger, _ := eventlogger.DefaultTestLogger()
	return newKubernetesExecutorWithClient(k8sJobRequest(), logger, client)
}

func assertNoJobResources(t *testing.T, clientset *fake.Clientset) {
	pods, err := clientset.CoreV1().Pods("default").List(context.Background(), v1.ListOptions{})
	assert.NoError(t, err)
	for _, pod := range pods.Items {
		assert.NotEqual(t, "semaphore-job-"+k8sTestJobID, pod.Name, "job pod was left behind")
	}

	secrets, err := clientset.CoreV1().Secrets("default").List(context.Background(), v1.ListOptions{})
	assert.NoError(t, err)
	assert.Empty(t, secrets.Items, "job secrets were left behind")
}

func agentPodObject() *corev1.Pod {
	return &corev1.Pod{ObjectMeta: v1.ObjectMeta{Name: "agent-pod", Namespace: "default", UID: "agent-uid"}}
}

func countActions(clientset *fake.Clientset, verb string) int {
	n := 0
	for _, action := range clientset.Actions() {
		if action.GetVerb() == verb {
			n++
		}
	}

	return n
}

func Test__KubernetesExecutor__StopDuringPrepare(t *testing.T) {
	fakeClientset := fake.NewSimpleClientset(agentPodObject())
	clientset := &gatedClientset{
		Clientset:     fakeClientset,
		createStarted: make(chan struct{}),
		release:       make(chan struct{}),
	}

	e := newK8sExecutor(t, clientset)

	prepareDone := make(chan int)
	go func() { prepareDone <- e.Prepare() }()

	// The env secret exists, and the pod is being created.
	<-clientset.createStarted

	stopDone := make(chan struct{})
	go func() {
		e.Stop()
		close(stopDone)
	}()

	// Stop must wait for the pod creation to finish,
	// otherwise it has nothing to delete yet.
	select {
	case <-stopDone:
		t.Fatal("Stop() returned while the pod was still being created")
	case <-time.After(200 * time.Millisecond):
	}

	close(clientset.release)
	<-prepareDone
	<-stopDone

	assertNoJobResources(t, fakeClientset)

	// And the executor does not start a shell for a stopped job.
	assert.Equal(t, 1, e.Start())
}

func Test__KubernetesExecutor__StopBeforePrepare(t *testing.T) {
	clientset := fake.NewSimpleClientset(agentPodObject())
	e := newK8sExecutor(t, clientset)

	e.Stop()
	assert.Equal(t, 1, e.Prepare())
	assert.Equal(t, 0, countActions(clientset, "create"), "nothing is created for a stopped job")
	assertNoJobResources(t, clientset)
}

func Test__KubernetesExecutor__PrepareWithoutStop(t *testing.T) {
	clientset := fake.NewSimpleClientset(agentPodObject())
	e := newK8sExecutor(t, clientset)

	assert.Equal(t, 0, e.Prepare())

	pod, err := clientset.CoreV1().Pods("default").Get(context.Background(), "semaphore-job-"+k8sTestJobID, v1.GetOptions{})
	if assert.NoError(t, err) {
		assert.Equal(t, k8sTestJobID, pod.Labels[kubernetes.JobIDLabel])
		if assert.Len(t, pod.OwnerReferences, 1) {
			assert.Equal(t, "agent-pod", pod.OwnerReferences[0].Name)
		}
	}

	_, err = clientset.CoreV1().Secrets("default").Get(context.Background(), "semaphore-job-"+k8sTestJobID+"-secret", v1.GetOptions{})
	assert.NoError(t, err)

	// Cleanup removes everything, and only once.
	assert.Equal(t, 0, e.Cleanup())
	assertNoJobResources(t, clientset)
	deletes := countActions(clientset, "delete")
	assert.Equal(t, 2, deletes)

	assert.Equal(t, 0, e.Stop())
	assert.Equal(t, deletes, countActions(clientset, "delete"))
}

func Test__KubernetesExecutor__CleanupAfterFailedPodCreation(t *testing.T) {
	clientset := fake.NewSimpleClientset(agentPodObject())
	clientset.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewServiceUnavailable("nope")
	})

	e := newK8sExecutor(t, clientset)
	assert.Equal(t, 1, e.Prepare())

	// The secret was created before the pod creation failed. It is removed,
	// and the pod that was never created does not count as a failure.
	assert.Equal(t, 0, e.Stop())
	assertNoJobResources(t, clientset)
}

// Wraps the fake clientset so some requests hang until their context is done,
// like a slow or unreachable API server.
type hangingClientset struct {
	*fake.Clientset
	hangPodCreate bool
	hangDeletes   bool
	hanging       chan struct{}
	hangingOnce   sync.Once
}

func newHangingClientset(objects ...runtime.Object) *hangingClientset {
	return &hangingClientset{Clientset: fake.NewSimpleClientset(objects...), hanging: make(chan struct{})}
}

func (c *hangingClientset) hang(ctx context.Context) error {
	c.hangingOnce.Do(func() { close(c.hanging) })
	<-ctx.Done()
	return ctx.Err()
}

func (c *hangingClientset) CoreV1() typedcorev1.CoreV1Interface {
	return &hangingCoreV1{CoreV1Interface: c.Clientset.CoreV1(), c: c}
}

type hangingCoreV1 struct {
	typedcorev1.CoreV1Interface
	c *hangingClientset
}

func (h *hangingCoreV1) Pods(namespace string) typedcorev1.PodInterface {
	return &hangingPods{PodInterface: h.CoreV1Interface.Pods(namespace), c: h.c}
}

func (h *hangingCoreV1) Secrets(namespace string) typedcorev1.SecretInterface {
	return &hangingSecrets{SecretInterface: h.CoreV1Interface.Secrets(namespace), c: h.c}
}

type hangingPods struct {
	typedcorev1.PodInterface
	c *hangingClientset
}

func (p *hangingPods) Create(ctx context.Context, pod *corev1.Pod, opts v1.CreateOptions) (*corev1.Pod, error) {
	if p.c.hangPodCreate {
		return nil, p.c.hang(ctx)
	}

	return p.PodInterface.Create(ctx, pod, opts)
}

func (p *hangingPods) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	if p.c.hangDeletes {
		return p.c.hang(ctx)
	}

	return p.PodInterface.Delete(ctx, name, opts)
}

type hangingSecrets struct {
	typedcorev1.SecretInterface
	c *hangingClientset
}

func (s *hangingSecrets) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	if s.c.hangDeletes {
		return s.c.hang(ctx)
	}

	return s.SecretInterface.Delete(ctx, name, opts)
}

func newK8sExecutorWithPolling(t *testing.T, clientset k8sclient.Interface, attempts int) *KubernetesExecutor {
	validator, err := kubernetes.NewImageValidator([]string{})
	assert.NoError(t, err)

	client, err := kubernetes.NewKubernetesClient(clientset, kubernetes.Config{
		Namespace:          "default",
		ImageValidator:     validator,
		OwnerPodName:       "agent-pod",
		PodPollingAttempts: attempts,
		PodPollingInterval: 10 * time.Millisecond,
		DeleteAttempts:     2,
		DeleteInterval:     time.Millisecond,
	})
	assert.NoError(t, err)

	logger, _ := eventlogger.DefaultTestLogger()
	return newKubernetesExecutorWithClient(k8sJobRequest(), logger, client)
}

func setJobPodPhase(t *testing.T, clientset *fake.Clientset, phase corev1.PodPhase) {
	pods := clientset.CoreV1().Pods("default")
	pod, err := pods.Get(context.Background(), "semaphore-job-"+k8sTestJobID, v1.GetOptions{})
	if assert.NoError(t, err) {
		pod.Status.Phase = phase
		_, err = pods.UpdateStatus(context.Background(), pod, v1.UpdateOptions{})
		assert.NoError(t, err)
	}
}

func countJobPodGets(clientset *fake.Clientset) int {
	n := 0
	for _, action := range clientset.Actions() {
		get, ok := action.(k8stesting.GetAction)
		if ok && action.GetResource().Resource == "pods" && get.GetName() == "semaphore-job-"+k8sTestJobID {
			n++
		}
	}

	return n
}

func returnsWithin(t *testing.T, d time.Duration, fn func() int) int {
	done := make(chan int, 1)
	go func() { done <- fn() }()

	select {
	case code := <-done:
		return code
	case <-time.After(d):
		t.Fatalf("did not return within %v", d)
		return -1
	}
}

func Test__KubernetesExecutor__StopWhileWaitingForPod(t *testing.T) {
	clientset := fake.NewSimpleClientset(agentPodObject())

	// Enough polling to wait for ~100s if the stop didn't interrupt it.
	e := newK8sExecutorWithPolling(t, clientset, 10000)
	assert.Equal(t, 0, e.Prepare())
	setJobPodPhase(t, clientset, corev1.PodPending)

	startDone := make(chan int, 1)
	go func() { startDone <- e.Start() }()
	assert.Eventually(t, func() bool { return countJobPodGets(clientset) >= 3 }, 5*time.Second, 10*time.Millisecond)

	assert.Equal(t, 0, returnsWithin(t, 5*time.Second, e.Stop))

	select {
	case code := <-startDone:
		assert.Equal(t, 1, code)
	case <-time.After(5 * time.Second):
		t.Fatal("Start() kept waiting for the pod after the job was stopped")
	}

	assertNoJobResources(t, clientset)
}

func Test__KubernetesExecutor__StopWhileShellStarts(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip()
	}

	clientset := fake.NewSimpleClientset(agentPodObject())
	e := newK8sExecutor(t, clientset)

	shellStarting := make(chan struct{})
	release := make(chan struct{})
	var sh *shell.Shell
	e.startShell = func() (*shell.Shell, error) {
		close(shellStarting)
		<-release

		// A real local shell stands in for the kubectl exec session.
		s, err := shell.NewShell(os.TempDir())
		if err != nil {
			return nil, err
		}

		sh = s
		return s, s.Start()
	}

	assert.Equal(t, 0, e.Prepare())

	startDone := make(chan int, 1)
	go func() { startDone <- e.Start() }()
	<-shellStarting

	// The shell is not visible to Stop() yet, so Start() has to close it.
	assert.Equal(t, 0, returnsWithin(t, 5*time.Second, e.Stop))
	assertNoJobResources(t, clientset)
	close(release)

	assert.Equal(t, 1, <-startDone)
	assert.Nil(t, e.Shell)

	select {
	case <-sh.ExitSignal:
	case <-time.After(5 * time.Second):
		t.Fatal("the shell session of a stopped job was left open")
	}
}

func Test__KubernetesExecutor__StartWithoutStop(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip()
	}

	clientset := fake.NewSimpleClientset(agentPodObject())
	e := newK8sExecutor(t, clientset)
	e.startShell = func() (*shell.Shell, error) {
		s, err := shell.NewShell(os.TempDir())
		if err != nil {
			return nil, err
		}

		return s, s.Start()
	}

	assert.Equal(t, 0, e.Prepare())
	assert.Equal(t, 0, e.Start())
	assert.NotNil(t, e.Shell)

	assert.Equal(t, 0, e.Stop())
	assertNoJobResources(t, clientset)
}

func Test__KubernetesExecutor__StopAbortsHangingRequests(t *testing.T) {
	clientset := newHangingClientset(agentPodObject())
	clientset.hangPodCreate = true
	e := newK8sExecutor(t, clientset)

	prepareDone := make(chan int, 1)
	go func() { prepareDone <- e.Prepare() }()
	<-clientset.hanging

	// Without aborting the pod creation, Stop() waits for its 30s timeout.
	assert.Equal(t, 0, returnsWithin(t, 5*time.Second, e.Stop))
	assert.Equal(t, 1, <-prepareDone)
	assertNoJobResources(t, clientset.Clientset)
}

func Test__KubernetesExecutor__CleanupIsBounded(t *testing.T) {
	clientset := newHangingClientset(agentPodObject())
	e := newK8sExecutor(t, clientset)
	assert.Equal(t, 0, e.Prepare())

	clientset.hangDeletes = true
	e.cleanupTimeout = 200 * time.Millisecond

	// Every delete hangs: cleanup gives up once its deadline passes,
	// instead of waiting for each attempt's own timeout.
	started := time.Now()
	assert.Equal(t, 0, returnsWithin(t, 5*time.Second, e.Stop))
	assert.Less(t, time.Since(started), 2*time.Second)
}

func Test__KubernetesExecutor__StopBetweenPrepareAndStart(t *testing.T) {
	clientset := fake.NewSimpleClientset(agentPodObject())

	// Enough polling to wait for ~100s if Start() waited for the deleted pod.
	e := newK8sExecutorWithPolling(t, clientset, 10000)
	assert.Equal(t, 0, e.Prepare())
	assert.Equal(t, 0, e.Stop())
	assertNoJobResources(t, clientset)

	gets := countJobPodGets(clientset)
	assert.Equal(t, 1, returnsWithin(t, 2*time.Second, e.Start))
	assert.Equal(t, gets, countJobPodGets(clientset), "Start() waited for the pod of a stopped job")
}

func Test__KubernetesExecutor__CommandsWithoutShell(t *testing.T) {
	t.Run("stopped before it started -> commands fail, no panic", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(agentPodObject())
		e := newK8sExecutor(t, clientset)
		assert.Equal(t, 0, e.Stop())
		assert.Equal(t, 1, e.Prepare())

		assert.NotZero(t, e.RunCommandWithOptions(CommandOptions{Command: "echo hello", Alias: "post-job hook"}))
		assert.NotZero(t, e.RunCommand("echo hello", false, ""))
		_, code := e.GetOutputFromCommand("echo hello")
		assert.NotZero(t, code)
	})

	t.Run("Prepare failed without a stop -> commands fail, no panic", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(agentPodObject())
		clientset.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(corev1.Resource("pods"), "", nil)
		})

		e := newK8sExecutor(t, clientset)
		assert.Equal(t, 1, e.Prepare())
		assert.NotZero(t, e.RunCommandWithOptions(CommandOptions{Command: "echo hello"}))
		assert.Equal(t, 0, e.Stop())
	})
}
