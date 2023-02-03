package docker

import (
	"encoding/base64"
	"testing"

	"github.com/semaphoreci/agent/pkg/api"
	"github.com/stretchr/testify/assert"
)

func Test__NewDockerConfig(t *testing.T) {
	t.Run("no credentials", func(t *testing.T) {
		_, err := NewDockerConfig([]api.ImagePullCredentials{})
		assert.ErrorContains(t, err, "no credentials")
	})

	t.Run("no strategy", func(t *testing.T) {
		_, err := NewDockerConfig([]api.ImagePullCredentials{{EnvVars: []api.EnvVar{}}})
		assert.ErrorContains(t, err, "DOCKER_CREDENTIAL_TYPE not set")
	})

	t.Run("unsupported strategy", func(t *testing.T) {
		_, err := NewDockerConfig([]api.ImagePullCredentials{
			{
				EnvVars: []api.EnvVar{
					{Name: "DOCKER_CREDENTIAL_TYPE", Value: base64.StdEncoding.EncodeToString([]byte("WHATEVER"))},
				},
			},
		})

		assert.ErrorContains(t, err, "unknown DOCKER_CREDENTIAL_TYPE: 'WHATEVER'")
	})

	t.Run("dockerhub - no DOCKERHUB_USERNAME leads to error", func(t *testing.T) {
		_, err := NewDockerConfig([]api.ImagePullCredentials{
			{
				EnvVars: []api.EnvVar{
					{Name: "DOCKER_CREDENTIAL_TYPE", Value: base64.StdEncoding.EncodeToString([]byte(api.ImagePullCredentialsStrategyDockerHub))},
				},
			},
		})

		assert.ErrorContains(t, err, "no env var 'DOCKERHUB_USERNAME' found")
	})

	t.Run("dockerhub - no DOCKERHUB_PASSWORD leads to error", func(t *testing.T) {
		_, err := NewDockerConfig([]api.ImagePullCredentials{
			{
				EnvVars: []api.EnvVar{
					{Name: "DOCKER_CREDENTIAL_TYPE", Value: base64.StdEncoding.EncodeToString([]byte(api.ImagePullCredentialsStrategyDockerHub))},
					{Name: "DOCKERHUB_USERNAME", Value: base64.StdEncoding.EncodeToString([]byte("dockerhubuser"))},
				},
			},
		})

		assert.ErrorContains(t, err, "no env var 'DOCKERHUB_PASSWORD' found")
	})

	t.Run("dockerhub - returns config", func(t *testing.T) {
		dockerCfg, err := NewDockerConfig([]api.ImagePullCredentials{
			{
				EnvVars: []api.EnvVar{
					{Name: "DOCKER_CREDENTIAL_TYPE", Value: base64.StdEncoding.EncodeToString([]byte(api.ImagePullCredentialsStrategyDockerHub))},
					{Name: "DOCKERHUB_USERNAME", Value: base64.StdEncoding.EncodeToString([]byte("dockerhubuser"))},
					{Name: "DOCKERHUB_PASSWORD", Value: base64.StdEncoding.EncodeToString([]byte("dockerhubpass"))},
				},
			},
		})

		assert.NoError(t, err)
		assert.Equal(t, *dockerCfg, DockerConfig{
			Auths: map[string]DockerConfigAuthEntry{
				"docker.io": {
					Username: "dockerhubuser",
					Password: "dockerhubpass",
					Auth:     base64.StdEncoding.EncodeToString([]byte("dockerhubuser:dockerhubpass")),
				},
			},
		})
	})

	t.Run("generic docker - no DOCKER_USERNAME leads to error", func(t *testing.T) {
		_, err := NewDockerConfig([]api.ImagePullCredentials{
			{
				EnvVars: []api.EnvVar{
					{Name: "DOCKER_CREDENTIAL_TYPE", Value: base64.StdEncoding.EncodeToString([]byte(api.ImagePullCredentialsStrategyGenericDocker))},
				},
			},
		})

		assert.ErrorContains(t, err, "no env var 'DOCKER_USERNAME' found")
	})

	t.Run("generic docker - no DOCKER_PASSWORD leads to error", func(t *testing.T) {
		_, err := NewDockerConfig([]api.ImagePullCredentials{
			{
				EnvVars: []api.EnvVar{
					{Name: "DOCKER_CREDENTIAL_TYPE", Value: base64.StdEncoding.EncodeToString([]byte(api.ImagePullCredentialsStrategyGenericDocker))},
					{Name: "DOCKER_USERNAME", Value: base64.StdEncoding.EncodeToString([]byte("dockeruser"))},
				},
			},
		})

		assert.ErrorContains(t, err, "no env var 'DOCKER_PASSWORD' found")
	})

	t.Run("generic docker - no DOCKER_URL leads to error", func(t *testing.T) {
		_, err := NewDockerConfig([]api.ImagePullCredentials{
			{
				EnvVars: []api.EnvVar{
					{Name: "DOCKER_CREDENTIAL_TYPE", Value: base64.StdEncoding.EncodeToString([]byte(api.ImagePullCredentialsStrategyGenericDocker))},
					{Name: "DOCKER_USERNAME", Value: base64.StdEncoding.EncodeToString([]byte("dockeruser"))},
					{Name: "DOCKER_PASSWORD", Value: base64.StdEncoding.EncodeToString([]byte("dockerpass"))},
				},
			},
		})

		assert.ErrorContains(t, err, "no env var 'DOCKER_URL' found")
	})

	t.Run("generic docker - returns config", func(t *testing.T) {
		dockerCfg, err := NewDockerConfig([]api.ImagePullCredentials{
			{
				EnvVars: []api.EnvVar{
					{Name: "DOCKER_CREDENTIAL_TYPE", Value: base64.StdEncoding.EncodeToString([]byte(api.ImagePullCredentialsStrategyGenericDocker))},
					{Name: "DOCKER_USERNAME", Value: base64.StdEncoding.EncodeToString([]byte("dockeruser"))},
					{Name: "DOCKER_PASSWORD", Value: base64.StdEncoding.EncodeToString([]byte("dockerpass"))},
					{Name: "DOCKER_URL", Value: base64.StdEncoding.EncodeToString([]byte("custom-registry.com"))},
				},
			},
		})

		assert.NoError(t, err)
		assert.Equal(t, *dockerCfg, DockerConfig{
			Auths: map[string]DockerConfigAuthEntry{
				"custom-registry.com": {
					Username: "dockeruser",
					Password: "dockerpass",
					Auth:     base64.StdEncoding.EncodeToString([]byte("dockeruser:dockerpass")),
				},
			},
		})
	})

	t.Run("GCR - no /tmp/gcr/keyfile.json leads to error", func(t *testing.T) {
		_, err := NewDockerConfig([]api.ImagePullCredentials{
			{
				EnvVars: []api.EnvVar{
					{Name: "DOCKER_CREDENTIAL_TYPE", Value: base64.StdEncoding.EncodeToString([]byte(api.ImagePullCredentialsStrategyGCR))},
				},
			},
		})

		assert.ErrorContains(t, err, "no file '/tmp/gcr/keyfile.json' found")
	})

	t.Run("GCR - no GCR_HOSTNAME leads to error", func(t *testing.T) {
		_, err := NewDockerConfig([]api.ImagePullCredentials{
			{
				EnvVars: []api.EnvVar{
					{Name: "DOCKER_CREDENTIAL_TYPE", Value: base64.StdEncoding.EncodeToString([]byte(api.ImagePullCredentialsStrategyGCR))},
				},
				Files: []api.File{
					{Path: "/tmp/gcr/keyfile.json", Content: base64.StdEncoding.EncodeToString([]byte("aosidaoshd0a9hsd"))},
				},
			},
		})

		assert.ErrorContains(t, err, "no env var 'GCR_HOSTNAME' found")
	})

	t.Run("GCR - returns config", func(t *testing.T) {
		dockerCfg, err := NewDockerConfig([]api.ImagePullCredentials{
			{
				EnvVars: []api.EnvVar{
					{Name: "DOCKER_CREDENTIAL_TYPE", Value: base64.StdEncoding.EncodeToString([]byte(api.ImagePullCredentialsStrategyGCR))},
					{Name: "GCR_HOSTNAME", Value: base64.StdEncoding.EncodeToString([]byte("gcr.io"))},
				},
				Files: []api.File{
					{Path: "/tmp/gcr/keyfile.json", Content: base64.StdEncoding.EncodeToString([]byte("aosidaoshd0a9hsd"))},
				},
			},
		})

		assert.NoError(t, err)
		assert.Equal(t, *dockerCfg, DockerConfig{
			Auths: map[string]DockerConfigAuthEntry{
				"gcr.io": {
					Username: "_json_key",
					Password: "aosidaoshd0a9hsd",
					Auth:     base64.StdEncoding.EncodeToString([]byte("_json_key:aosidaoshd0a9hsd")),
				},
			},
		})
	})
}
