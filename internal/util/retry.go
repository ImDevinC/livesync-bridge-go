package util

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"
)

// RetryConfig configures retry behavior
type RetryConfig struct {
	MaxRetries     int           // Maximum number of retry attempts
	InitialBackoff time.Duration // Initial backoff duration
	MaxBackoff     time.Duration // Maximum backoff duration
	Multiplier     float64       // Backoff multiplier
}

// DefaultRetryConfig returns a sensible default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     30 * time.Second,
		Multiplier:     2.0,
	}
}

// QuickRetryConfig returns a configuration for quick retries
func QuickRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 50 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		Multiplier:     2.0,
	}
}

// RetryableFunc is a function that can be retried
type RetryableFunc func() error

// ShouldRetryFunc determines if an error should trigger a retry
type ShouldRetryFunc func(error) bool

// DefaultShouldRetry returns true for common transient errors
func DefaultShouldRetry(err error) bool {
	if err == nil {
		return false
	}
	// Add more sophisticated error checking here
	// For now, retry all errors (conservative approach)
	return true
}

// Retry executes a function with exponential backoff retry logic
func Retry(ctx context.Context, config RetryConfig, fn RetryableFunc, shouldRetry ShouldRetryFunc) error {
	if shouldRetry == nil {
		shouldRetry = DefaultShouldRetry
	}

	var lastErr error
	backoff := config.InitialBackoff

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Try the operation
		err := fn()
		if err == nil {
			// Success
			if attempt > 0 {
				slog.Debug("Retry succeeded", "attempt", attempt+1)
			}
			return nil
		}

		lastErr = err

		// Check if we should retry this error
		if !shouldRetry(err) {
			slog.Debug("Error not retryable", "error", err)
			return err
		}

		// Check if we've exhausted retries
		if attempt >= config.MaxRetries {
			slog.Debug("Max retries exhausted", "attempts", attempt+1, "error", err)
			break
		}

		// Log retry attempt
		slog.Debug("Operation failed, retrying",
			"attempt", attempt+1,
			"maxRetries", config.MaxRetries,
			"backoff", backoff,
			"error", err,
		)

		// Wait with exponential backoff
		select {
		case <-time.After(backoff):
			// Continue to next retry
		case <-ctx.Done():
			return ctx.Err()
		}

		// Calculate next backoff (exponential with cap)
		backoff = time.Duration(float64(backoff) * config.Multiplier)
		if backoff > config.MaxBackoff {
			backoff = config.MaxBackoff
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", config.MaxRetries+1, lastErr)
}

// RetryWithJitter executes a function with exponential backoff and jitter
func RetryWithJitter(ctx context.Context, config RetryConfig, fn RetryableFunc, shouldRetry ShouldRetryFunc) error {
	if shouldRetry == nil {
		shouldRetry = DefaultShouldRetry
	}

	var lastErr error
	backoff := config.InitialBackoff

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Try the operation
		err := fn()
		if err == nil {
			if attempt > 0 {
				slog.Debug("Retry succeeded", "attempt", attempt+1)
			}
			return nil
		}

		lastErr = err

		if !shouldRetry(err) {
			slog.Debug("Error not retryable", "error", err)
			return err
		}

		if attempt >= config.MaxRetries {
			slog.Debug("Max retries exhausted", "attempts", attempt+1, "error", err)
			break
		}

		// Add jitter (±25% randomization)
		jitter := time.Duration(float64(backoff) * (0.75 + 0.5*float64(time.Now().UnixNano()%100)/100.0))

		slog.Debug("Operation failed, retrying",
			"attempt", attempt+1,
			"maxRetries", config.MaxRetries,
			"backoff", jitter,
			"error", err,
		)

		// Wait with jittered backoff
		select {
		case <-time.After(jitter):
			// Continue to next retry
		case <-ctx.Done():
			return ctx.Err()
		}

		// Calculate next backoff
		backoff = time.Duration(float64(backoff) * config.Multiplier)
		if backoff > config.MaxBackoff {
			backoff = config.MaxBackoff
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", config.MaxRetries+1, lastErr)
}

// CalculateBackoff calculates the backoff duration for a given attempt
func CalculateBackoff(attempt int, config RetryConfig) time.Duration {
	backoff := float64(config.InitialBackoff) * math.Pow(config.Multiplier, float64(attempt))
	if backoff > float64(config.MaxBackoff) {
		backoff = float64(config.MaxBackoff)
	}
	return time.Duration(backoff)
}
