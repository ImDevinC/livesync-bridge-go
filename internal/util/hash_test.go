package util

import (
	"testing"
	"time"
)

func TestComputeHash(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "empty",
			input:    []byte{},
			expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:     "hello world",
			input:    []byte("hello world"),
			expected: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeHash(tt.input)
			if result != tt.expected {
				t.Errorf("ComputeHash() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestMakeUniqueString(t *testing.T) {
	// Test that it generates non-empty strings
	s1 := MakeUniqueString()
	if s1 == "" {
		t.Error("MakeUniqueString() returned empty string")
	}

	// Test that it generates unique strings over multiple calls
	// Use a map to track uniqueness over 100 iterations
	seen := make(map[string]bool)
	duplicates := 0
	iterations := 100

	for i := 0; i < iterations; i++ {
		s := MakeUniqueString()
		if seen[s] {
			duplicates++
		}
		seen[s] = true
		// Small delay to ensure different timestamps on systems with low timer resolution
		if i < iterations-1 {
			time.Sleep(time.Microsecond)
		}
	}

	// Allow at most 1 duplicate (extremely unlikely with proper implementation)
	if duplicates > 1 {
		t.Errorf("MakeUniqueString() generated %d duplicates in %d iterations", duplicates, iterations)
	}

	// Test format (timestamp-randomstring)
	if len(s1) < 10 {
		t.Errorf("MakeUniqueString() generated string too short: %s", s1)
	}
}
