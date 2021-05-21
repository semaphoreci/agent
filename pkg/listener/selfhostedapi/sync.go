package selfhostedapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

type AgentState string
type AgentAction string

const AgentStateWaitingForJobs = "waiting-for-jobs"
const AgentStateRunningJob = "running-job"
const AgentStateStoppingJob = "stopping-job"
const AgentStateFinishedJob = "finished-job"

const AgentActionWaitForJobs = "wait-for-jobs"
const AgentActionRunJob = "run-job"
const AgentActionStopJob = "stop-job"
const AgentActionShutdown = "shutdown"
const AgentActionContinue = "continue"

type SyncRequest struct {
	State AgentState `json:"state"`
	JobID string     `json:"job_id"`
}

type SyncResponse struct {
	Action AgentAction `json:"action"`
	JobID  string      `json:"job_id"`
}

func (a *Api) SyncPath() string {
	return fmt.Sprintf("%s://%s/api/v1/self_hosted_agents/sync", a.Scheme, a.Endpoint)
}

func (a *Api) Sync(req *SyncRequest) (*SyncResponse, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	r, err := http.NewRequest("POST", a.SyncPath(), bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}

	a.authorize(r, a.AccessToken)

	resp, err := a.client.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	response := &SyncResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		return nil, err
	}

	return response, nil
}
