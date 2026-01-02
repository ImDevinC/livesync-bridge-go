package peer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/imdevinc/livesync-bridge/internal/storage"
	"github.com/imdevinc/livesync-bridge/internal/util"
)

// BasePeer provides common functionality for all peer implementations
type BasePeer struct {
	name       string
	peerType   string
	group      string
	baseDir    string
	dispatcher DispatchFunc
	cache      *util.Cache
	store      *storage.Store
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex
}

// NewBasePeer creates a new base peer with common functionality
func NewBasePeer(name, peerType, group, baseDir string, dispatcher DispatchFunc, store *storage.Store) (*BasePeer, error) {
	cache, err := util.NewCache(300) // Cache up to 300 file hashes
	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &BasePeer{
		name:       name,
		peerType:   peerType,
		group:      group,
		baseDir:    baseDir,
		dispatcher: dispatcher,
		cache:      cache,
		store:      store,
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

// Name returns the peer's unique name
func (b *BasePeer) Name() string {
	return b.name
}

// Type returns the peer type
func (b *BasePeer) Type() string {
	return b.peerType
}

// Group returns the peer's group
func (b *BasePeer) Group() string {
	return b.group
}

// Context returns the peer's context (for cancellation)
func (b *BasePeer) Context() context.Context {
	return b.ctx
}

// Store returns the underlying persistent storage
func (b *BasePeer) Store() *storage.Store {
	return b.store
}

// StorageKeyPrefix returns the storage key prefix for this peer
func (b *BasePeer) StorageKeyPrefix() string {
	return fmt.Sprintf("%s-%s-%s-", b.name, b.peerType, b.baseDir)
}

// DeleteSetting removes a peer-specific setting from persistent storage
func (b *BasePeer) DeleteSetting(key string) error {
	fullKey := b.getSettingKey(key)
	return b.store.Delete(fullKey)
}

// Stop cancels the peer's context
func (b *BasePeer) Stop() error {
	b.cancel()
	return nil
}

// ToLocalPath converts a global path to a local path with baseDir prefix
func (b *BasePeer) ToLocalPath(globalPath string) string {
	return util.ToLocalPath(globalPath, b.baseDir)
}

// ToGlobalPath converts a local path to a global path by removing baseDir prefix
func (b *BasePeer) ToGlobalPath(localPath string) string {
	return util.ToGlobalPath(localPath, b.baseDir)
}

// IsRepeating checks if this file change is a repeat (already processed)
// Uses hash-based deduplication to prevent infinite loops
func (b *BasePeer) IsRepeating(path string, data *FileData) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Compute hash of the data
	var hash string
	if data == nil {
		// Deletion marker
		hash = util.ComputeHashString("\x01Deleted")
	} else {
		hash = util.ComputeHash(data.Data)
	}

	// Check cache
	if cachedHash, ok := b.cache.Get(path); ok && cachedHash == hash {
		return true // Already processed
	}

	return false
}

// MarkProcessed marks a file as processed by adding to cache
func (b *BasePeer) MarkProcessed(path string, data *FileData) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Compute hash
	var hash string
	if data == nil {
		hash = util.ComputeHashString("\x01Deleted")
	} else {
		hash = util.ComputeHash(data.Data)
	}

	// Add to cache
	b.cache.Set(path, hash)
}

// SetSetting stores a peer-specific setting in persistent storage
func (b *BasePeer) SetSetting(key, value string) error {
	fullKey := b.getSettingKey(key)
	return b.store.Set(fullKey, value)
}

// GetSetting retrieves a peer-specific setting from persistent storage
func (b *BasePeer) GetSetting(key string) (string, error) {
	fullKey := b.getSettingKey(key)
	return b.store.Get(fullKey)
}

// GetSettingWithDefault retrieves a setting or returns default if not found
func (b *BasePeer) GetSettingWithDefault(key, defaultValue string) string {
	fullKey := b.getSettingKey(key)
	return b.store.GetWithDefault(fullKey, defaultValue)
}

// HasSetting checks if a setting exists
func (b *BasePeer) HasSetting(key string) bool {
	fullKey := b.getSettingKey(key)
	return b.store.Has(fullKey)
}

// getSettingKey creates a namespaced key for this peer
func (b *BasePeer) getSettingKey(key string) string {
	return fmt.Sprintf("%s-%s-%s-%s", b.name, b.peerType, b.baseDir, key)
}

// Dispatch sends a change to the hub for routing to other peers
// NOTE: This method passes nil as source, which means it cannot properly
// exclude the source peer from receiving the change. Concrete peer implementations
// should use DispatchFrom() instead.
func (b *BasePeer) Dispatch(path string, data *FileData) error {
	// Check if already processed
	if b.IsRepeating(path, data) {
		b.LogDebug("Skipping repeated change", "path", path)
		return nil
	}

	// Mark as processed before dispatching
	b.MarkProcessed(path, data)

	// Convert to global path
	globalPath := b.ToGlobalPath(path)

	// Dispatch to hub
	return b.dispatcher(nil, globalPath, data) // nil source means we're the source
}

// DispatchFrom sends a change to the hub with a specific source peer
// This is the preferred method for concrete peer implementations to use,
// as it allows the hub to properly exclude the source from receiving the change.
func (b *BasePeer) DispatchFrom(source Peer, path string, data *FileData) error {
	// Check if already processed
	if b.IsRepeating(path, data) {
		b.LogDebug("Skipping repeated change", "path", path)
		return nil
	}

	// Mark as processed before dispatching
	b.MarkProcessed(path, data)

	// Convert to global path
	globalPath := b.ToGlobalPath(path)

	// Dispatch to hub with source
	return b.dispatcher(source, globalPath, data)
}

// Logging helpers

// LogInfo logs an informational message
func (b *BasePeer) LogInfo(msg string, args ...any) {
	allArgs := append([]any{"peer", b.name}, args...)
	slog.Info(msg, allArgs...)
}

// LogDebug logs a debug message
func (b *BasePeer) LogDebug(msg string, args ...any) {
	allArgs := append([]any{"peer", b.name}, args...)
	slog.Debug(msg, allArgs...)
}

// LogWarn logs a warning message
func (b *BasePeer) LogWarn(msg string, args ...any) {
	allArgs := append([]any{"peer", b.name}, args...)
	slog.Warn(msg, allArgs...)
}

// LogError logs an error message
func (b *BasePeer) LogError(msg string, args ...any) {
	allArgs := append([]any{"peer", b.name}, args...)
	slog.Error(msg, allArgs...)
}

// LogReceive logs a received operation (file incoming)
func (b *BasePeer) LogReceive(msg string, args ...any) {
	allArgs := append([]any{"peer", b.name, "direction", "<--"}, args...)
	slog.Info(msg, allArgs...)
}

// LogSend logs a sent operation (file outgoing)
func (b *BasePeer) LogSend(msg string, args ...any) {
	allArgs := append([]any{"peer", b.name, "direction", "-->"}, args...)
	slog.Info(msg, allArgs...)
}
