package configutil

import (
	"fmt"
	"strings"
	"time"
)

// PositiveDuration parses a duration string that must be greater than zero.
func PositiveDuration(name string, value string) (time.Duration, error) {
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("%s is invalid: %w", name, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return duration, nil
}

// MustDuration parses a duration that has already passed validation.
func MustDuration(value string) time.Duration {
	duration, _ := time.ParseDuration(strings.TrimSpace(value))
	return duration
}
