# CouchDB Peer

The CouchDB peer provides synchronization between local storage and Self-hosted LiveSync CouchDB vaults with end-to-end encryption (E2EE).

## Features

- **End-to-End Encryption (E2EE)**: AES-256-GCM encryption with PBKDF2 key derivation
- **Compression**: Optional gzip compression for efficient storage
- **Chunking**: Automatic chunking for files larger than configurable threshold (default 100KB)
- **Real-time Sync**: Continuous monitoring of CouchDB changes feed
- **LiveSync Compatible**: Full compatibility with Self-hosted LiveSync document format

## Configuration

```json
{
  "type": "couchdb",
  "name": "remote-sync",
  "group": "sync",
  "url": "http://localhost:5984",
  "username": "admin",
  "password": "password",
  "database": "test_vault",
  "passphrase": "test-encryption-key",
  "baseDir": "",
  "enableCompression": true,
  "customChunkSize": 102400
}
```

### Configuration Fields

- `type`: Must be `"couchdb"`
- `name`: Unique peer name
- `group`: Sync group (peers in same group sync with each other)
- `url`: CouchDB server URL (e.g., `http://localhost:5984`)
- `username`: CouchDB username
- `password`: CouchDB password
- `database`: Database name in CouchDB
- `passphrase`: Encryption passphrase for E2EE
- `baseDir`: Optional base directory path prefix (usually empty for CouchDB)
- `enableCompression`: Enable/disable gzip compression (optional, default: true)
- `customChunkSize`: Chunk size threshold in bytes (optional, default: 102400)

## Architecture

### Encryption Pipeline

1. **Key Derivation**: Uses PBKDF2 with 100,000 iterations
   - Passphrase + database name as salt → 32-byte AES key
   
2. **Encryption**: AES-256-GCM
   - Generates random 12-byte nonce for each encryption
   - Authenticated encryption with integrity check
   - Format: `[nonce (12 bytes)][encrypted data][auth tag (16 bytes)]`

3. **Compression** (if enabled):
   - Applies gzip compression after encryption
   - Only stores compressed version if smaller than original
   - Compression metadata stored in document

### Document Format

#### Regular Documents (LiveSyncDocument)
```json
{
  "_id": "folder:subfolder:file.md",
  "type": "plain",
  "path": "folder:subfolder:file.md",
  "data": "<base64-encoded-encrypted-data>",
  "ctime": 1704067200000,
  "mtime": 1704153600000,
  "size": 1024
}
```

#### Chunk Documents (for large files)
```json
{
  "_id": "h:abc123def456...",
  "type": "leaf",
  "data": "<base64-encoded-encrypted-chunk>"
}
```

Main document references chunks in `data` field as comma-separated list of chunk IDs.

### Document Types

- `plain`: Regular text file (markdown, code, etc.)
- `newnote`: Newly created note (same as plain)
- `leaf`: Chunk document containing part of a large file

### Path Conversion

- Filesystem: `/folder/subfolder/file.md`
- Document ID: `folder:subfolder:file.md`
- Rationale: CouchDB uses `:` separator (LiveSync convention)

## Testing

### Unit Tests (No CouchDB Required)

```bash
go test ./internal/couchdbpeer -v
```

Tests that don't require CouchDB:
- `TestDeriveKey` - Key derivation ✅
- `TestCompressDecompress` - Compression ✅
- `TestCompressSmallData` - Edge case compression ✅
- `TestDocPathToID` - Path conversion ✅
- `TestIsPlainText` - File type detection ✅

### Integration Tests (Requires CouchDB)

#### Setup CouchDB with Docker

```bash
# Start CouchDB
docker run -d \
  --name couchdb-test \
  -p 5984:5984 \
  -e COUCHDB_USER=admin \
  -e COUCHDB_PASSWORD=password \
  couchdb:latest

# Wait for startup
sleep 5

# Verify CouchDB is running
curl http://localhost:5984/
```

#### Run Integration Tests

```bash
# Set environment variable
export COUCHDB_URL=http://localhost:5984

# Run all tests
go test ./internal/couchdbpeer -v

# Or run specific test
go test ./internal/couchdbpeer -v -run TestPutGetDelete
```

Integration tests (all passing ✅):
- `TestEncryptDecrypt` - Encryption round-trip ✅
- `TestEncryptDecryptEmpty` - Empty data encryption ✅
- `TestPutGetDelete` - Basic CRUD operations ✅
- `TestPutLargeFile` - Chunking and reassembly (75KB file) ✅
- `TestEncryptionRoundTrip` - Verify data is encrypted in database ✅
- `TestRepeatingDetection` - Deduplication ✅
- `TestType` - Peer type getter ✅
- `TestStartStop` - Lifecycle management ✅

