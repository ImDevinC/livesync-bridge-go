package couchdbpeer

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/imdevinc/livesync-bridge/internal/config"
	"github.com/imdevinc/livesync-bridge/internal/peer"
)

// ConflictStrategy defines the type for conflict resolution strategies
type ConflictStrategy string

// Conflict resolution strategy constants
const (
	TimestampWins ConflictStrategy = "timestamp-wins" // Winner chosen by modification timestamp (default)
	LocalWins     ConflictStrategy = "local-wins"     // Local version always wins
	RemoteWins    ConflictStrategy = "remote-wins"    // Remote version always wins
	Manual        ConflictStrategy = "manual"         // Manual resolution required (returns error)
)

// getConflictStrategy returns the conflict resolution strategy from peer config
// Returns the default strategy (TimestampWins) if not configured
func getConflictStrategy(cfg config.PeerCouchDBConf) ConflictStrategy {
	if cfg.ConflictResolution == nil || *cfg.ConflictResolution == "" {
		return TimestampWins
	}
	return ConflictStrategy(*cfg.ConflictResolution)
}

// isValidStrategy checks if a strategy string is valid
func isValidStrategy(s string) bool {
	switch ConflictStrategy(s) {
	case TimestampWins, LocalWins, RemoteWins, Manual:
		return true
	default:
		return false
	}
}

// String returns the string representation of the strategy
func (s ConflictStrategy) String() string {
	return string(s)
}

// Validate checks if the strategy is valid
func (s ConflictStrategy) Validate() error {
	if !isValidStrategy(string(s)) {
		return fmt.Errorf("invalid conflict resolution strategy: %s (valid: timestamp-wins, local-wins, remote-wins, manual)", s)
	}
	return nil
}

// resolveConflict orchestrates conflict resolution between local and remote documents
// It fetches the latest remote document, determines the strategy, and applies the appropriate resolution
func (p *CouchDBPeer) resolveConflict(ctx context.Context, localDoc LiveSyncDocument) (*LiveSyncDocument, error) {
	// Fetch latest remote document with current revision
	remoteDocRaw, err := p.client.Get(ctx, localDoc.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote document: %w", err)
	}

	// Convert to LiveSyncDocument - we need to parse the raw document
	// The client.Get returns a Document with generic data
	var remoteDoc LiveSyncDocument
	remoteDoc.ID = remoteDocRaw.ID
	remoteDoc.Rev = remoteDocRaw.Rev

	// Get the full document by using the DB directly
	row := p.client.DB().Get(ctx, localDoc.ID)
	if row.Err() != nil {
		return nil, fmt.Errorf("failed to get full remote document: %w", row.Err())
	}
	if err := row.ScanDoc(&remoteDoc); err != nil {
		return nil, fmt.Errorf("failed to parse remote document: %w", err)
	}

	// Get conflict resolution strategy from config
	strategy := getConflictStrategy(p.config)

	p.LogInfo("Resolving conflict",
		"path", localDoc.Path,
		"strategy", strategy,
		"localRev", localDoc.Rev,
		"remoteRev", remoteDoc.Rev,
	)

	// Route to appropriate strategy function
	var resolvedDoc *LiveSyncDocument
	switch strategy {
	case TimestampWins:
		resolvedDoc, err = p.resolveByTimestamp(ctx, &localDoc, &remoteDoc)
	case LocalWins:
		resolvedDoc, err = p.resolveLocalWins(ctx, &localDoc, &remoteDoc)
	case RemoteWins:
		resolvedDoc, err = p.resolveRemoteWins(ctx, &localDoc, &remoteDoc)
	case Manual:
		return nil, fmt.Errorf("manual conflict resolution required for %s (local: %s, remote: %s)",
			localDoc.Path, localDoc.Rev, remoteDoc.Rev)
	default:
		return nil, fmt.Errorf("unknown conflict resolution strategy: %s", strategy)
	}

	if err != nil {
		return nil, fmt.Errorf("conflict resolution failed: %w", err)
	}

	p.LogInfo("Conflict resolved",
		"path", localDoc.Path,
		"strategy", strategy,
		"resolvedRev", resolvedDoc.Rev,
	)

	return resolvedDoc, nil
}

