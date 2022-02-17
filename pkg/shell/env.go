package shell

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"

	"github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
)

type Environment struct {
	env map[string]string
}

func EnvFromAPI(envVars []api.EnvVar) (*Environment, error) {
	newEnv := Environment{}
	for _, envVar := range envVars {
		value, err := envVar.Decode()
		if err != nil {
			return nil, err
		}

		newEnv.Set(envVar.Name, shellQuote(string(value)))
	}

	return &newEnv, nil
}

func (e *Environment) IsEmpty() bool {
	return e.env != nil || len(e.env) == 0
}

func (e *Environment) Set(name, value string) {
	if e.env == nil {
		e.env = map[string]string{}
	}

	e.env[name] = value
}

func (e *Environment) Merge(envVars []config.HostEnvVar) {
	for _, envVar := range envVars {
		e.Set(envVar.Name, envVar.Value)
	}
}

func (e *Environment) Append(otherEnv *Environment) {
	for name, value := range otherEnv.env {
		e.Set(name, value)
	}
}

func (e *Environment) ToArray() []string {
	arr := []string{}
	for name, value := range e.env {
		arr = append(arr, fmt.Sprintf("%s=%s", name, value))
	}

	return arr
}

func (e *Environment) ToFile(fileName string, callback func(name string)) error {
	fileContent := ""
	for name, value := range e.env {
		fileContent += fmt.Sprintf("export %s=%s\n", name, shellQuote(value))
		callback(name)
	}

	// #nosec
	err := ioutil.WriteFile(fileName, []byte(fileContent), 0644)
	if err != nil {
		return err
	}

	return nil
}

func shellQuote(s string) string {
	pattern := regexp.MustCompile(`[^\w@%+=:,./-]`)

	if len(s) == 0 {
		return "''"
	}
	if pattern.MatchString(s) {
		return "'" + strings.Replace(s, "'", "'\"'\"'", -1) + "'"
	}

	return s
}
