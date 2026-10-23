package executors

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	api "github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
)

// interpolationEscaper escapes each "$" compose would interpolate. A "$$" pair, which
// compose already reads as a literal "$", is listed first so it is kept as is.
var interpolationEscaper = strings.NewReplacer("$$", "$$", "$", "$$")

type DockerComposeFile struct {
	configuration   api.Compose
	exposeKvmDevice bool
	fileInjections  []config.FileInjection
}

func ConstructDockerComposeFile(conf api.Compose, exposeKvmDevice bool, fileInjections []config.FileInjection) (string, error) {
	f := DockerComposeFile{
		configuration:   conf,
		exposeKvmDevice: exposeKvmDevice,
		fileInjections:  fileInjections,
	}
	return f.Construct()
}

func (f *DockerComposeFile) Construct() (string, error) {
	dockerCompose := ""
	dockerCompose += "version: \"2.0\"\n"
	dockerCompose += "\n"

	dockerCompose += "services:\n"

	main, rest := f.configuration.Containers[0], f.configuration.Containers[1:]

	// main service links up all the services
	service, err := f.ServiceWithLinks(main, rest)
	if err != nil {
		return "", err
	}

	dockerCompose += service
	dockerCompose += "\n"

	for _, c := range rest {
		service, err := f.Service(c)
		if err != nil {
			return "", err
		}

		dockerCompose += service
		dockerCompose += "\n"
	}

	return dockerCompose, nil
}

func (f *DockerComposeFile) Service(container api.Container) (string, error) {
	result := ""
	result += fmt.Sprintf("  %s:\n", container.Name)
	result += fmt.Sprintf("    image: %s\n", container.Image)

	if f.exposeKvmDevice {
		result += "    devices:\n"
		result += "      - \"/dev/kvm:/dev/kvm\"\n"
	}

	if container.Command != "" {
		result += fmt.Sprintf("    command: %s\n", container.Command)
	}

	if container.User != "" {
		result += fmt.Sprintf("    user: %s\n", container.User)
	}

	if container.Entrypoint != "" {
		result += fmt.Sprintf("    entrypoint: %s\n", container.Entrypoint)
	}

	if len(container.EnvVars) > 0 {
		result += "    environment:\n"

		for _, e := range container.EnvVars {
			value, err := e.Decode()
			if err != nil {
				return "", fmt.Errorf("error decoding env var '%s' of container '%s': %v", e.Name, container.Name, err)
			}

			// YAML strings hold only valid UTF-8; quoting would silently re-encode other bytes.
			if !utf8.Valid(value) {
				return "", fmt.Errorf("env var '%s' of container '%s' is not valid UTF-8", e.Name, container.Name)
			}

			// Quoted so any value is a valid YAML string. Trailing whitespace is trimmed,
			// since values are often encoded from `echo` output.
			trimmed := strings.TrimRight(string(value), " \t\r\n")
			entry := e.Name + "=" + interpolationEscaper.Replace(trimmed)
			result += fmt.Sprintf("      - %s\n", strconv.Quote(entry))
		}
	}

	return result, nil
}

func (f *DockerComposeFile) ServiceWithLinks(c api.Container, links []api.Container) (string, error) {
	result, err := f.Service(c)
	if err != nil {
		return "", err
	}

	if len(links) > 0 {
		result += "    links:\n"

		for _, link := range links {
			result += fmt.Sprintf("      - %s\n", link.Name)
		}
	}

	if len(f.fileInjections) > 0 {
		result += "    volumes:\n"
		for _, fileInjection := range f.fileInjections {
			result += fmt.Sprintf("      - %s:%s\n", fileInjection.HostPath, fileInjection.Destination)
		}
	}

	return result, nil
}
