package storagepeer

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/imdevinc/livesync-bridge/internal/config"
	"github.com/imdevinc/livesync-bridge/internal/peer"
	"github.com/imdevinc/livesync-bridge/internal/storage"
)

// Helper function to create a temporary directory for tests
func createTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "storagepeer-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	return dir
}

// Helper function to create a temporary storage
func createTempStore(t *testing.T) *storage.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	t.Cleanup(func() {
		store.Close()
	})
	return store
}

// Mock dispatcher for testing
type mockDispatcher struct {
	calls []dispatchCall
	mu    sync.Mutex
}

type dispatchCall struct {
	source peer.Peer
	path   string
	data   *peer.FileData
}

func (md *mockDispatcher) dispatch(source peer.Peer, path string, data *peer.FileData) error {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.calls = append(md.calls, dispatchCall{source, path, data})
	return nil
}

func (md *mockDispatcher) callCount() int {
	md.mu.Lock()
	defer md.mu.Unlock()
	return len(md.calls)
}

func (md *mockDispatcher) getCall(index int) dispatchCall {
	md.mu.Lock()
	defer md.mu.Unlock()
	if index < len(md.calls) {
		return md.calls[index]
	}
	return dispatchCall{}
}

// TestNewStoragePeer tests creating a new storage peer
func TestNewStoragePeer(t *testing.T) {
	dir := createTempDir(t)
	defer os.RemoveAll(dir)

	store := createTempStore(t)
	mock := &mockDispatcher{}

	conf := config.PeerStorageConf{
		Name:    "test-peer",
		Type:    "storage",
		Group:   "test-group",
		BaseDir: dir,
	}

	sp, err := NewStoragePeer(conf, mock.dispatch, store)
	if err != nil {
		t.Fatalf("NewStoragePeer failed: %v", err)
	}

	if sp.Name() != "test-peer" {
		t.Errorf("Expected name 'test-peer', got '%s'", sp.Name())
	}

	if sp.Type() != "storage" {
		t.Errorf("Expected type 'storage', got '%s'", sp.Type())
	}

	if sp.Group() != "test-group" {
		t.Errorf("Expected group 'test-group', got '%s'", sp.Group())
	}

	// Clean up
	sp.Stop()
}

// TestStoragePeerPutGet tests writing and reading files
func TestStoragePeerPutGet(t *testing.T) {
	dir := createTempDir(t)
	defer os.RemoveAll(dir)

	store := createTempStore(t)
	mock := &mockDispatcher{}

	conf := config.PeerStorageConf{
		Name:    "test-peer",
		Type:    "storage",
		Group:   "test-group",
		BaseDir: dir,
	}

	sp, err := NewStoragePeer(conf, mock.dispatch, store)
	if err != nil {
		t.Fatalf("NewStoragePeer failed: %v", err)
	}
	defer sp.Stop()

	// Test Put
	testData := []byte("Hello, World!")
	fileData := &peer.FileData{
		Data:    testData,
		Deleted: false,
	}

	success, err := sp.Put("test.txt", fileData)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if !success {
		t.Error("Put should return success=true")
	}

	// Verify file was written
	filePath := filepath.Join(dir, "test.txt")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != string(testData) {
		t.Errorf("Expected content '%s', got '%s'", testData, content)
	}

	// Test Get
	retrieved, err := sp.Get("test.txt")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(retrieved.Data) != string(testData) {
		t.Errorf("Expected data '%s', got '%s'", testData, retrieved.Data)
	}
}

// TestStoragePeerDelete tests deleting files
func TestStoragePeerDelete(t *testing.T) {
	dir := createTempDir(t)
	defer os.RemoveAll(dir)

	store := createTempStore(t)
	mock := &mockDispatcher{}

	conf := config.PeerStorageConf{
		Name:    "test-peer",
		Type:    "storage",
		Group:   "test-group",
		BaseDir: dir,
	}

	sp, err := NewStoragePeer(conf, mock.dispatch, store)
	if err != nil {
		t.Fatalf("NewStoragePeer failed: %v", err)
	}
	defer sp.Stop()

	// Create a file first
	testData := []byte("Test content")
	fileData := &peer.FileData{
		Data:    testData,
		Deleted: false,
	}

	_, err = sp.Put("test.txt", fileData)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify file exists
	filePath := filepath.Join(dir, "test.txt")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatal("File should exist after Put")
	}

	// Delete the file
	success, err := sp.Delete("test.txt")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if !success {
		t.Error("Delete should return success=true")
	}

	// Verify file was deleted
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("File should not exist after Delete")
	}
}

