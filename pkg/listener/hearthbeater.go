package listener

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
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
		log.Errorf("Heartbeat failed: %v", err)
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Heartbeat failed: %v", err)
		return
	}

	log.Debug(string(body))
}
