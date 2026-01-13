package couchdbpeer

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/imdevinc/livesync-bridge/internal/config"
	"github.com/imdevinc/livesync-bridge/internal/peer"
	"github.com/imdevinc/livesync-bridge/internal/storage"
	"github.com/imdevinc/livesync-bridge/internal/util"
	"github.com/imdevinc/livesync-bridge/pkg/couchdb"
	"golang.org/x/crypto/pbkdf2"
)

// CouchDBPeer implements syncing with CouchDB using LiveSync E2EE format
type CouchDBPeer struct {
	*peer.BasePeer
	client        *couchdb.Client
	config        config.PeerCouchDBConf
	encryptionKey []byte // Derived from passphrase
	baseDir       string // Cached for easy access
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// LiveSyncDocument represents a document in LiveSync format
type LiveSyncDocument struct {
	ID          string                 `json:"_id"`
	Rev         string                 `json:"_rev,omitempty"`
	Type        string                 `json:"type"` // "plain" or "newnote"
	Path        string                 `json:"path"`
	Data        string                 `json:"data"` // Base64 encoded, encrypted if encrypted
	CTime       int64                  `json:"ctime"`
	MTime       int64                  `json:"mtime"`
	Size        int                    `json:"size"`
	Deleted     bool                   `json:"deleted,omitempty"`
	Children    []string               `json:"children,omitempty"` // For chunked files
	Eden        map[string]interface{} `json:"eden,omitempty"`
	Attachments map[string]interface{} `json:"_attachments,omitempty"`
}

// ChunkDocument represents a chunk of a large file
type ChunkDocument struct {
	ID   string `json:"_id"`
	Rev  string `json:"_rev,omitempty"`
	Type string `json:"type"` // "leaf"
	Data string `json:"data"` // Base64 encoded, encrypted
}

const (
	// Default chunk size for large files (100KB)
	DefaultChunkSize = 100 * 1024

	// Minimum file size to trigger chunking
	MinChunkSize = 50 * 1024

	// PBKDF2 iterations for key derivation
	PBKDF2Iterations = 100000

	// AES-256 key size
	KeySize = 32

	// Document type prefixes
	DocTypeNote  = "newnote"
	DocTypePlain = "plain"
	DocTypeLeaf  = "leaf"

	// Settings keys
	docMetaPrefix   = "doc-meta-"
	syncedDocPrefix = "synced-doc-"
)

// NewCouchDBPeer creates a new CouchDB peer
func NewCouchDBPeer(cfg config.PeerCouchDBConf, dispatcher peer.DispatchFunc, store *storage.Store) (*CouchDBPeer, error) {
	base, err := peer.NewBasePeer(cfg.Name, "couchdb", cfg.Group, cfg.BaseDir, dispatcher, store)
	if err != nil {
		return nil, fmt.Errorf("failed to create base peer: %w", err)
	}

	// Create CouchDB client
	couchCfg := couchdb.Config{
		URL:      cfg.URL,
		Username: cfg.Username,
		Password: cfg.Password,
		Database: cfg.Database,
		Timeout:  30 * time.Second,
	}

	ctx := context.Background()
	client, err := couchdb.NewClient(ctx, couchCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create CouchDB client: %w", err)
	}

	// Derive encryption key from passphrase
	var encryptionKey []byte
	if cfg.Passphrase != "" {
		encryptionKey = deriveKey(cfg.Passphrase, cfg.Database)
	}

	p := &CouchDBPeer{
		BasePeer:      base,
		client:        client,
		config:        cfg,
		encryptionKey: encryptionKey,
		baseDir:       cfg.BaseDir,
	}

	return p, nil
}

// deriveKey derives an encryption key from passphrase using PBKDF2
func deriveKey(passphrase, salt string) []byte {
	return pbkdf2.Key([]byte(passphrase), []byte(salt), PBKDF2Iterations, KeySize, sha256.New)
}

// Type returns the peer type
func (p *CouchDBPeer) Type() string {
	return "couchdb"
}

// shouldSyncDocument checks if a document should be synced based on filters
func (p *CouchDBPeer) shouldSyncDocument(doc *LiveSyncDocument) bool {
	// Skip design documents
	if strings.HasPrefix(doc.ID, "_design/") {
		return false
	}

	// Skip chunk documents
	if doc.Type == DocTypeLeaf {
		return false
	}

	// Check baseDir filter
	if p.baseDir != "" && !strings.HasPrefix(doc.Path, p.baseDir) {
		return false
	}

	return true
}

// hasDocumentChanged checks if a document has changed since last sync
func (p *CouchDBPeer) hasDocumentChanged(path string, doc *LiveSyncDocument) bool {
	// Get stored metadata
	metaKey := docMetaPrefix + p.Name() + "-" + path
	storedMeta, err := p.GetSetting(metaKey)
	if err != nil || storedMeta == "" {
		// No stored metadata, consider it new
		return true
	}

	// Current metadata: mtime-size-rev
	currentMeta := fmt.Sprintf("%d-%d-%s", doc.MTime, doc.Size, doc.Rev)

	return storedMeta != currentMeta
}

// updateDocumentMetadata stores document metadata for change tracking
func (p *CouchDBPeer) updateDocumentMetadata(path string, doc *LiveSyncDocument) error {
	metaKey := docMetaPrefix + p.Name() + "-" + path
	meta := fmt.Sprintf("%d-%d-%s", doc.MTime, doc.Size, doc.Rev)
	return p.SetSetting(metaKey, meta)
}

// markDocumentSynced marks a document as synced for deletion detection
func (p *CouchDBPeer) markDocumentSynced(docID string) error {
	syncKey := syncedDocPrefix + p.Name() + "-" + docID
	return p.SetSetting(syncKey, "1")
}

// isDocumentSynced checks if a document has been synced before
func (p *CouchDBPeer) isDocumentSynced(docID string) bool {
	syncKey := syncedDocPrefix + p.Name() + "-" + docID
	return p.HasSetting(syncKey)
}

// removeDocumentSynced removes sync tracking for a deleted document
func (p *CouchDBPeer) removeDocumentSynced(docID string) error {
	syncKey := syncedDocPrefix + p.Name() + "-" + docID
	return p.DeleteSetting(syncKey)
}

// scanInitialDocuments performs initial sync of existing CouchDB documents
func (p *CouchDBPeer) scanInitialDocuments(ctx context.Context) error {
	p.LogInfo("Scanning CouchDB for existing documents")

	var totalDocs, newDocs, updatedDocs, skippedDocs int
	const progressInterval = 50 // Log progress every N documents

	// Track all document IDs seen during scan for deletion detection
	seenDocIDs := make(map[string]bool)

	// Use streaming iterator for memory efficiency
	docChan, errChan := p.client.AllDocsIterator(ctx, "", 100)

	// Phase 1: Process active documents
	for {
		select {
		case doc, ok := <-docChan:
			if !ok {
				// Channel closed, Phase 1 complete - now detect deletions
				goto deletionDetection
			}

			totalDocs++

			// Parse as LiveSyncDocument
			docJSON, err := json.Marshal(doc)
			if err != nil {
				p.LogDebug("Failed to marshal document", "error", err)
				continue
			}

			var lsDoc LiveSyncDocument
			if err := json.Unmarshal(docJSON, &lsDoc); err != nil {
				p.LogDebug("Failed to parse document", "error", err)
				continue
			}

			// Track that we've seen this document
			seenDocIDs[lsDoc.ID] = true

			// Apply filters
			if !p.shouldSyncDocument(&lsDoc) {
				skippedDocs++
				continue
			}

			// Convert to global path
			globalPath := p.ToGlobalPath(lsDoc.Path)

			// Check if document changed
			if !p.hasDocumentChanged(globalPath, &lsDoc) {
				skippedDocs++
				// Still mark as synced even if unchanged
				if err := p.markDocumentSynced(lsDoc.ID); err != nil {
					p.LogDebug("Failed to mark document as synced", "docID", lsDoc.ID, "error", err)
				}
				continue
			}

			// Convert document to data
			data, err := p.documentToData(&lsDoc)
			if err != nil {
				p.LogWarn("Failed to convert document", "path", globalPath, "error", err)
				skippedDocs++
				continue
			}

			// Create FileData
			fileData := &peer.FileData{
				CTime:   time.Unix(lsDoc.CTime, 0),
				MTime:   time.Unix(lsDoc.MTime, 0),
				Size:    int64(lsDoc.Size),
				Data:    data,
				Deleted: lsDoc.Deleted,
			}

			// Dispatch to hub (but don't echo back to ourselves)
			if !p.IsRepeating(globalPath, fileData) {
				if err := p.DispatchFrom(p, globalPath, fileData); err != nil {
					p.LogWarn("Failed to dispatch document", "path", globalPath, "error", err)
					continue
				}

				// Mark as processed and update metadata
				p.MarkProcessed(globalPath, fileData)
				if err := p.updateDocumentMetadata(globalPath, &lsDoc); err != nil {
					p.LogWarn("Failed to update metadata", "path", globalPath, "error", err)
				}

				// Determine if new or updated based on metadata existence
				if storedMeta, _ := p.GetSetting(docMetaPrefix + p.Name() + "-" + globalPath); storedMeta == "" {
					newDocs++
				} else {
					updatedDocs++
				}
			} else {
				skippedDocs++
			}

			// Mark document as synced for deletion detection
			if err := p.markDocumentSynced(lsDoc.ID); err != nil {
				p.LogDebug("Failed to mark document as synced", "docID", lsDoc.ID, "error", err)
			}

			// Log progress
			if totalDocs%progressInterval == 0 {
				p.LogInfo("Initial sync progress",
					"processed", totalDocs,
					"new", newDocs,
					"updated", updatedDocs,
					"skipped", skippedDocs,
				)
			}

		case err := <-errChan:
			if err != nil {
				return fmt.Errorf("error during initial sync: %w", err)
			}

		case <-ctx.Done():
			p.LogInfo("Initial sync cancelled")
			return ctx.Err()
		}
	}

deletionDetection:
	// Phase 2: Detect and process deletions
	p.LogInfo("Checking for deleted documents")
	var deletedDocs int

	// Get all previously synced document IDs and check for deletions
	syncPrefix := syncedDocPrefix + p.Name() + "-"
	err := p.Store().IteratePrefix(syncPrefix, func(key string, value string) error {
		// Extract document ID from key: "synced-doc-{peer-name}-{docID}"
		docID := key[len(syncPrefix):]

		// If this document wasn't seen in the current scan, it was deleted
		if !seenDocIDs[docID] {
			p.LogDebug("Detected deleted document", "docID", docID)

			// In LiveSync, document IDs are typically the path, so use it directly
			deletedPath := docID

			// Dispatch deletion event
			fileData := &peer.FileData{
				MTime:   time.Now(),
				Deleted: true,
			}

			if err := p.DispatchFrom(p, deletedPath, fileData); err != nil {
				p.LogWarn("Failed to dispatch deletion", "path", deletedPath, "error", err)
			} else {
				deletedDocs++
				// Clean up metadata and sync tracking
				_ = p.DeleteSetting(docMetaPrefix + p.Name() + "-" + deletedPath)
				_ = p.removeDocumentSynced(docID)
			}
		}

		return nil
	})

	if err != nil {
		p.LogWarn("Error during deletion detection", "error", err)
	}

	p.LogInfo("Initial sync completed",
		"total", totalDocs,
		"new", newDocs,
		"updated", updatedDocs,
		"skipped", skippedDocs,
		"deleted", deletedDocs,
	)

	return nil
}

// Start begins monitoring the CouchDB changes feed
func (p *CouchDBPeer) Start() error {
	ctx, cancel := context.WithCancel(p.Context())
	p.cancel = cancel

	p.LogInfo("Starting CouchDB peer")

	// Get last known sequence from storage
	lastSeq, _ := p.GetSetting("since")

	// Check if initial sync is needed and enabled
	initialSyncEnabled := true
	if p.config.InitialSync != nil {
		initialSyncEnabled = *p.config.InitialSync
	}

	// Perform initial sync if:
	// 1. No stored sequence (first run)
	// 2. InitialSync is enabled in config (default: true)
	if lastSeq == "" && initialSyncEnabled {
		p.LogInfo("Performing initial sync of existing documents")
		if err := p.scanInitialDocuments(ctx); err != nil {
			p.LogError("Initial sync failed", "error", err)
			return fmt.Errorf("initial sync failed: %w", err)
		}
		p.LogInfo("Initial sync completed successfully")

		// Set sequence to "now" after successful initial sync
		lastSeq = "now"
		_ = p.SetSetting("since", lastSeq)
	}

	if lastSeq == "" {
		lastSeq = "now"
	}

	p.LogInfo(fmt.Sprintf("Watching changes from sequence: %s", lastSeq))

	// Start changes feed
	p.wg.Add(1)
	go p.watchChanges(ctx, lastSeq)

	return nil
}

// isCouchDBConflict checks if an error is a CouchDB document conflict error
func isCouchDBConflict(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Conflict") || strings.Contains(err.Error(), "Document update conflict")
}

// putDocumentWithRetry attempts to put a document with retry logic for conflicts
func (p *CouchDBPeer) putDocumentWithRetry(ctx context.Context, doc LiveSyncDocument) (string, error) {
	config := util.QuickRetryConfig() // 3 retries, 50ms initial, 5s max

	var resultRev string
	err := util.RetryWithJitter(ctx, config, func() error {
		// Get latest revision if document exists
		if existingDoc, err := p.client.Get(ctx, doc.ID); err == nil {
			doc.Rev = existingDoc.Rev
		}

		rev, err := p.client.Put(ctx, doc.ID, doc)
		if err != nil {
			return err
		}
		resultRev = rev
		return nil
	}, func(err error) bool {
		// Only retry on conflict errors
		return isCouchDBConflict(err)
	})

	// If all retries failed with a conflict, apply conflict resolution
	if err != nil && isCouchDBConflict(err) {
		p.LogInfo("Retries exhausted, applying conflict resolution",
			"path", doc.Path,
			"docID", doc.ID,
		)

		resolvedDoc, resolveErr := p.resolveConflict(ctx, doc)
		if resolveErr != nil {
			return "", fmt.Errorf("conflict resolution failed after retries: %w", resolveErr)
		}

		p.LogInfo("Document conflict resolved successfully",
			"path", doc.Path,
			"docID", doc.ID,
			"resolvedRev", resolvedDoc.Rev,
		)

		return resolvedDoc.Rev, nil
	}

	return resultRev, err
}

// deleteDocumentWithRetry attempts to delete a document with retry logic for conflicts
func (p *CouchDBPeer) deleteDocumentWithRetry(ctx context.Context, docID string, initialRev string, localMTime int64) error {
	config := util.QuickRetryConfig() // 3 retries, 50ms initial, 5s max

	rev := initialRev
	err := util.RetryWithJitter(ctx, config, func() error {
		// Get latest revision before each delete attempt
		if existingDoc, getErr := p.client.Get(ctx, docID); getErr == nil {
			rev = existingDoc.Rev
		}

		deleteErr := p.client.Delete(ctx, docID, rev)
		return deleteErr
	}, func(err error) bool {
		// Only retry on conflict errors
		return isCouchDBConflict(err)
	})

	// If all retries failed with a conflict, apply delete conflict resolution
	if err != nil && isCouchDBConflict(err) {
		p.LogInfo("Delete retries exhausted, applying conflict resolution",
			"docID", docID,
		)

		// Determine if delete should proceed based on conflict resolution strategy
		shouldDelete, resolveErr := p.resolveDeleteConflict(ctx, docID, localMTime)
		if resolveErr != nil {
			return fmt.Errorf("delete conflict resolution failed: %w", resolveErr)
		}

		if !shouldDelete {
			// Remote version wins, delete is canceled
			p.LogInfo("Delete canceled by conflict resolution, remote version accepted",
				"docID", docID,
			)
			return nil // Not an error - conflict resolved by accepting remote
		}

		// Delete should proceed - get latest revision and force delete
		p.LogInfo("Delete approved by conflict resolution, forcing delete",
			"docID", docID,
		)

		latestDoc, getErr := p.client.Get(ctx, docID)
		if getErr != nil {
			return fmt.Errorf("failed to get latest revision for forced delete: %w", getErr)
		}

		if deleteErr := p.client.Delete(ctx, docID, latestDoc.Rev); deleteErr != nil {
			return fmt.Errorf("forced delete failed after conflict resolution: %w", deleteErr)
		}

		p.LogInfo("Document deleted successfully after conflict resolution",
			"docID", docID,
		)

		return nil
	}

	return err
}

// deleteChunkWithRetry attempts to delete a chunk with retry logic for conflicts
func (p *CouchDBPeer) deleteChunkWithRetry(ctx context.Context, chunkID string, initialRev string) error {
	config := util.QuickRetryConfig() // 3 retries, 50ms initial, 5s max

	rev := initialRev
	err := util.RetryWithJitter(ctx, config, func() error {
		// Get latest revision before each delete attempt
		if existingChunk, getErr := p.client.Get(ctx, chunkID); getErr == nil {
			rev = existingChunk.Rev
		}

		deleteErr := p.client.Delete(ctx, chunkID, rev)
		return deleteErr
	}, func(err error) bool {
		// Only retry on conflict errors
		return isCouchDBConflict(err)
	})

	// If all retries failed with a conflict, force delete with latest revision
	if err != nil && isCouchDBConflict(err) {
		p.LogInfo("Chunk delete retries exhausted, forcing delete with latest revision",
			"chunkID", chunkID,
		)

		// Get latest revision and force delete
		latestChunk, getErr := p.client.Get(ctx, chunkID)
		if getErr != nil {
			return fmt.Errorf("failed to get latest chunk revision for forced delete: %w", getErr)
		}

		if deleteErr := p.client.Delete(ctx, chunkID, latestChunk.Rev); deleteErr != nil {
			return fmt.Errorf("forced chunk delete failed: %w", deleteErr)
		}

		p.LogInfo("Chunk deleted successfully after retry exhaustion",
			"chunkID", chunkID,
		)

		return nil
	}

	return err
}

// putChunkWithRetry attempts to put a chunk document with retry logic for conflicts
func (p *CouchDBPeer) putChunkWithRetry(ctx context.Context, chunk ChunkDocument) (string, error) {
	config := util.QuickRetryConfig() // 3 retries, 50ms initial, 5s max

	var resultRev string
	err := util.RetryWithJitter(ctx, config, func() error {
		// Get latest revision if chunk exists
		if existingChunk, err := p.client.Get(ctx, chunk.ID); err == nil {
			chunk.Rev = existingChunk.Rev
		}

		rev, err := p.client.Put(ctx, chunk.ID, chunk)
		if err != nil {
			return err
		}
		resultRev = rev
		return nil
	}, func(err error) bool {
		// Only retry on conflict errors
		return isCouchDBConflict(err)
	})

	// If all retries failed with a conflict, apply chunk conflict resolution
	if err != nil && isCouchDBConflict(err) {
		p.LogInfo("Chunk retries exhausted, applying conflict resolution",
			"chunkID", chunk.ID,
		)

		// Delete the existing chunk and recreate with local data
		existingChunk, getErr := p.client.Get(ctx, chunk.ID)
		if getErr != nil {
			return "", fmt.Errorf("conflict resolution failed: cannot get existing chunk: %w", getErr)
		}

		// Delete existing chunk
		deleteErr := p.client.Delete(ctx, chunk.ID, existingChunk.Rev)
		if deleteErr != nil {
			return "", fmt.Errorf("conflict resolution failed: cannot delete existing chunk: %w", deleteErr)
		}

		p.LogInfo("Deleted conflicting chunk, recreating with local data",
			"chunkID", chunk.ID,
		)

		// Recreate chunk with local data (no revision needed for new document)
		chunk.Rev = ""
		newRev, putErr := p.client.Put(ctx, chunk.ID, chunk)
		if putErr != nil {
			return "", fmt.Errorf("conflict resolution failed: cannot recreate chunk: %w", putErr)
		}

		p.LogInfo("Chunk conflict resolved successfully",
			"chunkID", chunk.ID,
			"resolvedRev", newRev,
		)

		return newRev, nil
	}

	return resultRev, err
}

// Put sends a file to CouchDB
func (p *CouchDBPeer) Put(path string, fileData *peer.FileData) (bool, error) {
	ctx := p.Context()

	// Check for repeating
	if p.IsRepeating(path, fileData) {
		p.LogDebug(fmt.Sprintf("Skipping repeating put: %s", path))
		return false, nil
	}

	p.LogReceive(fmt.Sprintf("Putting %s (%d bytes)", path, len(fileData.Data)))

	localPath := p.ToLocalPath(path)

	// Determine document type (plain text or binary)
	docType := DocTypeNote // binary
	if isPlainText(path) {
		docType = DocTypePlain
	}

	// Compress if enabled
	processedData := fileData.Data
	if p.config.EnableCompression != nil && *p.config.EnableCompression {
		compressed, err := compress(fileData.Data)
		if err == nil && len(compressed) < len(fileData.Data) {
			processedData = compressed
		}
	}

	// Encrypt if passphrase is set
	if p.encryptionKey != nil {
		encrypted, err := p.encrypt(processedData)
		if err != nil {
			return false, fmt.Errorf("encryption failed: %w", err)
		}
		processedData = encrypted
	}

	// Check if file needs chunking
	chunkSize := DefaultChunkSize
	if p.config.CustomChunkSize != nil {
		chunkSize = *p.config.CustomChunkSize
	}

	ctime := fileData.CTime.Unix()
	mtime := fileData.MTime.Unix()

	if len(processedData) > chunkSize && len(processedData) > MinChunkSize {
		// Mark as processed BEFORE writing to prevent changes feed loop
		p.MarkProcessed(path, fileData)

		// Chunk the file
		err := p.putChunked(ctx, localPath, processedData, ctime, mtime, docType, int(fileData.Size))
		if err != nil {
			return false, err
		}
	} else {
		// Mark as processed BEFORE writing to prevent changes feed loop
		p.MarkProcessed(path, fileData)

		// Small file - store directly
		doc := LiveSyncDocument{
			ID:    docPathToID(localPath),
			Type:  docType,
			Path:  localPath,
			Data:  base64.StdEncoding.EncodeToString(processedData),
			CTime: ctime,
			MTime: mtime,
			Size:  int(fileData.Size), // Original size
		}

		rev, err := p.putDocumentWithRetry(ctx, doc)
		if err != nil {
			return false, fmt.Errorf("failed to put document: %w", err)
		}

		p.LogSend(fmt.Sprintf("Saved %s (rev: %s)", path, rev))
	}

	return true, nil
}

// Delete removes a file from CouchDB
func (p *CouchDBPeer) Delete(path string) (bool, error) {
	ctx := p.Context()

	// Check for repeating
	if p.IsRepeating(path, nil) {
		p.LogDebug(fmt.Sprintf("Skipping repeating delete: %s", path))
		return false, nil
	}

	p.LogReceive(fmt.Sprintf("Deleting %s", path))

	localPath := p.ToLocalPath(path)
	docID := docPathToID(localPath)

	// Get current document to get revision and parse as LiveSyncDocument
	doc, err := p.client.Get(ctx, docID)
	if err != nil {
		// Document might not exist, which is fine
		p.LogDebug(fmt.Sprintf("Document %s not found for deletion", path))
		return false, nil
	}

	// Parse as LiveSyncDocument to check for children and get MTime
	var lsDoc LiveSyncDocument
	row := p.client.DB().Get(ctx, docID)
	if row.Err() != nil {
		p.LogDebug(fmt.Sprintf("Could not parse document %s for deletion: %v", path, row.Err()))
		return false, nil
	}
	if err := row.ScanDoc(&lsDoc); err != nil {
		p.LogDebug(fmt.Sprintf("Could not scan document %s for deletion: %v", path, err))
		return false, nil
	}

	// Delete all chunks with retry logic
	if len(lsDoc.Children) > 0 {
		for _, chunkID := range lsDoc.Children {
			chunkDoc, getErr := p.client.Get(ctx, chunkID)
			if getErr == nil {
				if deleteErr := p.deleteChunkWithRetry(ctx, chunkID, chunkDoc.Rev); deleteErr != nil {
					p.LogWarn("Failed to delete chunk, continuing with document delete",
						"chunkID", chunkID,
						"error", deleteErr,
					)
				}
			}
		}
	}

	// Mark as processed BEFORE deleting to prevent changes feed loop
	p.MarkProcessed(path, nil)

	// Delete main document with retry and conflict resolution
	err = p.deleteDocumentWithRetry(ctx, docID, doc.Rev, lsDoc.MTime)
	if err != nil {
		return false, fmt.Errorf("failed to delete document: %w", err)
	}

	p.LogSend(fmt.Sprintf("Deleted %s", path))

	return true, nil
}

// Get retrieves a file from CouchDB
func (p *CouchDBPeer) Get(path string) (*peer.FileData, error) {
	ctx := p.Context()
	localPath := p.ToLocalPath(path)
	docID := docPathToID(localPath)

	// Use client's DB to get document directly
	row := p.client.DB().Get(ctx, docID)
	if row.Err() != nil {
		return nil, fmt.Errorf("failed to get document: %w", row.Err())
	}

	// Scan directly into LiveSyncDocument
	var lsDoc LiveSyncDocument
	if err := row.ScanDoc(&lsDoc); err != nil {
		return nil, fmt.Errorf("failed to parse document: %w", err)
	}

	// Get data
	data, err := p.documentToData(&lsDoc)
	if err != nil {
		return nil, fmt.Errorf("failed to convert document: %w", err)
	}

	return &peer.FileData{
		CTime:   time.Unix(lsDoc.CTime, 0),
		MTime:   time.Unix(lsDoc.MTime, 0),
		Size:    int64(lsDoc.Size),
		Data:    data,
		Deleted: lsDoc.Deleted,
	}, nil
}

// watchChanges monitors the CouchDB changes feed with automatic reconnection
func (p *CouchDBPeer) watchChanges(ctx context.Context, since string) {
	defer p.wg.Done()

	// Retry configuration for reconnection
	const (
		initialBackoff    = 1 * time.Second
		maxBackoff        = 60 * time.Second
		backoffMultiplier = 2.0
	)

	attemptNum := 0
	backoff := initialBackoff

	// Infinite retry loop
	for {
		// If this is a reconnection attempt, wait with exponential backoff
		if attemptNum > 0 {
			p.LogInfo(fmt.Sprintf("Reconnecting to changes feed (attempt %d, waiting %v)", attemptNum, backoff))

			select {
			case <-time.After(backoff):
				// Continue to reconnection
			case <-ctx.Done():
				p.LogInfo("Watch changes stopped during backoff")
				return
			}

			// Calculate next backoff with exponential growth (capped at maxBackoff)
			backoff = time.Duration(float64(backoff) * backoffMultiplier)
			if backoff > maxBackoff {
				backoff = maxBackoff
			}

			// Reload the last known sequence from storage in case it was updated
			if storedSeq, err := p.GetSetting("since"); err == nil && storedSeq != "" {
				since = storedSeq
			}
		}

		attemptNum++
		p.LogInfo(fmt.Sprintf("Starting changes feed from sequence: %s", since))

		// Create changes feed options
		opts := couchdb.ChangesOptions{
			Since:       since,
			IncludeDocs: true,
			Continuous:  true,
			Heartbeat:   30 * time.Second,
		}

		// Start changes feed
		changesChan, errChan := p.client.Changes(ctx, opts)

		// Inner loop - process changes until error or context cancellation
	changesLoop:
		for {
			select {
			case change, ok := <-changesChan:
				if !ok {
					p.LogInfo("Changes feed closed")
					// Channel closed, break to retry
					break changesLoop
				}

				// Save sequence
				_ = p.SetSetting("since", change.Seq)
				since = change.Seq

				// Skip if no document or not in our baseDir
				if change.Doc == nil {
					continue
				}

				// Parse document
				docJSON, err := json.Marshal(change.Doc)
				if err != nil {
					p.LogDebug(fmt.Sprintf("Failed to marshal document %s: %v", change.ID, err))
					continue
				}

				var lsDoc LiveSyncDocument
				if err := json.Unmarshal(docJSON, &lsDoc); err != nil {
					p.LogDebug(fmt.Sprintf("Failed to parse document %s: %v", change.ID, err))
					continue
				}

				// Check if path is in our baseDir
				if p.baseDir != "" && !strings.HasPrefix(lsDoc.Path, p.baseDir) {
					continue
				}

				// Skip chunk documents
				if lsDoc.Type == DocTypeLeaf {
					continue
				}

				// Convert to global path
				globalPath := p.ToGlobalPath(lsDoc.Path)

				// Handle deletion
				if change.Deleted || lsDoc.Deleted {
					// When a document is deleted, lsDoc.Path might be empty
					// Reconstruct path from document ID
					if globalPath == "" || globalPath == "/" {
						// Convert document ID back to path (reverse of docPathToID)
						docPath := strings.ReplaceAll(change.ID, ":", "/")
						globalPath = p.ToGlobalPath(docPath)
					}

					p.LogInfo(fmt.Sprintf("Change detected: %s (deleted)", globalPath))
					fileData := &peer.FileData{
						Deleted: true,
					}
					if !p.IsRepeating(globalPath, fileData) {
						p.DispatchFrom(p, globalPath, fileData)
						p.MarkProcessed(globalPath, fileData)

						// Clean up metadata and sync tracking
						_ = p.DeleteSetting(docMetaPrefix + p.Name() + "-" + globalPath)
						_ = p.removeDocumentSynced(lsDoc.ID)
					}
					continue
				}

				// Handle update/create
				data, err := p.documentToData(&lsDoc)
				if err != nil {
					p.LogDebug(fmt.Sprintf("Failed to convert document %s: %v", globalPath, err))
					continue
				}

				fileData := &peer.FileData{
					CTime:   time.Unix(lsDoc.CTime, 0),
					MTime:   time.Unix(lsDoc.MTime, 0),
					Size:    int64(lsDoc.Size),
					Data:    data,
					Deleted: false,
				}

				p.LogInfo(fmt.Sprintf("Change detected: %s (%d bytes)", globalPath, len(data)))

				if !p.IsRepeating(globalPath, fileData) {
					p.DispatchFrom(p, globalPath, fileData)
					p.MarkProcessed(globalPath, fileData)

					// Update metadata and mark document as synced
					_ = p.updateDocumentMetadata(globalPath, &lsDoc)
					_ = p.markDocumentSynced(lsDoc.ID)
				}

			case err := <-errChan:
				if ctx.Err() != nil {
					// Context cancelled, normal shutdown
					p.LogInfo("Watch changes stopped")
					return
				}
				// Log error and break inner loop to trigger reconnection
				p.LogError(fmt.Sprintf("Changes feed error: %v", err))
				break changesLoop

			case <-ctx.Done():
				p.LogInfo("Watch changes stopped")
				return
			}
		}

		// Inner loop exited due to error or channel close, will retry from top of outer loop
	}
}

// documentToData converts a LiveSync document to raw data
func (p *CouchDBPeer) documentToData(doc *LiveSyncDocument) ([]byte, error) {
	ctx := p.Context()

	var data []byte
	var err error

	// Check if document is chunked
	if len(doc.Children) > 0 {
		// Reassemble from chunks
		data, err = p.reassembleChunks(ctx, doc.Children)
		if err != nil {
			return nil, fmt.Errorf("failed to reassemble chunks: %w", err)
		}
	} else {
		// Single document
		data, err = base64.StdEncoding.DecodeString(doc.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode data: %w", err)
		}
	}

	// Decrypt if encrypted
	if p.encryptionKey != nil {
		decrypted, err := p.decrypt(data)
		if err != nil {
			return nil, fmt.Errorf("decryption failed: %w", err)
		}
		data = decrypted
	}

	// Decompress if compressed
	if p.config.EnableCompression != nil && *p.config.EnableCompression {
		decompressed, err := decompress(data)
		if err == nil {
			data = decompressed
		}
	}

	return data, nil
}

// encrypt encrypts data using AES-256-GCM
func (p *CouchDBPeer) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(p.encryptionKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Encrypt and prepend nonce
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// decrypt decrypts data using AES-256-GCM
func (p *CouchDBPeer) decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(p.encryptionKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// putChunked stores a large file as multiple chunks
func (p *CouchDBPeer) putChunked(ctx context.Context, path string, data []byte, ctime, mtime int64, docType string, originalSize int) error {
	chunkSize := DefaultChunkSize
	if p.config.CustomChunkSize != nil {
		chunkSize = *p.config.CustomChunkSize
	}

	// Split into chunks
	var chunks []string
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[i:end]

		// Create chunk ID
		chunkID := fmt.Sprintf("h:%s:%d", docPathToID(path), i/chunkSize)

		// Create chunk document
		chunkDoc := ChunkDocument{
			ID:   chunkID,
			Type: DocTypeLeaf,
			Data: base64.StdEncoding.EncodeToString(chunk),
		}

		// Save chunk with retry
		_, err := p.putChunkWithRetry(ctx, chunkDoc)
		if err != nil {
			return fmt.Errorf("failed to put chunk %s: %w", chunkID, err)
		}

		chunks = append(chunks, chunkID)
	}

	// Create main document with references to chunks
	mainDoc := LiveSyncDocument{
		ID:       docPathToID(path),
		Type:     docType,
		Path:     path,
		Data:     "", // Empty for chunked documents
		CTime:    ctime,
		MTime:    mtime,
		Size:     originalSize,
		Children: chunks,
	}

	_, err := p.putDocumentWithRetry(ctx, mainDoc)
	if err != nil {
		return fmt.Errorf("failed to put main document: %w", err)
	}

	p.LogInfo(fmt.Sprintf("Stored chunked document: %s (%d chunks)", path, len(chunks)))

	return nil
}

// reassembleChunks reassembles a file from chunks
func (p *CouchDBPeer) reassembleChunks(ctx context.Context, chunkIDs []string) ([]byte, error) {
	var result bytes.Buffer

	for _, chunkID := range chunkIDs {
		doc, err := p.client.Get(ctx, chunkID)
		if err != nil {
			return nil, fmt.Errorf("failed to get chunk %s: %w", chunkID, err)
		}

		// Parse chunk document
		docJSON, _ := json.Marshal(doc)
		var chunkDoc ChunkDocument
		if err := json.Unmarshal(docJSON, &chunkDoc); err != nil {
			return nil, fmt.Errorf("failed to parse chunk: %w", err)
		}

		// Decode chunk data
		chunkData, err := base64.StdEncoding.DecodeString(chunkDoc.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode chunk data: %w", err)
		}

		result.Write(chunkData)
	}

	return result.Bytes(), nil
}

// compress compresses data using gzip
func compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)

	_, err := writer.Write(data)
	if err != nil {
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// decompress decompresses gzip data
func decompress(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

// docPathToID converts a path to a document ID
func docPathToID(path string) string {
	// Remove leading slash
	path = strings.TrimPrefix(path, "/")
	// Replace slashes with colons (LiveSync convention)
	return strings.ReplaceAll(path, "/", ":")
}

// isPlainText determines if a file should be stored as plain text
func isPlainText(path string) bool {
	plainTextExts := []string{
		".md", ".txt", ".json", ".xml", ".html", ".css", ".js",
		".ts", ".yaml", ".yml", ".toml", ".ini", ".csv",
	}

	pathLower := strings.ToLower(path)
	for _, ext := range plainTextExts {
		if strings.HasSuffix(pathLower, ext) {
			return true
		}
	}
	return false
}