// resolveByTimestamp resolves conflict by comparing modification timestamps
// Local wins if local.MTime > remote.MTime
// Remote wins if remote.MTime > local.MTime
// Remote wins as tiebreaker when timestamps are equal (remote.MTime == local.MTime)
func (p *CouchDBPeer) resolveByTimestamp(ctx context.Context, local, remote *LiveSyncDocument) (*LiveSyncDocument, error) {
	p.LogInfo("Comparing timestamps",
		"path", local.Path,
		"localMTime", local.MTime,
		"remoteMTime", remote.MTime,
	)

	// Compare timestamps
	if local.MTime > remote.MTime {
		// Local is newer - force local version to remote
		p.LogInfo("Timestamp winner: LOCAL (newer)",
			"path", local.Path,
			"localMTime", local.MTime,
			"remoteMTime", remote.MTime,
		)
		return p.forceUpdateRemote(ctx, local, remote)
	}

	// Remote wins (either remote is newer OR timestamps are equal - tiebreaker)
	if local.MTime == remote.MTime {
		p.LogInfo("Timestamp winner: REMOTE (tiebreaker - equal timestamps)",
			"path", local.Path,
			"mtime", local.MTime,
		)
	} else {
		p.LogInfo("Timestamp winner: REMOTE (newer)",
			"path", local.Path,
			"localMTime", local.MTime,
			"remoteMTime", remote.MTime,
		)
	}

	return p.acceptRemoteVersion(ctx, local, remote)
}

// resolveLocalWins resolves conflict by forcing local version to remote
// Always chooses the local version regardless of timestamps or other factors
func (p *CouchDBPeer) resolveLocalWins(ctx context.Context, local, remote *LiveSyncDocument) (*LiveSyncDocument, error) {
	p.LogInfo("Resolving conflict with local-wins strategy",
		"path", local.Path,
		"localRev", local.Rev,
		"remoteRev", remote.Rev,
	)

	// Force local version to remote using remote's revision
	return p.forceUpdateRemote(ctx, local, remote)
}

// resolveRemoteWins resolves conflict by accepting remote version
// Always chooses the remote version and dispatches it to local storage peers
func (p *CouchDBPeer) resolveRemoteWins(ctx context.Context, local, remote *LiveSyncDocument) (*LiveSyncDocument, error) {
	p.LogInfo("Resolving conflict with remote-wins strategy",
		"path", remote.Path,
		"localRev", local.Rev,
		"remoteRev", remote.Rev,
	)

	// Accept remote version and dispatch to local peers
	return p.acceptRemoteVersion(ctx, local, remote)
}

