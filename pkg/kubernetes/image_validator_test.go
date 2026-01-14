package kubernetes

import (
	"testing"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/stretchr/testify/assert"
)

func Test__ImageValidator(t *testing.T) {
	t.Run("bad expression => error creating validator", func(t *testing.T) {
		_, err := NewImageValidator([]string{"(.*)\\((.*)\\) ?(? U)"})
		assert.Error(t, err)
	})

	t.Run("no expressions => no restrictions", func(t *testing.T) {
		imageValidator, err := NewImageValidator([]string{})
		assert.NoError(t, err)
		assert.NoError(t, imageValidator.Validate([]api.Container{
			{Image: "registry.semaphoreci.com/ruby:2.7"},
			{Image: "docker.io/redis"},
			{Image: "postgres/9.6"},
		}))
	})

	t.Run("single regex with all invalid images", func(t *testing.T) {
		imageValidator, err := NewImageValidator([]string{
			"^custom-registry-1\\.com\\/.+",
		})

		assert.NoError(t, err)
		assert.ErrorContains(t, imageValidator.Validate([]api.Container{
			{Image: "registry.semaphoreci.com/ruby:2.7"},
			{Image: "docker.io/redis"},
			{Image: "postgres/9.6"},
		}), "image 'registry.semaphoreci.com/ruby:2.7' is not allowed")
	})

	t.Run("single regex with some invalid images", func(t *testing.T) {
		imageValidator, err := NewImageValidator([]string{
			"^registry\\.semaphoreci\\.com\\/.+",
		})

		assert.NoError(t, err)
		assert.ErrorContains(t, imageValidator.Validate([]api.Container{
			{Image: "registry.semaphoreci.com/ruby:2.7"},
			{Image: "docker.io/redis"},
			{Image: "postgres/9.6"},
		}), "image 'docker.io/redis' is not allowed")
	})
}
