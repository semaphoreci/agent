package selfhostedapi

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	log "github.com/sirupsen/logrus"
)

type RefreshTokenResponse struct {
	Token string `json:"token"`
}

func (a *API) RefreshTokenPath() string {
	return a.BasePath() + "/refresh"
}

func (a *API) RefreshToken() (string, error) {
	r, err := http.NewRequest("POST", a.RefreshTokenPath(), nil)
	if err != nil {
		return "", err
	}

	log.Info("Refreshing token for current job logs...")

	a.authorize(r, a.AccessToken)

	resp, err := a.client.Do(r)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to refresh token, got HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	response := &RefreshTokenResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		return "", err
	}

	log.Infof("Successfully refreshed token for current job logs.")
	return response.Token, nil
}
