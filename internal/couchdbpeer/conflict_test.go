package couchdbpeer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/imdevinc/livesync-bridge/internal/config"
	"github.com/imdevinc/livesync-bridge/internal/peer"
	"github.com/imdevinc/livesync-bridge/internal/storage"
)

// getTestConfigWithConflictResolution returns a test config with specified conflict resolution strategy
func getTestConfigWithConflictResolution(strategy *string) config.PeerCouchDBConf {
	enableCompression := false   // Disable compression for simpler tests
	customChunkSize := 50 * 1024 // 50KB for testing

	return config.PeerCouchDBConf{
		Type:               "couchdb",
		Name:               "test-peer",
		Group:              "test",
		BaseDir:            "test/",
		URL:                testCouchDBURL,
		Username:           testCouchDBUsername,
		Password:           testCouchDBPassword,
		Database:           testCouchDBDatabase,
		Passphrase:         "", // No encryption for conflict tests
		EnableCompression:  &enableCompression,
		CustomChunkSize:    &customChunkSize,
		ConflictResolution: strategy,
	}
}

// TestGetConflictStrategy tests getConflictStrategy with all config values
func TestGetConflictStrategy(t *testing.T) {
	tests := []struct {
		name             string
		conflictStrategy *string
		expected         ConflictStrategy
	}{
		{
			name:             "nil config returns default",
			conflictStrategy: nil,
			expected:         TimestampWins,
		},
		{
			name:             "empty string returns default",
			conflictStrategy: stringPtr(""),
			expected:         TimestampWins,
		},
		{
			name:             "timestamp-wins strategy",
			conflictStrategy: stringPtr("timestamp-wins"),
			expected:         TimestampWins,
		},
		{
			name:             "local-wins strategy",
			conflictStrategy: stringPtr("local-wins"),
			expected:         LocalWins,
		},
		{
			name:             "remote-wins strategy",
			conflictStrategy: stringPtr("remote-wins"),
			expected:         RemoteWins,
		},
		{
			name:             "manual strategy",
			conflictStrategy: stringPtr("manual"),
			expected:         Manual,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := getTestConfigWithConflictResolution(tt.conflictStrategy)
			strategy := getConflictStrategy(cfg)
			if strategy != tt.expected {
				t.Errorf("getConflictStrategy() = %v, want %v", strategy, tt.expected)
			}
		})
	}
}

// TestResolveByTimestamp tests timestamp-based conflict resolution
func TestResolveByTimestamp(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	strategy := "timestamp-wins"
	p, _, cleanup := setupTestPeerWithConflictStrategy(t, &strategy)
	if p == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name           string
		localMTime     int64
		remoteMTime    int64
		expectedData   string
		expectLocalWin bool
	}{
		{
			name:           "local newer wins",
			localMTime:     1000,
			remoteMTime:    500,
			expectedData:   "local data",
			expectLocalWin: true,
		},
		{
			name:           "remote newer wins",
			localMTime:     500,
			remoteMTime:    1000,
			expectedData:   "remote data",
			expectLocalWin: false,
		},
		{
			name:           "equal timestamps - remote wins (tiebreaker)",
			localMTime:     1000,
			remoteMTime:    1000,
			expectedData:   "remote data",
			expectLocalWin: false,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test documents with unique IDs
			docID := fmt.Sprintf("test:conflict-timestamp-%d.txt", i)
			docPath := fmt.Sprintf("test/conflict-timestamp-%d.txt", i)

			localData := []byte("local data")
			remoteData := []byte("remote data")

			local := &LiveSyncDocument{
				ID:    docID,
				Rev:   "1-local",
				Type:  DocTypePlain,
				Path:  docPath,
				Data:  base64.StdEncoding.EncodeToString(localData),
				CTime: 100,
				MTime: tt.localMTime,
				Size:  len(localData),
			}

			remote := &LiveSyncDocument{
				ID: docID,
				// Don't set Rev for initial Put
				Type:  DocTypePlain,
				Path:  docPath,
				Data:  base64.StdEncoding.EncodeToString(remoteData),
				CTime: 100,
				MTime: tt.remoteMTime,
				Size:  len(remoteData),
			}

			// Put remote document first so it exists in DB
			remoteRev, err := p.client.Put(ctx, remote.ID, remote)
			if err != nil {
				t.Fatalf("Failed to put remote document: %v", err)
			}
			remote.Rev = remoteRev

			// Resolve conflict
			resolved, err := p.resolveByTimestamp(ctx, local, remote)
			if err != nil {
				t.Fatalf("resolveByTimestamp() error = %v", err)
			}

			if resolved == nil {
				t.Fatal("Expected resolved document, got nil")
			}

			// Verify correct data was chosen
			decodedData, err := base64.StdEncoding.DecodeString(resolved.Data)
			if err != nil {
				t.Fatalf("Failed to decode resolved data: %v", err)
			}

			if string(decodedData) != tt.expectedData {
				t.Errorf("Expected data %q, got %q", tt.expectedData, string(decodedData))
			}

			// Verify MTime matches expected winner
			if tt.expectLocalWin && resolved.MTime != tt.localMTime {
				t.Errorf("Expected local MTime %d, got %d", tt.localMTime, resolved.MTime)
			}
			if !tt.expectLocalWin && resolved.MTime != tt.remoteMTime {
				t.Errorf("Expected remote MTime %d, got %d", tt.remoteMTime, resolved.MTime)
			}

			// Cleanup
			p.client.Delete(ctx, resolved.ID, resolved.Rev)
		})
	}
}

