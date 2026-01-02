# LiveSync Bridge - Go Port Planning Document

## Project Overview

**Original Project**: https://github.com/vrtmrz/livesync-bridge  
**Original Language**: TypeScript/Deno  
**Target Language**: Go  
**Module Path**: github.com/imdevinc/livesync-bridge  
**Purpose**: A custom replicator between Self-hosted LiveSync remote vaults and storage

## Implementation Decisions

All key decisions have been made:

1. **✓ Compatibility**: Compatible subset (core functionality first)
2. **✓ Features**: Must-have: basic sync, E2EE, file watching, groups, compression, chunking, offline scanning. Deferred: remote tweaks, path obfuscation, script processor
3. **✓ Persistent Storage**: BoltDB (go.etcd.io/bbolt)
4. **✓ Logging**: log/slog (standard library)
5. **✓ Go Version**: Go 1.25+
6. **✓ Testing**: Create test vault, use Docker CouchDB
7. **✓ CouchDB Client**: Use existing library (github.com/go-kivik/kivik)
8. **✓ CLI Framework**: Standard library `flag` package
9. **✓ Error Handling**: Retry with exponential backoff
10. **✓ Encryption**: Use Go crypto libraries with LiveSync parameters
11. **✓ Concurrency**: One goroutine per peer

## What the Project Does

LiveSync Bridge synchronizes data between multiple "peers" which can be:
1. **CouchDB databases** - Remote vaults with optional E2EE (End-to-End Encryption)
2. **Local filesystem storage** - Directory-based storage with file watching

Key features:
- Multi-directional synchronization between any combination of peers
- Support for different passphrases per vault (E2EE)
- Folder-based synchronization (sync specific folders between vaults)
- Group-based peer organization (peers in same group sync together)
- Path obfuscation support
- Configurable chunk sizes for CouchDB
- File processing hooks (run scripts on file changes)
- Offline change detection and scanning

## Architecture Analysis

### Core Components

1. **main.ts** (~30 lines)
   - Entry point
   - Loads configuration from JSON file
   - Initializes Hub
   - Handles `--reset` flag to clear localStorage

2. **Hub.ts** (~53 lines)
   - Central coordinator
   - Creates and manages peers
   - Dispatches changes between peers in same group
   - Routes file operations (put/delete) from one peer to others

3. **Peer.ts** (~77 lines)
   - Abstract base class for all peer types
   - Path transformation (global ↔ local paths)
   - LRU cache for deduplication (prevents echo/loops)
   - Hash-based change detection
   - Logging utilities
   - LocalStorage-based settings persistence

4. **PeerCouchDB.ts** (~192 lines)
   - CouchDB peer implementation
   - Uses DirectFileManipulator library for CouchDB operations
   - Handles E2EE encryption/decryption
   - Watches CouchDB changes feed
   - Supports "remote tweaks" (configuration from remote DB)
   - Chunk management for large files

5. **PeerStorage.ts** (~332 lines)
   - Filesystem peer implementation
   - File watching (Deno.watchFs or chokidar)
   - Offline change scanning
   - File modification detection via mtime+size tracking
   - Optional script processor (run external commands on file changes)
   - Platform-specific path handling

6. **types.ts** (~52 lines)
   - Configuration types
   - FileData structure (ctime, mtime, size, data, deleted flag)

7. **util.ts** (~25 lines)
   - SHA-256 hash computation
   - Unique string generation

### Dependencies

#### External Libraries (from deno.jsonc)
- **PouchDB/CouchDB**: Core database operations
  - pouchdb-core, pouchdb-adapter-http, pouchdb-replication
  - pouchdb-find, pouchdb-merge, pouchdb-utils, pouchdb-errors
- **File watching**: chokidar (optional, fallback to Deno.watchFs)
- **File system**: @std/fs, @std/path
- **Utilities**: 
  - octagonal-wheels (LRU cache, concurrency locks, task scheduling)
  - xxhash-wasm (hashing)
  - fflate (compression)
  - diff-match-patch (diffing)
  - minimatch (pattern matching)

#### Internal Library (livesync-commonlib submodule)
- DirectFileManipulator/DirectFileManipulatorV2 (CouchDB file operations)
- Encryption/decryption utilities
- String/binary conversion utilities
- Logger
- Utils (blob creation, content comparison)

### Data Flow

