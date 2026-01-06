package storagepeer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/imdevinc/livesync-bridge/internal/config"
	"github.com/imdevinc/livesync-bridge/internal/peer"
	"github.com/imdevinc/livesync-bridge/internal/storage"
	"github.com/imdevinc/livesync-bridge/internal/util"
)

const (
	// Debounce time for file changes (milliseconds)
	debounceDelay = 250 * time.Millisecond

	// Settings keys
	fileStatPrefix = "file-stat-"
)

// isBinaryData checks if data contains binary (non-text) content
func isBinaryData(data []byte) bool {
	// Check first 8KB for null bytes or other binary markers
	checkLen := len(data)
	if checkLen > 8192 {
		checkLen = 8192
	}

	for i := 0; i < checkLen; i++ {
		// Null byte indicates binary
		if data[i] == 0 {
			return true
		}
	}

	return false
}

// StoragePeer implements a filesystem-based peer that watches for changes
// and synchronizes files with other peers through the hub.
type StoragePeer struct {
	*peer.BasePeer

	rootDir string            // Absolute path to watched directory
	watcher *fsnotify.Watcher // File system watcher

	// Debouncing state
	mu            sync.Mutex
	pendingEvents map[string]*time.Timer // path -> timer

	startOnce sync.Once
	stopOnce  sync.Once
}

// NewStoragePeer creates a new storage peer that monitors a filesystem directory.
func NewStoragePeer(conf config.PeerStorageConf, dispatcher peer.DispatchFunc, store *storage.Store) (*StoragePeer, error) {
	// Validate and resolve root directory
	rootDir := conf.BaseDir
	if rootDir == "" {
		return nil, fmt.Errorf("storage peer %s: baseDir cannot be empty", conf.Name)
	}

	absPath, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("storage peer %s: failed to resolve baseDir: %w", conf.Name, err)
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return nil, fmt.Errorf("storage peer %s: failed to create directory: %w", conf.Name, err)
	}

	// Create base peer with empty baseDir (storage peer uses rootDir as physical location)
	basePeer, err := peer.NewBasePeer(conf.Name, "storage", conf.Group, "", dispatcher, store)
	if err != nil {
		return nil, fmt.Errorf("storage peer %s: failed to create base peer: %w", conf.Name, err)
	}

	// Create watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("storage peer %s: failed to create watcher: %w", conf.Name, err)
	}

	sp := &StoragePeer{
		BasePeer:      basePeer,
		rootDir:       absPath,
		watcher:       watcher,
		pendingEvents: make(map[string]*time.Timer),
	}

	return sp, nil
}

// Start begins watching the filesystem and scanning for offline changes.
func (sp *StoragePeer) Start() error {
	var startErr error
	sp.startOnce.Do(func() {
		sp.LogInfo("Starting storage peer", "rootDir", sp.rootDir)

		// Scan for offline changes first
		if err := sp.scanOfflineChanges(sp.Context()); err != nil {
			startErr = fmt.Errorf("failed to scan offline changes: %w", err)
			return
		}

		// Scan for offline deletions
		if err := sp.scanOfflineDeletions(sp.Context()); err != nil {
			startErr = fmt.Errorf("failed to scan offline deletions: %w", err)
			return
		}

		// Start watching directory tree
		if err := sp.watchDirectoryTree(sp.rootDir); err != nil {
			startErr = fmt.Errorf("failed to watch directory: %w", err)
			return
		}

		// Start event processing goroutine
		go sp.processEvents(sp.Context())

		sp.LogInfo("Storage peer started successfully")
	})

	return startErr
}

// Stop stops the storage peer and cleans up resources.
func (sp *StoragePeer) Stop() error {
	var stopErr error
	sp.stopOnce.Do(func() {
		sp.LogInfo("Stopping storage peer")

		// Cancel pending timers
		sp.mu.Lock()
		for _, timer := range sp.pendingEvents {
			timer.Stop()
		}
		sp.pendingEvents = make(map[string]*time.Timer)
		sp.mu.Unlock()

		// Close watcher
		if sp.watcher != nil {
			stopErr = sp.watcher.Close()
		}

		// Call base peer Stop
		if err := sp.BasePeer.Stop(); err != nil && stopErr == nil {
			stopErr = err
		}

		sp.LogInfo("Storage peer stopped")
	})

	return stopErr
}