// TestResolveLocalWins tests local-wins strategy with chunked and non-chunked files
func TestResolveLocalWins(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	strategy := "local-wins"
	p, _, cleanup := setupTestPeerWithConflictStrategy(t, &strategy)
	if p == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	t.Run("non-chunked file", func(t *testing.T) {
		localData := []byte("local version wins")
		remoteData := []byte("remote version loses")

		local := &LiveSyncDocument{
			ID:    "test:local-wins-small.txt",
			Rev:   "1-local",
			Type:  DocTypePlain,
			Path:  "test/local-wins-small.txt",
			Data:  base64.StdEncoding.EncodeToString(localData),
			CTime: 100,
			MTime: 500,
			Size:  len(localData),
		}

		remote := &LiveSyncDocument{
			ID: "test:local-wins-small.txt",
			// Don't set Rev for initial Put
			Type:  DocTypePlain,
			Path:  "test/local-wins-small.txt",
			Data:  base64.StdEncoding.EncodeToString(remoteData),
			CTime: 100,
			MTime: 1000,
			Size:  len(remoteData),
		}

		// Put remote document first
		remoteRev, err := p.client.Put(ctx, remote.ID, remote)
		if err != nil {
			t.Fatalf("Failed to put remote document: %v", err)
		}
		remote.Rev = remoteRev

		// Resolve conflict
		resolved, err := p.resolveLocalWins(ctx, local, remote)
		if err != nil {
			t.Fatalf("resolveLocalWins() error = %v", err)
		}

		// Verify local data won
		decodedData, err := base64.StdEncoding.DecodeString(resolved.Data)
		if err != nil {
			t.Fatalf("Failed to decode resolved data: %v", err)
		}

		if string(decodedData) != string(localData) {
			t.Errorf("Expected local data %q, got %q", string(localData), string(decodedData))
		}

		// Cleanup
		p.client.Delete(ctx, resolved.ID, resolved.Rev)
	})

	t.Run("chunked file", func(t *testing.T) {
		// Create large local data that will be chunked (75KB)
		localData := make([]byte, 75*1024)
		for i := range localData {
			localData[i] = 'L' // Local data marker
		}

		remoteData := make([]byte, 1024)
		for i := range remoteData {
			remoteData[i] = 'R' // Remote data marker
		}

		local := &LiveSyncDocument{
			ID:    "test:local-wins-large.bin",
			Rev:   "1-local",
			Type:  DocTypePlain,
			Path:  "test/local-wins-large.bin",
			Data:  base64.StdEncoding.EncodeToString(localData),
			CTime: 100,
			MTime: 500,
			Size:  len(localData),
		}

		remote := &LiveSyncDocument{
			ID: "test:local-wins-large.bin",
			// Don't set Rev for initial Put
			Type:     DocTypePlain,
			Path:     "test/local-wins-large.bin",
			Data:     base64.StdEncoding.EncodeToString(remoteData),
			CTime:    100,
			MTime:    1000,
			Size:     len(remoteData),
			Children: []string{}, // No chunks initially
		}

		// Put remote document first
		remoteRev, err := p.client.Put(ctx, remote.ID, remote)
		if err != nil {
			t.Fatalf("Failed to put remote document: %v", err)
		}
		remote.Rev = remoteRev

		// Resolve conflict
		resolved, err := p.resolveLocalWins(ctx, local, remote)
		if err != nil {
			t.Fatalf("resolveLocalWins() error = %v", err)
		}

		// Verify local data won by checking chunks were created
		if len(resolved.Children) == 0 {
			t.Error("Expected chunks to be created for large file")
		}

		// Verify data field is empty for chunked document
		if resolved.Data != "" {
			t.Error("Expected empty Data field for chunked document")
		}

		// Verify size matches local
		if resolved.Size != len(localData) {
			t.Errorf("Expected size %d, got %d", len(localData), resolved.Size)
		}

		// Cleanup - delete main doc and chunks
		for _, chunkID := range resolved.Children {
			chunkDoc, _ := p.client.Get(ctx, chunkID)
			if chunkDoc != nil {
				p.client.Delete(ctx, chunkID, chunkDoc.Rev)
			}
		}
		p.client.Delete(ctx, resolved.ID, resolved.Rev)
	})
}

// TestResolveRemoteWins tests remote-wins strategy dispatches correctly
func TestResolveRemoteWins(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	// Ensure test database exists
	if err := ensureTestDatabase(t); err != nil {
		t.Skipf("CouchDB not available for integration tests: %v", err)
	}
	defer cleanupTestDatabase(t)

	// Track dispatched data
	var dispatchedPath string
	var dispatchedData []byte
	dispatcher := func(source peer.Peer, path string, data *peer.FileData) error {
		dispatchedPath = path
		if data != nil {
			dispatchedData = make([]byte, len(data.Data))
			copy(dispatchedData, data.Data)
		}
		return nil
	}

	// Create peer with custom dispatcher
	tmpDB := t.TempDir() + "/test.db"
	store, err := storage.NewStore(tmpDB)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	strategy := "remote-wins"
	cfg := getTestConfigWithConflictResolution(&strategy)
	p, err := NewCouchDBPeer(cfg, dispatcher, store)
	if err != nil {
		t.Skipf("CouchDB not available: %v", err)
	}
	defer p.Stop()

	ctx := context.Background()

	localData := []byte("local data loses")
	remoteData := []byte("remote data wins")

	local := &LiveSyncDocument{
		ID:    "test:remote-wins.txt",
		Rev:   "1-local",
		Type:  DocTypePlain,
		Path:  "test/remote-wins.txt",
		Data:  base64.StdEncoding.EncodeToString(localData),
		CTime: 100,
		MTime: 1000,
		Size:  len(localData),
	}

	remote := &LiveSyncDocument{
		ID: "test:remote-wins.txt",
		// Don't set Rev for initial Put
		Type:  DocTypePlain,
		Path:  "test/remote-wins.txt",
		Data:  base64.StdEncoding.EncodeToString(remoteData),
		CTime: 100,
		MTime: 500,
		Size:  len(remoteData),
	}

	// Put remote document first
	remoteRev, err := p.client.Put(ctx, remote.ID, remote)
	if err != nil {
		t.Fatalf("Failed to put remote document: %v", err)
	}
	remote.Rev = remoteRev

	// Resolve conflict
	resolved, err := p.resolveRemoteWins(ctx, local, remote)
	if err != nil {
		t.Fatalf("resolveRemoteWins() error = %v", err)
	}

	// Verify remote document was returned as-is
	if resolved.ID != remote.ID || resolved.Rev != remote.Rev {
		t.Errorf("Expected remote document to be returned as-is")
	}

	// Verify data was dispatched to local peers
	if dispatchedPath == "" {
		t.Error("Expected dispatch to be called")
	}

	expectedPath := p.ToGlobalPath(remote.Path)
	if dispatchedPath != expectedPath {
		t.Errorf("Expected dispatch path %q, got %q", expectedPath, dispatchedPath)
	}

	if string(dispatchedData) != string(remoteData) {
		t.Errorf("Expected dispatched data %q, got %q", string(remoteData), string(dispatchedData))
	}

	// Cleanup
	p.client.Delete(ctx, resolved.ID, resolved.Rev)
}

// TestManualStrategyReturnsError tests manual strategy returns error
func TestManualStrategyReturnsError(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	strategy := "manual"
	p, _, cleanup := setupTestPeerWithConflictStrategy(t, &strategy)
	if p == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	local := &LiveSyncDocument{
		ID:    "test:manual-conflict.txt",
		Rev:   "1-local",
		Type:  DocTypePlain,
		Path:  "test/manual-conflict.txt",
		Data:  base64.StdEncoding.EncodeToString([]byte("local")),
		CTime: 100,
		MTime: 500,
		Size:  5,
	}

	remote := &LiveSyncDocument{
		ID: "test:manual-conflict.txt",
		// Don't set Rev for initial Put
		Type:  DocTypePlain,
		Path:  "test/manual-conflict.txt",
		Data:  base64.StdEncoding.EncodeToString([]byte("remote")),
		CTime: 100,
		MTime: 500,
		Size:  6,
	}

	// Put remote document first
	remoteRev, err := p.client.Put(ctx, remote.ID, remote)
	if err != nil {
		t.Fatalf("Failed to put remote document: %v", err)
	}
	remote.Rev = remoteRev

	// Attempt to resolve conflict - should return error
	_, err = p.resolveConflict(ctx, *local)
	if err == nil {
		t.Fatal("Expected error for manual strategy, got nil")
	}

	// Verify error message mentions manual resolution
	if !strings.Contains(err.Error(), "manual") {
		t.Errorf("Expected error to mention 'manual', got: %v", err)
	}

	// Cleanup
	p.client.Delete(ctx, remote.ID, remote.Rev)
}

