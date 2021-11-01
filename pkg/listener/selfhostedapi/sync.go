package selfhostedapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	log "github.com/sirupsen/logrus"
)

type AgentState string
type AgentAction string

const AgentStateWaitingForJobs = "waiting-for-jobs"
const AgentStateStartingJob = "starting-job"
const AgentStateRunningJob = "running-job"
const AgentStateStoppingJob = "stopping-job"
const AgentStateFinishedJob = "finished-job"
const AgentStateFailedToFetchJob = "failed-to-fetch-job"
const AgentStateFailedToConstructJob = "failed-to-construct-job"
const AgentStateFailedToSendCallback = "failed-to-send-callback"

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

func (a *API) SyncPath() string {
	return a.BasePath() + "/sync"
}

func (a *API) Sync(req *SyncRequest) (*SyncResponse, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	log.Infof("SYNC request (state: %s, job: %s)", req.State, req.JobID)

	r, err := http.NewRequest("POST", a.SyncPath(), bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}

	a.authorize(r, a.AccessToken)

	resp, err := a.client.Do(r)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to sync with upstream, got HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	response := &SyncResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		return nil, err
	}

	log.Infof("SYNC response (action: %s, job: %s)", response.Action, response.JobID)

	return response, nil
}
