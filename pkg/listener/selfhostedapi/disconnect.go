package selfhostedapi

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

func (a *API) DisconnectPath() string {
	return a.BasePath() + "/disconnect"
}

func (a *API) Disconnect() (string, error) {
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

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("error while disconnecting, status: %d, body: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}
