package executors

import (
	"fmt"

	api "github.com/semaphoreci/agent/pkg/api"
)

type DockerComposeFile struct {
	configuration api.Compose
}

func ConstructDockerComposeFile(conf api.Compose) string {
	f := DockerComposeFile{configuration: conf}
	return f.Construct()
}

func (f *DockerComposeFile) Construct() string {
	dockerCompose := ""
	dockerCompose += "version: \"2.0\"\n"
	dockerCompose += "\n"
	dockerCompose += "services:\n"

	main, rest := f.configuration.Containers[0], f.configuration.Containers[1:]

	// main service links up all the services
	dockerCompose += f.ServiceWithLinks(main, rest)
	dockerCompose += "\n"

	for _, c := range rest {
		dockerCompose += f.Service(c)
		dockerCompose += "\n"
	}

	return dockerCompose
}

func (f *DockerComposeFile) Service(container api.Container) string {
	result := ""
	result += fmt.Sprintf("  %s:\n", container.Name)
	result += fmt.Sprintf("    image: %s\n", container.Image)

	if container.Command != "" {
		result += fmt.Sprintf("    command: %s\n", container.Command)
	}

	if len(container.EnvVars) > 0 {
		result += "    environment:\n"

		for _, e := range container.EnvVars {
			result += fmt.Sprintf("      - %s=%s\n", e.Name, e.Value)
		}
	}

	return result
}

func (f *DockerComposeFile) ServiceWithLinks(c api.Container, links []api.Container) string {
	result := f.Service(c)

	if len(links) > 0 {
		result += "    links:\n"

		for _, link := range links {
			result += fmt.Sprintf("      - %s\n", link.Name)
		}
	}

	return result
}
