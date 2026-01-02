# LiveSync Bridge Architecture

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         Hub                                  │
│  - Manages all peers                                         │
│  - Routes changes between peers in same group                │
│  - Filters by group membership                               │
└───────────────┬─────────────────────────────┬────────────────┘
                │                             │
        ┌───────▼────────┐           ┌────────▼───────┐
        │   Peer Group   │           │  Peer Group    │
        │    "main"      │           │     "sub"      │
        └───────┬────────┘           └────────┬───────┘
                │                             │
    ┌───────────┴───────────┐     ┌──────────┴──────────┐
    │                       │     │                     │
┌───▼────┐            ┌─────▼──┐  │                ┌────▼────┐
│CouchDB │            │Storage │  │                │Storage  │
│  Peer  │            │  Peer  │  │                │  Peer   │
│  test1 │◄──────────►│vault/  │  │                │vault2/  │
└────────┘            └────────┘  │                └─────────┘
                                  │
                              ┌───▼────┐
                              │CouchDB │
                              │  Peer  │
                              │  test2 │
                              └────────┘
```

## Component Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                          main.go                               │
│  - Parse CLI arguments (--config, --reset)                     │
│  - Load configuration from JSON                                │
│  - Initialize Hub                                              │
│  - Handle graceful shutdown                                    │
└───────────────────────────┬────────────────────────────────────┘
                            │
┌───────────────────────────▼────────────────────────────────────┐
│                            Hub                                  │
│                                                                 │
│  func Start()                                                   │
│    - Create peers from config                                  │
│    - Start all peers                                           │
│                                                                 │
│  func Dispatch(source Peer, path string, data FileData|false)  │
│    - Find peers in same group (exclude source)                 │
│    - Call Put() or Delete() on each target peer               │
│                                                                 │
└───────────────────────────┬────────────────────────────────────┘
                            │
                            │ manages
                            │
            ┌───────────────▼──────────────┐
            │         Peer Interface        │
            │                               │
            │  Start() error                │
            │  Stop() error                 │
            │  Put(path, data) (bool, err)  │
            │  Delete(path) (bool, err)     │
            │  Get(path) (data, err)        │
            └───────────┬───────────────────┘
                        │
            ┌───────────┴──────────┐
            │                      │
    ┌───────▼──────┐      ┌────────▼─────────┐
    │ Storage Peer │      │  CouchDB Peer    │
    │              │      │                  │
    │ - File watch │      │ - Changes feed   │
    │ - Read/Write │      │ - E2EE           │
    │ - Processor  │      │ - Chunking       │
    └──────────────┘      └──────────────────┘
```

## Data Flow - File Change in Storage Peer

```
┌──────────────┐
│ Filesystem   │
│   Change     │
└──────┬───────┘
       │
       │ fsnotify event
       │
┌──────▼───────────────────────────────────────────┐
│ Storage Peer                                     │
│                                                  │
│ 1. Watcher detects change                        │
│ 2. Read file content                             │
│ 3. Compute SHA-256 hash                          │
│ 4. Check LRU cache (prevent echo)                │
│ 5. If not in cache, dispatch to Hub              │
└──────┬───────────────────────────────────────────┘
       │
       │ Dispatch(source, path, data)
       │
┌──────▼───────────────────────────────────────────┐
│ Hub                                              │
│                                                  │
│ 1. Filter peers by group                         │
│ 2. Exclude source peer                           │
│ 3. For each target peer:                         │
│    - Call peer.Put(path, data)                   │
└──────┬───────────────────────────────────────────┘
       │
       │ Put(path, data)
       │
┌──────▼───────────────────────────────────────────┐
│ Target Peers (CouchDB, Storage, etc.)            │
│                                                  │
│ 1. Compute hash of incoming data                 │
│ 2. Check LRU cache (prevent re-dispatch)         │
│ 3. If not in cache:                              │
│    - Write to CouchDB / filesystem               │
│    - Add to cache                                │
│    - Return success                              │
└──────────────────────────────────────────────────┘
```

## CouchDB Peer - Changes Feed

```
┌────────────────────────────────────────────┐
│ CouchDB Database                           │
│ - Obsidian LiveSync vault                  │
│ - E2EE encrypted documents                 │
└────────┬───────────────────────────────────┘
         │
         │ _changes?feed=continuous&since=...
         │
┌────────▼───────────────────────────────────┐
│ CouchDB Peer                               │
│                                            │
│ Changes Feed Listener:                     │
│ 1. Connect to _changes endpoint            │
│ 2. Receive change notification             │
│ 3. Fetch document                          │
│ 4. Decrypt if E2EE enabled                 │
│ 5. Reassemble chunks if chunked            │
│ 6. Filter by baseDir                       │
│ 7. Compute hash                            │
│ 8. Check cache                             │
│ 9. Dispatch to Hub if new                  │
│ 10. Update "since" checkpoint              │
└────────┬───────────────────────────────────┘
         │
         │ Dispatch(source, path, data)
         │
┌────────▼───────────────────────────────────┐
│ Hub → Other Peers                          │
└────────────────────────────────────────────┘
```

## Storage Peer - File Watching

