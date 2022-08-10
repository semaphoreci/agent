package random

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test__RandomDuration(t *testing.T) {
	t.Run("min can't be zero or negative", func(t *testing.T) {
		duration, err := DurationInRange(-1, 0)
		assert.Nil(t, duration)
		assert.ErrorContains(t, err, "min cannot be less than or equal to zero")
	})

	t.Run("max cannot be below or equal to min", func(t *testing.T) {
		duration, err := DurationInRange(100, 50)
		assert.Nil(t, duration)
		assert.ErrorContains(t, err, "max cannot be greater than or equal to zero")
	})

	t.Run("duration is in range", func(t *testing.T) {
		duration, err := DurationInRange(50, 100)
		assert.Nil(t, err)
		assert.GreaterOrEqual(t, int(*duration/time.Millisecond), 50)
		assert.LessOrEqual(t, int(*duration/time.Millisecond), 100)
	})
}
