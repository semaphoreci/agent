package listener

func StartJobProcessor() (*JobProcessor, error) {
	p := &JobProcessor{}

	go p.Start()

	return p, nil
}

type JobProcessor struct {
}

func (p *JobProcessor) Start() {
}
