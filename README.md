# LiveSync Bridge - Go Port Project

[![CI](https://github.com/ImDevinC/livesync-bridge-go/actions/workflows/ci.yml/badge.svg)](https://github.com/ImDevinC/livesync-bridge-go/actions/workflows/ci.yml)
[![Release](https://github.com/ImDevinC/livesync-bridge-go/actions/workflows/release.yml/badge.svg)](https://github.com/ImDevinC/livesync-bridge-go/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/imdevinc/livesync-bridge-go)](https://goreportcard.com/report/github.com/imdevinc/livesync-bridge-go)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

This project is a Go-compatible reimplementation of [livesync-bridge](https://github.com/vrtmrz/livesync-bridge), which is currently written in TypeScript/Deno.

## Project Status

**Current Phase**: Phase 8 - Testing Complete ✅

### Completed
- ✅ Phase 1-4: Foundation (config, CLI, storage, cache, retry, hub, peer interface)
- ✅ Phase 5: Storage Peer (filesystem sync with file watching)
- ✅ Phase 6: CouchDB Client Wrapper (full Kivik v4 wrapper)
- ✅ Phase 7: CouchDB Peer (E2EE, compression, chunking, changes feed)
- ✅ Phase 8: Integration testing with live CouchDB - **All tests passing!**

### Test Status
- **Total Lines**: ~6,145 lines of Go code
- **Test Lines**: ~3,247 lines of test code
- **Unit Tests**: 67 tests passing (no CouchDB required)
- **Integration Tests**: 13/13 tests passing with CouchDB
- **Coverage**: 29.8% (couchdbpeer with integration tests)
- **Build Status**: ✅ Compiles successfully
- **End-to-End**: ✅ Storage ↔ CouchDB sync working with encryption

## What is LiveSync Bridge?

LiveSync Bridge is a custom replicator that synchronizes data between:
- **Self-hosted LiveSync remote vaults** (CouchDB databases with optional E2EE)
- **Local filesystem storage** (directory-based storage)

It enables multi-directional synchronization between any combination of peers, with support for:
- Different encryption passphrases per vault
- Folder-based selective synchronization
- Path obfuscation
- File change detection and processing

## Why Port to Go?

Potential benefits of a Go implementation:
- Single binary distribution (no runtime dependencies)
- Better performance for file operations
- Easier deployment in containerized environments
- Lower memory footprint
- Native concurrency support

## Project Structure

```
livesync-bridge-go/
├── cmd/
│   └── livesync-bridge/      # Main application entry point
├── internal/
│   ├── config/               # Configuration loading and types
│   ├── hub/                  # Central dispatcher
│   ├── peer/                 # Peer interface and base implementation
│   ├── storage/              # BoltDB persistent storage
│   ├── storagepeer/          # Filesystem peer (COMPLETE)
│   ├── couchdbpeer/          # CouchDB peer (COMPLETE)
│   └── util/                 # Cache, hash, path, retry utilities
├── pkg/
│   └── couchdb/              # CouchDB client wrapper (COMPLETE)
├── configs/                  # Configuration files
│   ├── config.sample.json
│   ├── test-config.json
│   └── couchdb-test-config.json
├── bin/                      # Compiled binaries
├── data/                     # Runtime data (BoltDB files)
└── README.md
```

## Documentation

- **[USAGE.md](USAGE.md)** - Complete usage guide with configuration examples
- **[RELEASE.md](RELEASE.md)** - Release process and versioning guide
- **[CHANGELOG.md](CHANGELOG.md)** - Version history and changes
- **[PLAN.md](PLAN.md)** - Original implementation plan
- **[TESTING.md](TESTING.md)** - Complete testing guide with examples
- **[pkg/couchdb/README.md](pkg/couchdb/README.md)** - CouchDB client library documentation
- **[internal/couchdbpeer/README.md](internal/couchdbpeer/README.md)** - CouchDB peer documentation

## Key Technical Components

1. **Hub** ✅ - Central coordinator that routes changes between peers
2. **Peer Interface** ✅ - Abstract interface for different peer types
3. **Storage Peer** ✅ - Filesystem-based peer with file watching (fsnotify)
4. **CouchDB Peer** ✅ - CouchDB-based peer with:
   - E2EE (AES-256-GCM encryption)
   - Compression (gzip)
   - Chunking for large files
   - Real-time changes feed monitoring
5. **Configuration** ✅ - JSON-based configuration system with validation
6. **Utilities** ✅ - Hashing, LRU caching, path conversion, retry logic

## Implementation Phases

1. ✅ Project setup and core structure
2. ✅ Utilities and support infrastructure (cache, hash, retry, path)
3. ✅ Abstract peer interface and base implementation
4. ✅ Hub implementation with group-based routing
5. ✅ Storage peer (filesystem with fsnotify)
6. ✅ CouchDB client library (Kivik v4 wrapper)
7. ✅ CouchDB peer (E2EE, compression, chunking)
8. 🔄 Testing and integration
9. 🔄 Documentation and polish
10. ⏳ Production readiness

See [PLAN.md](PLAN.md) for detailed breakdown of each phase.

## Source Code Analysis

The original Deno project consists of:
- ~754 lines of main application code (TypeScript)
- Additional livesync-commonlib dependency (submodule)
- 7 main TypeScript files
- Configuration-driven architecture

## Next Steps

- [ ] Run integration tests with live CouchDB instance
- [ ] End-to-end testing: Storage ↔ CouchDB sync
- [ ] Performance testing and benchmarking
- [ ] Add support for WebSocket peer (future)
- [ ] Deployment documentation
- [ ] Docker container packaging

## Getting Started

### Installation

#### Download Pre-built Binary (Recommended)

Download the latest release from the [Releases page](https://github.com/ImDevinC/livesync-bridge-go/releases).

**Linux:**
```bash
# AMD64
wget https://github.com/ImDevinC/livesync-bridge-go/releases/latest/download/livesync-bridge-linux-amd64
chmod +x livesync-bridge-linux-amd64
sudo mv livesync-bridge-linux-amd64 /usr/local/bin/livesync-bridge

# ARM64
wget https://github.com/ImDevinC/livesync-bridge-go/releases/latest/download/livesync-bridge-linux-arm64
chmod +x livesync-bridge-linux-arm64
sudo mv livesync-bridge-linux-arm64 /usr/local/bin/livesync-bridge
```

**macOS:**
```bash
# Intel
wget https://github.com/ImDevinC/livesync-bridge-go/releases/latest/download/livesync-bridge-darwin-amd64
chmod +x livesync-bridge-darwin-amd64
sudo mv livesync-bridge-darwin-amd64 /usr/local/bin/livesync-bridge

# Apple Silicon
wget https://github.com/ImDevinC/livesync-bridge-go/releases/latest/download/livesync-bridge-darwin-arm64
chmod +x livesync-bridge-darwin-arm64
sudo mv livesync-bridge-darwin-arm64 /usr/local/bin/livesync-bridge
```

**Windows:**
Download the `.exe` file for your architecture and add it to your PATH.

#### Build from Source

```bash
go build -o bin/livesync-bridge ./cmd/livesync-bridge
```

### Default Paths

LiveSync Bridge uses platform-specific directories for configuration and data storage:

**Linux/BSD:**
- Config: `~/.config/livesync-bridge/config.json` (or `$XDG_CONFIG_HOME/livesync-bridge/config.json`)
- Data: `~/.local/share/livesync-bridge/livesync-bridge.db` (or `$XDG_DATA_HOME/livesync-bridge/livesync-bridge.db`)

**macOS:**
- Config: `~/Library/Application Support/livesync-bridge/config.json`
- Data: `~/Library/Application Support/livesync-bridge/livesync-bridge.db`

**Windows:**
- Config: `%APPDATA%\livesync-bridge\config.json`
- Data: `%LOCALAPPDATA%\livesync-bridge\livesync-bridge.db`

You can override these defaults using:
1. **CLI flags** (highest priority): `--config` and `--db`
2. **Environment variables**: `LSB_CONFIG` and `LSB_DATA`
3. **XDG/platform defaults** (lowest priority): as shown above

### Build

```bash
go build -o bin/livesync-bridge ./cmd/livesync-bridge
```

### Run Tests

```bash
# Run all tests
go test ./... -cover

# Run specific package tests
go test ./internal/couchdbpeer -v

# Run integration tests (requires CouchDB)
export COUCHDB_URL=http://localhost:5984
go test ./internal/couchdbpeer -v
```

### Run Application

```bash
# With default paths (XDG/platform-specific)
./bin/livesync-bridge

# With custom config (using CLI flag)
./bin/livesync-bridge --config /path/to/config.json

# With custom config and database paths
./bin/livesync-bridge --config /path/to/config.json --db /path/to/data.db

# Using environment variables
export LSB_CONFIG=/path/to/config.json
export LSB_DATA=/path/to/data.db
./bin/livesync-bridge

# Reset state (clear BoltDB)
./bin/livesync-bridge --reset

# Show help
./bin/livesync-bridge --help
```

### Configuration Example

```json
{
  "peers": [
    {
      "type": "storage",
      "name": "local-vault",
      "group": "sync",
      "baseDir": "./vault"
    },
    {
      "type": "couchdb",
      "name": "remote-sync",
      "group": "sync",
      "url": "http://localhost:5984",
      "username": "admin",
      "password": "password",
      "database": "my_vault",
      "passphrase": "encryption-key",
      "enableCompression": true
    }
  ]
}
```

See [configs/config.sample.json](configs/config.sample.json) for more examples.

### Testing with CouchDB

```bash
# Start CouchDB with Docker
docker run -d --name couchdb-test -p 5984:5984 \
  -e COUCHDB_USER=admin \
  -e COUCHDB_PASSWORD=password \
  couchdb:latest

# Create test database
sleep 5
curl -X PUT http://admin:password@localhost:5984/test_vault

# Run bridge
./bin/livesync-bridge --config configs/couchdb-test-config.json
```

See [internal/couchdbpeer/README.md](internal/couchdbpeer/README.md) for detailed testing guide.

## License

TBD - Should match or be compatible with original project license.
