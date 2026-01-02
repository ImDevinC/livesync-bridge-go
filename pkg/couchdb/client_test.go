package couchdb

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-kivik/kivik/v4"
)

// Test configuration - can be overridden with environment variables
const (
	defaultTestURL      = "http://localhost:5984"
	defaultTestUsername = "admin"
	defaultTestPassword = "password"
	testDatabaseName    = "livesync_test"
)

// getTestConfig returns test configuration from environment or defaults
func getTestConfig() Config {
	url := os.Getenv("COUCHDB_URL")
	if url == "" {
		url = defaultTestURL
	}

	username := os.Getenv("COUCHDB_USERNAME")
	if username == "" {
		username = defaultTestUsername
	}

	password := os.Getenv("COUCHDB_PASSWORD")
	if password == "" {
		password = defaultTestPassword
	}

	return Config{
		URL:      url,
		Username: username,
		Password: password,
		Database: testDatabaseName,
		Timeout:  30 * time.Second,
	}
}

// setupTestDB creates a test database and returns a client
func setupTestDB(t *testing.T) (*Client, context.Context, func()) {
	t.Helper()
	ctx := context.Background()
	cfg := getTestConfig()

	// First connect without specifying database to create it
	cfg.Database = ""
	tempClient, err := createClientWithoutDB(ctx, cfg)
	if err != nil {
		t.Skipf("CouchDB not available for integration tests: %v", err)
		return nil, nil, nil
	}

	// Create test database
	err = tempClient.client.CreateDB(ctx, testDatabaseName)
	if err != nil {
		// Database might already exist, try to delete and recreate
		tempClient.client.DestroyDB(ctx, testDatabaseName)
		err = tempClient.client.CreateDB(ctx, testDatabaseName)
		if err != nil {
			tempClient.Close()
			t.Skipf("Failed to create test database (CouchDB may not be available): %v", err)
			return nil, nil, nil
		}
	}
	tempClient.Close()

	// Now connect to the test database
	cfg.Database = testDatabaseName
	client, err := NewClient(ctx, cfg)
	if err != nil {
		t.Skipf("Failed to create test client: %v", err)
		return nil, nil, nil
	}

	// Cleanup function
	cleanup := func() {
		client.Close()
		// Reconnect to delete database
		cfg.Database = ""
		tempClient, _ := createClientWithoutDB(ctx, cfg)
		if tempClient != nil {
			tempClient.client.DestroyDB(ctx, testDatabaseName)
			tempClient.Close()
		}
	}

	return client, ctx, cleanup
}

// createClientWithoutDB creates a client without requiring a database to exist
func createClientWithoutDB(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("CouchDB URL is required")
	}

	// Build DSN with authentication if provided
	dsn := cfg.URL
	if cfg.Username != "" && cfg.Password != "" {
		// Parse and add credentials to URL
		// For simplicity, just use the URL as-is for now
		dsn = fmt.Sprintf("http://%s:%s@%s", cfg.Username, cfg.Password, cfg.URL[7:]) // Strip http://
	}

	client, err := kivik.New("couch", dsn)
	if err != nil {
		return nil, err
	}

	return &Client{
		client:   client,
		username: cfg.Username,
		url:      cfg.URL,
	}, nil
}

// TestNewClient tests client creation
func TestNewClient(t *testing.T) {
	client, ctx, cleanup := setupTestDB(t)
	if client == nil {
		return // Skipped
	}
	defer cleanup()

	// Verify client properties
	if client.DBName() != testDatabaseName {
		t.Errorf("Expected database name %s, got %s", testDatabaseName, client.DBName())
	}

	if client.Username() != getTestConfig().Username {
		t.Errorf("Expected username %s, got %s", getTestConfig().Username, client.Username())
	}

	// Test that we can close the client
	err := client.Close()
	if err != nil {
		t.Errorf("Failed to close client: %v", err)
	}

	// Create a new client for cleanup
	_, err = NewClient(ctx, getTestConfig())
	if err != nil {
		t.Fatalf("Failed to recreate client: %v", err)
	}
}

