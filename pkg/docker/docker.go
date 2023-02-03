package docker

import (
	"fmt"

	"github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/aws"
)

func Username(credentials api.ImagePullCredentials) (string, error) {
	s, err := credentials.Strategy()
	if err != nil {
		return "", err
	}

	switch s {
	case api.ImagePullCredentialsStrategyDockerHub:
		return credentials.FindEnvVar("DOCKERHUB_USERNAME")
	case api.ImagePullCredentialsStrategyGenericDocker:
		return credentials.FindEnvVar("DOCKER_USERNAME")
	case api.ImagePullCredentialsStrategyECR:
		return "AWS", nil
	case api.ImagePullCredentialsStrategyGCR:
		return "_json_key", nil
	default:
		return "", fmt.Errorf("%s not supported", s)
	}
}

func Password(credentials api.ImagePullCredentials) (string, error) {
	s, err := credentials.Strategy()
	if err != nil {
		return "", err
	}

	switch s {
	case api.ImagePullCredentialsStrategyDockerHub:
		return credentials.FindEnvVar("DOCKERHUB_PASSWORD")
	case api.ImagePullCredentialsStrategyGenericDocker:
		return credentials.FindEnvVar("DOCKER_PASSWORD")
	case api.ImagePullCredentialsStrategyECR:
		return aws.GetECRLoginPassword(credentials)
	case api.ImagePullCredentialsStrategyGCR:
		fileContent, err := credentials.FindFile("/tmp/gcr/keyfile.json")
		if err != nil {
			return "", err
		}

		return fileContent, nil
	default:
		return "", fmt.Errorf("%s not supported", s)
	}
}

func Server(credentials api.ImagePullCredentials) (string, error) {
	s, err := credentials.Strategy()
	if err != nil {
		return "", err
	}

	switch s {
	case api.ImagePullCredentialsStrategyDockerHub:
		return "docker.io", nil
	case api.ImagePullCredentialsStrategyGenericDocker:
		return credentials.FindEnvVar("DOCKER_URL")
	case api.ImagePullCredentialsStrategyGCR:
		return credentials.FindEnvVar("GCR_HOSTNAME")
	case api.ImagePullCredentialsStrategyECR:
		return aws.GetECRServerURL(credentials)
	default:
		return "", fmt.Errorf("%s not supported", s)
	}
}
