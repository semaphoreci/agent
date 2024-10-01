package shell

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/semaphoreci/agent/pkg/api"
	"github.com/semaphoreci/agent/pkg/config"
)

type Environment struct {
	env map[string]string
}

func CreateEnvironment(envVars []api.EnvVar, HostEnvVars []config.HostEnvVar) (*Environment, error) {
	newEnv := Environment{}
	for _, envVar := range envVars {
		value, err := envVar.Decode()
		if err != nil {
			return nil, err
		}

		newEnv.Set(envVar.Name, string(value))
	}

	for _, envVar := range HostEnvVars {
		newEnv.Set(envVar.Name, envVar.Value)
	}

	return &newEnv, nil
}

/*
 * Create an environment by reading a file created with
 * an environment dump in Windows.
 */
func CreateEnvironmentFromFile(fileName string) (*Environment, error) {
	// #nosec
	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	contents := string(bytes)
	contents = strings.TrimSpace(contents)
	contents = strings.Replace(contents, "\r\n", "\n", -1)

	lines := strings.Split(contents, "\n")
	environment := Environment{env: map[string]string{}}

	for _, line := range lines {
		nameAndValue := strings.SplitN(line, "=", 2)
		if len(nameAndValue) == 2 {
			environment.Set(nameAndValue[0], nameAndValue[1])
		}
	}

	return &environment, nil
}

func (e *Environment) Set(name, value string) {
	if e.env == nil {
		e.env = map[string]string{}
	}

	e.env[name] = value
}

func (e *Environment) Get(key string) (string, bool) {
	v, ok := e.env[key]
	return v, ok
}

func (e *Environment) Remove(key string) {
	_, ok := e.Get(key)
	if ok {
		delete(e.env, key)
	}
}

func (e *Environment) Keys() []string {
	var keys []string
	for k := range e.env {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	return keys
}

func (e *Environment) Append(otherEnv *Environment, callback func(name, value string)) {
	for _, name := range otherEnv.Keys() {
		value, _ := otherEnv.Get(name)
		e.Set(name, value)
		if callback != nil {
			callback(name, value)
		}
	}
}

func (e *Environment) ToSlice() []string {
	arr := []string{}
	for name, value := range e.env {
		arr = append(arr, fmt.Sprintf("%s=%s", name, value))
	}

	return arr
}

func (e *Environment) ToCommands() []string {
	commands := []string{}

	for _, name := range e.Keys() {
		value, _ := e.Get(name)
		commands = append(commands, fmt.Sprintf("export %s=%s\n", name, shellQuote(value)))
	}

	return commands
}

func (e *Environment) ToFile(fileName string, callback func(name string)) error {
	fileContent := ""
	for _, name := range e.Keys() {
		value, _ := e.Get(name)
		if runtime.GOOS == "windows" {
			fileContent += fmt.Sprintf("$env:%s = %q\n", name, escapePowershellQuotes(value))
		} else {
			fileContent += fmt.Sprintf("export %s=%s\n", name, shellQuote(value))
		}

		if callback != nil {
			callback(name)
		}
	}

	// #nosec
	err := ioutil.WriteFile(fileName, []byte(fileContent), 0644)
	if err != nil {
		return err
	}

	return nil
}

func escapePowershellQuotes(s string) string {
	return strings.Replace(s, "\"", "`\"", -1)
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
