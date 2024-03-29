package docker

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/aws"
)

var dockerComposeV2VersionRegex = regexp.MustCompile(`^Docker Compose version (.*)$`)
var dockerComposeV1VersionRegex = regexp.MustCompile(`^docker-compose version (.*),.*$`)

type DockerConfig struct {
	Auths map[string]DockerConfigAuthEntry `json:"auths" datapolicy:"token"`
}

type DockerConfigAuthEntry struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty" datapolicy:"password"`
	Auth     string `json:"auth,omitempty" datapolicy:"token"`
}

func NewDockerConfig(credentials []api.ImagePullCredentials) (*DockerConfig, error) {
	if len(credentials) == 0 {
		return nil, fmt.Errorf("no credentials")
	}

	dockerConfig := DockerConfig{Auths: map[string]DockerConfigAuthEntry{}}

	for _, credential := range credentials {
		strategy, err := credential.Strategy()
		if err != nil {
			return nil, err
		}

		u, err := configUsername(strategy, credential)
		if err != nil {
			return nil, err
		}

		p, err := configPassword(strategy, credential)
		if err != nil {
			return nil, err
		}

		server, err := configServerURL(strategy, credential)
		if err != nil {
			return nil, err
		}

		e := DockerConfigAuthEntry{
			Username: u,
			Password: p,
			Auth:     base64.StdEncoding.EncodeToString([]byte(u + ":" + p)),
		}

		dockerConfig.Auths[server] = e
	}

	return &dockerConfig, nil
}

func configUsername(strategy string, credentials api.ImagePullCredentials) (string, error) {
	switch strategy {
	case api.ImagePullCredentialsStrategyDockerHub:
		return credentials.FindEnvVar("DOCKERHUB_USERNAME")
	case api.ImagePullCredentialsStrategyGenericDocker:
		return credentials.FindEnvVar("DOCKER_USERNAME")
	case api.ImagePullCredentialsStrategyECR:
		return "AWS", nil
	case api.ImagePullCredentialsStrategyGCR:
		return "_json_key", nil
	default:
		return "", fmt.Errorf("%s not supported", strategy)
	}
}

func configPassword(strategy string, credentials api.ImagePullCredentials) (string, error) {
	switch strategy {
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
		return "", fmt.Errorf("%s not supported", strategy)
	}
}

func configServerURL(strategy string, credentials api.ImagePullCredentials) (string, error) {
	switch strategy {
	case api.ImagePullCredentialsStrategyDockerHub:
		return "docker.io", nil
	case api.ImagePullCredentialsStrategyGenericDocker:
		return credentials.FindEnvVar("DOCKER_URL")
	case api.ImagePullCredentialsStrategyGCR:
		return credentials.FindEnvVar("GCR_HOSTNAME")
	case api.ImagePullCredentialsStrategyECR:
		return aws.GetECRServerURL(credentials)
	default:
		return "", fmt.Errorf("%s not supported", strategy)
	}
}

func DockerComposePluginVersion() (string, error) {
	output, err := exec.Command("docker", "compose", "version").Output()
	if err != nil {
		return "", err
	}

	match := dockerComposeV2VersionRegex.FindStringSubmatch(strings.Trim(string(output), "\n"))
	if match == nil {
		return "", fmt.Errorf("error finding docker compose v2 version: '%s'", string(output))
	}

	return match[1], nil
}

// NOTE: this doesn't necessarily points to a docker compose v1 installation.
// Sometimes, 'docker-compose' is an alias to the docker compose v2 plugin.
func DockerComposeCLIVersion() (string, error) {
	output, err := exec.Command("docker-compose", "--version").Output()
	if err != nil {
		return "", err
	}

	match := dockerComposeV1VersionRegex.FindStringSubmatch(strings.Trim(string(output), "\n"))
	if match == nil {
		return "", fmt.Errorf("error finding docker compose v1 version: %s", string(output))
	}

	return match[1], nil
}

func DockerComposeVersion() (string, error) {
	version, err := DockerComposePluginVersion()
	if err == nil {
		return version, nil
	}

	return DockerComposeCLIVersion()
}
