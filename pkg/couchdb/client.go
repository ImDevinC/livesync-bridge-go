package couchdb

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/go-kivik/kivik/v4"
	_ "github.com/go-kivik/kivik/v4/couchdb" // CouchDB driver
)

// Client wraps Kivik CouchDB client with additional functionality
type Client struct {
	client   *kivik.Client
	db       *kivik.DB
	dbName   string
	username string
	url      string
}

// Config holds configuration for connecting to CouchDB
type Config struct {
	URL      string // CouchDB server URL (e.g., "http://localhost:5984")
	Username string // Username for authentication
	Password string // Password for authentication
	Database string // Database name
	Timeout  time.Duration
}

// Document represents a CouchDB document with metadata
type Document struct {
	ID          string                 `json:"_id"`
	Rev         string                 `json:"_rev,omitempty"`
	Deleted     bool                   `json:"_deleted,omitempty"`
	Attachments map[string]interface{} `json:"_attachments,omitempty"`
	Data        map[string]interface{} `json:"-"` // Additional fields
}

// Change represents a change notification from CouchDB changes feed
type Change struct {
	Seq     string                 `json:"seq"`
	ID      string                 `json:"id"`
	Changes []string               `json:"changes"` // Revision strings
	Deleted bool                   `json:"deleted,omitempty"`
	Doc     map[string]interface{} `json:"doc,omitempty"` // Raw document data
}

// ChangesOptions configures the changes feed
type ChangesOptions struct {
	Since       string        // Start sequence
	IncludeDocs bool          // Include full documents
	Continuous  bool          // Continuous feed
	Heartbeat   time.Duration // Heartbeat interval
	Timeout     time.Duration // Timeout for feed
	Filter      string        // Filter function
	Limit       int           // Max number of changes
}

// BulkResult represents the result of a bulk operation
type BulkResult struct {
	ID    string `json:"id"`
	Rev   string `json:"rev,omitempty"`
	Error string `json:"error,omitempty"`
}

// NewClient creates a new CouchDB client
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("CouchDB URL is required")
	}
	if cfg.Database == "" {
		return nil, fmt.Errorf("database name is required")
	}

	// Set default timeout
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	// Build DSN with authentication if provided
	dsn := cfg.URL
	if cfg.Username != "" && cfg.Password != "" {
		u, err := url.Parse(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("invalid URL: %w", err)
		}
		u.User = url.UserPassword(cfg.Username, cfg.Password)
		dsn = u.String()
	}

	// Create Kivik client with authenticated DSN
	client, err := kivik.New("couch", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to create CouchDB client: %w", err)
	}

	// Verify database exists
	exists, err := client.DBExists(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to check database existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("database %s does not exist", cfg.Database)
	}

	// Open database
	db := client.DB(cfg.Database)
	if db.Err() != nil {
		return nil, fmt.Errorf("failed to open database %s: %w", cfg.Database, db.Err())
	}

	return &Client{
		client:   client,
		db:       db,
		dbName:   cfg.Database,
		username: cfg.Username,
		url:      cfg.URL,
	}, nil
}

// Close closes the CouchDB client connection
func (c *Client) Close() error {
	return c.client.Close()
}

// DBName returns the database name
func (c *Client) DBName() string {
	return c.dbName
}

// URL returns the CouchDB server URL
func (c *Client) URL() string {
	return c.url
}

// Username returns the authenticated username
func (c *Client) Username() string {
	return c.username
}

// DB returns the underlying Kivik DB instance for advanced operations
func (c *Client) DB() *kivik.DB {
	return c.db
}

// Put creates or updates a document
func (c *Client) Put(ctx context.Context, id string, doc interface{}) (rev string, err error) {
	rev, err = c.db.Put(ctx, id, doc)
	if err != nil {
		return "", fmt.Errorf("failed to put document %s: %w", id, err)
	}
	return rev, nil
}

// Get retrieves a document by ID
func (c *Client) Get(ctx context.Context, id string) (*Document, error) {
	row := c.db.Get(ctx, id)
	if row.Err() != nil {
		return nil, fmt.Errorf("failed to get document %s: %w", id, row.Err())
	}

	var doc Document
	if err := row.ScanDoc(&doc); err != nil {
		return nil, fmt.Errorf("failed to scan document %s: %w", id, err)
	}

	return &doc, nil
}

// Delete deletes a document
func (c *Client) Delete(ctx context.Context, id, rev string) error {
	_, err := c.db.Delete(ctx, id, rev)
	if err != nil {
		return fmt.Errorf("failed to delete document %s: %w", id, err)
	}
	return nil
}

