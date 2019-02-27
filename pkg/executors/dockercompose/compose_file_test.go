package dockercompose

import (
	"testing"

	api "github.com/semaphoreci/agent/pkg/api"
	assert "github.com/stretchr/testify/assert"
)

func Test__ComposeFileConstruction(t *testing.T) {
	conf := api.Compose{
		Containers: []api.Container{
			api.Container{
				Name:  "main",
				Image: "ruby:2.6",
			},
			api.Container{
				Name:    "db",
				Image:   "postgres:9.6",
				Command: "postgres start",
				EnvVars: []api.EnvVar{
					api.EnvVar{
						Name:  "FOO",
						Value: "BAR",
					},
					api.EnvVar{
						Name:  "FAZ",
						Value: "ZEZ",
					},
				},
			},
		},
	}

	expected := `version: "2.0"

services:
  main:
    image: ruby:2.6
    links:
      - db

  db:
    image: postgres:9.6
    command: postgres start
    environment:
      - FOO=BAR
      - FAZ=ZEZ

`

	compose := ConstructComposeFile(conf)
	assert.Equal(t, expected, compose)
}
