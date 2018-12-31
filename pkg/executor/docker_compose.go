package executor

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/kr/pty"
)

type Container struct {
	Name  string
	Image string
}

type DockerCompose struct {
	Containers []Container
}

func NewDockerComposeExecutor(containers []Container) (*DockerCompose, error) {
	dc := &DockerCompose{
		Containers: containers,
	}

	return dc, nil
}

func (dc *DockerCompose) Build() error {
	template := ""
	template += `version: "2"` + "\n"
	template += `services:` + "\n"

	main := dc.Containers[0]
	template += `  ` + main.Name + ":\n"
	template += `    image: "` + main.Image + `"` + "\n"

	if len(dc.Containers) > 1 {
		template += `    links:` + "\n"

		// first we add links
		for _, c := range dc.Containers[1:] {
			template += `      - ` + c.Name + "\n"
		}

		// then we define the rest of the services
		for _, c := range dc.Containers[1:] {
			template += `  ` + c.Name + ":\n"
			template += `    image: "` + c.Image + `"` + "\n"
		}
	}

	err := ioutil.WriteFile("/tmp/dc1", []byte(template), 0644)
	if err != nil {
		return err
	}

	return nil
}

func (dc *DockerCompose) Setup() error {
	fmt.Println("Setup")

	cmd := exec.Command("bash", "-c", "docker-compose -f /tmp/dc1 pull --include-deps")

	err := cmd.Start()
	if err != nil {
		return err
	}

	err = cmd.Wait()
	if err != nil {
		return err
	}

	fmt.Println("Setup 2")

	cmd = exec.Command("bash", "-c", `docker-compose -f /tmp/dc1 run --name AAA4 -d main bash -c "sleep infinity"`)

	err = cmd.Start()
	if err != nil {
		return err
	}

	err = cmd.Wait()
	if err != nil {
		return err
	}

	fmt.Println(cmd.Output())

	return nil
}

func (dc *DockerCompose) AddFile(path string, content string) error {
	fmt.Println("Adding file " + path)

	err := ioutil.WriteFile("/tmp/1", []byte(content), 0644)
	if err != nil {
		return err
	}

	cmd := exec.Command("bash", "-c", "docker cp /tmp/1 "+path)

	err = cmd.Start()
	if err != nil {
		return err
	}

	err = cmd.Wait()
	if err != nil {
		return err
	}

	return nil
}

func (dc *DockerCompose) ExportEnvVar(path string, container string) error {
	fmt.Printf("Exporting env var")

	return nil
}

func (dc *DockerCompose) Run(command string) (*os.File, error) {
	cmd := exec.Command("bash", "-c", "docker exec -ti AAA3 "+command)

	return pty.Start(cmd)
}
