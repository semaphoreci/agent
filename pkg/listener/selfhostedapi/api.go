package selfhostedapi

import "net/http"

type Api struct {
	Endpoint string
	Token    string
	Scheme   string

	client *http.Client
}

func New(scheme string, endpoint string, token string) *Api {
	return &Api{
		Endpoint: endpoint,
		Token: token,
		Scheme: scheme,
		client: &http.Client{},
	}
}

func (a *Api) authorize(req *http.Request, token string) {
	req.Header.Set("Authorization", "Token " + a.Token)
}