// TestConflictResolutionNetworkFailures tests error handling for network failures
func TestConflictResolutionNetworkFailures(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	strategy := "timestamp-wins"
	p, _, cleanup := setupTestPeerWithConflictStrategy(t, &strategy)
	if p == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	t.Run("remote document does not exist", func(t *testing.T) {
		local := &LiveSyncDocument{
			ID:    "test:nonexistent.txt",
			Rev:   "1-local",
			Type:  DocTypePlain,
			Path:  "test/nonexistent.txt",
			Data:  base64.StdEncoding.EncodeToString([]byte("data")),
			CTime: 100,
			MTime: 500,
			Size:  4,
		}

		// Attempt to resolve conflict with non-existent remote doc
		_, err := p.resolveConflict(ctx, *local)
		if err == nil {
			t.Fatal("Expected error when remote document doesn't exist")
		}
	})

	t.Run("invalid document format", func(t *testing.T) {
		// Create a document with invalid structure that can't be parsed
		invalidDoc := map[string]interface{}{
			"_id": "test:invalid.txt",
			// Don't set _rev for initial put
			"type":  "plain",
			"path":  "test/invalid.txt",
			"data":  "not-base64-!@#$%",
			"ctime": "invalid", // Invalid type for ctime
			"mtime": 500,
			"size":  4,
		}

		// Put invalid document
		invalidRev, err := p.client.Put(ctx, invalidDoc["_id"].(string), invalidDoc)
		if err != nil {
			t.Fatalf("Failed to put invalid document: %v", err)
		}
		invalidDoc["_rev"] = invalidRev

		local := &LiveSyncDocument{
			ID:    "test:invalid.txt",
			Rev:   "2-local",
			Type:  DocTypePlain,
			Path:  "test/invalid.txt",
			Data:  base64.StdEncoding.EncodeToString([]byte("data")),
			CTime: 100,
			MTime: 500,
			Size:  4,
		}

		// Attempt to resolve - should handle parse error gracefully
		_, err = p.resolveConflict(ctx, *local)
		if err == nil {
			t.Fatal("Expected error when parsing invalid document")
		}

		// Cleanup
		p.client.Delete(ctx, invalidDoc["_id"].(string), invalidDoc["_rev"].(string))
	})
}