**Test Coverage**: 29.8% (with integration tests)

**Note**: The tests automatically create and clean up the test database `livesync_peer_test`.

#### Cleanup

```bash
# Stop and remove CouchDB container
docker stop couchdb-test
docker rm couchdb-test
```

### End-to-End Testing

Test synchronization between storage peer and CouchDB peer:

```bash
# Start CouchDB
docker run -d --name couchdb-test -p 5984:5984 \
  -e COUCHDB_USER=admin \
  -e COUCHDB_PASSWORD=password \
  couchdb:latest

# Create test database
sleep 5
curl -X PUT http://admin:password@localhost:5984/test_vault

# Run bridge with test config
./bin/livesync-bridge --config configs/couchdb-test-config.json

# In another terminal, add files to test-data/vault1/
mkdir -p test-data/vault1
echo "# Test Document" > test-data/vault1/test.md

# Verify file appears in CouchDB
curl http://admin:password@localhost:5984/test_vault/_all_docs

# Verify file is encrypted
curl http://admin:password@localhost:5984/test_vault/test.md
# Should see encrypted base64 data, not plaintext
```

## Performance

### Benchmarks

```bash
go test ./internal/couchdbpeer -bench=. -benchmem
```

- `BenchmarkEncryption` - Encryption performance (1MB data)
- `BenchmarkCompression` - Compression performance (1MB data)

### Typical Performance (on modern hardware)

- Encryption: ~100-200 MB/s
- Compression: ~50-100 MB/s (depends on data compressibility)
- Chunking overhead: ~5-10ms per file >100KB

## Troubleshooting

### Connection Issues

**Error**: `dial tcp 127.0.0.1:5984: connect: connection refused`

Solution: Ensure CouchDB is running and accessible at the configured URL.

```bash
curl http://localhost:5984/
```

### Authentication Issues

**Error**: `401 Unauthorized`

Solution: Verify username and password in configuration match CouchDB credentials.

### Database Not Found

**Error**: `404 Not Found`

Solution: Create the database manually or ensure auto-creation is enabled.

```bash
curl -X PUT http://admin:password@localhost:5984/database_name
```

### Encryption Key Mismatch

**Symptom**: Files sync but content is garbled

Solution: Ensure all peers syncing to the same CouchDB database use the **same passphrase**.

### Performance Issues

**Symptom**: Slow sync for large files

Solutions:
- Enable compression: `"enableCompression": true`
- Adjust chunk size: `"customChunkSize": 204800` (200KB)
- Check network latency to CouchDB server

## Implementation Details

### Changes Feed Monitoring

The peer continuously monitors CouchDB's `_changes` feed:

```go
changes, err := client.Changes(ctx, kivik.Options{
    "feed":         "continuous",
    "include_docs": true,
    "since":        lastSeq,
})
```

### Deduplication

Uses hash-based deduplication to avoid redundant operations:

```go
if p.IsRepeating(path, fileData.Data) {
    return false, nil // Skip duplicate
}
p.MarkProcessed(path, fileData.Data)
```

### Sequence Tracking

Stores last processed sequence number in persistent storage:

```go
p.SetSetting("last_sequence", seq)
```

Resumes from last sequence on restart for efficient incremental sync.

## Security Considerations

1. **Passphrase Storage**: Passphrases are stored in configuration files. Consider using environment variables or secret management systems for production.

2. **TLS/SSL**: Always use HTTPS for CouchDB connections in production:
   ```json
   "url": "https://couchdb.example.com:6984"
   ```

3. **Key Derivation**: Uses PBKDF2 with 100,000 iterations. The database name is used as salt, so each database has a unique derived key even with the same passphrase.

4. **Nonce Uniqueness**: Each encryption operation uses a cryptographically random nonce, preventing nonce reuse attacks.

5. **Authenticated Encryption**: AES-GCM provides both confidentiality and integrity/authenticity.

## Future Enhancements

- [ ] Support for CouchDB conflict resolution
- [ ] Configurable PBKDF2 iterations
- [ ] Support for multiple encryption algorithms
- [ ] Attachment optimization for media files
- [ ] Selective sync (filter by path patterns)
- [ ] Two-way sync conflict detection and resolution

## Related Documentation

- [CouchDB Client Documentation](../../pkg/couchdb/README.md)
- [Self-hosted LiveSync](https://github.com/vrtmrz/obsidian-livesync)
- [Project Configuration](../../configs/config.sample.json)
