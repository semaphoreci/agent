package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

type Container struct {
	Name       string   `json:"name" yaml:"name"`
	Image      string   `json:"image" yaml:"image"`
	Command    string   `json:"command" yaml:"command"`
	Entrypoint string   `json:"entrypoint" yaml:"entrypoint"`
	User       string   `json:"user" yaml:"user"`
	EnvVars    []EnvVar `json:"env_vars" yaml:"env_vars"`
}

type ImagePullCredentials struct {
	EnvVars []EnvVar `json:"env_vars" yaml:"env_vars"`
	Files   []File   `json:"files" yaml:"files"`
}

type Compose struct {
	ImagePullCredentials []ImagePullCredentials `json:"image_pull_credentials" yaml:"image_pull_credentials"`
	Containers           []Container            `json:"containers" yaml:"containers"`
	HostSetupCommands    []Command              `json:"host_setup_commands" yaml:"host_setup_commands"`
}

type Command struct {
	Directive string `json:"directive" yaml:"directive"`
	Alias     string `json:"alias" yaml:"alias"`
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

func (f *File) NormalizePath(homeDir string) string {
	// convert path to platform-specific one first
	path := filepath.FromSlash(f.Path)

	if filepath.IsAbs(path) {
		return path
	}

	if strings.HasPrefix(path, "~") {
		return strings.ReplaceAll(path, "~", homeDir)
	}

	return filepath.Join(homeDir, path)
}

func (f *File) ParseMode() (fs.FileMode, error) {
	fileMode, err := strconv.ParseUint(f.Mode, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("bad file permission '%s'", f.Mode)
	}

	return fs.FileMode(fileMode), nil
}

type Callbacks struct {
	Finished         string `json:"finished" yaml:"finished"`
	TeardownFinished string `json:"teardown_finished" yaml:"teardown_finished"`
}

type Logger struct {
	Method         string `json:"method" yaml:"method"`
	URL            string `json:"url" yaml:"url"`
	Token          string `json:"token" yaml:"token"`
	MaxSizeInBytes int    `json:"max_size_in_bytes" yaml:"max_size_in_bytes"`
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
	Logger    Logger    `json:"logger" yaml:"logger"`
}

func (j *JobRequest) FindEnvVar(varName string) (string, error) {
	return findEnvVar(j.EnvVars, varName)
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

	// #nosec
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

func (c *ImagePullCredentials) ToCmdEnvVars() ([]string, error) {
	envs := []string{}

	for _, env := range c.EnvVars {
		name := env.Name
		value, err := env.Decode()
		if err != nil {
			return envs, fmt.Errorf("error decoding '%s': %v", env.Name, err)
		}

		envs = append(envs, fmt.Sprintf("%s=%s", name, string(value)))
	}

	return envs, nil
}

func (c *ImagePullCredentials) FindFile(path string) (string, error) {
	for _, f := range c.Files {
		if f.Path == path {
			v, err := f.Decode()
			if err != nil {
				return "", fmt.Errorf("error decoding '%s': %v", path, err)
			}

			return string(v), nil
		}
	}

	return "", fmt.Errorf("no file '%s' found", path)
}

func (c *ImagePullCredentials) FindEnvVar(varName string) (string, error) {
	return findEnvVar(c.EnvVars, varName)
}

func findEnvVar(envVars []EnvVar, varName string) (string, error) {
	for _, envVar := range envVars {
		if envVar.Name == varName {
			v, err := envVar.Decode()
			if err != nil {
				return "", fmt.Errorf("error decoding '%s': %v", varName, err)
			}

			return string(v), nil
		}
	}

	return "", fmt.Errorf("no env var '%s' found", varName)
}

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
				return "", fmt.Errorf("unknown DOCKER_CREDENTIAL_TYPE: '%s'", v)
			}
		}
	}

	return "", fmt.Errorf("DOCKER_CREDENTIAL_TYPE not set")
}