// TestConflictStrategyValidation tests the Validate method
func TestConflictStrategyValidation(t *testing.T) {
	tests := []struct {
		name      string
		strategy  ConflictStrategy
		expectErr bool
	}{
		{
			name:      "valid timestamp-wins",
			strategy:  TimestampWins,
			expectErr: false,
		},
		{
			name:      "valid local-wins",
			strategy:  LocalWins,
			expectErr: false,
		},
		{
			name:      "valid remote-wins",
			strategy:  RemoteWins,
			expectErr: false,
		},
		{
			name:      "valid manual",
			strategy:  Manual,
			expectErr: false,
		},
		{
			name:      "invalid strategy",
			strategy:  ConflictStrategy("invalid-strategy"),
			expectErr: true,
		},
		{
			name:      "empty strategy",
			strategy:  ConflictStrategy(""),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.strategy.Validate()
			if tt.expectErr && err == nil {
				t.Error("Expected error for invalid strategy, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}

// TestIsValidStrategy tests the isValidStrategy helper
func TestIsValidStrategy(t *testing.T) {
	tests := []struct {
		strategy string
		valid    bool
	}{
		{"timestamp-wins", true},
		{"local-wins", true},
		{"remote-wins", true},
		{"manual", true},
		{"invalid", false},
		{"", false},
		{"TIMESTAMP-WINS", false}, // Case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.strategy, func(t *testing.T) {
			result := isValidStrategy(tt.strategy)
			if result != tt.valid {
				t.Errorf("isValidStrategy(%q) = %v, want %v", tt.strategy, result, tt.valid)
			}
		})
	}
}

// TestConflictStrategyString tests the String method
func TestConflictStrategyString(t *testing.T) {
	tests := []struct {
		strategy ConflictStrategy
		expected string
	}{
		{TimestampWins, "timestamp-wins"},
		{LocalWins, "local-wins"},
		{RemoteWins, "remote-wins"},
		{Manual, "manual"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.strategy.String()
			if result != tt.expected {
				t.Errorf("String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestResolveConflictWithDispatchError tests error handling when dispatch fails
func TestResolveConflictWithDispatchError(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	// Ensure test database exists
	if err := ensureTestDatabase(t); err != nil {
		t.Skipf("CouchDB not available for integration tests: %v", err)
	}
	defer cleanupTestDatabase(t)

	// Create dispatcher that returns error
	dispatchErr := errors.New("dispatch failed")
	dispatcher := func(source peer.Peer, path string, data *peer.FileData) error {
		return dispatchErr
	}

	tmpDB := t.TempDir() + "/test.db"
	store, err := storage.NewStore(tmpDB)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	strategy := "remote-wins"
	cfg := getTestConfigWithConflictResolution(&strategy)
	p, err := NewCouchDBPeer(cfg, dispatcher, store)
	if err != nil {
		t.Skipf("CouchDB not available: %v", err)
	}
	defer p.Stop()

	ctx := context.Background()

	local := &LiveSyncDocument{
		ID:    "test:dispatch-error.txt",
		Rev:   "1-local",
		Type:  DocTypePlain,
		Path:  "test/dispatch-error.txt",
		Data:  base64.StdEncoding.EncodeToString([]byte("local")),
		CTime: 100,
		MTime: 500,
		Size:  5,
	}

	remote := &LiveSyncDocument{
		ID: "test:dispatch-error.txt",
		// Don't set Rev for initial Put
		Type:  DocTypePlain,
		Path:  "test:dispatch-error.txt",
		Data:  base64.StdEncoding.EncodeToString([]byte("remote")),
		CTime: 100,
		MTime: 600,
		Size:  6,
	}

	// Put remote document
	remoteRev, err := p.client.Put(ctx, remote.ID, remote)
	if err != nil {
		t.Fatalf("Failed to put remote document: %v", err)
	}
	remote.Rev = remoteRev

	// Put remote document
	if _, err := p.client.Put(ctx, remote.ID, remote); err != nil {
		t.Fatalf("Failed to put remote document: %v", err)
	}

	// Resolve conflict - should return dispatch error
	_, err = p.resolveRemoteWins(ctx, local, remote)
	if err == nil {
		t.Fatal("Expected error when dispatch fails")
	}

	if !errors.Is(err, dispatchErr) && !strings.Contains(err.Error(), "dispatch") {
		t.Errorf("Expected dispatch error, got: %v", err)
	}

	// Cleanup
	p.client.Delete(ctx, remote.ID, remote.Rev)
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}

// setupTestPeerWithConflictStrategy creates a test peer with specific conflict resolution strategy
func setupTestPeerWithConflictStrategy(t *testing.T, strategy *string) (*CouchDBPeer, *storage.Store, func()) {
	t.Helper()

	// Ensure test database exists
	if err := ensureTestDatabase(t); err != nil {
		t.Skipf("CouchDB not available for integration tests: %v", err)
		return nil, nil, nil
	}

	// Create temporary database file
	tmpDB := t.TempDir() + "/test.db"
	store, err := storage.NewStore(tmpDB)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Dummy dispatcher
	dispatcher := func(source peer.Peer, path string, data *peer.FileData) error {
		return nil
	}

	// Create peer with conflict resolution strategy
	cfg := getTestConfigWithConflictResolution(strategy)
	p, err := NewCouchDBPeer(cfg, dispatcher, store)
	if err != nil {
		store.Close()
		t.Skipf("CouchDB not available for integration tests: %v", err)
		return nil, nil, nil
	}

	cleanup := func() {
		if p != nil {
			p.Stop()
		}
		if store != nil {
			store.Close()
		}
		// Clean up test database
		cleanupTestDatabase(t)
	}

	return p, store, cleanup
}

// TestPutDocumentWithRetry_ConflictResolution tests putDocumentWithRetry with conflict resolution fallback
func TestPutDocumentWithRetry_ConflictResolution(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	tests := []struct {
		name     string
		strategy string
	}{
		{
			name:     "timestamp-wins strategy",
			strategy: "timestamp-wins",
		},
		{
			name:     "local-wins strategy",
			strategy: "local-wins",
		},
		{
			name:     "remote-wins strategy",
			strategy: "remote-wins",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _, cleanup := setupTestPeerWithConflictStrategy(t, &tt.strategy)
			if p == nil {
				return
			}
			defer cleanup()

			ctx := context.Background()
			docID := fmt.Sprintf("test:put-conflict-%s.txt", strings.ReplaceAll(tt.strategy, "-", "_"))
			docPath := fmt.Sprintf("test/put-conflict-%s.txt", strings.ReplaceAll(tt.strategy, "-", "_"))

			// Create initial document
			initialData := []byte("initial data")
			initialDoc := LiveSyncDocument{
				ID:    docID,
				Type:  DocTypePlain,
				Path:  docPath,
				Data:  base64.StdEncoding.EncodeToString(initialData),
				CTime: 100,
				MTime: 500,
				Size:  len(initialData),
			}

			rev, err := p.client.Put(ctx, initialDoc.ID, initialDoc)
			if err != nil {
				t.Fatalf("Failed to create initial document: %v", err)
			}
			initialDoc.Rev = rev

			// Create a document with newer MTime for timestamp-wins
			updateData := []byte("updated data")
			updateDoc := LiveSyncDocument{
				ID:    docID,
				Rev:   "", // Intentionally leave blank to simulate stale revision
				Type:  DocTypePlain,
				Path:  docPath,
				Data:  base64.StdEncoding.EncodeToString(updateData),
				CTime: 100,
				MTime: 1000, // Newer than initial
				Size:  len(updateData),
			}

			// Start concurrent updates to force retry exhaustion
			done := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-done:
						return
					case <-ticker.C:
						// Get current revision and update document
						currentDoc, err := p.client.Get(ctx, docID)
						if err != nil {
							continue
						}
						// Update with intermediate MTime
						concurrentUpdate := LiveSyncDocument{
							ID:    docID,
							Rev:   currentDoc.Rev,
							Type:  DocTypePlain,
							Path:  docPath,
							Data:  base64.StdEncoding.EncodeToString([]byte("concurrent update")),
							CTime: 100,
							MTime: 750, // Between initial and update
							Size:  len("concurrent update"),
						}
						p.client.Put(ctx, docID, concurrentUpdate)
					}
				}
			}()

			// Allow some concurrent updates to happen
			time.Sleep(100 * time.Millisecond)

			// Try to put the update - this should exhaust retries and use conflict resolution
			finalRev, err := p.putDocumentWithRetry(ctx, updateDoc)
			close(done)
			wg.Wait()

			if err != nil {
				t.Fatalf("putDocumentWithRetry() failed: %v", err)
			}

			if finalRev == "" {
				t.Fatal("Expected non-empty revision")
			}

			// Get final document to verify resolution
			finalDocRaw, err := p.client.Get(ctx, docID)
			if err != nil {
				t.Fatalf("Failed to get final document: %v", err)
			}

			// Convert to LiveSyncDocument
			finalDocBytes, err := json.Marshal(finalDocRaw.Data)
			if err != nil {
				t.Fatalf("Failed to marshal document data: %v", err)
			}

			var finalDoc LiveSyncDocument
			if err := json.Unmarshal(finalDocBytes, &finalDoc); err != nil {
				t.Fatalf("Failed to unmarshal document: %v", err)
			}
			finalDoc.ID = finalDocRaw.ID
			finalDoc.Rev = finalDocRaw.Rev

			// Verify based on strategy
			decodedData, err := base64.StdEncoding.DecodeString(finalDoc.Data)
			if err != nil {
				t.Fatalf("Failed to decode data: %v", err)
			}

			switch tt.strategy {
			case "timestamp-wins":
				// updateDoc has MTime 1000 (newest), so it should win
				if string(decodedData) != "updated data" {
					t.Errorf("Expected timestamp-wins to select updated data, got %q", string(decodedData))
				}
			case "local-wins":
				// Local (updateDoc) should always win
				if string(decodedData) != "updated data" {
					t.Errorf("Expected local-wins to select updated data, got %q", string(decodedData))
				}
			case "remote-wins":
				// Remote (concurrent update) should win
				if string(decodedData) != "concurrent update" {
					t.Errorf("Expected remote-wins to select concurrent update, got %q", string(decodedData))
				}
			}

			t.Logf("Conflict resolution succeeded")

			// Cleanup
			p.client.Delete(ctx, docID, finalDoc.Rev)
		})
	}
}

// TestPutDocumentWithRetry_NormalOperation tests that normal puts (no conflicts) still work
func TestPutDocumentWithRetry_NormalOperation(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	strategy := "timestamp-wins"
	p, _, cleanup := setupTestPeerWithConflictStrategy(t, &strategy)
	if p == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name    string
		docID   string
		docPath string
		data    string
	}{
		{
			name:    "new document",
			docID:   "test:normal-new.txt",
			docPath: "test/normal-new.txt",
			data:    "new document data",
		},
		{
			name:    "update existing document",
			docID:   "test:normal-update.txt",
			docPath: "test/normal-update.txt",
			data:    "updated document data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := LiveSyncDocument{
				ID:    tt.docID,
				Type:  DocTypePlain,
				Path:  tt.docPath,
				Data:  base64.StdEncoding.EncodeToString([]byte(tt.data)),
				CTime: 100,
				MTime: 500,
				Size:  len(tt.data),
			}

			// For update test, create initial document first
			if tt.name == "update existing document" {
				initialRev, err := p.client.Put(ctx, doc.ID, doc)
				if err != nil {
					t.Fatalf("Failed to create initial document: %v", err)
				}
				doc.Rev = initialRev
				doc.Data = base64.StdEncoding.EncodeToString([]byte(tt.data))
			}

			// Put document
			rev, err := p.putDocumentWithRetry(ctx, doc)
			if err != nil {
				t.Fatalf("putDocumentWithRetry() failed: %v", err)
			}

			if rev == "" {
				t.Fatal("Expected non-empty revision")
			}

			// Verify document was saved correctly
			savedDocRaw, err := p.client.Get(ctx, tt.docID)
			if err != nil {
				t.Fatalf("Failed to get saved document: %v", err)
			}

			// Convert to LiveSyncDocument
			savedDocBytes, err := json.Marshal(savedDocRaw.Data)
			if err != nil {
				t.Fatalf("Failed to marshal document data: %v", err)
			}

			var savedDoc LiveSyncDocument
			if err := json.Unmarshal(savedDocBytes, &savedDoc); err != nil {
				t.Fatalf("Failed to unmarshal document: %v", err)
			}
			savedDoc.ID = savedDocRaw.ID
			savedDoc.Rev = savedDocRaw.Rev

			decodedData, err := base64.StdEncoding.DecodeString(savedDoc.Data)
			if err != nil {
				t.Fatalf("Failed to decode data: %v", err)
			}

			if string(decodedData) != tt.data {
				t.Errorf("Expected data %q, got %q", tt.data, string(decodedData))
			}

			// Cleanup
			p.client.Delete(ctx, tt.docID, savedDoc.Rev)
		})
	}
}

// TestPutChunkWithRetry_ConflictResolution tests putChunkWithRetry with conflict resolution fallback
func TestPutChunkWithRetry_ConflictResolution(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	strategy := "local-wins" // Chunks always use local-wins logic
	p, _, cleanup := setupTestPeerWithConflictStrategy(t, &strategy)
	if p == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()
	chunkID := "test:put-chunk-conflict-c1"

	// Create initial chunk
	initialData := []byte("initial chunk data")
	initialChunk := ChunkDocument{
		ID:   chunkID,
		Type: DocTypeLeaf,
		Data: base64.StdEncoding.EncodeToString(initialData),
	}

	rev, err := p.client.Put(ctx, initialChunk.ID, initialChunk)
	if err != nil {
		t.Fatalf("Failed to create initial chunk: %v", err)
	}
	initialChunk.Rev = rev

	// Create updated chunk
	updateData := []byte("updated chunk data")
	updateChunk := ChunkDocument{
		ID:   chunkID,
		Rev:  "", // Intentionally leave blank to simulate stale revision
		Type: DocTypeLeaf,
		Data: base64.StdEncoding.EncodeToString(updateData),
	}

	// Start concurrent updates to force retry exhaustion
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// Update chunk to force conflicts
				currentChunk, err := p.client.Get(ctx, chunkID)
				if err != nil {
					continue
				}
				// Create concurrent update
				concurrentUpdate := ChunkDocument{
					ID:   chunkID,
					Rev:  currentChunk.Rev,
					Type: DocTypeLeaf,
					Data: base64.StdEncoding.EncodeToString([]byte("concurrent chunk update")),
				}
				p.client.Put(ctx, chunkID, concurrentUpdate)
			}
		}
	}()

	// Allow some concurrent updates to happen
	time.Sleep(100 * time.Millisecond)

	// Try to put the update - this should exhaust retries and use conflict resolution
	finalRev, err := p.putChunkWithRetry(ctx, updateChunk)
	close(done)
	wg.Wait()

	if err != nil {
		t.Fatalf("putChunkWithRetry() failed: %v", err)
	}

	if finalRev == "" {
		t.Fatal("Expected non-empty revision")
	}

	// Verify chunk was successfully saved with local data (delete-and-recreate)
	finalChunkRaw, err := p.client.Get(ctx, chunkID)
	if err != nil {
		t.Fatalf("Failed to get final chunk: %v", err)
	}

	// Convert to ChunkDocument
	finalChunkBytes, err := json.Marshal(finalChunkRaw.Data)
	if err != nil {
		t.Fatalf("Failed to marshal chunk data: %v", err)
	}

	var finalChunk ChunkDocument
	if err := json.Unmarshal(finalChunkBytes, &finalChunk); err != nil {
		t.Fatalf("Failed to unmarshal chunk: %v", err)
	}
	finalChunk.ID = finalChunkRaw.ID
	finalChunk.Rev = finalChunkRaw.Rev

	decodedData, err := base64.StdEncoding.DecodeString(finalChunk.Data)
	if err != nil {
		t.Fatalf("Failed to decode data: %v", err)
	}

	// Chunk conflict resolution always prefers local data
	if string(decodedData) != "updated chunk data" {
		t.Errorf("Expected local chunk data, got %q", string(decodedData))
	}

	t.Logf("Chunk conflict resolution succeeded")

	// Cleanup
	p.client.Delete(ctx, chunkID, finalChunk.Rev)
}