// Put writes a file to the storage directory.
func (sp *StoragePeer) Put(path string, data *peer.FileData) (bool, error) {
	localPath := sp.ToLocalPath(path)
	storagePath := filepath.Join(sp.rootDir, localPath)

	sp.LogReceive("file", "path", path)

	// Check if repeating
	if sp.IsRepeating(path, data) {
		sp.LogDebug("Skipping repeated file", "path", path)
		return false, nil
	}

	// Create parent directory first, removing any file conflicts in the path
	dir := filepath.Dir(storagePath)
	if err := sp.removeFileConflictsInParentPath(dir); err != nil {
		return false, fmt.Errorf("failed to resolve parent path conflicts for %s: %w", dir, err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Check if path exists as a directory and remove it if so
	removed, err := sp.removeDirectoryIfExists(storagePath)
	if err != nil {
		return false, fmt.Errorf("failed to handle directory conflict at %s: %w", storagePath, err)
	}
	if removed {
		sp.LogInfo("Resolved directory conflict", "path", path)
	}

	// Write file with retry logic
	err = util.Retry(sp.Context(), util.DefaultRetryConfig(), func() error {
		return os.WriteFile(storagePath, data.Data, 0644)
	}, nil)

	if err != nil {
		return false, fmt.Errorf("failed to write file %s: %w", storagePath, err)
	}

	// Update file stat in storage
	if err := sp.updateFileStat(localPath, storagePath); err != nil {
		sp.LogWarn("Failed to update file stat", "path", localPath, "error", err)
	}

	// Mark as processed to prevent echo
	sp.MarkProcessed(path, data)

	return true, nil
}

// Delete removes a file from the storage directory.
func (sp *StoragePeer) Delete(path string) (bool, error) {
	localPath := sp.ToLocalPath(path)
	storagePath := filepath.Join(sp.rootDir, localPath)

	sp.LogReceive("deletion", "path", path)

	// Check if repeating
	if sp.IsRepeating(path, nil) {
		sp.LogDebug("Skipping repeated deletion", "path", path)
		return false, nil
	}

	// Check if file exists
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		sp.LogDebug("File already deleted", "path", path)
		return false, nil
	}

	// Delete file with retry logic
	err := util.Retry(sp.Context(), util.DefaultRetryConfig(), func() error {
		return os.Remove(storagePath)
	}, nil)

	if err != nil {
		return false, fmt.Errorf("failed to delete file %s: %w", storagePath, err)
	}

	// Remove file stat from storage
	if err := sp.DeleteSetting(fileStatPrefix + localPath); err != nil {
		sp.LogWarn("Failed to delete file stat", "path", localPath, "error", err)
	}

	// Mark as processed
	sp.MarkProcessed(path, nil)

	// Try to clean up empty parent directories
	// First ensure parent path is valid (no file conflicts)
	parentDir := filepath.Dir(storagePath)
	if err := sp.removeFileConflictsInParentPath(parentDir); err != nil {
		sp.LogWarn("Failed to resolve parent path conflicts during cleanup", "path", parentDir, "error", err)
		// Continue with operation even if parent path resolution fails
	}
	sp.cleanupEmptyDirs(parentDir)

	return true, nil
}

// Get reads a file from the storage directory.
func (sp *StoragePeer) Get(path string) (*peer.FileData, error) {
	localPath := sp.ToLocalPath(path)
	storagePath := filepath.Join(sp.rootDir, localPath)

	// Check if file exists
	info, err := os.Stat(storagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", path)
		}
		return nil, fmt.Errorf("failed to stat file %s: %w", storagePath, err)
	}

	// Read file
	data, err := os.ReadFile(storagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", storagePath, err)
	}

	return &peer.FileData{
		Data:    data,
		MTime:   info.ModTime(),
		Size:    info.Size(),
		Deleted: false,
	}, nil
}

// watchDirectoryTree recursively adds all directories to the watcher.
func (sp *StoragePeer) watchDirectoryTree(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Only watch directories
		if d.IsDir() {
			// Skip hidden directories (starting with .)
			if strings.HasPrefix(d.Name(), ".") && path != root {
				return filepath.SkipDir
			}

			if err := sp.watcher.Add(path); err != nil {
				return fmt.Errorf("failed to watch %s: %w", path, err)
			}
			sp.LogDebug("Watching directory", "path", path)
		}

		return nil
	})
}