// TestStoragePeerRepeating tests deduplication
func TestStoragePeerRepeating(t *testing.T) {
	dir := createTempDir(t)
	defer os.RemoveAll(dir)

	store := createTempStore(t)
	mock := &mockDispatcher{}

	conf := config.PeerStorageConf{
		Name:    "test-peer",
		Type:    "storage",
		Group:   "test-group",
		BaseDir: dir,
	}

	sp, err := NewStoragePeer(conf, mock.dispatch, store)
	if err != nil {
		t.Fatalf("NewStoragePeer failed: %v", err)
	}
	defer sp.Stop()

	// First Put should succeed
	testData := []byte("Test content")
	fileData := &peer.FileData{
		Data:    testData,
		Deleted: false,
	}

	success, err := sp.Put("test.txt", fileData)
	if err != nil {
		t.Fatalf("First Put failed: %v", err)
	}
	if !success {
		t.Error("First Put should succeed")
	}

	// Second Put with same data should be detected as repeating
	success, err = sp.Put("test.txt", fileData)
	if err != nil {
		t.Fatalf("Second Put failed: %v", err)
	}
	if success {
		t.Error("Second Put should be skipped (repeating)")
	}
}

// TestStoragePeerWatching tests file system watching
func TestStoragePeerWatching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping file watching test in short mode")
	}

	dir := createTempDir(t)
	defer os.RemoveAll(dir)

	store := createTempStore(t)
	mock := &mockDispatcher{}

	conf := config.PeerStorageConf{
		Name:    "test-peer",
		Type:    "storage",
		Group:   "test-group",
		BaseDir: dir,
	}

	sp, err := NewStoragePeer(conf, mock.dispatch, store)
	if err != nil {
		t.Fatalf("NewStoragePeer failed: %v", err)
	}
	defer sp.Stop()

	// Start watching
	if err := sp.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give watcher time to initialize
	// macOS FSEvents can be slower than Linux inotify
	initWait := 100 * time.Millisecond
	if runtime.GOOS == "darwin" {
		initWait = 250 * time.Millisecond
	}
	time.Sleep(initWait)

	// Create a file directly on filesystem
	testFile := filepath.Join(dir, "watched.txt")
	testContent := []byte("Watched content")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Wait for debounce + processing
	// macOS FSEvents needs more time than Linux inotify
	processWait := 500 * time.Millisecond
	if runtime.GOOS == "darwin" {
		processWait = 1000 * time.Millisecond
	}
	time.Sleep(processWait)

	// Check if file change was dispatched
	if mock.callCount() == 0 {
		t.Error("Expected file change to be dispatched")
	}

	// Verify the dispatched data
	if mock.callCount() > 0 {
		call := mock.getCall(0)
		if call.path != "watched.txt" {
			t.Errorf("Expected path 'watched.txt', got '%s'", call.path)
		}
		if string(call.data.Data) != string(testContent) {
			t.Errorf("Expected data '%s', got '%s'", testContent, call.data.Data)
		}
	}
}

