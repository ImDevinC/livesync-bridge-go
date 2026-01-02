package couchdbpeer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-kivik/kivik/v4"
	_ "github.com/go-kivik/kivik/v4/couchdb"
	"github.com/imdevinc/livesync-bridge/internal/config"
	"github.com/imdevinc/livesync-bridge/internal/peer"
	"github.com/imdevinc/livesync-bridge/internal/storage"
)

// Test configuration
const (
	testCouchDBURL      = "http://localhost:5984"
	testCouchDBUsername = "admin"
	testCouchDBPassword = "password"
	testCouchDBDatabase = "livesync_peer_test"
	testPassphrase      = "test-passphrase-123"
)

// getTestConfig returns a test configuration
func getTestConfig() config.PeerCouchDBConf {
	enableCompression := true
	customChunkSize := 50 * 1024 // 50KB for testing

	return config.PeerCouchDBConf{
		Type:              "couchdb",
		Name:              "test-peer",
		Group:             "test",
		BaseDir:           "test/",
		URL:               testCouchDBURL,
		Username:          testCouchDBUsername,
		Password:          testCouchDBPassword,
		Database:          testCouchDBDatabase,
		Passphrase:        testPassphrase,
		EnableCompression: &enableCompression,
		CustomChunkSize:   &customChunkSize,
	}
}

// setupTestPeer creates a test peer with in-memory storage
func setupTestPeer(t *testing.T) (*CouchDBPeer, *storage.Store, func()) {
	t.Helper()

	// Ensure test database exists
	if err := ensureTestDatabase(t); err != nil {
		t.Skipf("CouchDB not available for integration tests: %v", err)
		return nil, nil, nil
	}

	// Create temporary database file
	tmpDB := t.TempDir() + "/test.db"
	store, err := storage.NewStore(tmpDB)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Dummy dispatcher
	dispatcher := func(source peer.Peer, path string, data *peer.FileData) error {
		return nil
	}

	// Create peer
	cfg := getTestConfig()
	p, err := NewCouchDBPeer(cfg, dispatcher, store)
	if err != nil {
		store.Close()
		t.Skipf("CouchDB not available for integration tests: %v", err)
		return nil, nil, nil
	}

	cleanup := func() {
		if p != nil {
			p.Stop()
		}
		if store != nil {
			store.Close()
		}
		// Clean up test database
		cleanupTestDatabase(t)
	}

	return p, store, cleanup
}

// ensureTestDatabase creates the test database if it doesn't exist
func ensureTestDatabase(t *testing.T) error {
	t.Helper()

	ctx := context.Background()

	// Build DSN with authentication
	dsn := fmt.Sprintf("%s://%s:%s@%s",
		"http",
		testCouchDBUsername,
		testCouchDBPassword,
		strings.TrimPrefix(testCouchDBURL, "http://"))

	// Create Kivik client
	client, err := kivik.New("couch", dsn)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	// Check if database exists
	exists, err := client.DBExists(ctx, testCouchDBDatabase)
	if err != nil {
		return fmt.Errorf("failed to check database existence: %w", err)
	}

	// Create database if it doesn't exist
	if !exists {
		if err := client.CreateDB(ctx, testCouchDBDatabase); err != nil {
			return fmt.Errorf("failed to create database: %w", err)
		}
		t.Logf("Created test database: %s", testCouchDBDatabase)
	}

	return nil
}

// cleanupTestDatabase removes the test database
func cleanupTestDatabase(t *testing.T) {
	t.Helper()

	ctx := context.Background()

	// Build DSN with authentication
	dsn := fmt.Sprintf("%s://%s:%s@%s",
		"http",
		testCouchDBUsername,
		testCouchDBPassword,
		strings.TrimPrefix(testCouchDBURL, "http://"))

	// Create Kivik client
	client, err := kivik.New("couch", dsn)
	if err != nil {
		return // CouchDB not available, skip cleanup
	}
	defer client.Close()

	// Delete the test database
	if err := client.DestroyDB(ctx, testCouchDBDatabase); err != nil {
		// Ignore errors during cleanup
		t.Logf("Warning: Failed to delete test database: %v", err)
	} else {
		t.Logf("Deleted test database: %s", testCouchDBDatabase)
	}
}

// TestDeriveKey tests key derivation
func TestDeriveKey(t *testing.T) {
	passphrase := "test-password"
	salt := "test-salt"

	key1 := deriveKey(passphrase, salt)
	key2 := deriveKey(passphrase, salt)

	if len(key1) != KeySize {
		t.Errorf("Expected key size %d, got %d", KeySize, len(key1))
	}

	if !bytes.Equal(key1, key2) {
		t.Error("Same passphrase and salt should produce same key")
	}

	// Different salt should produce different key
	key3 := deriveKey(passphrase, "different-salt")
	if bytes.Equal(key1, key3) {
		t.Error("Different salt should produce different key")
	}
}