// processEvents handles file system events with debouncing.
func (sp *StoragePeer) processEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-sp.watcher.Events:
			if !ok {
				return
			}

			sp.handleEvent(ctx, event)

		case err, ok := <-sp.watcher.Errors:
			if !ok {
				return
			}
			sp.LogError("Watcher error", "error", err)
		}
	}
}

// handleEvent processes a single file system event with debouncing.
func (sp *StoragePeer) handleEvent(ctx context.Context, event fsnotify.Event) {
	// Skip if the path is outside our root (shouldn't happen)
	if !strings.HasPrefix(event.Name, sp.rootDir) {
		return
	}

	// Get relative path
	relPath, err := filepath.Rel(sp.rootDir, event.Name)
	if err != nil {
		sp.LogError("Failed to get relative path", "path", event.Name, "error", err)
		return
	}

	// Skip hidden files
	if strings.HasPrefix(filepath.Base(relPath), ".") {
		return
	}

	// Handle directory creation separately
	if event.Op&fsnotify.Create == fsnotify.Create {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			// Add new directory to watcher
			if err := sp.watchDirectoryTree(event.Name); err != nil {
				sp.LogError("Failed to watch new directory", "path", event.Name, "error", err)
			}
			return
		}
	}

	// Debounce file events
	sp.debounceEvent(ctx, relPath)
}

// debounceEvent adds or resets a timer for the given path.
func (sp *StoragePeer) debounceEvent(ctx context.Context, localPath string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	// Cancel existing timer if any
	if timer, exists := sp.pendingEvents[localPath]; exists {
		timer.Stop()
	}

	// Create new timer
	sp.pendingEvents[localPath] = time.AfterFunc(debounceDelay, func() {
		sp.processChange(ctx, localPath)

		// Clean up timer
		sp.mu.Lock()
		delete(sp.pendingEvents, localPath)
		sp.mu.Unlock()
	})
}

// processChange handles a debounced file change.
func (sp *StoragePeer) processChange(ctx context.Context, localPath string) {
	storagePath := filepath.Join(sp.rootDir, localPath)
	globalPath := sp.ToGlobalPath(localPath)

	// Check if file exists
	info, err := os.Stat(storagePath)
	if os.IsNotExist(err) {
		// File was deleted
		sp.dispatchDeletion(globalPath)
		return
	}

	if err != nil {
		sp.LogError("Failed to stat file", "path", localPath, "error", err)
		return
	}

	// Skip directories
	if info.IsDir() {
		return
	}

	// Read and dispatch file
	sp.dispatchFile(localPath, storagePath, info)
}

// dispatchFile reads a file and dispatches it to other peers.
func (sp *StoragePeer) dispatchFile(localPath, storagePath string, info fs.FileInfo) {
	// Read file
	data, err := os.ReadFile(storagePath)
	if err != nil {
		sp.LogError("Failed to read file", "path", localPath, "error", err)
		return
	}

	globalPath := sp.ToGlobalPath(localPath)

	// Create file data
	fileData := &peer.FileData{
		Data:    data,
		MTime:   info.ModTime(),
		Size:    info.Size(),
		Deleted: false,
	}

	// Dispatch using DispatchFrom with self as source
	// This allows the hub to properly exclude this peer from receiving the change
	if err := sp.DispatchFrom(sp, globalPath, fileData); err != nil {
		sp.LogError("Failed to dispatch file", "path", globalPath, "error", err)
		return
	}

	sp.LogSend("file", "file", globalPath)

	// Update file stat
	if err := sp.updateFileStat(localPath, storagePath); err != nil {
		sp.LogWarn("Failed to update file stat", "path", localPath, "error", err)
	}
}

// dispatchDeletion dispatches a file deletion to other peers.
func (sp *StoragePeer) dispatchDeletion(globalPath string) {
	// Create deletion marker
	fileData := &peer.FileData{
		Deleted: true,
	}

	// Dispatch using DispatchFrom with self as source
	if err := sp.DispatchFrom(sp, globalPath, fileData); err != nil {
		sp.LogError("Failed to dispatch deletion", "path", globalPath, "error", err)
		return
	}

	sp.LogSend("deletion", "path", globalPath)

	// Remove file stat
	localPath := sp.ToLocalPath(globalPath)
	if err := sp.DeleteSetting(fileStatPrefix + localPath); err != nil {
		sp.LogWarn("Failed to delete file stat", "path", localPath, "error", err)
	}
}

