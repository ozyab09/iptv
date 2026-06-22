package utils

import (
	"log"
	"time"
)

type RetryableFunc func() error

func Retry(maxAttempts int, delay time.Duration, backoff float64, fn RetryableFunc) error {
	currentDelay := delay
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt < maxAttempts {
			log.Printf("Attempt %d/%d failed: %v. Retrying in %.1fs...", attempt, maxAttempts, err, currentDelay.Seconds())
			time.Sleep(currentDelay)
			currentDelay = time.Duration(float64(currentDelay) * backoff)
		} else {
			log.Printf("All %d attempts failed: %v", maxAttempts, err)
		}
	}
	return lastErr
}
