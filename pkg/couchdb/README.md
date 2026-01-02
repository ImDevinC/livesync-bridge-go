# CouchDB Client Package

This package provides a CouchDB client wrapper for the livesync-bridge project, built on top of the [Kivik](https://github.com/go-kivik/kivik) library.

## Features

- **CRUD Operations**: Create, Read, Update, Delete documents
- **Bulk Operations**: Efficient batch document operations
- **Changes Feed**: Real-time monitoring of database changes
- **Attachments**: Full attachment support (upload, download, delete)
- **Authentication**: Cookie-based authentication via DSN
- **Error Handling**: Comprehensive error wrapping with context

## Usage

### Creating a Client

```go
import (
    "context"
    "time"
    "github.com/imdevinc/livesync-bridge/pkg/couchdb"
)

ctx := context.Background()
cfg := couchdb.Config{
    URL:      "http://localhost:5984",
    Username: "admin",
    Password: "password",
    Database: "my_database",
    Timeout:  30 * time.Second,
}

client, err := couchdb.NewClient(ctx, cfg)
if err != nil {
    // Handle error
}
defer client.Close()
```

### Document Operations

```go
// Create/Update document
rev, err := client.Put(ctx, "doc-id", map[string]interface{}{
    "name": "John Doe",
    "email": "john@example.com",
})

// Get document
doc, err := client.Get(ctx, "doc-id")

// Delete document
err = client.Delete(ctx, "doc-id", rev)

// List all documents
docs, err := client.AllDocs(ctx, "") // or with prefix: client.AllDocs(ctx, "prefix-")
```

### Bulk Operations

```go
docs := []interface{}{
    map[string]interface{}{"_id": "doc1", "name": "First"},
    map[string]interface{}{"_id": "doc2", "name": "Second"},
}

results, err := client.BulkDocs(ctx, docs)
for _, result := range results {
    if result.Error != "" {
        // Handle error for this document
    }
}
```

### Changes Feed

```go
opts := couchdb.ChangesOptions{
    Since:       "now",
    IncludeDocs: true,
    Continuous:  true,
    Heartbeat:   30 * time.Second,
}

changesChan, errChan := client.Changes(ctx, opts)

for {
    select {
    case change := <-changesChan:
        fmt.Printf("Change: %s (seq: %s)\n", change.ID, change.Seq)
        if change.Doc != nil {
            // Process document
        }
    case err := <-errChan:
        // Handle error
    case <-ctx.Done():
        return
    }
}
```

### Attachments

```go
// Upload attachment
content := []byte("file content")
rev, err := client.PutAttachment(ctx, "doc-id", currentRev, "file.txt", "text/plain", content)

// Download attachment
content, contentType, err := client.GetAttachment(ctx, "doc-id", "file.txt")

// Delete attachment
newRev, err := client.DeleteAttachment(ctx, "doc-id", currentRev, "file.txt")
```

## Testing

The package includes comprehensive integration tests that run against a real CouchDB instance.

### Prerequisites

You need a running CouchDB instance. The easiest way is using Docker:

```bash
docker run -d \
  --name couchdb-test \
  -p 5984:5984 \
  -e COUCHDB_USER=admin \
  -e COUCHDB_PASSWORD=password \
  couchdb:latest
```

Wait a few seconds for CouchDB to initialize, then verify it's running:

```bash
curl http://admin:password@localhost:5984
```

### Running Tests

Run all tests with default configuration (localhost:5984):

```bash
go test ./pkg/couchdb -v
```

Or specify custom CouchDB connection via environment variables:

```bash
COUCHDB_URL=http://localhost:5984 \
COUCHDB_USERNAME=admin \
COUCHDB_PASSWORD=password \
go test ./pkg/couchdb -v
```

### Test Coverage

The test suite includes:

- **Connection Tests** (`TestNewClient`, `TestNewClientErrors`)
  - Client creation
  - Error handling
  - Configuration validation

- **CRUD Tests** (`TestPutAndGet`, `TestUpdate`, `TestDelete`)
  - Document creation
  - Document retrieval
  - Document updates
  - Document deletion

- **Query Tests** (`TestAllDocs`)
  - List all documents
  - Prefix filtering

- **Bulk Operations** (`TestBulkDocs`)
  - Batch document creation
  - Error handling per document

- **Changes Feed Tests** (`TestChanges`, `TestChangesContinuous`)
  - One-time changes feed
  - Continuous monitoring
  - Document inclusion
  - Sequence tracking

- **Attachment Tests** (`TestPutAttachment`, `TestGetAttachment`, `TestDeleteAttachment`)
  - Upload attachments
  - Download attachments
  - Delete attachments
  - Content type handling

### Skipping Tests

If CouchDB is not available, tests will be automatically skipped with a message:

```
CouchDB not available for integration tests. Set COUCHDB_URL environment variable to run tests.
```

### Cleanup

The test suite automatically:
- Creates a test database (`livesync_test`) before each test
- Cleans up all test data after tests complete
- Removes the test database

To manually stop and remove the Docker container:

```bash
docker stop couchdb-test
docker rm couchdb-test
```

## Implementation Notes

### Kivik v4 API

This client uses Kivik v4, which has some API differences from earlier versions:

- Authentication via DSN URL instead of separate `Authenticate()` method
- `client.DBExists(ctx, name)` instead of `db.Exists(ctx)`
- `client.Close()` takes no arguments
- `BulkDocs` returns `[]BulkResult` directly (not an iterator)
- Changes feed uses `Changes()` method returning revisions as `[]string`

### Error Handling

All methods wrap errors with context using `fmt.Errorf` and `%w` for proper error chain preservation.

### Concurrency

The Changes feed runs in a goroutine and returns channels for asynchronous processing. Always ensure proper context cancellation to avoid goroutine leaks.

## Dependencies

- `github.com/go-kivik/kivik/v4` - CouchDB client library
- `github.com/go-kivik/kivik/v4/couchdb` - CouchDB driver

## License

Part of the livesync-bridge project.