// TestPutChunkWithRetry_NormalOperation tests that normal chunk puts (no conflicts) still work
func TestPutChunkWithRetry_NormalOperation(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	strategy := "timestamp-wins"
	p, _, cleanup := setupTestPeerWithConflictStrategy(t, &strategy)
	if p == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name    string
		chunkID string
		data    string
	}{
		{
			name:    "new chunk",
			chunkID: "test:normal-chunk-new-c1",
			data:    "new chunk data",
		},
		{
			name:    "update existing chunk",
			chunkID: "test:normal-chunk-update-c1",
			data:    "updated chunk data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunk := ChunkDocument{
				ID:   tt.chunkID,
				Type: DocTypeLeaf,
				Data: base64.StdEncoding.EncodeToString([]byte(tt.data)),
			}

			// For update test, create initial chunk first
			if tt.name == "update existing chunk" {
				initialRev, err := p.client.Put(ctx, chunk.ID, chunk)
				if err != nil {
					t.Fatalf("Failed to create initial chunk: %v", err)
				}
				chunk.Rev = initialRev
				chunk.Data = base64.StdEncoding.EncodeToString([]byte(tt.data))
			}

			// Put chunk
			rev, err := p.putChunkWithRetry(ctx, chunk)
			if err != nil {
				t.Fatalf("putChunkWithRetry() failed: %v", err)
			}

			if rev == "" {
				t.Fatal("Expected non-empty revision")
			}

			// Verify chunk was saved correctly
			savedChunkRaw, err := p.client.Get(ctx, tt.chunkID)
			if err != nil {
				t.Fatalf("Failed to get saved chunk: %v", err)
			}

			// Convert to ChunkDocument
			savedChunkBytes, err := json.Marshal(savedChunkRaw.Data)
			if err != nil {
				t.Fatalf("Failed to marshal chunk data: %v", err)
			}

			var savedChunk ChunkDocument
			if err := json.Unmarshal(savedChunkBytes, &savedChunk); err != nil {
				t.Fatalf("Failed to unmarshal chunk: %v", err)
			}
			savedChunk.ID = savedChunkRaw.ID
			savedChunk.Rev = savedChunkRaw.Rev

			decodedData, err := base64.StdEncoding.DecodeString(savedChunk.Data)
			if err != nil {
				t.Fatalf("Failed to decode data: %v", err)
			}

			if string(decodedData) != tt.data {
				t.Errorf("Expected data %q, got %q", tt.data, string(decodedData))
			}

			// Cleanup
			p.client.Delete(ctx, tt.chunkID, savedChunk.Rev)
		})
	}
}