```
┌─────────────────────────────────────────────┐
│ File Watcher (fsnotify)                     │
│ - Recursive directory watching              │
│ - Debouncing for rapid changes              │
└────────┬────────────────────────────────────┘
         │
         │ File event (create/modify/delete)
         │
┌────────▼────────────────────────────────────┐
│ Storage Peer Event Handler                  │
│                                             │
│ 1. Debounce (wait for stable state)         │
│ 2. Check mtime+size against last known      │
│ 3. If changed:                              │
│    - Read file content                      │
│    - Compute hash                           │
│    - Check LRU cache                        │
│    - Dispatch to Hub                        │
│ 4. Update stored mtime+size                 │
│ 5. Run processor script (if configured)     │
└────────┬────────────────────────────────────┘
         │
         │ Dispatch(source, path, data)
         │
┌────────▼────────────────────────────────────┐
│ Hub → Other Peers                           │
└─────────────────────────────────────────────┘
```

## Path Transformations

```
Global Path: "blog-post.md"
     │
     │ Peer has baseDir="shared/"
     │
     ▼
Local Path: "shared/blog-post.md"
     │
     │ Storage peer with baseDir="./vault/"
     │
     ▼
Storage Path: "./vault/shared/blog-post.md"
                 (OS-specific absolute path)
```

## Deduplication / Loop Prevention

```
┌─────────────────────────────────────────────┐
│ Peer A (Storage)                            │
│                                             │
│ 1. File changes: "doc.md"                   │
│ 2. Compute hash: "abc123..."                │
│ 3. Cache.Get("doc.md") → nil                │
│ 4. Cache.Set("doc.md", "abc123...")         │
│ 5. Dispatch to Hub                          │
└────────┬────────────────────────────────────┘
         │
         │ Hub routes to Peer B
         │
┌────────▼────────────────────────────────────┐
│ Peer B (CouchDB)                            │
│                                             │
│ 1. Receive: Put("doc.md", data)             │
│ 2. Compute hash: "abc123..."                │
│ 3. Cache.Get("doc.md") → nil                │
│ 4. Cache.Set("doc.md", "abc123...")         │
│ 5. Write to CouchDB                         │
│ 6. CouchDB change triggers...               │
└────────┬────────────────────────────────────┘
         │
         │ Changes feed sees write
         │
┌────────▼────────────────────────────────────┐
│ Peer B (CouchDB) - Change Handler          │
│                                             │
│ 1. Receive change: "doc.md"                 │
│ 2. Compute hash: "abc123..."                │
│ 3. Cache.Get("doc.md") → "abc123..."        │
│ 4. Hash matches! SKIP DISPATCH              │
│    (Prevents routing back to Peer A)        │
└─────────────────────────────────────────────┘

Result: No infinite loop!
```

## Concurrency Model (Go-specific)

```
┌─────────────────────────────────────────────┐
│ Main Goroutine                              │
│ - Load config                               │
│ - Start Hub                                 │
│ - Wait for signals                          │
└────────┬────────────────────────────────────┘
         │
         │ spawns goroutines
         │
         ├─────────────────┬─────────────────┐
         │                 │                 │
┌────────▼────────┐ ┌──────▼──────┐ ┌───────▼────────┐
│ Storage Peer    │ │ CouchDB Peer│ │ CouchDB Peer   │
│ Goroutine       │ │ Goroutine   │ │ Goroutine      │
│                 │ │             │ │                │
│ - File watcher  │ │ - Changes   │ │ - Changes      │
│   loop          │ │   feed loop │ │   feed loop    │
│                 │ │             │ │                │
│ Channel: events │ │ Channel: ch │ │ Channel: ch    │
└────────┬────────┘ └──────┬──────┘ └───────┬────────┘
         │                 │                 │
         │ send to Hub via channels          │
         │                 │                 │
┌────────▼─────────────────▼─────────────────▼────────┐
│ Hub Dispatcher                                       │
│ - Receives changes on channels                       │
│ - Routes to target peers                             │
│ - Synchronization: mutex on cache, persistent store  │
└──────────────────────────────────────────────────────┘
```

## Configuration Example Flow

```json
{
  "peers": [
    {
      "type": "couchdb",
      "name": "vault-a",
      "group": "main",        ← Peers in "main" sync together
      "baseDir": "shared/",   ← Only sync "shared/" folder
      "url": "http://localhost:5984",
      "database": "vault_a",
      "passphrase": "secret1"
    },
    {
      "type": "storage",
      "name": "local",
      "group": "main",        ← Also in "main" group
      "baseDir": "./vault/"
    },
    {
      "type": "couchdb",
      "name": "vault-b",
      "group": "other",       ← Different group, won't sync with above
      "database": "vault_b",
      ...
    }
  ]
}
```

Result:
- `vault-a:shared/*` ↔ `local:./vault/*` (synced)
- `vault-b` is isolated (different group)

## Key Design Patterns

### 1. Hub-and-Spoke
- Hub is central coordinator
- Peers are independent, unaware of each other
- Simplifies adding new peer types

### 2. Abstract Interface
- Peer interface allows pluggable implementations
- Easy to add new peer types (S3, WebDAV, etc.)

### 3. Change Deduplication
- LRU cache with content hashing
- Prevents infinite loops and unnecessary operations

### 4. Group-based Routing
- Flexible topology
- Can create isolated sync groups
- Can have multiple groups in one instance

### 5. Path Abstraction
- Global → Local → Storage path transformations
- Allows baseDir-based filtering
- Platform-agnostic sync