```
1. Peer detects change (file watch or CouchDB changes feed)
   ↓
2. Peer reads file/document data
   ↓
3. Peer computes hash and checks LRU cache (prevents loops)
   ↓
4. Peer calls dispatch() on Hub
   ↓
5. Hub iterates all peers in same group (excluding source)
   ↓
6. Hub calls put() or delete() on each target peer
   ↓
7. Target peer writes data and checks hash (prevents re-dispatch)
```

### Configuration Structure

```json
{
  "peers": [
    {
      "type": "couchdb",
      "name": "unique-name",
      "group": "group-name",  // optional, peers in same group sync
      "url": "http://localhost:5984",
      "database": "db-name",
      "username": "user",
      "password": "pass",
      "passphrase": "e2ee-passphrase",
      "obfuscatePassphrase": "path-obfuscation-passphrase",
      "baseDir": "folder/",   // sync only this folder
      "customChunkSize": 100,
      "minimumChunkSize": 20,
      "useRemoteTweaks": true // fetch config from remote DB
    },
    {
      "type": "storage",
      "name": "unique-name",
      "group": "group-name",
      "baseDir": "./local-path/",
      "scanOfflineChanges": true,
      "useChokidar": false,
      "processor": {
        "cmd": "script.sh",
        "args": ["$filename", "$mode"]
      }
    }
  ]
}
```

## Go Port Implementation Plan

### Phase 1: Project Setup & Core Structure

**Goal**: Set up Go project structure and implement core abstractions

#### 1.1 Project Initialization
- [ ] Create Go module: `go mod init github.com/yourusername/livesync-bridge-go`
- [ ] Set up directory structure:
  ```
  livesync-bridge-go/
  ├── cmd/
  │   └── livesync-bridge/
  │       └── main.go
  ├── internal/
  │   ├── config/
  │   ├── hub/
  │   ├── peer/
  │   ├── storage/
  │   └── util/
  ├── pkg/
  │   └── couchdb/
  ├── configs/
  │   └── config.sample.json
  ├── go.mod
  ├── go.sum
  └── README.md
  ```

#### 1.2 Configuration Module
- [ ] Implement `internal/config/types.go` (Config, PeerConf structs)
- [ ] Implement `internal/config/loader.go` (JSON loading)
- [ ] Add validation for configuration

**Files**: config/types.go, config/loader.go

**Go packages needed**: encoding/json, os

#### 1.3 Core Types
- [ ] Implement `internal/peer/types.go` (FileData, DispatchFunc)
- [ ] Implement path utilities (global ↔ local path conversion)

**Files**: peer/types.go, util/path.go

### Phase 2: Utilities & Support Infrastructure

**Goal**: Implement utility functions and caching

#### 2.1 Hashing & Deduplication
- [ ] Implement `internal/util/hash.go` (SHA-256 hashing)
- [ ] Implement `internal/util/unique.go` (unique string generation)

**Go packages needed**: crypto/sha256, encoding/hex

#### 2.2 LRU Cache
- [ ] Implement or import LRU cache for change deduplication
- [ ] Thread-safe cache implementation

**Go packages needed**: 
- Option 1: github.com/hashicorp/golang-lru
- Option 2: Implement custom sync.Map-based cache

#### 2.3 Persistent Storage (localStorage replacement)
- [ ] Implement key-value storage using local files or BoltDB
- [ ] Scoped storage per peer (like localStorage keys)

**Go packages needed**: 
- Option 1: os + JSON files
- Option 2: go.etcd.io/bbolt (BoltDB)
- Option 3: github.com/syndtr/goleveldb

#### 2.4 Logging
- [ ] Implement structured logging
- [ ] Log levels (DEBUG, INFO, NOTICE, VERBOSE)
- [ ] Per-peer logging prefixes

**Go packages needed**: 
- Option 1: log/slog (Go 1.21+)
- Option 2: github.com/sirupsen/logrus
- Option 3: go.uber.org/zap

### Phase 3: Abstract Peer Interface

**Goal**: Implement the base Peer abstraction

#### 3.1 Peer Interface
- [ ] Define `internal/peer/peer.go` interface:
  ```go
  type Peer interface {
      Start() error
      Stop() error
      Put(path string, data FileData) (bool, error)
      Delete(path string) (bool, error)
      Get(path string) (FileData, error)
      Name() string
      Group() string
  }
  ```

#### 3.2 Base Peer Implementation
- [ ] Implement `internal/peer/base.go` with common functionality:
  - Path conversion (ToLocalPath, ToGlobalPath)
  - Cache management
  - Hash computation and deduplication (IsRepeating)
  - Settings persistence
  - Logging helpers

**Files**: peer/peer.go, peer/base.go

