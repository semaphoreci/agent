package retry

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
)

type RetryOptions struct {
	Task                 string
	MaxAttempts          int
	DelayBetweenAttempts time.Duration
	Fn                   func() error
	HideError            bool
}

func RetryWithConstantWait(options RetryOptions) error {
	return RetryWithConstantWaitAndContext(context.TODO(), options)
}

func RetryWithConstantWaitAndContext(ctx context.Context, options RetryOptions) error {
	if options.Fn == nil {
		return fmt.Errorf("options.Fn cannot be nil")
	}

	for attempt := 1; ; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := options.Fn()
		if err == nil {
			return nil
		}

		if attempt >= options.MaxAttempts {
			return fmt.Errorf("[%s] failed after [%d] attempts - giving up: %v", options.Task, attempt, err)
		}

		if !options.HideError {
			log.Errorf(
				"[%s] attempt [%d] failed with [%v] - retrying in %s",
				options.Task,
				attempt,
				err,
				options.DelayBetweenAttempts,
			)
		}

		time.Sleep(options.DelayBetweenAttempts)
	}
}