// forceUpdateRemote forces local version to remote with remote's revision
// Handles both chunked and non-chunked files
func (p *CouchDBPeer) forceUpdateRemote(ctx context.Context, local, remote *LiveSyncDocument) (*LiveSyncDocument, error) {
	// First, if remote has chunks, delete all of them
	if len(remote.Children) > 0 {
		p.LogInfo("Deleting remote chunks before force update",
			"path", local.Path,
			"chunkCount", len(remote.Children),
		)

		for _, chunkID := range remote.Children {
			// Get chunk to get its revision
			chunkDoc, err := p.client.Get(ctx, chunkID)
			if err != nil {
				// Log but continue - chunk might already be deleted
				p.LogDebug(fmt.Sprintf("Failed to get chunk %s for deletion: %v", chunkID, err))
				continue
			}

			// Delete the chunk
			if err := p.client.Delete(ctx, chunkID, chunkDoc.Rev); err != nil {
				// Log but continue - chunk might already be deleted
				p.LogDebug(fmt.Sprintf("Failed to delete chunk %s: %v", chunkID, err))
			}
		}
	}

	// Check if local data needs chunking
	chunkSize := DefaultChunkSize
	if p.config.CustomChunkSize != nil {
		chunkSize = *p.config.CustomChunkSize
	}

	// Decode local data to check size for chunking decision
	localData, err := base64.StdEncoding.DecodeString(local.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode local data: %w", err)
	}

	// If local data is large, we need to chunk it
	if len(localData) > chunkSize && len(localData) > MinChunkSize {
		// Create chunks from local data
		var chunks []string
		for i := 0; i < len(localData); i += chunkSize {
			end := i + chunkSize
			if end > len(localData) {
				end = len(localData)
			}
			chunk := localData[i:end]

			// Create chunk ID
			chunkID := fmt.Sprintf("h:%s:%d", local.ID, i/chunkSize)

			// Create chunk document
			chunkDoc := ChunkDocument{
				ID:   chunkID,
				Type: DocTypeLeaf,
				Data: base64.StdEncoding.EncodeToString(chunk),
			}

			// Save chunk with retry
			_, err := p.putChunkWithRetry(ctx, chunkDoc)
			if err != nil {
				return nil, fmt.Errorf("failed to put chunk %s: %w", chunkID, err)
			}

			chunks = append(chunks, chunkID)
		}

		p.LogInfo("Created new chunks for force update",
			"path", local.Path,
			"chunkCount", len(chunks),
		)

		// Update main document with local data + remote revision
		// For chunked documents, Data is empty and Children contains chunk IDs
		updatedDoc := LiveSyncDocument{
			ID:       local.ID,
			Rev:      remote.Rev, // Use remote's revision to avoid conflict
			Type:     local.Type,
			Path:     local.Path,
			Data:     "", // Empty for chunked documents
			CTime:    local.CTime,
			MTime:    local.MTime,
			Size:     local.Size,
			Children: chunks,
		}

		// Put the updated main document
		newRev, err := p.client.Put(ctx, updatedDoc.ID, updatedDoc)
		if err != nil {
			return nil, fmt.Errorf("failed to update main document: %w", err)
		}

		updatedDoc.Rev = newRev
		return &updatedDoc, nil
	}

	// Not chunked - just update the main document
	updatedDoc := LiveSyncDocument{
		ID:    local.ID,
		Rev:   remote.Rev, // Use remote's revision to avoid conflict
		Type:  local.Type,
		Path:  local.Path,
		Data:  local.Data,
		CTime: local.CTime,
		MTime: local.MTime,
		Size:  local.Size,
	}

	// Put the updated document
	newRev, err := p.client.Put(ctx, updatedDoc.ID, updatedDoc)
	if err != nil {
		return nil, fmt.Errorf("failed to update document: %w", err)
	}

	updatedDoc.Rev = newRev
	return &updatedDoc, nil
}

// acceptRemoteVersion accepts remote version as-is and dispatches to local peers
// Returns the remote document to signal successful resolution
func (p *CouchDBPeer) acceptRemoteVersion(ctx context.Context, local, remote *LiveSyncDocument) (*LiveSyncDocument, error) {
	// Convert remote document to data bytes for dispatch
	data, err := p.documentToData(remote)
	if err != nil {
		return nil, fmt.Errorf("failed to convert remote document to data: %w", err)
	}

	// Create FileData for dispatch
	fileData := &peer.FileData{
		CTime:   time.Unix(remote.CTime, 0),
		MTime:   time.Unix(remote.MTime, 0),
		Size:    int64(remote.Size),
		Data:    data,
		Deleted: false,
	}

	// Convert to global path for dispatch
	globalPath := p.ToGlobalPath(remote.Path)

	p.LogInfo("Accepting remote version and dispatching to local peers",
		"path", globalPath,
		"remoteRev", remote.Rev,
		"size", len(data),
	)

	// Check if already processed to prevent loops
	if !p.IsRepeating(globalPath, fileData) {
		// Dispatch to local storage peers
		if err := p.DispatchFrom(p, globalPath, fileData); err != nil {
			return nil, fmt.Errorf("failed to dispatch remote version: %w", err)
		}

		// Mark as processed to prevent future loops
		p.MarkProcessed(globalPath, fileData)

		// Update metadata and mark document as synced
		if err := p.updateDocumentMetadata(globalPath, remote); err != nil {
			p.LogWarn("Failed to update metadata after accepting remote version",
				"path", globalPath,
				"error", err,
			)
		}

		if err := p.markDocumentSynced(remote.ID); err != nil {
			p.LogWarn("Failed to mark document as synced after accepting remote version",
				"path", globalPath,
				"error", err,
			)
		}
	} else {
		p.LogDebug("Remote version already processed, skipping dispatch",
			"path", globalPath,
		)
	}

	// Return the remote document as-is
	return remote, nil
}

