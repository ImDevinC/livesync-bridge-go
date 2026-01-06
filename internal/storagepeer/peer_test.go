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
