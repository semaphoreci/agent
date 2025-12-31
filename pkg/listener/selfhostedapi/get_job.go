package selfhostedapi

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	api "github.com/semaphoreci/agent/pkg/api"
)

func (a *API) GetJobPath(jobID string) string {
	return a.BasePath() + fmt.Sprintf("/jobs/%s", jobID)
}

func (a *API) GetJob(jobID string) (*api.JobRequest, error) {
	r, err := http.NewRequest("GET", a.GetJobPath(jobID), nil)
	if err != nil {
		return nil, err
	}

	a.authorize(r, a.AccessToken)
	r.Header.Set("User-Agent", a.UserAgent)

	resp, err := a.client.Do(r)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to describe job, got HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	response := &api.JobRequest{}
	if err := json.Unmarshal(body, response); err != nil {
		return nil, err
	}

	return response, nil
}
