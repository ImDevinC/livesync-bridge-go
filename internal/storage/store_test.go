package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Fatal("Store is nil")
	}

	// Check file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}
}

func TestStoreSetGet(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Set a value
	err = store.Set("key1", "value1")
	if err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// Get the value
	value, err := store.Get("key1")
	if err != nil {
		t.Fatalf("Failed to get value: %v", err)
	}
	if value != "value1" {
		t.Errorf("Expected 'value1', got '%s'", value)
	}

	// Get non-existent key
	_, err = store.Get("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent key")
	}
}

func TestStoreHas(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	store.Set("key1", "value1")

	if !store.Has("key1") {
		t.Error("Expected key1 to exist")
	}
	if store.Has("nonexistent") {
		t.Error("Expected nonexistent key to not exist")
	}
}

func TestStoreDelete(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	store.Set("key1", "value1")

	if !store.Has("key1") {
		t.Error("Expected key1 to exist")
	}

	err = store.Delete("key1")
	if err != nil {
		t.Fatalf("Failed to delete key: %v", err)
	}

	if store.Has("key1") {
		t.Error("Expected key1 to be deleted")
	}
}

func TestStoreClear(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	store.Set("key1", "value1")
	store.Set("key2", "value2")

	if !store.Has("key1") || !store.Has("key2") {
		t.Error("Expected keys to exist")
	}

	err = store.Clear()
	if err != nil {
		t.Fatalf("Failed to clear store: %v", err)
	}

	if store.Has("key1") || store.Has("key2") {
		t.Error("Expected store to be empty after clear")
	}
}

func TestStoreKeys(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	store.Set("key1", "value1")
	store.Set("key2", "value2")
	store.Set("key3", "value3")

	keys, err := store.Keys()
	if err != nil {
		t.Fatalf("Failed to get keys: %v", err)
	}

	if len(keys) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(keys))
	}

	// Check all keys exist in result
	keyMap := make(map[string]bool)
	for _, k := range keys {
		keyMap[k] = true
	}

	if !keyMap["key1"] || !keyMap["key2"] || !keyMap["key3"] {
		t.Error("Not all keys returned")
	}
}

func TestStoreGetWithDefault(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	store.Set("key1", "value1")

	// Existing key
	value := store.GetWithDefault("key1", "default")
	if value != "value1" {
		t.Errorf("Expected 'value1', got '%s'", value)
	}

	// Non-existent key
	value = store.GetWithDefault("nonexistent", "default")
	if value != "default" {
		t.Errorf("Expected 'default', got '%s'", value)
	}
}

func TestStorePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create store and set value
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	store.Set("key1", "value1")
	store.Close()

	// Reopen and check value persists
	store, err = NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to reopen store: %v", err)
	}
	defer store.Close()

	value, err := store.Get("key1")
	if err != nil {
		t.Fatalf("Failed to get value after reopen: %v", err)
	}
	if value != "value1" {
		t.Errorf("Expected 'value1', got '%s'", value)
	}
}

func TestStoreIteratePrefix(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Set up test data
	store.Set("file-stat-a.txt", "100-200")
	store.Set("file-stat-b.txt", "300-400")
	store.Set("file-stat-c.txt", "500-600")
	store.Set("other-key", "other-value")
	store.Set("file-data-x.txt", "data")

	// Test iterating with "file-stat-" prefix
	collected := make(map[string]string)
	err = store.IteratePrefix("file-stat-", func(key, value string) error {
		collected[key] = value
		return nil
	})
	if err != nil {
		t.Fatalf("IteratePrefix failed: %v", err)
	}

	// Should only get keys with file-stat- prefix
	if len(collected) != 3 {
		t.Errorf("Expected 3 keys with prefix, got %d", len(collected))
	}

	if collected["file-stat-a.txt"] != "100-200" {
		t.Errorf("Expected '100-200', got '%s'", collected["file-stat-a.txt"])
	}
	if collected["file-stat-b.txt"] != "300-400" {
		t.Errorf("Expected '300-400', got '%s'", collected["file-stat-b.txt"])
	}
	if collected["file-stat-c.txt"] != "500-600" {
		t.Errorf("Expected '500-600', got '%s'", collected["file-stat-c.txt"])
	}

	// Should not include other keys
	if _, exists := collected["other-key"]; exists {
		t.Error("Should not include 'other-key'")
	}
	if _, exists := collected["file-data-x.txt"]; exists {
		t.Error("Should not include 'file-data-x.txt'")
	}
}

func TestStoreIteratePrefixEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// No keys with the prefix
	store.Set("other-key", "value")

	count := 0
	err = store.IteratePrefix("file-stat-", func(key, value string) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("IteratePrefix failed: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected 0 iterations, got %d", count)
	}
}