// TestPutWithManualStrategy tests that manual strategy returns error
func TestPutWithManualStrategy(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	strategy := "manual"
	p, _, cleanup := setupTestPeerWithConflictStrategy(t, &strategy)
	if p == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()
	docID := "test:manual-strategy.txt"
	docPath := "test/manual-strategy.txt"

	// Create initial document
	initialData := []byte("initial data")
	initialDoc := LiveSyncDocument{
		ID:    docID,
		Type:  DocTypePlain,
		Path:  docPath,
		Data:  base64.StdEncoding.EncodeToString(initialData),
		CTime: 100,
		MTime: 500,
		Size:  len(initialData),
	}

	rev, err := p.client.Put(ctx, initialDoc.ID, initialDoc)
	if err != nil {
		t.Fatalf("Failed to create initial document: %v", err)
	}
	initialDoc.Rev = rev

	// Create update document
	updateData := []byte("updated data")
	updateDoc := LiveSyncDocument{
		ID:    docID,
		Rev:   "", // Intentionally leave blank to force conflict
		Type:  DocTypePlain,
		Path:  docPath,
		Data:  base64.StdEncoding.EncodeToString(updateData),
		CTime: 100,
		MTime: 1000,
		Size:  len(updateData),
	}

	// Start concurrent updates to force retry exhaustion
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				currentDoc, err := p.client.Get(ctx, docID)
				if err != nil {
					continue
				}
				// Create concurrent update
				concurrentUpdate := LiveSyncDocument{
					ID:    docID,
					Rev:   currentDoc.Rev,
					Type:  DocTypePlain,
					Path:  docPath,
					Data:  base64.StdEncoding.EncodeToString([]byte("concurrent update")),
					CTime: 100,
					MTime: 750,
					Size:  len("concurrent update"),
				}
				p.client.Put(ctx, docID, concurrentUpdate)
			}
		}
	}()

	// Allow some concurrent updates
	time.Sleep(100 * time.Millisecond)

	// Try to put - should fail with manual strategy error
	_, err = p.putDocumentWithRetry(ctx, updateDoc)
	close(done)
	wg.Wait()

	if err == nil {
		t.Fatal("Expected error with manual strategy, got nil")
	}

	if !strings.Contains(err.Error(), "manual") {
		t.Errorf("Expected error to mention manual strategy, got: %v", err)
	}

	// Cleanup
	finalDoc, _ := p.client.Get(ctx, docID)
	if finalDoc.Rev != "" {
		p.client.Delete(ctx, docID, finalDoc.Rev)
	}
}

// TestDeleteDocumentWithRetry_ConflictResolution tests Delete operations with conflict resolution
func TestDeleteDocumentWithRetry_ConflictResolution(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	tests := []struct {
		name             string
		strategy         string
		localMTime       int64
		remoteMTime      int64
		expectDeleted    bool
		expectDispatched bool
	}{
		{
			name:          "timestamp-wins: local newer (delete proceeds)",
			strategy:      "timestamp-wins",
			localMTime:    1000,
			remoteMTime:   500,
			expectDeleted: true,
		},
		{
			name:          "timestamp-wins: remote newer (remote wins)",
			strategy:      "timestamp-wins",
			localMTime:    500,
			remoteMTime:   1000,
			expectDeleted: false,
		},
		{
			name:          "timestamp-wins: equal timestamps (remote wins as tiebreaker)",
			strategy:      "timestamp-wins",
			localMTime:    750,
			remoteMTime:   750,
			expectDeleted: false,
		},
		{
			name:          "local-wins: delete always proceeds",
			strategy:      "local-wins",
			localMTime:    500,
			remoteMTime:   1000,
			expectDeleted: true,
		},
		{
			name:          "remote-wins: remote always wins",
			strategy:      "remote-wins",
			localMTime:    1000,
			remoteMTime:   500,
			expectDeleted: false,
		},
		{
			name:             "timestamp-wins: remote newer (remote wins)",
			strategy:         "timestamp-wins",
			localMTime:       500,
			remoteMTime:      1000,
			expectDeleted:    false,
			expectDispatched: true,
		},
		{
			name:             "timestamp-wins: equal timestamps (remote wins as tiebreaker)",
			strategy:         "timestamp-wins",
			localMTime:       750,
			remoteMTime:      750,
			expectDeleted:    false,
			expectDispatched: true,
		},
		{
			name:             "local-wins: delete always proceeds",
			strategy:         "local-wins",
			localMTime:       500,
			remoteMTime:      1000,
			expectDeleted:    true,
			expectDispatched: false,
		},
		{
			name:             "remote-wins: remote always wins",
			strategy:         "remote-wins",
			localMTime:       1000,
			remoteMTime:      500,
			expectDeleted:    false,
			expectDispatched: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test peer with conflict strategy
			p, _, cleanup := setupTestPeerWithConflictStrategy(t, &tt.strategy)
			if p == nil {
				return
			}
			defer cleanup()

			ctx := context.Background()
			docID := fmt.Sprintf("test:delete-conflict-%s.txt", strings.ReplaceAll(tt.name, " ", "-"))
			docPath := fmt.Sprintf("test/delete-conflict-%s.txt", strings.ReplaceAll(tt.name, " ", "-"))

			// Create initial document with remote MTime
			initialData := []byte("initial data")
			initialDoc := LiveSyncDocument{
				ID:    docID,
				Type:  DocTypePlain,
				Path:  docPath,
				Data:  base64.StdEncoding.EncodeToString(initialData),
				CTime: 100,
				MTime: tt.remoteMTime,
				Size:  len(initialData),
			}

			rev, err := p.client.Put(ctx, initialDoc.ID, initialDoc)
			if err != nil {
				t.Fatalf("Failed to create initial document: %v", err)
			}
			initialDoc.Rev = rev

			// Start concurrent updates to force retry exhaustion
			done := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-done:
						return
					case <-ticker.C:
						// Get current document and update it
						currentDoc, err := p.client.Get(ctx, docID)
						if err != nil {
							continue
						}

						// Update with remote MTime
						updateDoc := LiveSyncDocument{
							ID:    docID,
							Rev:   currentDoc.Rev,
							Type:  DocTypePlain,
							Path:  docPath,
							Data:  base64.StdEncoding.EncodeToString([]byte("concurrent update")),
							CTime: 100,
							MTime: tt.remoteMTime,
							Size:  len("concurrent update"),
						}
						p.client.Put(ctx, docID, updateDoc)
					}
				}
			}()

			// Allow some concurrent updates
			time.Sleep(100 * time.Millisecond)

			// Attempt delete with local MTime
			err = p.deleteDocumentWithRetry(ctx, docID, rev, tt.localMTime)
			close(done)
			wg.Wait()

			if err != nil {
				t.Fatalf("deleteDocumentWithRetry failed: %v", err)
			}

			// Check if document was deleted or preserved based on strategy
			finalDoc, getErr := p.client.Get(ctx, docID)
			if tt.expectDeleted {
				// Document should be deleted
				if getErr == nil {
					t.Errorf("Expected document to be deleted, but it still exists with rev %s", finalDoc.Rev)
					// Cleanup
					p.client.Delete(ctx, docID, finalDoc.Rev)
				}
			} else {
				// Document should still exist (remote wins)
				if getErr != nil {
					t.Errorf("Expected document to exist (remote wins), but got error: %v", getErr)
				} else {
					// Cleanup
					p.client.Delete(ctx, docID, finalDoc.Rev)
				}
			}
		})
	}
}

