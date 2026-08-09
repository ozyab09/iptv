package utils

import (
	"context"
	"math/rand/v2"
	"time"
)

type RetryableFunc func() error

func Retry(maxAttempts int, delay time.Duration, backoff float64, fn RetryableFunc) error {
	return RetryWithContext(context.Background(), maxAttempts, delay, backoff, fn)
}

// RetryWithContext calls fn until it succeeds, the attempts are exhausted, or
// ctx is cancelled. Waiting between attempts is interruptible by ctx.
func RetryWithContext(ctx context.Context, maxAttempts int, delay time.Duration, backoff float64, fn RetryableFunc) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	currentDelay := delay
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt < maxAttempts {
			wait := retryDelayWithJitter(currentDelay)
			NewSanitizedLoggerWithPrefix("[retry]").Warning("Attempt %d/%d failed: %v. Retrying in %.1fs...", attempt, maxAttempts, err, wait.Seconds())
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return ctx.Err()
			case <-timer.C:
			}
			currentDelay = time.Duration(float64(currentDelay) * backoff)
		} else {
			NewSanitizedLoggerWithPrefix("[retry]").Error("All %d attempts failed: %v", maxAttempts, err)
		}
	}
	return lastErr
}

// retryDelayWithJitter applies bounded ±10% jitter to reduce synchronized
// retry bursts when multiple runs hit the same source at once.
func retryDelayWithJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	spread := delay / 10
	if spread == 0 {
		return delay
	}
	return delay - spread + time.Duration(rand.Int64N(int64(2*spread)+1))
}
