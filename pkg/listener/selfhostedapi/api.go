package selfhostedapi

import "net/http"

type Api struct {
	Endpoint string
	Scheme   string

	RegisterToken string
	AccessToken   string

	client *http.Client
}

func New(scheme string, endpoint string, token string) *Api {
	return &Api{
		Endpoint:      endpoint,
		RegisterToken: token,
		Scheme:        scheme,
		client:        &http.Client{},
	}
}

func (a *Api) authorize(req *http.Request, token string) {
	req.Header.Set("Authorization", "Token "+a.RegisterToken)
}

func (a *Api) SetAccessToken(token string) {
	a.AccessToken = token
}
