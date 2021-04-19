package listener

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

func StartHeartBeater(endpoint string) (*HearthBeater, error) {
	h := &HearthBeater{
		Endpoint: endpoint,
	}

	go h.Start()

	return h, nil
}

type HearthBeater struct {
	Endpoint string
	Ticker   *time.Ticker
}

func (h *HearthBeater) Start() {
	h.Ticker = time.NewTicker(5 * time.Second)

	go func() {
		for {
			select {
			case <-h.Ticker.C:
				h.Pulse()
			}
		}
	}()
}

func (h *HearthBeater) Pulse() {
	resp, err := http.Post(h.Endpoint, "application/json", bytes.NewBuffer([]byte("{}")))
	if err != nil {
		fmt.Println("Hearthbear failed")
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Hearthbear failed")
		return
	}

	fmt.Println(string(body))
}
