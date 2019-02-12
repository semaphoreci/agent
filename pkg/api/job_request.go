package api

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	yaml "gopkg.in/yaml.v2"
)

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
	Started          string `json:"started" yaml:"started"`
	Finished         string `json:"finished" yaml:"finished"`
	TeardownFinished string `json:"teardown_finished" yaml:"teardown_finished"`
}

type JobRequest struct {
	Commands         []Command `json:"commands" yaml:"commands"`
	EpilogueCommands []Command `json:"epilogue_commands" yaml:"epilogue_commands"`
	EnvVars          []EnvVar  `json:"env_vars" yaml:"env_vars"`
	Files            []File    `json:"files" yaml:"file"`
	Callbacks        Callbacks `json:"callbacks" yaml:"callbacks"`
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