// TestEncryptDecrypt tests encryption and decryption
func TestEncryptDecrypt(t *testing.T) {
	p, _, cleanup := setupTestPeer(t)
	if p == nil {
		return
	}
	defer cleanup()

	testData := []byte("Hello, this is test data for encryption!")

	// Encrypt
	encrypted, err := p.encrypt(testData)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	if bytes.Equal(encrypted, testData) {
		t.Error("Encrypted data should be different from plaintext")
	}

	// Decrypt
	decrypted, err := p.decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decryption failed: %v", err)
	}

	if !bytes.Equal(decrypted, testData) {
		t.Error("Decrypted data doesn't match original")
	}
}

// TestEncryptDecryptEmpty tests encryption of empty data
func TestEncryptDecryptEmpty(t *testing.T) {
	p, _, cleanup := setupTestPeer(t)
	if p == nil {
		return
	}
	defer cleanup()

	testData := []byte{}

	encrypted, err := p.encrypt(testData)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	decrypted, err := p.decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decryption failed: %v", err)
	}

	if !bytes.Equal(decrypted, testData) {
		t.Error("Decrypted data doesn't match original")
	}
}

// TestCompressDecompress tests compression
func TestCompressDecompress(t *testing.T) {
	testData := []byte("This is some test data that should compress well. " +
		"Repeat. Repeat. Repeat. Repeat. Repeat. Repeat.")

	compressed, err := compress(testData)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}

	if len(compressed) >= len(testData) {
		t.Error("Compressed data should be smaller than original")
	}

	decompressed, err := decompress(compressed)
	if err != nil {
		t.Fatalf("Decompression failed: %v", err)
	}

	if !bytes.Equal(decompressed, testData) {
		t.Error("Decompressed data doesn't match original")
	}
}

// TestCompressSmallData tests compression of small data
func TestCompressSmallData(t *testing.T) {
	testData := []byte("hi")

	compressed, err := compress(testData)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}

	// Small data might not compress well
	decompressed, err := decompress(compressed)
	if err != nil {
		t.Fatalf("Decompression failed: %v", err)
	}

	if !bytes.Equal(decompressed, testData) {
		t.Error("Decompressed data doesn't match original")
	}
}

// TestDocPathToID tests document ID generation
func TestDocPathToID(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"document.md", "document.md"},
		{"/document.md", "document.md"},
		{"folder/document.md", "folder:document.md"},
		{"/folder/subfolder/doc.txt", "folder:subfolder:doc.txt"},
		{"test/", "test:"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := docPathToID(tt.path)
			if result != tt.expected {
				t.Errorf("docPathToID(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

// TestIsPlainText tests plain text detection
func TestIsPlainText(t *testing.T) {
	tests := []struct {
		path      string
		plainText bool
	}{
		{"document.md", true},
		{"file.txt", true},
		{"config.json", true},
		{"style.css", true},
		{"script.js", true},
		{"image.png", false},
		{"video.mp4", false},
		{"archive.zip", false},
		{"Document.MD", true}, // Case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isPlainText(tt.path)
			if result != tt.plainText {
				t.Errorf("isPlainText(%q) = %v, want %v", tt.path, result, tt.plainText)
			}
		})
	}
}

// TestPutGetDelete tests basic CRUD operations (requires CouchDB)
func TestPutGetDelete(t *testing.T) {
	// Skip if CouchDB URL not set or using default
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	p, _, cleanup := setupTestPeer(t)
	if p == nil {
		return
	}
	defer cleanup()

	// Start peer
	if err := p.Start(); err != nil {
		t.Fatalf("Failed to start peer: %v", err)
	}

	// Test data
	path := "test/document.md"
	content := []byte("# Test Document\n\nThis is test content.")
	fileData := &peer.FileData{
		CTime: time.Now(),
		MTime: time.Now(),
		Size:  int64(len(content)),
		Data:  content,
	}

	// Put document
	ok, err := p.Put(path, fileData)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if !ok {
		t.Error("Put returned false")
	}

	// Wait a bit for write to complete
	time.Sleep(100 * time.Millisecond)

	// Get document
	retrieved, err := p.Get(path)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !bytes.Equal(retrieved.Data, content) {
		t.Errorf("Retrieved data doesn't match. Got %q, want %q", string(retrieved.Data), string(content))
	}

	// Delete document
	ok, err = p.Delete(path)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if !ok {
		t.Error("Delete returned false")
	}

	// Wait for delete to complete
	time.Sleep(100 * time.Millisecond)

	// Verify deletion
	_, err = p.Get(path)
	if err == nil {
		t.Error("Expected error when getting deleted document")
	}
}

// TestPutLargeFile tests chunking (requires CouchDB)
func TestPutLargeFile(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	p, _, cleanup := setupTestPeer(t)
	if p == nil {
		return
	}
	defer cleanup()

	if err := p.Start(); err != nil {
		t.Fatalf("Failed to start peer: %v", err)
	}

	// Create large file (75KB, should trigger chunking with 50KB chunk size)
	path := "test/large-file.bin"
	content := make([]byte, 75*1024)
	for i := range content {
		content[i] = byte(i % 256)
	}

	fileData := &peer.FileData{
		CTime: time.Now(),
		MTime: time.Now(),
		Size:  int64(len(content)),
		Data:  content,
	}

	// Put large file
	ok, err := p.Put(path, fileData)
	if err != nil {
		t.Fatalf("Put large file failed: %v", err)
	}
	if !ok {
		t.Error("Put returned false")
	}

	time.Sleep(200 * time.Millisecond)

	// Get and verify
	retrieved, err := p.Get(path)
	if err != nil {
		t.Fatalf("Get large file failed: %v", err)
	}

	if !bytes.Equal(retrieved.Data, content) {
		t.Errorf("Retrieved data doesn't match (got %d bytes, want %d bytes)",
			len(retrieved.Data), len(content))
	}

	// Cleanup
	p.Delete(path)
}

// TestEncryptionRoundTrip tests full encryption pipeline
func TestEncryptionRoundTrip(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	p, _, cleanup := setupTestPeer(t)
	if p == nil {
		return
	}
	defer cleanup()

	if err := p.Start(); err != nil {
		t.Fatalf("Failed to start peer: %v", err)
	}

	// Test with sensitive data
	path := "test/secret.txt"
	content := []byte("This is secret data that should be encrypted!")

	fileData := &peer.FileData{
		CTime: time.Now(),
		MTime: time.Now(),
		Size:  int64(len(content)),
		Data:  content,
	}

	// Put encrypted file
	ok, err := p.Put(path, fileData)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if !ok {
		t.Error("Put returned false")
	}

	time.Sleep(100 * time.Millisecond)

	// Verify data is encrypted in database
	ctx := context.Background()
	docID := docPathToID(p.ToLocalPath(path))
	doc, err := p.client.Get(ctx, docID)
	if err != nil {
		t.Fatalf("Failed to get raw document: %v", err)
	}

	// Parse document as LiveSyncDocument
	docBytes, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("Failed to marshal document: %v", err)
	}

	var lsDoc LiveSyncDocument
	if err := json.Unmarshal(docBytes, &lsDoc); err != nil {
		t.Fatalf("Failed to parse LiveSyncDocument: %v", err)
	}

	// Verify data is base64 encoded and encrypted (not plaintext)
	decoded, err := base64.StdEncoding.DecodeString(lsDoc.Data)
	if err != nil {
		t.Error("Data field is not valid base64")
	}

	// The decoded data should NOT match plaintext (it should be encrypted)
	if bytes.Equal(decoded, content) {
		t.Error("Data in database is not encrypted!")
	}

	// Get through peer (should decrypt)
	retrieved, err := p.Get(path)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !bytes.Equal(retrieved.Data, content) {
		t.Error("Retrieved decrypted data doesn't match original")
	}

	// Cleanup
	p.Delete(path)
}