// TestStoragePeerOfflineChanges tests scanning for offline changes
func TestStoragePeerOfflineChanges(t *testing.T) {
	dir := createTempDir(t)
	defer os.RemoveAll(dir)

	store := createTempStore(t)
	mock := &mockDispatcher{}

	// Create some files before starting the peer
	testFile1 := filepath.Join(dir, "existing1.txt")
	testFile2 := filepath.Join(dir, "existing2.txt")
	os.WriteFile(testFile1, []byte("Content 1"), 0644)
	os.WriteFile(testFile2, []byte("Content 2"), 0644)

	conf := config.PeerStorageConf{
		Name:    "test-peer",
		Type:    "storage",
		Group:   "test-group",
		BaseDir: dir,
	}

	sp, err := NewStoragePeer(conf, mock.dispatch, store)
	if err != nil {
		t.Fatalf("NewStoragePeer failed: %v", err)
	}
	defer sp.Stop()

	// Start should trigger offline scan
	if err := sp.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give scan time to complete
	time.Sleep(200 * time.Millisecond)

	// Should have dispatched both files
	if mock.callCount() < 2 {
		t.Errorf("Expected at least 2 dispatches, got %d", mock.callCount())
	}
}

// TestStoragePeerDirectories tests handling of directories
func TestStoragePeerDirectories(t *testing.T) {
	dir := createTempDir(t)
	defer os.RemoveAll(dir)

	store := createTempStore(t)
	mock := &mockDispatcher{}

	conf := config.PeerStorageConf{
		Name:    "test-peer",
		Type:    "storage",
		Group:   "test-group",
		BaseDir: dir,
	}

	sp, err := NewStoragePeer(conf, mock.dispatch, store)
	if err != nil {
		t.Fatalf("NewStoragePeer failed: %v", err)
	}
	defer sp.Stop()

	// Put file in nested directory
	testData := []byte("Nested content")
	fileData := &peer.FileData{
		Data:    testData,
		Deleted: false,
	}

	success, err := sp.Put("sub/dir/test.txt", fileData)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if !success {
		t.Error("Put should succeed")
	}

	// Verify directory structure was created
	filePath := filepath.Join(dir, "sub", "dir", "test.txt")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("File in nested directory should exist")
	}

	// Verify content
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != string(testData) {
		t.Errorf("Expected content '%s', got '%s'", testData, content)
	}
}

// TestStoragePeerHiddenFiles tests that hidden files are ignored
func TestStoragePeerHiddenFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping hidden files test in short mode")
	}

	dir := createTempDir(t)
	defer os.RemoveAll(dir)

	store := createTempStore(t)
	mock := &mockDispatcher{}

	conf := config.PeerStorageConf{
		Name:    "test-peer",
		Type:    "storage",
		Group:   "test-group",
		BaseDir: dir,
	}

	sp, err := NewStoragePeer(conf, mock.dispatch, store)
	if err != nil {
		t.Fatalf("NewStoragePeer failed: %v", err)
	}
	defer sp.Stop()

	// Start watching
	if err := sp.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	initWait := 100 * time.Millisecond
	if runtime.GOOS == "darwin" {
		initWait = 250 * time.Millisecond
	}
	time.Sleep(initWait)

	// Create a hidden file
	hiddenFile := filepath.Join(dir, ".hidden.txt")
	if err := os.WriteFile(hiddenFile, []byte("Hidden"), 0644); err != nil {
		t.Fatalf("Failed to write hidden file: %v", err)
	}

	// Wait for potential dispatch
	processWait := 500 * time.Millisecond
	if runtime.GOOS == "darwin" {
		processWait = 1000 * time.Millisecond
	}
	time.Sleep(processWait)

	// Should not have dispatched hidden file
	if mock.callCount() > 0 {
		t.Error("Hidden files should not be dispatched")
	}
}

// TestIsBinaryData tests binary detection
func TestIsBinaryData(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name:     "text data",
			data:     []byte("Hello, World!"),
			expected: false,
		},
		{
			name:     "binary data with null byte",
			data:     []byte{0x00, 0x01, 0x02, 0x03},
			expected: true,
		},
		{
			name:     "JSON data",
			data:     []byte(`{"key": "value"}`),
			expected: false,
		},
		{
			name:     "empty data",
			data:     []byte{},
			expected: false,
		},
		{
			name:     "binary in middle",
			data:     []byte("text\x00more"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBinaryData(tt.data)
			if result != tt.expected {
				t.Errorf("isBinaryData() = %v, want %v", result, tt.expected)
			}
		})
	}
}