// scanOfflineChanges walks the directory tree and detects changes that occurred while offline.
func (sp *StoragePeer) scanOfflineChanges(ctx context.Context) error {
	sp.LogInfo("Scanning for offline changes")

	changeCount := 0

	// Walk directory tree
	err := filepath.WalkDir(sp.rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Skip hidden files and directories
		if strings.HasPrefix(d.Name(), ".") && path != sp.rootDir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(sp.rootDir, path)
		if err != nil {
			return err
		}

		// Get file info
		info, err := d.Info()
		if err != nil {
			sp.LogWarn("Failed to get file info", "path", relPath, "error", err)
			return nil
		}

		// Check if file has changed
		if sp.hasFileChanged(relPath, path, info) {
			sp.dispatchFile(relPath, path, info)
			changeCount++
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	sp.LogInfo("Offline scan complete", "changed", changeCount)
	return nil
}

// scanOfflineDeletions detects files that were deleted while offline
func (sp *StoragePeer) scanOfflineDeletions(ctx context.Context) error {
	sp.LogInfo("Scanning for offline deletions")

	deletionCount := 0

	// Collect paths to delete (don't modify storage during iteration)
	var pathsToDelete []string

	// Build the full prefix including peer namespace
	// Keys are stored as: {name}-{type}-{baseDir}-{key}
	// For storage peer, baseDir is empty, so it's: {name}-{type}--{key}
	// We need to iterate with the full peer prefix + file-stat-
	fullPrefix := fmt.Sprintf("%s-%s--%s", sp.Name(), sp.Type(), fileStatPrefix)

	// Iterate all stored file stats to find deleted files
	err := sp.Store().IteratePrefix(fullPrefix, func(key, value string) error {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Extract local path from key (remove the full peer prefix)
		// Key format: {name}-{type}--file-stat-{localPath}
		localPath := strings.TrimPrefix(key, fullPrefix)
		storagePath := filepath.Join(sp.rootDir, localPath)

		// Check if file still exists
		if _, err := os.Stat(storagePath); os.IsNotExist(err) {
			// File was deleted while offline
			pathsToDelete = append(pathsToDelete, localPath)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to scan for deletions: %w", err)
	}

	// Now process the deletions
	for _, localPath := range pathsToDelete {
		globalPath := sp.ToGlobalPath(localPath)
		sp.LogDebug("Detected offline deletion", "path", globalPath)

		// Dispatch deletion
		sp.dispatchDeletion(globalPath)
		deletionCount++
	}

	sp.LogInfo("Offline deletion scan complete", "deleted", deletionCount)
	return nil
}

// hasFileChanged checks if a file has changed since last sync.
func (sp *StoragePeer) hasFileChanged(localPath, storagePath string, info fs.FileInfo) bool {
	// Get stored file stat
	storedStat, err := sp.GetSetting(fileStatPrefix + localPath)
	if err != nil || storedStat == "" {
		// No stored stat, consider it changed
		return true
	}

	// Current stat: mtime-size
	currentStat := fmt.Sprintf("%d-%d", info.ModTime().Unix(), info.Size())

	return storedStat != currentStat
}

// updateFileStat updates the stored file stat for a file.
func (sp *StoragePeer) updateFileStat(localPath, storagePath string) error {
	info, err := os.Stat(storagePath)
	if err != nil {
		return err
	}

	stat := fmt.Sprintf("%d-%d", info.ModTime().Unix(), info.Size())
	return sp.SetSetting(fileStatPrefix+localPath, stat)
}

// cleanupEmptyDirs removes empty parent directories up to the root.
func (sp *StoragePeer) cleanupEmptyDirs(dir string) {
	// Don't delete the root directory
	if dir == sp.rootDir || !strings.HasPrefix(dir, sp.rootDir) {
		return
	}

	// Check if directory is empty
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) > 0 {
		return
	}

	// Remove empty directory
	if err := os.Remove(dir); err != nil {
		sp.LogDebug("Failed to remove empty directory", "path", dir, "error", err)
		return
	}

	sp.LogDebug("Removed empty directory", "path", dir)

	// Recursively try to clean up parent
	sp.cleanupEmptyDirs(filepath.Dir(dir))
}

// removeDirectoryIfExists removes a directory and all its contents if the path is a directory.
// Returns true if a directory was removed, false otherwise.
func (sp *StoragePeer) removeDirectoryIfExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // Path doesn't exist, nothing to do
		}
		return false, err
	}

	// Not a directory, nothing to remove
	if !info.IsDir() {
		return false, nil
	}

	sp.LogWarn("Removing directory to resolve path conflict", "path", path)

	// Recursively remove directory and contents
	if err := os.RemoveAll(path); err != nil {
		return false, fmt.Errorf("failed to remove directory: %w", err)
	}

	// Clean up file stats for all files that were in this directory
	if err := sp.cleanupDirectoryStats(path); err != nil {
		sp.LogWarn("Failed to clean up directory stats", "path", path, "error", err)
	}

	sp.LogInfo("Removed directory due to path conflict", "path", path)
	return true, nil
}

