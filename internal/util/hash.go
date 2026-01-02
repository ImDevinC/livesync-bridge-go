package util

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"time"
)

// ComputeHash calculates SHA-256 hash of data and returns hex string
func ComputeHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// ComputeHashString is a convenience function for string data
func ComputeHashString(data string) string {
	return ComputeHash([]byte(data))
}

// MakeUniqueString generates a unique string for IDs
func MakeUniqueString() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const length = 30

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rng.Intn(len(charset))]
	}

	return fmt.Sprintf("%d-%s", time.Now().UnixMilli(), string(b))
}
