package executors

import (
	"testing"

	"github.com/semaphoreci/agent/pkg/config"
	assert "github.com/stretchr/testify/assert"
)

func Test__KubectlCommand(t *testing.T) {
	podName := "semaphore-job-123"

	t.Run("exec strategy uses kubectl exec and spawns a new bash", func(t *testing.T) {
		exe, args := kubectlCommand(config.KubernetesExecutionStrategyExec, podName)
		assert.Equal(t, "kubectl", exe)
		assert.Equal(t, []string{"exec", "-it", podName, "-c", "main", "--", "bash", "--login"}, args)
	})

	t.Run("empty strategy defaults to exec", func(t *testing.T) {
		exe, args := kubectlCommand("", podName)
		assert.Equal(t, "kubectl", exe)
		assert.Equal(t, []string{"exec", "-it", podName, "-c", "main", "--", "bash", "--login"}, args)
	})

	t.Run("attach strategy attaches to the existing PID 1 shell", func(t *testing.T) {
		exe, args := kubectlCommand(config.KubernetesExecutionStrategyAttach, podName)
		assert.Equal(t, "kubectl", exe)
		assert.Equal(t, []string{"attach", "-i", "-t", podName, "-c", "main"}, args)
	})
}
