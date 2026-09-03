package agentapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
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
	Token            string `json:"token" yaml:"token"`
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

/*
 * A public key has no name to report, so a decode failure is identified by
 * its position in the job request. The key material itself is never logged:
 * these are the keys used to authorize SSH access into the job.
 */
func (p *PublicKey) DecodeAt(index int) ([]byte, error) {
	key, err := p.Decode()
	if err != nil {
		return key, fmt.Errorf(
			"error decoding SSH public key #%d (%s): %w",
			index, describeBase64Value(string(*p)), err,
		)
	}

	return key, nil
}

type JobRequest struct {
	JobID         string      `json:"job_id" yaml:"job_id"`
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
	value, err := base64.StdEncoding.DecodeString(e.Value)
	if err != nil {
		return value, fmt.Errorf("error decoding '%s' (%s): %w", e.Name, describeBase64Value(e.Value), err)
	}

	return value, nil
}

func (f *File) Decode() ([]byte, error) {
	content, err := base64.StdEncoding.DecodeString(f.Content)
	if err != nil {
		return content, fmt.Errorf("error decoding '%s' (%s): %w", f.Path, describeBase64Value(f.Content), err)
	}

	return content, nil
}

var newLines = strings.NewReplacer("\r", "", "\n", "")

/*
 * Describes why base64.StdEncoding might reject a value, without ever
 * including the value itself, since env vars and files hold secrets.
 * The base64 error alone only points at a byte offset, which is not enough
 * to tell a truncated value from one encoded with the wrong alphabet.
 */
func describeBase64Value(value string) string {
	description := fmt.Sprintf("length %d", len(value))

	/*
	 * Only claim the url-safe alphabet when the value is rejected by the
	 * standard one and accepted by the url-safe one - that combination is
	 * what points at a producer using the wrong encoder. Plaintext holding
	 * a '-' (a branch name, a date) decodes as neither.
	 */
	if !decodesAs(base64.StdEncoding, value) && decodesAs(base64.URLEncoding, value) {
		description += ", valid as url-safe base64"
	}

	// \r and \n are ignored by the decoder, so they don't count towards padding.
	if len(newLines.Replace(value))%4 != 0 {
		description += ", not padded to a multiple of 4"
	}

	if strings.ContainsAny(value, " \t\r\n") {
		description += ", contains whitespace"
	}

	return description
}

func decodesAs(encoding *base64.Encoding, value string) bool {
	_, err := encoding.DecodeString(value)
	return err == nil
}

/*
 * Decodes every env var and file in the job request, reporting all the ones
 * that are not valid base64. Only the fields that are always decoded when a
 * job runs are checked here: container env vars and image pull credentials are
 * decoded conditionally, or with the error ignored, so a bad value there does
 * not necessarily break the job.
 *
 * SSH public keys are a known gap rather than a safe omission: a key that
 * fails to decode does fail the job on the docker-compose executor. They are
 * left out because PublicKey has no name to report, so there is nothing
 * useful to say about which key is broken.
 */
func (j *JobRequest) ValidateEncoding() error {
	errs := []error{}
	collect := func(_ []byte, err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	// env vars first: they are exported before files are injected,
	// so this reports offenders in the order a job hits them.
	for _, envVar := range j.EnvVars {
		collect(envVar.Decode())
	}

	for _, file := range j.Files {
		collect(file.Decode())
	}

	// errors.Join returns nil for an empty slice, and keeps every offender
	// reachable through errors.Is/errors.As instead of flattening to a string.
	return errors.Join(errs...)
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
			return envs, err
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
				return "", err
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
				return "", err
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
