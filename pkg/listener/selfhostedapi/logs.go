package selfhostedapi

import (
	"bytes"
	"fmt"
	"net/http"
)

func (a *Api) LogsPath() string {
	return fmt.Sprintf("%s://%s/api/v1/self_hosted_agents/logs", a.Scheme, a.Endpoint)
}

func (a *Api) Logs(batch *bytes.Buffer) error {
	r, err := http.NewRequest("POST", a.LogsPath(), batch)
	if err != nil {
		return err
	}

	a.authorize(r, a.AccessToken)

	resp, err := a.client.Do(r)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("failed to submit logs")
	}

	return nil
}
