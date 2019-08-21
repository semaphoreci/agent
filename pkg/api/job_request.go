package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	yaml "gopkg.in/yaml.v2"
)

type Container struct {
	Name    string   `json:"name" yaml:"name"`
	Image   string   `json:"image" yaml:"image"`
	Command string   `json:"command" yaml:"command"`
	EnvVars []EnvVar `json:"env_vars" yaml:"env_vars"`
}

type ImagePullCredentials struct {
	EnvVars []EnvVar `json:"env_vars" yaml:"env_vars"`
	Files   []File   `json:"files" yaml:"files"`
}

type Compose struct {
	ImagePullCredentials []ImagePullCredentials `json:"image_pull_credentials" yaml:"image_pull_credentials"`
	Containers           []Container            `json:"containers" yaml:"containers"`
}

type Command struct {
	Directive string `json:"directive" yaml:"directive"`
}

type EnvVar struct {
	Name  string `json:"name" yaml:"name"`
	Value string `json:"value" yaml:"value"`
}

type File struct {
	Path    string `json:"path" yaml:"path"`
	Content string `json:"content" yaml:"content"`
	Mode    string `json:"mode" yaml:"mode"`
}

type Callbacks struct {
	Finished         string `json:"finished" yaml:"finished"`
	TeardownFinished string `json:"teardown_finished" yaml:"teardown_finished"`
}

type PublicKey string

func (p *PublicKey) Decode() ([]byte, error) {
	return base64.StdEncoding.DecodeString(string(*p))
}

type JobRequest struct {
	ID            string      `json:"id" yaml:"id"`
	Executor      string      `json:"executor" yaml:"executor"`
	Compose       Compose     `json:"compose" yaml:"compose"`
	Commands      []Command   `json:"commands" yaml:"commands"`
	SSHPublicKeys []PublicKey `json:"ssh_public_keys" yaml:"ssh_public_keys"`

	EpilogueAlwaysCommands []Command `json:"epilogue_always_commands" yaml:"epilogue_always_commands"`
	EpilogueOnPassCommands []Command `json:"epilogue_on_pass_commands" yaml:"epilogue_on_pass_commands"`
	EpilogueOnFailCommands []Command `json:"epilogue_on_fail_commands" yaml:"epilogue_on_fail_commands"`

	EnvVars   []EnvVar  `json:"env_vars" yaml:"env_vars"`
	Files     []File    `json:"files" yaml:"file"`
	Callbacks Callbacks `json:"callbacks" yaml:"callbacks"`
}

func NewRequestFromJSON(content []byte) (*JobRequest, error) {
	jobRequest := &JobRequest{}

	err := json.Unmarshal([]byte(content), jobRequest)

	if err != nil {
		return nil, err
	}

	return jobRequest, nil
}

func NewRequestFromYamlFile(path string) (*JobRequest, error) {
	filename, _ := filepath.Abs(path)
	yamlFile, err := ioutil.ReadFile(filename)

	jobRequest := &JobRequest{}

	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(yamlFile, jobRequest)
	if err != nil {
		return nil, err
	}

	return jobRequest, nil
}

func (e *EnvVar) Decode() ([]byte, error) {
	return base64.StdEncoding.DecodeString(e.Value)
}

func (f *File) Decode() ([]byte, error) {
	return base64.StdEncoding.DecodeString(f.Content)
}

const ImagePullCredentialsStrategyDockerHub = "DockerHub"
const ImagePullCredentialsStrategyGenericDocker = "GenericDocker"
const ImagePullCredentialsStrategyECR = "AWS_ECR"
const ImagePullCredentialsStrategyGCR = "GCR"

func (c *ImagePullCredentials) Strategy() (string, error) {
	for _, e := range c.EnvVars {
		if e.Name == "DOCKER_CREDENTIAL_TYPE" {
			v, err := e.Decode()

			if err != nil {
				return "", err
			}

			switch string(v) {
			case ImagePullCredentialsStrategyDockerHub:
				return ImagePullCredentialsStrategyDockerHub, nil
			case ImagePullCredentialsStrategyGenericDocker:
				return ImagePullCredentialsStrategyGenericDocker, nil
			case ImagePullCredentialsStrategyECR:
				return ImagePullCredentialsStrategyECR, nil
			case ImagePullCredentialsStrategyGCR:
				return ImagePullCredentialsStrategyGCR, nil
			default:
				return "", fmt.Errorf("Unknown DOCKER_CREDENTIAL_TYPE: '%s'", v)
			}
		}
	}

	return "", fmt.Errorf("DOCKER_CREDENTIAL_TYPE not set")
}
