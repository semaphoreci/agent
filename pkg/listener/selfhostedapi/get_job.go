package selfhostedapi

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/semaphoreci/agent/pkg/api"
)

func (a *Api) GetJobPath(jobID string) string {
	return a.BasePath() + fmt.Sprintf("/jobs/%s", jobID)
}

func (a *Api) GetJob(jobID string) (*api.JobRequest, error) {
	r, err := http.NewRequest("GET", a.GetJobPath(jobID), nil)
	if err != nil {
		return nil, err
	}

	a.authorize(r, a.AccessToken)

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