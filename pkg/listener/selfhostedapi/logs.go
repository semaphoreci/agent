package selfhostedapi

import (
	"bytes"
	"fmt"
	"net/http"
)

func (a *API) LogsPath(jobID string) string {
	return a.BasePath() + fmt.Sprintf("/jobs/%s/logs", jobID)
}

func (a *API) Logs(jobID string, batch *bytes.Buffer) error {
	r, err := http.NewRequest("POST", a.LogsPath(jobID), batch)
	if err != nil {
		return err
	}

	a.authorize(r, a.AccessToken)

	resp, err := a.client.Do(r)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to submit logs, got HTTP %d", resp.StatusCode)
	}

	return nil
}
