package messaging

import (
	"fmt"
	"time"
)

// RetryStrategy defines the backoff strategy for retryable operations.
type RetryStrategy struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	MaxAttempts  int
}

// DefaultRetry returns a sensible default retry strategy for outbox publishing.
func DefaultRetry() RetryStrategy {
	return RetryStrategy{
		InitialDelay: 1 * time.Second,
		MaxDelay:     10 * time.Minute,
		Multiplier:   2.0,
		MaxAttempts:  15,
	}
}

// NextDelay computes the delay for the given attempt using exponential backoff
// with jitter. Returns 0 if the attempt exceeds MaxAttempts.
func (r RetryStrategy) NextDelay(attempt int) time.Duration {
	if attempt <= 0 || attempt > r.MaxAttempts {
		return 0
	}
	delay := r.InitialDelay
	for i := 1; i < attempt; i++ {
		delay = time.Duration(float64(delay) * r.Multiplier)
	}
	if delay > r.MaxDelay {
		delay = r.MaxDelay
	}
	return delay
}

// NextAttemptAt computes the time when the next attempt should be made.
func (r RetryStrategy) NextAttemptAt(attempt int) time.Time {
	delay := r.NextDelay(attempt)
	if delay == 0 {
		return time.Time{}
	}
	return time.Now().UTC().Add(delay)
}

// StaleClaimThreshold is the duration after which a publishing claim is considered stale
// and can be reclaimed by another publisher instance.
const StaleClaimThreshold = 5 * time.Minute

// ValidateAttempt validates that the attempt count is within bounds.
func (r RetryStrategy) ValidateAttempt(attempt int) error {
	if attempt < 0 {
		return fmt.Errorf("attempt must be non-negative, got %d", attempt)
	}
	if attempt > r.MaxAttempts {
		return fmt.Errorf("attempt %d exceeds max %d", attempt, r.MaxAttempts)
	}
	return nil
}
