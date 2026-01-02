package peer

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/imdevinc/livesync-bridge/internal/storage"
)

func createTestStore(t *testing.T) *storage.Store {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test store: %v", err)
	}
	return store
}

func TestNewBasePeer(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	dispatcher := func(source Peer, path string, data *FileData) error {
		return nil
	}

	peer, err := NewBasePeer("test-peer", "storage", "main", "shared/", dispatcher, store)
	if err != nil {
		t.Fatalf("Failed to create base peer: %v", err)
	}

	if peer.Name() != "test-peer" {
		t.Errorf("Expected name 'test-peer', got '%s'", peer.Name())
	}
	if peer.Type() != "storage" {
		t.Errorf("Expected type 'storage', got '%s'", peer.Type())
	}
	if peer.Group() != "main" {
		t.Errorf("Expected group 'main', got '%s'", peer.Group())
	}
}

func TestBasePeerPathConversion(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	peer, _ := NewBasePeer("test", "storage", "main", "shared/", nil, store)

	// Test ToLocalPath
	local := peer.ToLocalPath("document.md")
	if local != "shared/document.md" {
		t.Errorf("ToLocalPath failed: got '%s'", local)
	}

	// Test ToGlobalPath
	global := peer.ToGlobalPath("shared/document.md")
	if global != "document.md" {
		t.Errorf("ToGlobalPath failed: got '%s'", global)
	}
}

func TestBasePeerIsRepeating(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	peer, _ := NewBasePeer("test", "storage", "main", "shared/", nil, store)

	data := &FileData{
		CTime: time.Now(),
		MTime: time.Now(),
		Size:  100,
		Data:  []byte("test content"),
	}

	// First time should not be repeating
	if peer.IsRepeating("doc.md", data) {
		t.Error("First occurrence should not be repeating")
	}

	// Mark as processed
	peer.MarkProcessed("doc.md", data)

	// Now should be repeating
	if !peer.IsRepeating("doc.md", data) {
		t.Error("Should be marked as repeating after processing")
	}

	// Different content should not be repeating
	differentData := &FileData{
		CTime: time.Now(),
		MTime: time.Now(),
		Size:  100,
		Data:  []byte("different content"),
	}
	if peer.IsRepeating("doc.md", differentData) {
		t.Error("Different content should not be repeating")
	}
}

func TestBasePeerDeletionMarker(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	peer, _ := NewBasePeer("test", "storage", "main", "shared/", nil, store)

	// Test deletion marker (nil data)
	if peer.IsRepeating("doc.md", nil) {
		t.Error("First deletion should not be repeating")
	}

	peer.MarkProcessed("doc.md", nil)

	if !peer.IsRepeating("doc.md", nil) {
		t.Error("Deletion should be marked as repeating")
	}
}

func TestBasePeerSettings(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	peer, _ := NewBasePeer("test", "storage", "main", "shared/", nil, store)

	// Set a setting
	err := peer.SetSetting("since", "12345")
	if err != nil {
		t.Fatalf("Failed to set setting: %v", err)
	}

	// Get the setting
	value, err := peer.GetSetting("since")
	if err != nil {
		t.Fatalf("Failed to get setting: %v", err)
	}
	if value != "12345" {
		t.Errorf("Expected '12345', got '%s'", value)
	}

	// Check HasSetting
	if !peer.HasSetting("since") {
		t.Error("Setting should exist")
	}
	if peer.HasSetting("nonexistent") {
		t.Error("Nonexistent setting should not exist")
	}

	// GetWithDefault for existing
	value = peer.GetSettingWithDefault("since", "default")
	if value != "12345" {
		t.Errorf("Expected '12345', got '%s'", value)
	}

	// GetWithDefault for nonexistent
	value = peer.GetSettingWithDefault("nonexistent", "default")
	if value != "default" {
		t.Errorf("Expected 'default', got '%s'", value)
	}
}

func TestBasePeerSettingsNamespacing(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	peer1, _ := NewBasePeer("peer1", "storage", "main", "shared/", nil, store)
	peer2, _ := NewBasePeer("peer2", "couchdb", "main", "docs/", nil, store)

	// Set same key for both peers
	peer1.SetSetting("since", "value1")
	peer2.SetSetting("since", "value2")

	// Values should be independent
	value1, _ := peer1.GetSetting("since")
	value2, _ := peer2.GetSetting("since")

	if value1 != "value1" {
		t.Errorf("Peer1 expected 'value1', got '%s'", value1)
	}
	if value2 != "value2" {
		t.Errorf("Peer2 expected 'value2', got '%s'", value2)
	}
}

func TestBasePeerDispatch(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	var dispatchedPath string
	var dispatchedData *FileData

	dispatcher := func(source Peer, path string, data *FileData) error {
		dispatchedPath = path
		dispatchedData = data
		return nil
	}

	peer, _ := NewBasePeer("test", "storage", "main", "shared/", dispatcher, store)

	data := &FileData{
		CTime: time.Now(),
		MTime: time.Now(),
		Size:  100,
		Data:  []byte("test content"),
	}

	// Dispatch
	err := peer.Dispatch("shared/document.md", data)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// Check dispatcher was called with global path
	if dispatchedPath != "document.md" {
		t.Errorf("Expected global path 'document.md', got '%s'", dispatchedPath)
	}
	if dispatchedData != data {
		t.Error("Dispatched data doesn't match")
	}

	// Second dispatch should be skipped (repeating)
	dispatchedPath = ""
	err = peer.Dispatch("shared/document.md", data)
	if err != nil {
		t.Fatalf("Second dispatch failed: %v", err)
	}
	if dispatchedPath != "" {
		t.Error("Second dispatch should have been skipped as repeating")
	}
}

func TestBasePeerContext(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	peer, _ := NewBasePeer("test", "storage", "main", "shared/", nil, store)

	ctx := peer.Context()
	if ctx == nil {
		t.Fatal("Context is nil")
	}

	// Context should not be done initially
	select {
	case <-ctx.Done():
		t.Error("Context should not be done initially")
	default:
		// Good
	}

	// Stop should cancel context
	peer.Stop()

	// Context should now be done
	select {
	case <-ctx.Done():
		// Good
	default:
		t.Error("Context should be done after Stop()")
	}
}
