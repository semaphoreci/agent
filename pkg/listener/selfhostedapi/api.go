package selfhostedapi

import (
	"fmt"
	"net/http"
)

type API struct {
	Endpoint string
	Scheme   string

	RegisterToken string
	AccessToken   string

	client *http.Client
}

func New(httpClient *http.Client, scheme string, endpoint string, token string) *API {
	return &API{
		Endpoint:      endpoint,
		RegisterToken: token,
		Scheme:        scheme,
		client:        httpClient,
	}
}

func (a *API) authorize(req *http.Request, token string) {
	req.Header.Set("Authorization", "Token "+token)
}

func (a *API) SetAccessToken(token string) {
	a.AccessToken = token
}

func (a *API) BasePath() string {
	return fmt.Sprintf("%s://%s/api/v1/self_hosted_agents", a.Scheme, a.Endpoint)
}
