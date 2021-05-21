package selfhostedapi

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
)

type RegisterRequest struct {
	Name string `json:"name"`
	OS   string `json:"os"`
}

type RegisterResponse struct {
	AccessToken string `json:"access_token"`
}

func (a *Api) RegisterPath() string {
	return a.BasePath() + "/register"
}

func (a *Api) Register(req *RegisterRequest) (*RegisterResponse, error) {
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

	response := &RegisterResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		log.Println(string(body))
		return nil, err
	}

	return response, nil
}