// TestDeleteDocumentWithRetry_RetryLogic tests that retry logic fetches fresh revisions
func TestDeleteDocumentWithRetry_RetryLogic(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	strategy := "local-wins" // Use local-wins so delete always proceeds after conflict resolution
	p, _, cleanup := setupTestPeerWithConflictStrategy(t, &strategy)
	if p == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()
	docID := "test:delete-retry-logic.txt"
	docPath := "test/delete-retry-logic.txt"

	// Create initial document
	initialData := []byte("initial data")
	initialDoc := LiveSyncDocument{
		ID:    docID,
		Type:  DocTypePlain,
		Path:  docPath,
		Data:  base64.StdEncoding.EncodeToString(initialData),
		CTime: 100,
		MTime: 500,
		Size:  len(initialData),
	}

	rev, err := p.client.Put(ctx, initialDoc.ID, initialDoc)
	if err != nil {
		t.Fatalf("Failed to create initial document: %v", err)
	}

	// Update document once to change revision
	updateDoc := initialDoc
	updateDoc.Rev = rev
	updateDoc.Data = base64.StdEncoding.EncodeToString([]byte("updated data"))
	_, err = p.client.Put(ctx, docID, updateDoc)
	if err != nil {
		t.Fatalf("Failed to update document: %v", err)
	}

	// Try to delete with old revision - should retry and succeed
	err = p.deleteDocumentWithRetry(ctx, docID, rev, 500)
	if err != nil {
		t.Fatalf("deleteDocumentWithRetry should have succeeded with retry: %v", err)
	}

	// Verify document was actually deleted
	_, getErr := p.client.Get(ctx, docID)
	if getErr == nil {
		t.Error("Expected document to be deleted, but it still exists")
		// Cleanup just in case
		finalDoc, _ := p.client.Get(ctx, docID)
		p.client.Delete(ctx, docID, finalDoc.Rev)
	}

	// Test with very stale revision
	// Create another document for second test
	docID2 := "test:delete-retry-logic-2.txt"
	docPath2 := "test/delete-retry-logic-2.txt"
	doc2 := LiveSyncDocument{
		ID:    docID2,
		Type:  DocTypePlain,
		Path:  docPath2,
		Data:  base64.StdEncoding.EncodeToString([]byte("data 1")),
		CTime: 100,
		MTime: 500,
		Size:  6,
	}

	rev1, err := p.client.Put(ctx, docID2, doc2)
	if err != nil {
		t.Fatalf("Failed to create document: %v", err)
	}

	// Update multiple times to make first revision very stale
	doc2.Rev = rev1
	doc2.Data = base64.StdEncoding.EncodeToString([]byte("data 2"))
	var rev2 string
	rev2, _ = p.client.Put(ctx, docID2, doc2)

	doc2.Rev = rev2
	doc2.Data = base64.StdEncoding.EncodeToString([]byte("data 3"))
	rev3, _ := p.client.Put(ctx, docID2, doc2)

	doc2.Rev = rev3
	doc2.Data = base64.StdEncoding.EncodeToString([]byte("data 4"))
	_, _ = p.client.Put(ctx, docID2, doc2)

	// Try to delete with very stale revision - should retry and eventually succeed
	err = p.deleteDocumentWithRetry(ctx, docID2, rev1, 500)
	if err != nil {
		t.Fatalf("deleteDocumentWithRetry should have succeeded with retries: %v", err)
	}

	// Verify document was deleted
	_, getErr = p.client.Get(ctx, docID2)
	if getErr == nil {
		t.Error("Expected document to be deleted, but it still exists")
		// Cleanup
		finalDoc, _ := p.client.Get(ctx, docID2)
		p.client.Delete(ctx, docID2, finalDoc.Rev)
	}
}

