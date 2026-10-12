package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test__NoRetriesIfFirstAttemptIsSuccessful(t *testing.T) {
	attempts := 0
	err := RetryWithConstantWait(RetryOptions{
		Task:                 "test",
		MaxAttempts:          5,
		DelayBetweenAttempts: 100 * time.Millisecond,
		Fn: func() error {
			attempts++
			return nil
		},
	})

	assert.Equal(t, attempts, 1)
	assert.Nil(t, err)
}

func Test__GivesUpAfterMaxRetries(t *testing.T) {
	attempts := 0
	err := RetryWithConstantWait(RetryOptions{
		Task:                 "test",
		MaxAttempts:          5,
		DelayBetweenAttempts: 100 * time.Millisecond,
		Fn: func() error {
			attempts++
			return errors.New("bad error")
		},
	})

	assert.Equal(t, attempts, 5)
	assert.NotNil(t, err)
}

func Test__GivesUpIfContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())

	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	err := RetryWithConstantWaitAndContext(ctx, RetryOptions{
		Task:                 "test",
		MaxAttempts:          10,
		DelayBetweenAttempts: 100 * time.Millisecond,
		Fn: func() error {
			return errors.New("bad error")
		},
	})

	assert.ErrorContains(t, err, "context canceled")
}

func Test__StopsWaitingBetweenAttemptsIfContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	started := time.Now()
	err := RetryWithConstantWaitAndContext(ctx, RetryOptions{
		Task:                 "test",
		MaxAttempts:          2,
		DelayBetweenAttempts: time.Minute,
		Fn: func() error {
			return errors.New("bad error")
		},
	})

	assert.ErrorContains(t, err, "context canceled")
	assert.Less(t, time.Since(started), 5*time.Second)
}

func Test__WaitsBetweenAttemptsIfContextIsNotCancelled(t *testing.T) {
	attempts := 0
	started := time.Now()
	err := RetryWithConstantWaitAndContext(context.TODO(), RetryOptions{
		Task:                 "test",
		MaxAttempts:          3,
		DelayBetweenAttempts: 100 * time.Millisecond,
		Fn: func() error {
			attempts++
			if attempts < 3 {
				return errors.New("bad error")
			}

			return nil
		},
	})

	assert.NoError(t, err)
	assert.Equal(t, 3, attempts)
	assert.GreaterOrEqual(t, time.Since(started), 200*time.Millisecond)
}
