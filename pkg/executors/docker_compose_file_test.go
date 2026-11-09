package executors

import (
	"encoding/base64"
	"testing"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
	assert "github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func Test__DockerComposeFileConstruction(t *testing.T) {
	conf := api.Compose{
		Containers: []api.Container{
			api.Container{
				Name:  "main",
				Image: "ruby:2.6",
			},
			api.Container{
				Name:       "db",
				Image:      "postgres:9.6",
				Command:    "postgres start",
				User:       "postgres",
				Entrypoint: "/docker-entrypoint-initdb.d/init-user-db.sh",
				EnvVars: []api.EnvVar{
					api.EnvVar{
						Name:  "FOO",
						Value: base64.StdEncoding.EncodeToString([]byte("BAR")),
					},
					api.EnvVar{
						Name:  "FAZ",
						Value: base64.StdEncoding.EncodeToString([]byte("ZEZ")),
					},
				},
			},
		},
	}

	expected := `version: "2.0"

services:
  main:
    image: ruby:2.6
    devices:
      - "/dev/kvm:/dev/kvm"
    links:
      - db

  db:
    image: postgres:9.6
    devices:
      - "/dev/kvm:/dev/kvm"
    command: postgres start
    user: postgres
    entrypoint: /docker-entrypoint-initdb.d/init-user-db.sh
    environment:
      - "FOO=BAR"
      - "FAZ=ZEZ"

`

	compose, err := ConstructDockerComposeFile(conf, true, []config.FileInjection{})
	assert.NoError(t, err)
	assert.Equal(t, expected, compose)
}

func Test__DockerComposeFile__ServiceEnvVars(t *testing.T) {
	encode := func(value string) string {
		return base64.StdEncoding.EncodeToString([]byte(value))
	}

	testCases := []struct {
		name        string
		encoded     string
		expected    string
		expectedErr string
	}{
		{name: "interpolation syntax", encoded: encode("pa${ss$WORD"), expected: "KEY=pa$${ss$$WORD"},
		{name: "already escaped dollar", encoded: encode("pa$$word $$$x"), expected: "KEY=pa$$word $$$$x"},
		{name: "quotes and backslashes", encoded: encode(`a"b\c'd`), expected: `KEY=a"b\c'd`},
		{name: "multi-line", encoded: encode("first\nsecond"), expected: "KEY=first\nsecond"},
		{name: "YAML syntax", encoded: encode("x: y # z"), expected: "KEY=x: y # z"},
		{name: "control and non-ASCII characters", encoded: encode("\x00\a\x1b\u2028ünï 🚀"), expected: "KEY=\x00\a\x1b\u2028ünï 🚀"},
		{name: "trailing whitespace", encoded: encode("  bar \t\r\n"), expected: "KEY=  bar"},
		{name: "empty", encoded: encode(""), expected: "KEY="},
		{name: "invalid base64", encoded: "c2VjcmV0!!!", expectedErr: "error decoding env var 'KEY' of container 'main'"},
		{name: "invalid UTF-8", encoded: encode("caf\xe9"), expectedErr: "env var 'KEY' of container 'main' is not valid UTF-8"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := DockerComposeFile{}
			service, err := f.Service(api.Container{
				Name:    "main",
				EnvVars: []api.EnvVar{{Name: "KEY", Value: tc.encoded}},
			})

			if tc.expectedErr != "" {
				assert.ErrorContains(t, err, tc.expectedErr)
				return
			}

			assert.NoError(t, err)

			// Compare what compose reads, not the quoting it reads it from.
			parsed := map[string]struct {
				Environment []string `yaml:"environment"`
			}{}
			assert.NoError(t, yaml.Unmarshal([]byte(service), &parsed))
			assert.Equal(t, []string{tc.expected}, parsed["main"].Environment)
		})
	}
}
