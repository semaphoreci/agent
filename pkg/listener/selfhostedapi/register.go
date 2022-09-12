package selfhostedapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	httputils "github.com/semaphoreci/agent/pkg/httputils"
	log "github.com/sirupsen/logrus"
)

type RegisterRequest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	PID         int    `json:"pid"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	Hostname    string `json:"hostname"`
	SingleJob   bool   `json:"single_job"`
	IdleTimeout int    `json:"idle_timeout"`
}

type RegisterResponse struct {
	Name  string `json:"name"`
	Token string `json:"token"`
}

func (a *API) RegisterPath() string {
	return a.BasePath() + "/register"
}

func (a *API) Register(req *RegisterRequest) (*RegisterResponse, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	r, err := http.NewRequest("POST", a.RegisterPath(), bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}

	a.authorize(r, a.RegisterToken)

	resp, err := a.client.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if !httputils.IsSuccessfulCode(resp.StatusCode) {
		return nil, fmt.Errorf("register request to %s got HTTP %d: %s", a.RegisterPath(), resp.StatusCode, body)
	}

	response := &RegisterResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		log.Debug(string(body))
		return nil, err
	}

	return response, nil
}