// AllDocs retrieves all documents with optional prefix filter
func (c *Client) AllDocs(ctx context.Context, prefix string) ([]*Document, error) {
	// Build options map
	optsMap := map[string]interface{}{
		"include_docs": true,
	}

	// If prefix provided, use startkey/endkey range query
	if prefix != "" {
		optsMap["startkey"] = prefix
		optsMap["endkey"] = prefix + "\ufff0" // High Unicode character for range end
	}

	rows := c.db.AllDocs(ctx, kivik.Params(optsMap))
	if rows.Err() != nil {
		return nil, fmt.Errorf("failed to query all docs: %w", rows.Err())
	}
	defer rows.Close()

	var docs []*Document
	for rows.Next() {
		var doc Document
		if err := rows.ScanDoc(&doc); err != nil {
			return nil, fmt.Errorf("failed to scan document: %w", err)
		}
		docs = append(docs, &doc)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating documents: %w", rows.Err())
	}

	return docs, nil
}

// BulkDocs performs bulk document operations
func (c *Client) BulkDocs(ctx context.Context, docs []interface{}) ([]BulkResult, error) {
	// BulkDocs returns []kivik.BulkResult directly in Kivik v4
	results, err := c.db.BulkDocs(ctx, docs)
	if err != nil {
		return nil, fmt.Errorf("bulk docs operation failed: %w", err)
	}

	// Convert kivik.BulkResult to our BulkResult type
	bulkResults := make([]BulkResult, len(results))
	for i, r := range results {
		errStr := ""
		if r.Error != nil {
			errStr = r.Error.Error()
		}
		bulkResults[i] = BulkResult{
			ID:    r.ID,
			Rev:   r.Rev,
			Error: errStr,
		}
	}

	return bulkResults, nil
}

// Changes monitors the changes feed
func (c *Client) Changes(ctx context.Context, opts ChangesOptions) (<-chan Change, <-chan error) {
	changeChan := make(chan Change, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(changeChan)
		defer close(errChan)

		// Build Kivik options map
		optsMap := map[string]interface{}{}
		if opts.Since != "" {
			optsMap["since"] = opts.Since
		}
		if opts.IncludeDocs {
			optsMap["include_docs"] = true
		}
		if opts.Continuous {
			optsMap["feed"] = "continuous"
		}
		if opts.Heartbeat > 0 {
			optsMap["heartbeat"] = int(opts.Heartbeat.Milliseconds())
		}
		if opts.Timeout > 0 {
			optsMap["timeout"] = int(opts.Timeout.Milliseconds())
		}
		if opts.Filter != "" {
			optsMap["filter"] = opts.Filter
		}
		if opts.Limit > 0 {
			optsMap["limit"] = opts.Limit
		}

		changes := c.db.Changes(ctx, kivik.Params(optsMap))
		if changes.Err() != nil {
			errChan <- fmt.Errorf("failed to start changes feed: %w", changes.Err())
			return
		}
		defer changes.Close()

		for changes.Next() {
			change := Change{
				ID:      changes.ID(),
				Seq:     changes.Seq(),
				Deleted: changes.Deleted(),
				Changes: changes.Changes(), // Returns []string of revisions
			}

			// Get document if included
			if opts.IncludeDocs {
				var doc map[string]interface{}
				if err := changes.ScanDoc(&doc); err == nil {
					change.Doc = doc
				}
			}

			select {
			case changeChan <- change:
			case <-ctx.Done():
				return
			}
		}

		if changes.Err() != nil {
			errChan <- fmt.Errorf("changes feed error: %w", changes.Err())
		}
	}()

	return changeChan, errChan
}

// PutAttachment adds or updates an attachment to a document
func (c *Client) PutAttachment(ctx context.Context, docID, rev, name, contentType string, content []byte) (string, error) {
	newRev, err := c.db.PutAttachment(ctx, docID, &kivik.Attachment{
		Filename:    name,
		ContentType: contentType,
		Content:     io.NopCloser(bytes.NewReader(content)),
	}, kivik.Rev(rev))
	if err != nil {
		return "", fmt.Errorf("failed to put attachment %s on document %s: %w", name, docID, err)
	}
	return newRev, nil
}

// GetAttachment retrieves an attachment from a document
func (c *Client) GetAttachment(ctx context.Context, docID, name string) ([]byte, string, error) {
	att, err := c.db.GetAttachment(ctx, docID, name)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get attachment %s from document %s: %w", name, docID, err)
	}
	defer att.Content.Close()

	// Read attachment content
	content, err := io.ReadAll(att.Content)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read attachment content: %w", err)
	}

	return content, att.ContentType, nil
}

// DeleteAttachment removes an attachment from a document
func (c *Client) DeleteAttachment(ctx context.Context, docID, rev, name string) (string, error) {
	// Note: DeleteAttachment signature is (ctx, docID, rev, filename, options...)
	newRev, err := c.db.DeleteAttachment(ctx, docID, rev, name)
	if err != nil {
		return "", fmt.Errorf("failed to delete attachment %s from document %s: %w", name, docID, err)
	}
	return newRev, nil
}
