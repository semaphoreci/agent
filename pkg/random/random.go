package random

import (
	"fmt"
	"math/rand"
	"time"
)

func DurationInRange(minMillis, maxMillis int) (*time.Duration, error) {
	if minMillis <= 0 {
		return nil, fmt.Errorf("min cannot be less than or equal to zero")
	}

	if minMillis >= maxMillis {
		return nil, fmt.Errorf("max cannot be greater than or equal to zero")
	}

	interval := rand.Intn(maxMillis-minMillis) + minMillis
	duration := time.Duration(interval) * time.Millisecond
	return &duration, nil
}
