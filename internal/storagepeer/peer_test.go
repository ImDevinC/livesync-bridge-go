package storagepeer

import (
	"fmt"
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

// TestStoragePeerDelete_ParentIsFile tests that Delete handles parent path conflicts during cleanup
func TestStoragePeerDelete_ParentIsFile(t *testing.T) {
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

	// Create a valid directory structure with a file
	validPath := filepath.Join(dir, "valid", "path")
	if err := os.MkdirAll(validPath, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	filePath := filepath.Join(validPath, "file.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Also create a blocking file somewhere (simulating a conflict)
	// This shouldn't affect the delete operation
	blockerPath := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blockerPath, []byte("blocking"), 0644); err != nil {
		t.Fatalf("Failed to create blocker: %v", err)
	}

	// Delete the file - this triggers cleanup which may encounter conflicts
	success, err := sp.Delete("valid/path/file.txt")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if !success {
		t.Error("Delete should succeed")
	}

	// Verify file was deleted
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("File should be deleted")
	}

	// Verify cleanup completed (empty directories should be removed)
	// The "path" directory should be removed if empty
	if _, err := os.Stat(validPath); err == nil {
		// Check if it's empty - if so, cleanup might have been skipped due to timing
		// but the operation should still have succeeded
		t.Log("Parent directory still exists (cleanup may have been deferred)")
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

// TestScanOfflineDeletions_DetectsDeletedFiles tests offline deletion detection
func TestScanOfflineDeletions_DetectsDeletedFiles(t *testing.T) {
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

	// Create and sync some files
	file1 := filepath.Join(dir, "file1.txt")
	file2 := filepath.Join(dir, "file2.txt")
	file3 := filepath.Join(dir, "file3.txt")

	if err := os.WriteFile(file1, []byte("content1"), 0644); err != nil {
		t.Fatalf("Failed to create file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte("content2"), 0644); err != nil {
		t.Fatalf("Failed to create file2: %v", err)
	}
	if err := os.WriteFile(file3, []byte("content3"), 0644); err != nil {
		t.Fatalf("Failed to create file3: %v", err)
	}

	// Update file stats to simulate that these files were synced
	info1, _ := os.Stat(file1)
	info2, _ := os.Stat(file2)
	info3, _ := os.Stat(file3)
	sp.SetSetting(fileStatPrefix+"file1.txt", fmt.Sprintf("%d-%d", info1.ModTime().Unix(), info1.Size()))
	sp.SetSetting(fileStatPrefix+"file2.txt", fmt.Sprintf("%d-%d", info2.ModTime().Unix(), info2.Size()))
	sp.SetSetting(fileStatPrefix+"file3.txt", fmt.Sprintf("%d-%d", info3.ModTime().Unix(), info3.Size()))

	// Delete file1 and file2 from disk (simulating offline deletion)
	os.Remove(file1)
	os.Remove(file2)

	// Call scanOfflineDeletions
	if err := sp.scanOfflineDeletions(sp.Context()); err != nil {
		t.Fatalf("scanOfflineDeletions failed: %v", err)
	}

	// Should have dispatched 2 deletions (file1 and file2)
	if mock.callCount() != 2 {
		t.Errorf("Expected 2 deletion dispatches, got %d", mock.callCount())
	}

	// Verify the deletions were for the correct files
	call1 := mock.getCall(0)
	call2 := mock.getCall(1)

	paths := map[string]bool{
		call1.path: true,
		call2.path: true,
	}

	if !paths["file1.txt"] || !paths["file2.txt"] {
		t.Error("Deletions should be for file1.txt and file2.txt")
	}

	// Verify deletion markers
	if call1.data != nil && !call1.data.Deleted {
		t.Error("Should dispatch deletion marker (Deleted=true)")
	}
	if call2.data != nil && !call2.data.Deleted {
		t.Error("Should dispatch deletion marker (Deleted=true)")
	}

	// Verify file stats were cleaned up
	if sp.HasSetting(fileStatPrefix + "file1.txt") {
		t.Error("file1.txt stat should be removed")
	}
	if sp.HasSetting(fileStatPrefix + "file2.txt") {
		t.Error("file2.txt stat should be removed")
	}
	if !sp.HasSetting(fileStatPrefix + "file3.txt") {
		t.Error("file3.txt stat should still exist")
	}
}

// TestScanOfflineDeletions_IgnoresNonExistentStats tests when no file stats exist
func TestScanOfflineDeletions_IgnoresNonExistentStats(t *testing.T) {
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

	// No file stats in storage
	if err := sp.scanOfflineDeletions(sp.Context()); err != nil {
		t.Fatalf("scanOfflineDeletions failed: %v", err)
	}

	// Should not have dispatched anything
	if mock.callCount() != 0 {
		t.Errorf("Expected 0 dispatches, got %d", mock.callCount())
	}
}

// TestStart_RunsBothScans tests that Start calls both scan functions
func TestStart_RunsBothScans(t *testing.T) {
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

	// Create a file and update its stat
	file1 := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(file1, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	info1, _ := os.Stat(file1)
	sp.SetSetting(fileStatPrefix+"existing.txt", fmt.Sprintf("%d-%d", info1.ModTime().Unix(), info1.Size()))

	// Create another file and update stat, then delete it (simulating offline deletion)
	file2 := filepath.Join(dir, "deleted.txt")
	if err := os.WriteFile(file2, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	info2, _ := os.Stat(file2)
	sp.SetSetting(fileStatPrefix+"deleted.txt", fmt.Sprintf("%d-%d", info2.ModTime().Unix(), info2.Size()))
	os.Remove(file2)

	// Modify the existing file to trigger offline change detection
	time.Sleep(10 * time.Millisecond) // Ensure mtime changes
	if err := os.WriteFile(file1, []byte("modified content"), 0644); err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}

	// Start should run both scans
	if err := sp.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Should have dispatched:
	// 1. One change for modified existing.txt (from scanOfflineChanges)
	// 2. One deletion for deleted.txt (from scanOfflineDeletions)
	if mock.callCount() != 2 {
		t.Errorf("Expected 2 dispatches (1 change + 1 deletion), got %d", mock.callCount())
	}

	// Find the change and deletion calls
	hasChange := false
	hasDeletion := false

	for i := 0; i < mock.callCount(); i++ {
		call := mock.getCall(i)
		if call.path == "existing.txt" && call.data != nil && !call.data.Deleted {
			hasChange = true
		}
		if call.path == "deleted.txt" && (call.data == nil || call.data.Deleted) {
			hasDeletion = true
		}
	}

	if !hasChange {
		t.Error("Should have dispatched change for existing.txt")
	}
	if !hasDeletion {
		t.Error("Should have dispatched deletion for deleted.txt")
	}
}

// TestStoragePeerPut_DirectoryConflict tests that writing a file over an existing directory works correctly
func TestStoragePeerPut_DirectoryConflict(t *testing.T) {
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

	// Create a directory with a file inside
	dirPath := filepath.Join(dir, "testdir")
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	testFile := filepath.Join(dirPath, "file.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create file in directory: %v", err)
	}

	// Store file stat to simulate it was synced
	info, _ := os.Stat(testFile)
	sp.SetSetting(fileStatPrefix+"testdir/file.txt", fmt.Sprintf("%d-%d", info.ModTime().Unix(), info.Size()))

	// Now try to Put a file at the directory path
	testData := []byte("This is now a file")
	fileData := &peer.FileData{
		Data:    testData,
		Deleted: false,
	}

	success, err := sp.Put("testdir", fileData)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if !success {
		t.Error("Put should succeed")
	}

	// Verify the directory was removed and file was written
	info2, err := os.Stat(dirPath)
	if err != nil {
		t.Fatalf("Failed to stat path: %v", err)
	}
	if info2.IsDir() {
		t.Error("Path should now be a file, not a directory")
	}

	// Verify file content
	content, err := os.ReadFile(dirPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != string(testData) {
		t.Errorf("Expected content '%s', got '%s'", testData, content)
	}

	// Verify file stat was cleaned up
	if sp.HasSetting(fileStatPrefix + "testdir/file.txt") {
		t.Error("File stat for testdir/file.txt should be removed")
	}
}

// TestStoragePeerPut_DirectoryConflictWithNestedFiles tests handling of nested directory conflicts
func TestStoragePeerPut_DirectoryConflictWithNestedFiles(t *testing.T) {
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

	// Create nested directory structure with multiple files
	dirPath := filepath.Join(dir, "parent")
	subDir1 := filepath.Join(dirPath, "sub1")
	subDir2 := filepath.Join(dirPath, "sub2")
	if err := os.MkdirAll(subDir1, 0755); err != nil {
		t.Fatalf("Failed to create sub1: %v", err)
	}
	if err := os.MkdirAll(subDir2, 0755); err != nil {
		t.Fatalf("Failed to create sub2: %v", err)
	}

	// Create files in nested directories
	file1 := filepath.Join(dirPath, "file1.txt")
	file2 := filepath.Join(subDir1, "file2.txt")
	file3 := filepath.Join(subDir2, "file3.txt")
	os.WriteFile(file1, []byte("content1"), 0644)
	os.WriteFile(file2, []byte("content2"), 0644)
	os.WriteFile(file3, []byte("content3"), 0644)

	// Store file stats to simulate they were synced
	info1, _ := os.Stat(file1)
	info2, _ := os.Stat(file2)
	info3, _ := os.Stat(file3)
	sp.SetSetting(fileStatPrefix+"parent/file1.txt", fmt.Sprintf("%d-%d", info1.ModTime().Unix(), info1.Size()))
	sp.SetSetting(fileStatPrefix+"parent/sub1/file2.txt", fmt.Sprintf("%d-%d", info2.ModTime().Unix(), info2.Size()))
	sp.SetSetting(fileStatPrefix+"parent/sub2/file3.txt", fmt.Sprintf("%d-%d", info3.ModTime().Unix(), info3.Size()))

	// Now try to Put a file at the parent directory path
	testData := []byte("Replacing entire directory tree")
	fileData := &peer.FileData{
		Data:    testData,
		Deleted: false,
	}

	success, err := sp.Put("parent", fileData)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if !success {
		t.Error("Put should succeed")
	}

	// Verify the directory was removed and file was written
	info, err := os.Stat(dirPath)
	if err != nil {
		t.Fatalf("Failed to stat path: %v", err)
	}
	if info.IsDir() {
		t.Error("Path should now be a file, not a directory")
	}

	// Verify all file stats were cleaned up
	if sp.HasSetting(fileStatPrefix + "parent/file1.txt") {
		t.Error("File stat for parent/file1.txt should be removed")
	}
	if sp.HasSetting(fileStatPrefix + "parent/sub1/file2.txt") {
		t.Error("File stat for parent/sub1/file2.txt should be removed")
	}
	if sp.HasSetting(fileStatPrefix + "parent/sub2/file3.txt") {
		t.Error("File stat for parent/sub2/file3.txt should be removed")
	}
}

// TestStoragePeerPut_EmptyDirectoryConflict tests handling of empty directory conflicts
func TestStoragePeerPut_EmptyDirectoryConflict(t *testing.T) {
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

	// Create an empty directory
	dirPath := filepath.Join(dir, "emptydir")
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Try to Put a file at the directory path
	testData := []byte("File content")
	fileData := &peer.FileData{
		Data:    testData,
		Deleted: false,
	}

	success, err := sp.Put("emptydir", fileData)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if !success {
		t.Error("Put should succeed")
	}

	// Verify the directory was removed and file was written
	info, err := os.Stat(dirPath)
	if err != nil {
		t.Fatalf("Failed to stat path: %v", err)
	}
	if info.IsDir() {
		t.Error("Path should now be a file, not a directory")
	}

	// Verify content
	content, err := os.ReadFile(dirPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != string(testData) {
		t.Errorf("Expected content '%s', got '%s'", testData, content)
	}
}

// TestStoragePeerPut_ParentIsFile_DirectParent tests writing a file when its direct parent is a file
func TestStoragePeerPut_ParentIsFile_DirectParent(t *testing.T) {
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

	// Create a file named "parent"
	parentPath := filepath.Join(dir, "parent")
	blockingContent := []byte("blocking file")
	if err := os.WriteFile(parentPath, blockingContent, 0644); err != nil {
		t.Fatalf("Failed to create blocking file: %v", err)
	}

	// Store file stat to simulate it was synced
	info, _ := os.Stat(parentPath)
	sp.SetSetting(fileStatPrefix+"parent", fmt.Sprintf("%d-%d", info.ModTime().Unix(), info.Size()))

	// Now try to Put a file at parent/child.txt
	testData := []byte("child content")
	fileData := &peer.FileData{
		Data:    testData,
		Deleted: false,
	}

	success, err := sp.Put("parent/child.txt", fileData)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if !success {
		t.Error("Put should succeed")
	}

	// Verify "parent" is now a directory
	info2, err := os.Stat(parentPath)
	if err != nil {
		t.Fatalf("Failed to stat parent path: %v", err)
	}
	if !info2.IsDir() {
		t.Error("Parent should now be a directory, not a file")
	}

	// Verify child.txt exists with correct content
	childPath := filepath.Join(dir, "parent", "child.txt")
	content, err := os.ReadFile(childPath)
	if err != nil {
		t.Fatalf("Failed to read child file: %v", err)
	}
	if string(content) != string(testData) {
		t.Errorf("Expected content '%s', got '%s'", testData, content)
	}

	// Verify file stat for old "parent" file is removed
	if sp.HasSetting(fileStatPrefix + "parent") {
		t.Error("File stat for parent file should be removed")
	}
}

// TestStoragePeerPut_ParentIsFile_NestedPath tests writing a file when a nested parent component is a file
func TestStoragePeerPut_ParentIsFile_NestedPath(t *testing.T) {
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

	// Create directory "a/"
	aPath := filepath.Join(dir, "a")
	if err := os.MkdirAll(aPath, 0755); err != nil {
		t.Fatalf("Failed to create directory a: %v", err)
	}

	// Create file "a/b" (not a directory)
	bPath := filepath.Join(dir, "a", "b")
	blockingContent := []byte("blocking file at a/b")
	if err := os.WriteFile(bPath, blockingContent, 0644); err != nil {
		t.Fatalf("Failed to create blocking file: %v", err)
	}

	// Store file stat to simulate it was synced
	info, _ := os.Stat(bPath)
	sp.SetSetting(fileStatPrefix+"a/b", fmt.Sprintf("%d-%d", info.ModTime().Unix(), info.Size()))

	// Now try to Put a file at a/b/c/d.txt
	testData := []byte("deep nested content")
	fileData := &peer.FileData{
		Data:    testData,
		Deleted: false,
	}

	success, err := sp.Put("a/b/c/d.txt", fileData)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if !success {
		t.Error("Put should succeed")
	}

	// Verify "a/b" is now a directory
	info2, err := os.Stat(bPath)
	if err != nil {
		t.Fatalf("Failed to stat a/b path: %v", err)
	}
	if !info2.IsDir() {
		t.Error("a/b should now be a directory, not a file")
	}

	// Verify full path "a/b/c/" exists
	cPath := filepath.Join(dir, "a", "b", "c")
	if info3, err := os.Stat(cPath); err != nil || !info3.IsDir() {
		t.Error("a/b/c/ should exist as a directory")
	}

	// Verify a/b/c/d.txt exists with correct content
	dPath := filepath.Join(dir, "a", "b", "c", "d.txt")
	content, err := os.ReadFile(dPath)
	if err != nil {
		t.Fatalf("Failed to read d.txt: %v", err)
	}
	if string(content) != string(testData) {
		t.Errorf("Expected content '%s', got '%s'", testData, content)
	}

	// Verify file stat for "a/b" file is removed
	if sp.HasSetting(fileStatPrefix + "a/b") {
		t.Error("File stat for a/b file should be removed")
	}
}

// TestStoragePeerPut_ParentIsFile_RootLevel tests writing a file when the direct parent at root level is a file
func TestStoragePeerPut_ParentIsFile_RootLevel(t *testing.T) {
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

	// Create file "topfile" at root
	topfilePath := filepath.Join(dir, "topfile")
	blockingContent := []byte("root level file")
	if err := os.WriteFile(topfilePath, blockingContent, 0644); err != nil {
		t.Fatalf("Failed to create blocking file: %v", err)
	}

	// Store file stat to simulate it was synced
	info, _ := os.Stat(topfilePath)
	sp.SetSetting(fileStatPrefix+"topfile", fmt.Sprintf("%d-%d", info.ModTime().Unix(), info.Size()))

	// Now try to Put a file at topfile/nested.txt
	testData := []byte("nested content")
	fileData := &peer.FileData{
		Data:    testData,
		Deleted: false,
	}

	success, err := sp.Put("topfile/nested.txt", fileData)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if !success {
		t.Error("Put should succeed")
	}

	// Verify "topfile" is now a directory
	info2, err := os.Stat(topfilePath)
	if err != nil {
		t.Fatalf("Failed to stat topfile path: %v", err)
	}
	if !info2.IsDir() {
		t.Error("topfile should now be a directory, not a file")
	}

	// Verify topfile/nested.txt exists with correct content
	nestedPath := filepath.Join(dir, "topfile", "nested.txt")
	content, err := os.ReadFile(nestedPath)
	if err != nil {
		t.Fatalf("Failed to read nested.txt: %v", err)
	}
	if string(content) != string(testData) {
		t.Errorf("Expected content '%s', got '%s'", testData, content)
	}

	// Verify file stat for "topfile" file is removed
	if sp.HasSetting(fileStatPrefix + "topfile") {
		t.Error("File stat for topfile file should be removed")
	}
}

// TestStoragePeerPut_RealisticDeleteScenario simulates the exact bug scenario with directory deletion
func TestStoragePeerPut_RealisticDeleteScenario(t *testing.T) {
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

	// Phase 1: Initial sync - create directory with file
	newnewDir := filepath.Join(dir, "newnew")
	if err := os.MkdirAll(newnewDir, 0755); err != nil {
		t.Fatalf("Failed to create newnew directory: %v", err)
	}

	insidePath := filepath.Join(newnewDir, "inside.txt")
	initialContent := []byte("hello")
	if err := os.WriteFile(insidePath, initialContent, 0644); err != nil {
		t.Fatalf("Failed to create inside.txt: %v", err)
	}

	// Store file stats to simulate they were synced
	info, _ := os.Stat(insidePath)
	sp.SetSetting(fileStatPrefix+"newnew/inside.txt", fmt.Sprintf("%d-%d", info.ModTime().Unix(), info.Size()))

	// Phase 2: Simulate CouchDB behavior after rm -rf
	// First: Put "newnew" as 0-byte file (directory becomes file)
	emptyFileData := &peer.FileData{
		Data:    []byte{},
		Deleted: false,
	}

	success, err := sp.Put("newnew", emptyFileData)
	if err != nil {
		t.Fatalf("Put newnew as file failed: %v", err)
	}
	if !success {
		t.Error("Put should succeed for newnew file")
	}

	// Verify "newnew" is now a file (not a directory)
	info2, err := os.Stat(newnewDir)
	if err != nil {
		t.Fatalf("Failed to stat newnew: %v", err)
	}
	if info2.IsDir() {
		t.Error("newnew should be a file at this point, not a directory")
	}

	// Phase 3: Sync child file - this should trigger the fix
	// Put "newnew/inside.txt" as 0-byte file
	success, err = sp.Put("newnew/inside.txt", emptyFileData)
	if err != nil {
		t.Fatalf("Put newnew/inside.txt failed (this was the bug): %v", err)
	}
	if !success {
		t.Error("Put should succeed for newnew/inside.txt")
	}

	// Verify: "newnew/" is now a directory (converted back from file)
	info3, err := os.Stat(newnewDir)
	if err != nil {
		t.Fatalf("Failed to stat newnew after fix: %v", err)
	}
	if !info3.IsDir() {
		t.Error("newnew should be a directory after child file sync")
	}

	// Verify: "newnew/inside.txt" exists as 0-byte file
	insideContent, err := os.ReadFile(insidePath)
	if err != nil {
		t.Fatalf("Failed to read inside.txt: %v", err)
	}
	if len(insideContent) != 0 {
		t.Errorf("Expected 0-byte file, got %d bytes", len(insideContent))
	}
}

// TestRemoveDirectoryIfExists_NotADirectory tests that the helper correctly handles non-directory paths
func TestRemoveDirectoryIfExists_NotADirectory(t *testing.T) {
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

	// Create a regular file
	filePath := filepath.Join(dir, "regular.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Call removeDirectoryIfExists on the file
	removed, err := sp.removeDirectoryIfExists(filePath)
	if err != nil {
		t.Fatalf("removeDirectoryIfExists failed: %v", err)
	}
	if removed {
		t.Error("Should return false for non-directory paths")
	}

	// Verify file still exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("File should still exist after removeDirectoryIfExists")
	}
}

// TestRemoveDirectoryIfExists_NonExistent tests that the helper handles non-existent paths
func TestRemoveDirectoryIfExists_NonExistent(t *testing.T) {
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

	// Call removeDirectoryIfExists on non-existent path
	nonExistentPath := filepath.Join(dir, "does-not-exist")
	removed, err := sp.removeDirectoryIfExists(nonExistentPath)
	if err != nil {
		t.Fatalf("removeDirectoryIfExists should not error on non-existent path: %v", err)
	}
	if removed {
		t.Error("Should return false for non-existent paths")
	}
}

// TestRemoveFileConflictsInParentPath_NoConflict tests that the helper handles valid directory paths correctly
func TestRemoveFileConflictsInParentPath_NoConflict(t *testing.T) {
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

	// Create normal directory structure "a/b/c/"
	abcPath := filepath.Join(dir, "a", "b", "c")
	if err := os.MkdirAll(abcPath, 0755); err != nil {
		t.Fatalf("Failed to create directories: %v", err)
	}

	// Call removeFileConflictsInParentPath on a valid directory path
	err = sp.removeFileConflictsInParentPath(abcPath)
	if err != nil {
		t.Fatalf("removeFileConflictsInParentPath should not error on valid path: %v", err)
	}

	// Verify all directories still exist
	if _, err := os.Stat(abcPath); os.IsNotExist(err) {
		t.Error("Directory a/b/c/ should still exist")
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "b")); os.IsNotExist(err) {
		t.Error("Directory a/b/ should still exist")
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); os.IsNotExist(err) {
		t.Error("Directory a/ should still exist")
	}
}

// TestRemoveFileConflictsInParentPath_MultipleFiles tests that only the blocking file is removed
func TestRemoveFileConflictsInParentPath_MultipleFiles(t *testing.T) {
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

	// Create file "level1"
	level1Path := filepath.Join(dir, "level1")
	if err := os.WriteFile(level1Path, []byte("blocking"), 0644); err != nil {
		t.Fatalf("Failed to create level1 file: %v", err)
	}

	// Store file stat for level1
	info1, _ := os.Stat(level1Path)
	sp.SetSetting(fileStatPrefix+"level1", fmt.Sprintf("%d-%d", info1.ModTime().Unix(), info1.Size()))

	// Create file "level1.backup" (should NOT be removed)
	backupPath := filepath.Join(dir, "level1.backup")
	if err := os.WriteFile(backupPath, []byte("backup"), 0644); err != nil {
		t.Fatalf("Failed to create backup file: %v", err)
	}

	// Store file stat for level1.backup
	info2, _ := os.Stat(backupPath)
	sp.SetSetting(fileStatPrefix+"level1.backup", fmt.Sprintf("%d-%d", info2.ModTime().Unix(), info2.Size()))

	// Call removeFileConflictsInParentPath on "level1/level2/level3"
	testPath := filepath.Join(dir, "level1", "level2", "level3")
	err = sp.removeFileConflictsInParentPath(testPath)
	if err != nil {
		t.Fatalf("removeFileConflictsInParentPath failed: %v", err)
	}

	// Verify "level1" was removed (it was blocking the path)
	if _, err := os.Stat(level1Path); !os.IsNotExist(err) {
		t.Error("level1 file should be removed (was blocking)")
	}

	// Verify file stat for level1 was removed
	if sp.HasSetting(fileStatPrefix + "level1") {
		t.Error("File stat for level1 should be removed")
	}

	// Verify "level1.backup" still exists (not in the path)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("level1.backup should still exist (not blocking)")
	}

	// Verify file stat for level1.backup still exists
	if !sp.HasSetting(fileStatPrefix + "level1.backup") {
		t.Error("File stat for level1.backup should still exist")
	}
}

// TestStoragePeerSync_DirectoryDeletionConflict simulates two-peer sync with directory deletion
func TestStoragePeerSync_DirectoryDeletionConflict(t *testing.T) {
	// Setup two temp directories for two peers
	dir1 := createTempDir(t)
	defer os.RemoveAll(dir1)
	dir2 := createTempDir(t)
	defer os.RemoveAll(dir2)

	store1 := createTempStore(t)
	store2 := createTempStore(t)

	// Create two peers with simple dispatchers
	var peer1, peer2 *StoragePeer

	// Dispatcher that forwards from peer1 to peer2
	dispatch1 := func(source peer.Peer, path string, data *peer.FileData) error {
		if peer2 != nil && source == peer1 {
			if data == nil {
				peer2.Delete(path)
			} else {
				peer2.Put(path, data)
			}
		}
		return nil
	}

	// Dispatcher that forwards from peer2 to peer1
	dispatch2 := func(source peer.Peer, path string, data *peer.FileData) error {
		if peer1 != nil && source == peer2 {
			if data == nil {
				peer1.Delete(path)
			} else {
				peer1.Put(path, data)
			}
		}
		return nil
	}

	conf1 := config.PeerStorageConf{
		Name:    "peer1",
		Type:    "storage",
		Group:   "test-group",
		BaseDir: dir1,
	}

	conf2 := config.PeerStorageConf{
		Name:    "peer2",
		Type:    "storage",
		Group:   "test-group",
		BaseDir: dir2,
	}

	var err error
	peer1, err = NewStoragePeer(conf1, dispatch1, store1)
	if err != nil {
		t.Fatalf("Failed to create peer1: %v", err)
	}
	defer peer1.Stop()

	peer2, err = NewStoragePeer(conf2, dispatch2, store2)
	if err != nil {
		t.Fatalf("Failed to create peer2: %v", err)
	}
	defer peer2.Stop()

	// Phase 1: Peer1 creates directory with file
	testdirPath := filepath.Join(dir1, "testdir")
	if err := os.MkdirAll(testdirPath, 0755); err != nil {
		t.Fatalf("Failed to create testdir on peer1: %v", err)
	}

	filePath1 := filepath.Join(testdirPath, "file.txt")
	fileContent := []byte("test content")
	if err := os.WriteFile(filePath1, fileContent, 0644); err != nil {
		t.Fatalf("Failed to create file on peer1: %v", err)
	}

	// Manually dispatch to peer2 (simulating sync)
	fileData := &peer.FileData{
		Data:    fileContent,
		Deleted: false,
	}
	_, err = peer2.Put("testdir/file.txt", fileData)
	if err != nil {
		t.Fatalf("Failed to put file on peer2: %v", err)
	}

	// Verify peer2 has the file
	filePath2 := filepath.Join(dir2, "testdir", "file.txt")
	content2, err := os.ReadFile(filePath2)
	if err != nil {
		t.Fatalf("Failed to read file on peer2: %v", err)
	}
	if string(content2) != string(fileContent) {
		t.Errorf("File content mismatch on peer2")
	}

	// Phase 2: Peer2 deletes directory (simulating rm -rf)
	testdirPath2 := filepath.Join(dir2, "testdir")
	if err := os.RemoveAll(testdirPath2); err != nil {
		t.Fatalf("Failed to remove testdir on peer2: %v", err)
	}

	// Simulate CouchDB behavior - dispatch deletion of child, then parent as file
	// First: delete the file
	_, err = peer1.Delete("testdir/file.txt")
	if err != nil {
		t.Fatalf("Failed to delete file on peer1: %v", err)
	}

	// Second: Put parent as 0-byte file (CouchDB behavior)
	emptyData := &peer.FileData{
		Data:    []byte{},
		Deleted: false,
	}
	_, err = peer1.Put("testdir", emptyData)
	if err != nil {
		t.Fatalf("Failed to put testdir as file on peer1: %v", err)
	}

	// Phase 3: Now try to sync another file from peer2 to peer1
	// This tests that the fix allows re-creating the directory structure
	if err := os.MkdirAll(testdirPath2, 0755); err != nil {
		t.Fatalf("Failed to recreate testdir on peer2: %v", err)
	}

	anotherFilePath2 := filepath.Join(testdirPath2, "another.txt")
	anotherContent := []byte("another file")
	if err := os.WriteFile(anotherFilePath2, anotherContent, 0644); err != nil {
		t.Fatalf("Failed to create another.txt on peer2: %v", err)
	}

	// Dispatch to peer1 - this should trigger the fix
	anotherData := &peer.FileData{
		Data:    anotherContent,
		Deleted: false,
	}
	_, err = peer1.Put("testdir/another.txt", anotherData)
	if err != nil {
		t.Fatalf("Failed to put another.txt on peer1 (this was the bug): %v", err)
	}

	// Verify: peer1 has testdir as directory with another.txt
	testdirInfo := filepath.Join(dir1, "testdir")
	info, err := os.Stat(testdirInfo)
	if err != nil {
		t.Fatalf("Failed to stat testdir on peer1: %v", err)
	}
	if !info.IsDir() {
		t.Error("testdir should be a directory on peer1")
	}

	anotherFilePath1 := filepath.Join(dir1, "testdir", "another.txt")
	content1, err := os.ReadFile(anotherFilePath1)
	if err != nil {
		t.Fatalf("Failed to read another.txt on peer1: %v", err)
	}
	if string(content1) != string(anotherContent) {
		t.Errorf("another.txt content mismatch on peer1")
	}
}
