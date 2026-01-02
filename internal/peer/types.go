package peer

import (
	"context"
	"time"
)

// FileData represents file content and metadata
type FileData struct {
	CTime   time.Time // Creation time (in practice, mtime from source)
	MTime   time.Time // Modification time
	Size    int64     // File size in bytes
	Data    []byte    // File content (can be text or binary)
	Deleted bool      // Whether this is a deletion marker
}

// DispatchFunc is the callback function for peers to notify the hub of changes
// The source parameter can be nil when the peer is dispatching its own changes
type DispatchFunc func(source Peer, path string, data *FileData) error

// Peer is the interface that all peer types must implement
type Peer interface {
	// Start begins watching for changes
	Start() error

	// Stop halts all peer operations
	Stop() error

	// Put writes or updates a file
	Put(path string, data *FileData) (bool, error)

	// Delete removes a file
	Delete(path string) (bool, error)

	// Get retrieves a file's data
	Get(path string) (*FileData, error)

	// Name returns the peer's unique name
	Name() string

	// Group returns the peer's group (for routing)
	Group() string

	// Type returns the peer type ("storage" or "couchdb")
	Type() string

	// Context returns the peer's context (for cancellation)
	Context() context.Context
}