### Phase 4: Hub Implementation

**Goal**: Implement the central coordination hub

#### 4.1 Hub Structure
- [ ] Implement `internal/hub/hub.go`:
  - Peer registration
  - Start/stop all peers
  - Dispatch routing

#### 4.2 Dispatch Logic
- [ ] Filter peers by group
- [ ] Exclude source peer
- [ ] Call put/delete on target peers
- [ ] Error handling and logging

**Files**: hub/hub.go

### Phase 5: Storage Peer Implementation

**Goal**: Implement filesystem-based peer

#### 5.1 Storage Peer Structure
- [ ] Implement `internal/storage/peer.go`
- [ ] File reading/writing
- [ ] mtime+size tracking for change detection

#### 5.2 File Watching
- [ ] Implement file watcher using fsnotify
- [ ] Debouncing for rapid changes
- [ ] Recursive directory watching

**Go packages needed**: github.com/fsnotify/fsnotify

#### 5.3 Offline Change Scanning
- [ ] Walk directory tree on startup
- [ ] Compare stored file stats with current
- [ ] Dispatch detected changes

**Go packages needed**: path/filepath, os

#### 5.4 Script Processor (DEFERRED)
- Execute external commands on file changes
- Environment variable substitution ($filename, $mode)
- Capture stdout/stderr

**Note**: This feature is deferred to a later phase.

**Files**: storage/peer.go, storage/watcher.go

### Phase 6: CouchDB Client Library

**Goal**: Implement CouchDB interaction layer (simplified version of DirectFileManipulator)

This is the most complex part as we need to replicate functionality from the livesync-commonlib.

#### 6.1 Basic CouchDB Client
- [ ] HTTP client for CouchDB REST API
- [ ] Authentication
- [ ] Basic CRUD operations (GET, PUT, DELETE documents)
- [ ] Changes feed listener (_changes endpoint)

**Go packages needed**: net/http, encoding/json

#### 6.2 Document Structure
- [ ] Implement Obsidian LiveSync document structures
- [ ] Chunked document handling
- [ ] Metadata handling (FileInfo, MetaEntry, ReadyEntry)

#### 6.3 Chunking
- [ ] Split large files into chunks
- [ ] Reassemble chunks on read
- [ ] Configurable chunk sizes

#### 6.4 Encryption (E2EE)
- [ ] AES encryption/decryption
- [ ] PBKDF2 key derivation
- [ ] Handle passphrase

**Go packages needed**: crypto/aes, crypto/cipher, crypto/rand, golang.org/x/crypto/pbkdf2

#### 6.5 Path Obfuscation (DEFERRED)
- Path hashing/obfuscation
- Obfuscation passphrase handling

**Note**: This feature is deferred to a later phase.

#### 6.6 Binary/Text Handling
- [ ] Base64 encoding/decoding
- [ ] Detect plain text vs binary files
- [ ] Blob creation utilities

**Go packages needed**: encoding/base64

#### 6.7 Compression
- [ ] Optional compression support
- [ ] fflate equivalent (gzip/deflate)

**Go packages needed**: compress/gzip, compress/flate

**Files**: 
- pkg/couchdb/client.go
- pkg/couchdb/documents.go
- pkg/couchdb/chunking.go
- pkg/couchdb/encryption.go
- pkg/couchdb/changes.go

### Phase 7: CouchDB Peer Implementation

**Goal**: Implement CouchDB-based peer

#### 7.1 CouchDB Peer Structure
- [ ] Implement `internal/couchdb/peer.go`
- [ ] Initialize DirectFileManipulator equivalent
- [ ] Load "since" from persistent storage

#### 7.2 Document Operations
- [ ] Put document (with E2EE if configured)
- [ ] Delete document
- [ ] Get document (with decryption)
- [ ] Get metadata only

#### 7.3 Changes Feed Watching
- [ ] Connect to _changes feed (continuous mode)
- [ ] Filter by baseDir
- [ ] Parse and dispatch changes
- [ ] Update "since" checkpoint

#### 7.4 Remote Tweaks (DEFERRED)
- Fetch MILESTONE_DOCID document
- Parse tweak_values
- Apply configuration overrides
- Validate encryption/compression settings

**Note**: This feature is deferred to a later phase.

#### 7.5 Conflict Detection
- [ ] Compare document mtime (within 1 hour tolerance)
- [ ] Compare content hashes
- [ ] Skip if content is identical

**Files**: couchdb/peer.go

### Phase 8: Main Application

**Goal**: Wire everything together