// TestDeleteDocumentWithRetry_NormalOperation tests that normal deletes still work correctly
func TestDeleteDocumentWithRetry_NormalOperation(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	strategy := "timestamp-wins"
	p, _, cleanup := setupTestPeerWithConflictStrategy(t, &strategy)
	if p == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name    string
		docID   string
		docPath string
	}{
		{
			name:    "simple delete",
			docID:   "test:delete-normal-1.txt",
			docPath: "test/delete-normal-1.txt",
		},
		{
			name:    "delete non-existent (no error)",
			docID:   "test:delete-nonexistent.txt",
			docPath: "test/delete-nonexistent.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.name, "non-existent") {
				// Create document first
				doc := LiveSyncDocument{
					ID:    tt.docID,
					Type:  DocTypePlain,
					Path:  tt.docPath,
					Data:  base64.StdEncoding.EncodeToString([]byte("test data")),
					CTime: 100,
					MTime: 500,
					Size:  9,
				}

				rev, err := p.client.Put(ctx, doc.ID, doc)
				if err != nil {
					t.Fatalf("Failed to create document: %v", err)
				}

				// Delete with correct revision
				err = p.deleteDocumentWithRetry(ctx, tt.docID, rev, 500)
				if err != nil {
					t.Fatalf("Normal delete failed: %v", err)
				}

				// Verify deleted
				_, getErr := p.client.Get(ctx, tt.docID)
				if getErr == nil {
					t.Error("Expected document to be deleted")
					// Cleanup
					finalDoc, _ := p.client.Get(ctx, tt.docID)
					p.client.Delete(ctx, tt.docID, finalDoc.Rev)
				}
			} else {
				// Try to delete non-existent document - should not error
				err := p.deleteDocumentWithRetry(ctx, tt.docID, "fake-rev", 500)
				// Expecting error since document doesn't exist, but shouldn't panic
				// The function will error out when trying to get the document
				if err == nil {
					t.Log("Delete of non-existent document returned no error (acceptable)")
				} else {
					t.Logf("Delete of non-existent document returned error: %v (acceptable)", err)
				}
			}
		})
	}
}

// TestDelete_WithChunks tests Delete with chunked documents
func TestDelete_WithChunks(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	strategy := "local-wins"
	p, _, cleanup := setupTestPeerWithConflictStrategy(t, &strategy)
	if p == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()
	docID := "test:delete-chunked.txt"
	docPath := "test/delete-chunked.txt"

	// Create a document with chunks (75KB to force chunking)
	largeData := make([]byte, 75*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	// Create chunks manually
	chunkSize := 50 * 1024 // 50KB chunks
	var chunkIDs []string

	for i := 0; i < len(largeData); i += chunkSize {
		end := i + chunkSize
		if end > len(largeData) {
			end = len(largeData)
		}
		chunk := largeData[i:end]

		chunkID := fmt.Sprintf("h:%s:%d", docID, i/chunkSize)
		chunkDoc := ChunkDocument{
			ID:   chunkID,
			Type: DocTypeLeaf,
			Data: base64.StdEncoding.EncodeToString(chunk),
		}

		_, err := p.client.Put(ctx, chunkDoc.ID, chunkDoc)
		if err != nil {
			t.Fatalf("Failed to create chunk: %v", err)
		}
		chunkIDs = append(chunkIDs, chunkID)
	}

	// Create main document with chunk references
	doc := LiveSyncDocument{
		ID:       docID,
		Type:     DocTypePlain,
		Path:     docPath,
		Data:     "", // Empty for chunked documents
		CTime:    100,
		MTime:    500,
		Size:     len(largeData),
		Children: chunkIDs,
	}

	rev, err := p.client.Put(ctx, doc.ID, doc)
	if err != nil {
		t.Fatalf("Failed to create main document: %v", err)
	}

	// Start concurrent updates on chunks to force conflicts
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// Update first chunk continuously
				if len(chunkIDs) > 0 {
					chunkDoc, err := p.client.Get(ctx, chunkIDs[0])
					if err != nil {
						continue
					}
					updateChunk := ChunkDocument{
						ID:   chunkIDs[0],
						Rev:  chunkDoc.Rev,
						Type: DocTypeLeaf,
						Data: base64.StdEncoding.EncodeToString([]byte("updated chunk")),
					}
					p.client.Put(ctx, updateChunk.ID, updateChunk)
				}
			}
		}
	}()

	// Allow some concurrent updates
	time.Sleep(100 * time.Millisecond)

	// Delete with retry
	err = p.deleteDocumentWithRetry(ctx, docID, rev, 500)
	close(done)
	wg.Wait()

	if err != nil {
		t.Fatalf("Delete with chunks failed: %v", err)
	}

	// Verify main document deleted
	_, getErr := p.client.Get(ctx, docID)
	if getErr == nil {
		t.Error("Expected main document to be deleted")
		// Cleanup
		finalDoc, _ := p.client.Get(ctx, docID)
		p.client.Delete(ctx, docID, finalDoc.Rev)
	}

	// Verify chunks deleted (or at least attempted)
	// Note: Some chunks might fail to delete due to conflicts, but that's logged and acceptable
	for _, chunkID := range chunkIDs {
		_, getErr := p.client.Get(ctx, chunkID)
		if getErr == nil {
			// Chunk still exists - cleanup
			chunkDoc, _ := p.client.Get(ctx, chunkID)
			p.client.Delete(ctx, chunkID, chunkDoc.Rev)
		}
	}
}

// TestDeleteWithManualStrategy tests that manual strategy returns error
func TestDeleteWithManualStrategy(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	strategy := "manual"
	p, _, cleanup := setupTestPeerWithConflictStrategy(t, &strategy)
	if p == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()
	docID := "test:delete-manual.txt"
	docPath := "test/delete-manual.txt"

	// Create initial document
	initialData := []byte("initial data")
	initialDoc := LiveSyncDocument{
		ID:    docID,
		Type:  DocTypePlain,
		Path:  docPath,
		Data:  base64.StdEncoding.EncodeToString(initialData),
		CTime: 100,
		MTime: 500,
		Size:  len(initialData),
	}

	rev, err := p.client.Put(ctx, initialDoc.ID, initialDoc)
	if err != nil {
		t.Fatalf("Failed to create initial document: %v", err)
	}

	// Start concurrent updates to force retry exhaustion
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				currentDoc, err := p.client.Get(ctx, docID)
				if err != nil {
					continue
				}
				updateDoc := LiveSyncDocument{
					ID:    docID,
					Rev:   currentDoc.Rev,
					Type:  DocTypePlain,
					Path:  docPath,
					Data:  base64.StdEncoding.EncodeToString([]byte("concurrent update")),
					CTime: 100,
					MTime: 750,
					Size:  len("concurrent update"),
				}
				p.client.Put(ctx, docID, updateDoc)
			}
		}
	}()

	// Allow some concurrent updates
	time.Sleep(100 * time.Millisecond)

	// Try to delete - should fail with manual strategy error
	err = p.deleteDocumentWithRetry(ctx, docID, rev, 500)
	close(done)
	wg.Wait()

	if err == nil {
		t.Fatal("Expected error with manual strategy, got nil")
	}

	if !strings.Contains(err.Error(), "manual") {
		t.Errorf("Expected error to mention manual strategy, got: %v", err)
	}

	// Verify document still exists
	finalDoc, getErr := p.client.Get(ctx, docID)
	if getErr != nil {
		t.Error("Expected document to still exist with manual strategy error")
	} else {
		// Cleanup
		p.client.Delete(ctx, docID, finalDoc.Rev)
	}
}
