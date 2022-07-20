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
type JobResult string
type ShutdownReason string

const AgentStateWaitingForJobs = "waiting-for-jobs"
const AgentStateStartingJob = "starting-job"
const AgentStateRunningJob = "running-job"
const AgentStateStoppingJob = "stopping-job"
const AgentStateFinishedJob = "finished-job"

const AgentActionWaitForJobs = "wait-for-jobs"
const AgentActionRunJob = "run-job"
const AgentActionStopJob = "stop-job"
const AgentActionShutdown = "shutdown"
const AgentActionContinue = "continue"

const JobResultStopped = "stopped"
const JobResultFailed = "failed"
const JobResultPassed = "passed"

const ShutdownReasonIdle = "idle"
const ShutdownReasonJobFinished = "job_finished"
const ShutdownReasonRequested = "requested"

type SyncRequest struct {
	State     AgentState `json:"state"`
	JobID     string     `json:"job_id"`
	JobResult JobResult  `json:"job_result"`
}

type SyncResponse struct {
	Action         AgentAction    `json:"action"`
	JobID          string         `json:"job_id"`
	ShutdownReason ShutdownReason `json:"shutdown_reason"`
}

func (a *API) SyncPath() string {
	return a.BasePath() + "/sync"
}

func (a *API) Sync(req *SyncRequest) (*SyncResponse, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	a.logSyncRequest(req)
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

	a.logSyncResponse(response)
	return response, nil
}

func (a *API) logSyncRequest(req *SyncRequest) {
	switch req.State {
	case AgentStateWaitingForJobs:
		log.Infof("SYNC request (state: %s)", req.State)
	case AgentStateStoppingJob, AgentStateStartingJob, AgentStateRunningJob:
		log.Infof("SYNC request (state: %s, job: %s)", req.State, req.JobID)
	case AgentStateFinishedJob:
		log.Infof("SYNC request (state: %s, job: %s, result: %s)", req.State, req.JobID, req.JobResult)
	default:
		log.Infof("SYNC request: %v", req)
	}
}

func (a *API) logSyncResponse(response *SyncResponse) {
	switch response.Action {
	case AgentActionContinue, AgentActionWaitForJobs:
		log.Infof("SYNC response (action: %s)", response.Action)
	case AgentActionRunJob, AgentActionStopJob:
		log.Infof("SYNC response (action: %s, job: %s)", response.Action, response.JobID)
	case AgentActionShutdown:
		log.Infof("SYNC response (action: %s, reason: %s)", response.Action, response.ShutdownReason)
	default:
		log.Infof("SYNC response: %v", response)
	}
}
