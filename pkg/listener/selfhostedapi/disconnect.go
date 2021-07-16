package selfhostedapi

import (
	"io/ioutil"
	"net/http"
)

func (a *Api) DisconnectPath() string {
	return a.BasePath() + "/disconnect"
}

func (a *Api) Disconnect() (string, error) {
	r, err := http.NewRequest("POST", a.DisconnectPath(), nil)
	if err != nil {
		return "", err
	}

	a.authorize(r, a.AccessToken)

	resp, err := a.client.Do(r)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