// resolveDeleteConflict determines if a delete operation should proceed when there's a conflict
// Returns true if delete should proceed, false if remote version should be accepted instead
func (p *CouchDBPeer) resolveDeleteConflict(ctx context.Context, docID string, localMTime int64) (bool, error) {
	// Get the full remote document
	var remoteDoc LiveSyncDocument
	row := p.client.DB().Get(ctx, docID)
	if row.Err() != nil {
		return false, fmt.Errorf("failed to get remote document: %w", row.Err())
	}
	if err := row.ScanDoc(&remoteDoc); err != nil {
		return false, fmt.Errorf("failed to parse remote document: %w", err)
	}

	// Get conflict resolution strategy from config
	strategy := getConflictStrategy(p.config)

	p.LogInfo("Resolving delete conflict",
		"docID", docID,
		"path", remoteDoc.Path,
		"strategy", strategy,
		"localMTime", localMTime,
		"remoteMTime", remoteDoc.MTime,
		"remoteRev", remoteDoc.Rev,
	)

	// Route to appropriate strategy
	switch strategy {
	case TimestampWins:
		// Compare timestamps - local wins (delete proceeds) if local >= remote
		if localMTime >= remoteDoc.MTime {
			p.LogInfo("Delete wins by timestamp",
				"docID", docID,
				"localMTime", localMTime,
				"remoteMTime", remoteDoc.MTime,
			)
			return true, nil
		}
		// Remote is newer, accept remote version
		p.LogInfo("Remote version wins by timestamp, canceling delete",
			"docID", docID,
			"localMTime", localMTime,
			"remoteMTime", remoteDoc.MTime,
		)
		// Dispatch remote version to local peers
		if err := p.acceptRemoteForDelete(ctx, &remoteDoc); err != nil {
			return false, fmt.Errorf("failed to accept remote version: %w", err)
		}
		return false, nil

	case LocalWins:
		// Local always wins - delete proceeds
		p.LogInfo("Delete wins by local-wins strategy", "docID", docID)
		return true, nil

	case RemoteWins:
		// Remote always wins - accept remote version
		p.LogInfo("Remote version wins by remote-wins strategy, canceling delete", "docID", docID)
		// Dispatch remote version to local peers
		if err := p.acceptRemoteForDelete(ctx, &remoteDoc); err != nil {
			return false, fmt.Errorf("failed to accept remote version: %w", err)
		}
		return false, nil

	case Manual:
		return false, fmt.Errorf("manual conflict resolution required for delete of %s (remoteRev: %s)",
			remoteDoc.Path, remoteDoc.Rev)

	default:
		return false, fmt.Errorf("unknown conflict resolution strategy: %s", strategy)
	}
}

// acceptRemoteForDelete dispatches remote document to local peers when delete is canceled due to conflict resolution
func (p *CouchDBPeer) acceptRemoteForDelete(ctx context.Context, remote *LiveSyncDocument) error {
	// Convert document to data
	data, err := p.documentToData(remote)
	if err != nil {
		return fmt.Errorf("failed to convert remote document: %w", err)
	}

	fileData := &peer.FileData{
		CTime:   time.Unix(remote.CTime, 0),
		MTime:   time.Unix(remote.MTime, 0),
		Size:    int64(remote.Size),
		Data:    data,
		Deleted: false,
	}

	// Convert to global path for dispatch
	globalPath := p.ToGlobalPath(remote.Path)

	p.LogInfo("Accepting remote version (canceling delete) and dispatching to local peers",
		"path", globalPath,
		"remoteRev", remote.Rev,
		"size", len(data),
	)

	// Check if already processed to prevent loops
	if !p.IsRepeating(globalPath, fileData) {
		// Dispatch to local storage peers
		if err := p.DispatchFrom(p, globalPath, fileData); err != nil {
			return fmt.Errorf("failed to dispatch remote version: %w", err)
		}

		// Mark as processed to prevent future loops
		p.MarkProcessed(globalPath, fileData)

		// Update metadata and mark document as synced
		if err := p.updateDocumentMetadata(globalPath, remote); err != nil {
			p.LogWarn("Failed to update metadata after accepting remote version",
				"path", globalPath,
				"error", err,
			)
		}

		if err := p.markDocumentSynced(remote.ID); err != nil {
			p.LogWarn("Failed to mark document as synced after accepting remote version",
				"path", globalPath,
				"error", err,
			)
		}
	} else {
		p.LogDebug("Remote version already processed, skipping dispatch",
			"path", globalPath,
		)
	}

	return nil
}