#### 8.1 CLI Argument Parsing
- [ ] --config flag for config file path
- [ ] --reset flag to clear persistent storage
- [ ] Environment variable support (LSB_CONFIG)

**Go packages needed**: flag or github.com/spf13/cobra

#### 8.2 Application Lifecycle
- [ ] Load configuration
- [ ] Initialize hub
- [ ] Register peers
- [ ] Start all peers
- [ ] Signal handling (graceful shutdown)

#### 8.3 Error Handling
- [ ] Graceful error recovery
- [ ] Logging of errors
- [ ] Retry logic for transient failures

**Files**: cmd/livesync-bridge/main.go

### Phase 9: Testing

**Goal**: Ensure correctness and reliability

#### 9.1 Unit Tests
- [ ] Configuration loading tests
- [ ] Path conversion tests
- [ ] Hash computation tests
- [ ] Cache tests
- [ ] Chunking tests
- [ ] Encryption tests

#### 9.2 Integration Tests
- [ ] Storage peer file operations
- [ ] CouchDB peer operations (with test database)
- [ ] Hub dispatch logic
- [ ] End-to-end sync tests

#### 9.3 Test Infrastructure
- [ ] Mock CouchDB server
- [ ] Temporary test directories
- [ ] Test fixtures

**Go packages needed**: testing, testify/assert (optional)

### Phase 10: Documentation & Polish

**Goal**: Make the project usable and maintainable

#### 10.1 Documentation
- [ ] README.md with usage instructions
- [ ] Configuration guide
- [ ] Build instructions
- [ ] Docker support (Dockerfile, docker-compose.yml)

#### 10.2 Examples
- [ ] Sample configuration files
- [ ] Example sync scenarios
- [ ] Script processor examples

#### 10.3 Performance Optimization
- [ ] Profile CPU usage
- [ ] Profile memory usage
- [ ] Optimize hot paths
- [ ] Connection pooling for CouchDB

## Key Go Libraries & Packages

### Standard Library
- `encoding/json` - Configuration and CouchDB documents
- `flag` - CLI argument parsing
- `log/slog` - Structured logging
- `crypto/*` - Hashing and encryption (sha256, aes, cipher, rand)
- `compress/gzip`, `compress/flate` - Compression
- `os`, `path/filepath` - File operations
- `context` - Cancellation and timeouts
- `sync` - Mutexes and concurrency primitives

### Third-Party Libraries (Confirmed)

#### Essential
- `github.com/fsnotify/fsnotify` - File watching
- `golang.org/x/crypto/pbkdf2` - Key derivation for E2EE
- `github.com/hashicorp/golang-lru/v2` - LRU cache for deduplication
- `go.etcd.io/bbolt` - Persistent key-value storage (BoltDB)
- `github.com/go-kivik/kivik/v4` - CouchDB client library

#### Testing
- `github.com/stretchr/testify` - Testing utilities (assertions)

## Critical Implementation Notes

### 1. Deduplication / Loop Prevention
The original uses an LRU cache to track recently seen file hashes. This prevents:
- Echo loops (peer A → peer B → peer A)
- Re-processing of identical data

**Implementation**: Compute SHA-256 hash of file content, check cache before dispatching.

### 2. Path Handling
- **Global paths**: Shared path space (e.g., "document.md")
- **Local paths**: Peer-specific paths with baseDir prefix (e.g., "blog/document.md")
- **Storage paths**: OS-specific paths (e.g., "./vault/blog/document.md")

**Critical**: Always convert between path formats correctly.

### 3. CouchDB Changes Feed
- Must use continuous mode for real-time sync
- Track "since" sequence to avoid re-processing
- Persist "since" to survive restarts
- Handle connection drops and reconnect

### 4. Encryption Complexity
The livesync-commonlib handles complex encryption:
- Different algorithms (AES-256-GCM, etc.)
- Dynamic iteration counts
- Chunk-level encryption
- Path obfuscation with separate passphrase

**Strategy**: Start with basic E2EE, enhance iteratively. Consider using existing encryption from Obsidian LiveSync if possible.

### 5. File Watching Quirks
- Need debouncing (multiple events for single change)
- Handle rename as delete+create
- Platform differences (Windows vs Linux vs macOS)
- Large directory trees can be slow to scan

**Go advantage**: fsnotify is well-tested and cross-platform.

### 6. Concurrency
Deno/TypeScript is single-threaded with async/await. Go is multi-threaded.

**Considerations**:
- Use channels for peer-to-hub communication
- Mutex for shared state (cache, settings)
- Worker pools for file operations
- Context for cancellation

