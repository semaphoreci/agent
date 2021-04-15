package listener

func StartHeartBeater() (*HearthBeater, error) {
	h := &HearthBeater{}

	go h.Start()

	return h, nil
}

type HearthBeater struct {
}

func (h *HearthBeater) Start() {
}
