package util

import (
	"testing"
)

func TestNewCache(t *testing.T) {
	cache, err := NewCache(10)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	if cache == nil {
		t.Fatal("Cache is nil")
	}
}

func TestCacheSetGet(t *testing.T) {
	cache, err := NewCache(10)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Set a value
	cache.Set("key1", "value1")

	// Get the value
	value, ok := cache.Get("key1")
	if !ok {
		t.Error("Key not found in cache")
	}
	if value != "value1" {
		t.Errorf("Expected 'value1', got '%s'", value)
	}

	// Get non-existent key
	_, ok = cache.Get("nonexistent")
	if ok {
		t.Error("Expected key to not exist")
	}
}

func TestCacheHas(t *testing.T) {
	cache, err := NewCache(10)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	cache.Set("key1", "value1")

	if !cache.Has("key1") {
		t.Error("Expected key1 to exist")
	}
	if cache.Has("nonexistent") {
		t.Error("Expected nonexistent key to not exist")
	}
}

func TestCacheLRU(t *testing.T) {
	// Create cache with size 3
	cache, err := NewCache(3)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add 4 items (should evict the oldest)
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")
	cache.Set("key4", "value4")

	// key1 should be evicted
	if cache.Has("key1") {
		t.Error("Expected key1 to be evicted")
	}

	// Others should still exist
	if !cache.Has("key2") || !cache.Has("key3") || !cache.Has("key4") {
		t.Error("Expected key2, key3, key4 to exist")
	}

	// Check length
	if cache.Len() != 3 {
		t.Errorf("Expected cache length 3, got %d", cache.Len())
	}
}

func TestCacheClear(t *testing.T) {
	cache, err := NewCache(10)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	if cache.Len() != 2 {
		t.Errorf("Expected cache length 2, got %d", cache.Len())
	}

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("Expected cache length 0 after clear, got %d", cache.Len())
	}
	if cache.Has("key1") {
		t.Error("Expected cache to be empty after clear")
	}
}