// TestRepeatingDetection tests that repeated puts are detected
func TestRepeatingDetection(t *testing.T) {
	if os.Getenv("COUCHDB_URL") == "" {
		t.Skip("Set COUCHDB_URL environment variable to run integration tests")
	}

	p, _, cleanup := setupTestPeer(t)
	if p == nil {
		return
	}
	defer cleanup()

	if err := p.Start(); err != nil {
		t.Fatalf("Failed to start peer: %v", err)
	}

	path := "test/repeat.txt"
	content := []byte("Same content")

	fileData := &peer.FileData{
		CTime: time.Now(),
		MTime: time.Now(),
		Size:  int64(len(content)),
		Data:  content,
	}

	// First put should succeed
	ok, err := p.Put(path, fileData)
	if err != nil {
		t.Fatalf("First put failed: %v", err)
	}
	if !ok {
		t.Error("First put returned false")
	}

	time.Sleep(50 * time.Millisecond)

	// Second put with same content should be detected as repeating
	ok, err = p.Put(path, fileData)
	if err != nil {
		t.Fatalf("Second put failed: %v", err)
	}
	if ok {
		t.Error("Expected second put to be detected as repeating")
	}

	// Cleanup
	p.Delete(path)
}

// TestType tests the Type method
func TestType(t *testing.T) {
	p, _, cleanup := setupTestPeer(t)
	if p == nil {
		return
	}
	defer cleanup()

	if p.Type() != "couchdb" {
		t.Errorf("Expected type 'couchdb', got '%s'", p.Type())
	}
}

// TestStartStop tests peer lifecycle
func TestStartStop(t *testing.T) {
	p, _, cleanup := setupTestPeer(t)
	if p == nil {
		return
	}
	defer cleanup()

	// Start
	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop
	if err := p.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// BenchmarkEncryption benchmarks encryption performance
func BenchmarkEncryption(b *testing.B) {
	cfg := getTestConfig()
	key := deriveKey(cfg.Passphrase, cfg.Database)

	p := &CouchDBPeer{
		encryptionKey: key,
	}

	data := make([]byte, 1024*1024) // 1MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.encrypt(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompression benchmarks compression performance
func BenchmarkCompression(b *testing.B) {
	data := make([]byte, 1024*1024) // 1MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := compress(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