// TestNewClientErrors tests error cases in client creation
func TestNewClientErrors(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "missing URL",
			config:  Config{Database: "test"},
			wantErr: true,
		},
		{
			name:    "missing database",
			config:  Config{URL: "http://localhost:5984"},
			wantErr: true,
		},
		{
			name: "non-existent database",
			config: Config{
				URL:      getTestConfig().URL,
				Username: getTestConfig().Username,
				Password: getTestConfig().Password,
				Database: "nonexistent_db_12345",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(ctx, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestPutAndGet tests creating and retrieving documents
func TestPutAndGet(t *testing.T) {
	client, ctx, cleanup := setupTestDB(t)
	if client == nil {
		return
	}
	defer cleanup()

	// Create a test document
	docID := "test-doc-1"
	testDoc := map[string]interface{}{
		"type":    "test",
		"message": "Hello CouchDB",
		"count":   42,
	}

	// Put document
	rev, err := client.Put(ctx, docID, testDoc)
	if err != nil {
		t.Fatalf("Failed to put document: %v", err)
	}
	if rev == "" {
		t.Error("Expected revision, got empty string")
	}

	// Get document
	doc, err := client.Get(ctx, docID)
	if err != nil {
		t.Fatalf("Failed to get document: %v", err)
	}

	if doc.ID != docID {
		t.Errorf("Expected ID %s, got %s", docID, doc.ID)
	}
	if doc.Rev != rev {
		t.Errorf("Expected revision %s, got %s", rev, doc.Rev)
	}
}

// TestUpdate tests updating an existing document
func TestUpdate(t *testing.T) {
	client, ctx, cleanup := setupTestDB(t)
	if client == nil {
		return
	}
	defer cleanup()

	docID := "test-doc-update"

	// Create initial document
	initialDoc := map[string]interface{}{
		"value": 1,
	}
	rev1, err := client.Put(ctx, docID, initialDoc)
	if err != nil {
		t.Fatalf("Failed to create document: %v", err)
	}

	// Update document (must include revision)
	updatedDoc := map[string]interface{}{
		"_rev":  rev1,
		"value": 2,
	}
	rev2, err := client.Put(ctx, docID, updatedDoc)
	if err != nil {
		t.Fatalf("Failed to update document: %v", err)
	}

	if rev1 == rev2 {
		t.Error("Expected revision to change after update")
	}

	// Verify update
	doc, err := client.Get(ctx, docID)
	if err != nil {
		t.Fatalf("Failed to get updated document: %v", err)
	}
	if doc.Rev != rev2 {
		t.Errorf("Expected revision %s, got %s", rev2, doc.Rev)
	}
}

// TestDelete tests deleting documents
func TestDelete(t *testing.T) {
	client, ctx, cleanup := setupTestDB(t)
	if client == nil {
		return
	}
	defer cleanup()

	docID := "test-doc-delete"

	// Create document
	testDoc := map[string]interface{}{"name": "to-delete"}
	rev, err := client.Put(ctx, docID, testDoc)
	if err != nil {
		t.Fatalf("Failed to create document: %v", err)
	}

	// Delete document
	err = client.Delete(ctx, docID, rev)
	if err != nil {
		t.Fatalf("Failed to delete document: %v", err)
	}

	// Verify deletion
	_, err = client.Get(ctx, docID)
	if err == nil {
		t.Error("Expected error when getting deleted document")
	}
}

// TestAllDocs tests listing all documents
func TestAllDocs(t *testing.T) {
	client, ctx, cleanup := setupTestDB(t)
	if client == nil {
		return
	}
	defer cleanup()

	// Create multiple documents
	docs := []struct {
		id   string
		data map[string]interface{}
	}{
		{"doc1", map[string]interface{}{"type": "a"}},
		{"doc2", map[string]interface{}{"type": "b"}},
		{"prefix-1", map[string]interface{}{"type": "c"}},
		{"prefix-2", map[string]interface{}{"type": "d"}},
	}

	for _, doc := range docs {
		_, err := client.Put(ctx, doc.id, doc.data)
		if err != nil {
			t.Fatalf("Failed to create document %s: %v", doc.id, err)
		}
	}

	// Test: Get all documents
	allDocs, err := client.AllDocs(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get all docs: %v", err)
	}
	if len(allDocs) < len(docs) {
		t.Errorf("Expected at least %d documents, got %d", len(docs), len(allDocs))
	}

	// Test: Get documents with prefix
	prefixDocs, err := client.AllDocs(ctx, "prefix-")
	if err != nil {
		t.Fatalf("Failed to get prefix docs: %v", err)
	}
	if len(prefixDocs) != 2 {
		t.Errorf("Expected 2 documents with prefix, got %d", len(prefixDocs))
	}
}

// TestBulkDocs tests bulk operations
func TestBulkDocs(t *testing.T) {
	client, ctx, cleanup := setupTestDB(t)
	if client == nil {
		return
	}
	defer cleanup()

	// Create multiple documents in bulk
	bulkDocs := []interface{}{
		map[string]interface{}{
			"_id":  "bulk1",
			"name": "Bulk Document 1",
		},
		map[string]interface{}{
			"_id":  "bulk2",
			"name": "Bulk Document 2",
		},
		map[string]interface{}{
			"_id":  "bulk3",
			"name": "Bulk Document 3",
		},
	}

	results, err := client.BulkDocs(ctx, bulkDocs)
	if err != nil {
		t.Fatalf("Failed to perform bulk operation: %v", err)
	}

	if len(results) != len(bulkDocs) {
		t.Errorf("Expected %d results, got %d", len(bulkDocs), len(results))
	}

	// Verify all succeeded
	for i, result := range results {
		if result.Error != "" {
			t.Errorf("Bulk operation %d failed: %s", i, result.Error)
		}
		if result.Rev == "" {
			t.Errorf("Expected revision for result %d", i)
		}
	}

	// Verify documents were created
	doc, err := client.Get(ctx, "bulk1")
	if err != nil {
		t.Errorf("Failed to get bulk document: %v", err)
	}
	if doc.ID != "bulk1" {
		t.Errorf("Expected ID bulk1, got %s", doc.ID)
	}
}

// TestChanges tests the changes feed
func TestChanges(t *testing.T) {
	client, ctx, cleanup := setupTestDB(t)
	if client == nil {
		return
	}
	defer cleanup()

	// Start changes feed in background
	opts := ChangesOptions{
		IncludeDocs: true,
		Limit:       10,
	}

	changesChan, errChan := client.Changes(ctx, opts)

	// Create some documents
	numDocs := 3
	for i := 0; i < numDocs; i++ {
		docID := fmt.Sprintf("change-doc-%d", i)
		_, err := client.Put(ctx, docID, map[string]interface{}{
			"index": i,
		})
		if err != nil {
			t.Fatalf("Failed to create document: %v", err)
		}
	}

	// Read changes
	changesReceived := 0
	timeout := time.After(5 * time.Second)

	for changesReceived < numDocs {
		select {
		case change, ok := <-changesChan:
			if !ok {
				t.Fatal("Changes channel closed unexpectedly")
			}
			changesReceived++
			if change.ID == "" {
				t.Error("Expected change to have document ID")
			}
			if change.Seq == "" {
				t.Error("Expected change to have sequence")
			}
			if opts.IncludeDocs && change.Doc == nil {
				t.Error("Expected change to include document")
			}
		case err := <-errChan:
			t.Fatalf("Changes feed error: %v", err)
		case <-timeout:
			t.Fatalf("Timeout waiting for changes. Received %d/%d", changesReceived, numDocs)
		}
	}

	if changesReceived != numDocs {
		t.Errorf("Expected %d changes, got %d", numDocs, changesReceived)
	}
}

// TestChangesContinuous tests continuous changes feed
func TestChangesContinuous(t *testing.T) {
	client, ctx, cleanup := setupTestDB(t)
	if client == nil {
		return
	}
	defer cleanup()

	// Create a cancellable context
	feedCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start continuous changes feed
	opts := ChangesOptions{
		Continuous:  true,
		IncludeDocs: true,
		Heartbeat:   1 * time.Second,
	}

	changesChan, errChan := client.Changes(feedCtx, opts)

	// Create documents after starting feed
	go func() {
		time.Sleep(100 * time.Millisecond)
		for i := 0; i < 3; i++ {
			docID := fmt.Sprintf("continuous-doc-%d", i)
			client.Put(ctx, docID, map[string]interface{}{"value": i})
			time.Sleep(100 * time.Millisecond)
		}
	}()

	// Collect changes
	changesReceived := 0
	timeout := time.After(5 * time.Second)

	for changesReceived < 3 {
		select {
		case change, ok := <-changesChan:
			if !ok {
				// Channel closed, which is expected when context is cancelled
				return
			}
			changesReceived++
			t.Logf("Received change: %s (seq: %s)", change.ID, change.Seq)
		case err := <-errChan:
			// This might be a context cancellation error, which is expected
			if feedCtx.Err() != nil {
				return
			}
			t.Fatalf("Changes feed error: %v", err)
		case <-timeout:
			cancel() // Stop the feed
			t.Fatalf("Timeout waiting for continuous changes. Received %d/3", changesReceived)
		}
	}

	// Cancel context to stop feed
	cancel()
}

// TestPutAttachment tests adding attachments
func TestPutAttachment(t *testing.T) {
	client, ctx, cleanup := setupTestDB(t)
	if client == nil {
		return
	}
	defer cleanup()

	docID := "doc-with-attachment"

	// Create base document
	rev, err := client.Put(ctx, docID, map[string]interface{}{
		"name": "Document with attachment",
	})
	if err != nil {
		t.Fatalf("Failed to create document: %v", err)
	}

	// Add attachment
	attachmentName := "test.txt"
	attachmentContent := []byte("Hello, this is test content!")
	contentType := "text/plain"

	newRev, err := client.PutAttachment(ctx, docID, rev, attachmentName, contentType, attachmentContent)
	if err != nil {
		t.Fatalf("Failed to put attachment: %v", err)
	}

	if newRev == rev {
		t.Error("Expected revision to change after adding attachment")
	}

	// Verify document has attachment
	doc, err := client.Get(ctx, docID)
	if err != nil {
		t.Fatalf("Failed to get document: %v", err)
	}

	if len(doc.Attachments) == 0 {
		t.Error("Expected document to have attachments")
	}
}

// TestGetAttachment tests retrieving attachments
func TestGetAttachment(t *testing.T) {
	client, ctx, cleanup := setupTestDB(t)
	if client == nil {
		return
	}
	defer cleanup()

	docID := "doc-get-attachment"
	attachmentName := "data.json"
	originalContent := []byte(`{"key": "value", "number": 123}`)
	contentType := "application/json"

	// Create document with attachment
	rev, err := client.Put(ctx, docID, map[string]interface{}{"type": "test"})
	if err != nil {
		t.Fatalf("Failed to create document: %v", err)
	}

	_, err = client.PutAttachment(ctx, docID, rev, attachmentName, contentType, originalContent)
	if err != nil {
		t.Fatalf("Failed to put attachment: %v", err)
	}

	// Get attachment
	content, retrievedType, err := client.GetAttachment(ctx, docID, attachmentName)
	if err != nil {
		t.Fatalf("Failed to get attachment: %v", err)
	}

	// Verify content
	if string(content) != string(originalContent) {
		t.Errorf("Expected content %s, got %s", string(originalContent), string(content))
	}

	// Verify content type
	if retrievedType != contentType {
		t.Errorf("Expected content type %s, got %s", contentType, retrievedType)
	}
}

// TestDeleteAttachment tests removing attachments
func TestDeleteAttachment(t *testing.T) {
	client, ctx, cleanup := setupTestDB(t)
	if client == nil {
		return
	}
	defer cleanup()

	docID := "doc-delete-attachment"
	attachmentName := "to-delete.txt"

	// Create document with attachment
	rev, err := client.Put(ctx, docID, map[string]interface{}{"type": "test"})
	if err != nil {
		t.Fatalf("Failed to create document: %v", err)
	}

	rev, err = client.PutAttachment(ctx, docID, rev, attachmentName, "text/plain", []byte("delete me"))
	if err != nil {
		t.Fatalf("Failed to put attachment: %v", err)
	}

	// Delete attachment
	newRev, err := client.DeleteAttachment(ctx, docID, rev, attachmentName)
	if err != nil {
		t.Fatalf("Failed to delete attachment: %v", err)
	}

	if newRev == rev {
		t.Error("Expected revision to change after deleting attachment")
	}

	// Verify attachment is gone
	_, _, err = client.GetAttachment(ctx, docID, attachmentName)
	if err == nil {
		t.Error("Expected error when getting deleted attachment")
	}
}