### 7. Graceful Shutdown
Must handle SIGINT/SIGTERM:
- Stop all file watchers
- Close CouchDB connections
- Flush pending operations
- Persist state

## Incremental Development Strategy

### Minimal Viable Product (MVP)
Build and test in this order:

1. **Storage ↔ Storage sync** (simplest)
   - No encryption
   - No CouchDB
   - Focus on file watching and hub dispatch

2. **CouchDB read/write** (without encryption)
   - Test with unencrypted CouchDB
   - Basic document operations
   - No chunking initially

3. **CouchDB ↔ Storage sync** (unencrypted)
   - Changes feed
   - Bidirectional sync
   - No E2EE

4. **Add E2EE**
   - Encryption/decryption
   - Passphrase handling
   - Encrypted CouchDB ↔ Storage

5. **Add chunking**
   - Large file support
   - Chunk management

6. **Add path obfuscation**
   - Path hashing
   - Obfuscation passphrase

7. **Add advanced features**
   - Remote tweaks
   - Script processor
   - Offline scanning

## Challenges & Risks

### High Complexity
1. **CouchDB Protocol**: Understanding Obsidian LiveSync's document structure
   - **Mitigation**: Study existing Obsidian LiveSync plugin code
   - **Alternative**: Use existing Go CouchDB library and adapt

2. **Encryption**: Replicating exact E2EE behavior
   - **Mitigation**: Extract and port encryption code carefully
   - **Risk**: Incompatibility with existing vaults if wrong

3. **Chunking**: Efficient chunk splitting/merging
   - **Mitigation**: Port chunk algorithms directly
   - **Testing**: Extensive tests with various file sizes

### Medium Complexity
4. **Cross-platform paths**: Windows vs Unix path handling
   - **Mitigation**: Use Go's filepath package consistently
   - **Testing**: Test on multiple platforms

5. **Change deduplication**: Preventing infinite loops
   - **Mitigation**: Robust hashing and cache logic
   - **Testing**: Test circular sync scenarios

### Dependencies
6. **livesync-commonlib**: Heavy dependency on TypeScript library
   - **Mitigation**: Port essential parts to Go
   - **Alternative**: Consider minimal subset of features initially

## Success Metrics

- [ ] Can sync two local directories bidirectionally
- [ ] Can sync local directory with CouchDB (unencrypted)
- [ ] Can sync with E2EE-enabled CouchDB
- [ ] No infinite loops or echo
- [ ] Handles large files (>1MB) with chunking
- [ ] Survives restart without data loss
- [ ] Compatible with existing Obsidian LiveSync vaults
- [ ] Performance: <100ms latency for small file changes
- [ ] Memory: <100MB for typical workloads

## Timeline Estimate

Based on complexity and assuming one developer:

- **Phase 1-2**: 1-2 days (setup, config, utilities)
- **Phase 3-4**: 1 day (peer interface, hub)
- **Phase 5**: 2-3 days (storage peer with file watching)
- **Phase 6**: 5-7 days (CouchDB library - most complex)
- **Phase 7**: 2-3 days (CouchDB peer)
- **Phase 8**: 1 day (main application)
- **Phase 9**: 3-4 days (testing)
- **Phase 10**: 1-2 days (documentation)

**Total**: 17-25 days

**Note**: Timeline assumes familiarity with Go. Add 50% if learning Go concurrently.

## Next Steps

Before diving into implementation, Devin should:

1. **Clarify requirements**: 
   - Do you need full E2EE compatibility with existing vaults?
   - Are there features you don't need (simplifications possible)?
   - Performance targets?

2. **Choose storage backend**:
   - BoltDB, LevelDB, or simple JSON files for persistent storage?

3. **Choose logging library**:
   - Standard library, logrus, or zap?

4. **Encryption scope**:
   - Full compatibility or basic E2EE first?

5. **Testing approach**:
   - Do you have existing vaults to test against?
   - Need Docker for CouchDB testing?

## Questions for Devin

1. Do you want to maintain 100% compatibility with the Deno version, or is a compatible subset acceptable?

2. Are there specific features you want to prioritize or skip?

3. Do you have existing Obsidian LiveSync vaults you want this to work with?

4. Target platforms? (Linux/macOS/Windows, or specific subset?)

5. Performance requirements? (How large are the vaults, how many peers?)

6. Would you like to start with a simplified MVP (e.g., storage-to-storage only), then add CouchDB later?

7. Do you prefer certain Go libraries or coding patterns?
