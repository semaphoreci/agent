package executors

import (
	"encoding/base64"
	"testing"

	api "github.com/semaphoreci/agent/pkg/api"
	assert "github.com/stretchr/testify/assert"
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
      - FOO=BAR
      - FAZ=ZEZ

`

	compose := ConstructDockerComposeFile(conf, true)
	assert.Equal(t, expected, compose)
}
