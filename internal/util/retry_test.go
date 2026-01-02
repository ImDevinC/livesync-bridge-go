package util

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetrySuccess(t *testing.T) {
	config := QuickRetryConfig()
	attempts := 0

	err := Retry(context.Background(), config, func() error {
		attempts++
		return nil // Success on first try
	}, nil)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", attempts)
	}
}

func TestRetryEventualSuccess(t *testing.T) {
	config := QuickRetryConfig()
	attempts := 0

	err := Retry(context.Background(), config, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil // Success on third try
	}, nil)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRetryMaxRetriesExhausted(t *testing.T) {
	config := RetryConfig{
		MaxRetries:     2,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}
	attempts := 0

	err := Retry(context.Background(), config, func() error {
		attempts++
		return errors.New("persistent error")
	}, nil)

	if err == nil {
		t.Error("Expected error, got nil")
	}
	if attempts != 3 { // Initial + 2 retries
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRetryNonRetryableError(t *testing.T) {
	config := QuickRetryConfig()
	attempts := 0
	permanentError := errors.New("permanent error")

	err := Retry(context.Background(), config, func() error {
		attempts++
		return permanentError
	}, func(err error) bool {
		// Don't retry permanent errors
		return err != permanentError
	})

	if err == nil {
		t.Error("Expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt (no retries), got %d", attempts)
	}
}

func TestRetryContextCancellation(t *testing.T) {
	config := RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Multiplier:     2.0,
	}
	attempts := 0

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after 2 attempts
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	err := Retry(ctx, config, func() error {
		attempts++
		return errors.New("error")
	}, nil)

	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
	// Should have attempted at least once, but stopped due to cancellation
	if attempts == 0 {
		t.Error("Expected at least one attempt")
	}
}

func TestRetryWithJitter(t *testing.T) {
	config := QuickRetryConfig()
	attempts := 0

	err := RetryWithJitter(context.Background(), config, func() error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		return nil
	}, nil)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestCalculateBackoff(t *testing.T) {
	config := RetryConfig{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Multiplier:     2.0,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 100 * time.Millisecond},
		{1, 200 * time.Millisecond},
		{2, 400 * time.Millisecond},
		{3, 800 * time.Millisecond},
		{4, 1 * time.Second}, // Capped at MaxBackoff
		{5, 1 * time.Second}, // Still capped
	}

	for _, tt := range tests {
		result := CalculateBackoff(tt.attempt, config)
		if result != tt.expected {
			t.Errorf("CalculateBackoff(%d) = %v, expected %v", tt.attempt, result, tt.expected)
		}
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxRetries <= 0 {
		t.Error("MaxRetries should be positive")
	}
	if config.InitialBackoff <= 0 {
		t.Error("InitialBackoff should be positive")
	}
	if config.MaxBackoff <= config.InitialBackoff {
		t.Error("MaxBackoff should be greater than InitialBackoff")
	}
	if config.Multiplier <= 1.0 {
		t.Error("Multiplier should be greater than 1.0")
	}
}