// removeFileConflictsInParentPath removes any file that blocks the creation
// of a directory path. It walks from dirPath up towards rootDir, checking if
// any path component exists as a file. If found, the file is removed along
// with its file stat entry.
// Returns nil if no conflicts exist or after successfully resolving a conflict.
func (sp *StoragePeer) removeFileConflictsInParentPath(dirPath string) error {
	// Validate inputs
	if dirPath == "" || dirPath == sp.rootDir {
		return nil
	}

	// Ensure dirPath is within rootDir
	relPath, err := filepath.Rel(sp.rootDir, dirPath)
	if err != nil {
		return nil // Can't compute relative path, assume outside rootDir
	}
	if strings.HasPrefix(relPath, "..") {
		return nil // Path is outside rootDir
	}

	// Build list of path components from root to target
	components := []string{}
	current := dirPath
	for current != sp.rootDir && current != filepath.Dir(current) {
		components = append([]string{current}, components...) // Prepend to get root-first order
		current = filepath.Dir(current)
	}

	// Check each component from root towards target
	for _, checkPath := range components {
		info, err := os.Stat(checkPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Path doesn't exist yet, continue to next component
				continue
			}
			// Some other error - if it's a "not a directory" error, a parent is blocking
			// Continue checking parents to find the blocking file
			continue
		}

		// If it's a directory, continue to next component
		if info.IsDir() {
			continue
		}

		// Found a file blocking the path - remove it
		sp.LogWarn("Removing file to resolve parent path conflict", "path", checkPath)

		// Remove the file
		if err := os.Remove(checkPath); err != nil {
			return fmt.Errorf("failed to remove blocking file %s: %w", checkPath, err)
		}

		// Clean up file stat entry
		relToRoot, err := filepath.Rel(sp.rootDir, checkPath)
		if err == nil {
			if err := sp.DeleteSetting(fileStatPrefix + relToRoot); err != nil {
				sp.LogDebug("Failed to delete file stat", "path", relToRoot, "error", err)
			}
		}

		sp.LogInfo("Removed file blocking parent path", "path", checkPath)
		return nil // Conflict resolved, no need to check further
	}

	return nil // No conflicts found
}

// cleanupDirectoryStats removes all file stat entries for files within a directory.
func (sp *StoragePeer) cleanupDirectoryStats(dirPath string) error {
	// Get relative path from rootDir
	relPath, err := filepath.Rel(sp.rootDir, dirPath)
	if err != nil {
		return err
	}

	// Build the full prefix including peer namespace
	// Keys are stored as: {name}-{type}-{baseDir}-{key}
	// For storage peer, baseDir is empty, so it's: {name}-{type}--{key}
	fullPrefix := fmt.Sprintf("%s-%s--%s", sp.Name(), sp.Type(), fileStatPrefix)

	// Collect all keys to delete
	var keysToDelete []string
	err = sp.Store().IteratePrefix(fullPrefix, func(key, value string) error {
		// Extract local path from key
		localPath := strings.TrimPrefix(key, fullPrefix)

		// Check if this file was in the deleted directory
		if strings.HasPrefix(localPath, relPath+string(filepath.Separator)) || localPath == relPath {
			keysToDelete = append(keysToDelete, fileStatPrefix+localPath)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Delete all collected keys
	for _, key := range keysToDelete {
		if err := sp.DeleteSetting(key); err != nil {
			sp.LogDebug("Failed to delete stat", "key", key, "error", err)
		}
	}

	if len(keysToDelete) > 0 {
		sp.LogDebug("Cleaned up directory stats", "path", relPath, "count", len(keysToDelete))
	}

	return nil
}
