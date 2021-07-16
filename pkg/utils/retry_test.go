package utils

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test__NoRetriesIfFirstAttemptIsSuccessful(t *testing.T) {
	attempts := 0
	err := RetryWithConstantWait("test", 5, 100*time.Millisecond, func() error {
		attempts++
		return nil
	})
	assert.Equal(t, attempts, 1)
	assert.Nil(t, err)
}

func Test__GivesUpAfterMaxRetries(t *testing.T) {
	attempts := 0
	err := RetryWithConstantWait("test", 5, 100*time.Millisecond, func() error {
		attempts++
		return errors.New("bad error")
	})
	assert.Equal(t, attempts, 5)
	assert.NotNil(t, err)
}
