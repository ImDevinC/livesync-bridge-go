# Go Implementation Quick Reference

## TypeScript/Deno → Go Translation Guide

### Type Mappings

| TypeScript | Go | Notes |
|------------|-----|-------|
| `interface` | `interface` or `struct` | Use interface for behavior, struct for data |
| `type` | `type` | Type aliases work similarly |
| `string` | `string` | |
| `number` | `int`, `int64`, `float64` | Choose based on range/precision needs |
| `boolean` | `bool` | |
| `Array<T>` or `T[]` | `[]T` | Go slices |
| `Map<K,V>` | `map[K]V` | |
| `Uint8Array` | `[]byte` | Go byte slices |
| `Promise<T>` | Use goroutines + channels | |
| `async/await` | goroutines + channels/contexts | |
| `Record<string, any>` | `map[string]interface{}` | |

### Deno API → Go Standard Library

| Deno | Go | Package |
|------|-----|---------|
| `Deno.readTextFile()` | `os.ReadFile()` | `os` |
| `Deno.writeTextFile()` | `os.WriteFile()` | `os` |
| `Deno.readFile()` | `os.ReadFile()` | `os` |
| `Deno.writeFile()` | `os.WriteFile()` | `os` |
| `Deno.open()` | `os.Open()`, `os.Create()` | `os` |
| `Deno.stat()` | `os.Stat()` | `os` |
| `Deno.remove()` | `os.Remove()` | `os` |
| `Deno.mkdir()` | `os.MkdirAll()` | `os` |
| `Deno.watchFs()` | Use fsnotify | `github.com/fsnotify/fsnotify` |
| `localStorage` | BoltDB / file-based | `go.etcd.io/bbolt` |
| `crypto.subtle.digest()` | `sha256.Sum256()` | `crypto/sha256` |
| `TextEncoder/TextDecoder` | `string(bytes)`, `[]byte(str)` | Built-in |
| `JSON.parse()` | `json.Unmarshal()` | `encoding/json` |
| `JSON.stringify()` | `json.Marshal()` | `encoding/json` |
| `fetch()` | `http.Get()`, `http.Post()` | `net/http` |

### Common Patterns

#### Async/Await → Goroutines

**TypeScript/Deno:**
```typescript
async function processFile(path: string) {
    const data = await Deno.readTextFile(path);
    await saveToDatabase(data);
}
```

**Go:**
```go
func processFile(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        return err
    }
    return saveToDatabase(data)
}

// For concurrent execution:
go func() {
    if err := processFile(path); err != nil {
        log.Printf("Error: %v", err)
    }
}()
```

#### Class → Struct with Methods

**TypeScript/Deno:**
```typescript
export class Peer {
    config: PeerConf;
    cache: LRUCache;
    
    constructor(conf: PeerConf) {
        this.config = conf;
        this.cache = new LRUCache(300);
    }
    
    async put(path: string, data: FileData): Promise<boolean> {
        // implementation
    }
}
```

**Go:**
```go
type Peer struct {
    config PeerConf
    cache  *lru.Cache
}

func NewPeer(conf PeerConf) *Peer {
    cache, _ := lru.New(300)
    return &Peer{
        config: conf,
        cache:  cache,
    }
}

func (p *Peer) Put(path string, data FileData) (bool, error) {
    // implementation
}
```

#### Abstract Class → Interface + Base Struct

**TypeScript/Deno:**
```typescript
export abstract class Peer {
    abstract put(path: string, data: FileData): Promise<boolean>;
    abstract delete(path: string): Promise<boolean>;
    
    async isRepeating(path: string, data: FileData): Promise<boolean> {
        // shared implementation
    }
}
```

**Go:**
```go
// Interface
type Peer interface {
    Put(path string, data FileData) (bool, error)
    Delete(path string) (bool, error)
    Start() error
    Stop() error
}

// Base implementation (composition)
type BasePeer struct {
    config PeerConf
    cache  *lru.Cache
}

func (b *BasePeer) IsRepeating(path string, data FileData) (bool, error) {
    // shared implementation
}

// Concrete implementation
type StoragePeer struct {
    BasePeer  // embedded
    watcher *fsnotify.Watcher
}

func (s *StoragePeer) Put(path string, data FileData) (bool, error) {
    // specific implementation
}
```

#### Event Emitter → Channels

**TypeScript/Deno:**
```typescript
watcher.on("change", async (path) => {
    await this.dispatch(path);
});
```

**Go:**
```go
// Producer
go func() {
    for {
        select {
        case event := <-watcher.Events:
            changeChan <- event.Name
        case <-ctx.Done():
            return
        }
    }
}()

// Consumer
go func() {
    for path := range changeChan {
        s.dispatch(path)
    }
}()
```

#### localStorage → Persistent Storage

**TypeScript/Deno:**
```typescript
localStorage.setItem(key, value);
const val = localStorage.getItem(key);
```

**Go (using BoltDB):**
```go
func (p *Peer) SetSetting(key, value string) error {
    return p.db.Update(func(tx *bolt.Tx) error {
        b := tx.Bucket([]byte("settings"))
        return b.Put([]byte(key), []byte(value))
    })
}

func (p *Peer) GetSetting(key string) (string, error) {
    var value string
    err := p.db.View(func(tx *bolt.Tx) error {
        b := tx.Bucket([]byte("settings"))
        v := b.Get([]byte(key))
        if v != nil {
            value = string(v)
        }
        return nil
    })
    return value, err
}
```

## Key Go Concepts for This Project

### 1. Error Handling

**Don't ignore errors!**

```go
// Bad
data, _ := os.ReadFile(path)

// Good
data, err := os.ReadFile(path)
if err != nil {
    return fmt.Errorf("failed to read file %s: %w", path, err)
}
```

### 2. Defer for Cleanup

```go
func processFile(path string) error {
    f, err := os.Open(path)
    if err != nil {
        return err
    }
    defer f.Close()  // Always closes, even if panic
    
    // Use file...
    return nil
}
```

### 3. Context for Cancellation

```go
func (p *StoragePeer) Start(ctx context.Context) error {
    for {
        select {
        case event := <-p.watcher.Events:
            p.handleEvent(event)
        case <-ctx.Done():
            return ctx.Err()  // Graceful shutdown
        }
    }
}
```

### 4. sync.Mutex for Shared State

```go
type Peer struct {
    mu    sync.RWMutex
    cache *lru.Cache
}

func (p *Peer) IsRepeating(path string, hash string) bool {
    p.mu.RLock()  // Read lock
    defer p.mu.RUnlock()
    
    cached, ok := p.cache.Get(path)
    return ok && cached == hash
}

func (p *Peer) SetCache(path string, hash string) {
    p.mu.Lock()  // Write lock
    defer p.mu.Unlock()
    
    p.cache.Add(path, hash)
}
```

### 5. WaitGroup for Coordinating Goroutines

```go
func (h *Hub) Start() error {
    var wg sync.WaitGroup
    
    for _, peer := range h.peers {
        wg.Add(1)
        go func(p Peer) {
            defer wg.Done()
            if err := p.Start(h.ctx); err != nil {
                log.Printf("Peer error: %v", err)
            }
        }(peer)
    }
    
    wg.Wait()
    return nil
}
```

## Project Structure Recommendations

```
livesync-bridge-go/
├── cmd/
│   └── livesync-bridge/
│       └── main.go                 # Entry point, CLI handling
├── internal/
│   ├── config/
│   │   ├── config.go               # Config loading
│   │   └── types.go                # Config types
│   ├── hub/
│   │   └── hub.go                  # Hub implementation
│   ├── peer/
│   │   ├── peer.go                 # Peer interface
│   │   ├── base.go                 # BasePeer implementation
│   │   └── types.go                # FileData, etc.
│   ├── storage/
│   │   ├── peer.go                 # StoragePeer
│   │   ├── watcher.go              # File watching
│   │   └── processor.go            # Script processor
│   ├── couchdb/
│   │   └── peer.go                 # CouchDBPeer
│   └── util/
│       ├── hash.go                 # Hashing utilities
│       ├── path.go                 # Path utilities
│       └── cache.go                # LRU cache wrapper
├── pkg/
│   └── couchdb/
│       ├── client.go               # CouchDB HTTP client
│       ├── changes.go              # Changes feed
│       ├── document.go             # Document structures
│       ├── chunking.go             # Chunk handling
│       ├── encryption.go           # E2EE
│       └── compression.go          # Compression
├── configs/
│   └── config.sample.json          # Sample configuration
├── go.mod
├── go.sum
└── README.md
```

**Note**: `internal/` prevents external imports, `pkg/` allows external use.

## Recommended Go Libraries

### Essential
```bash
go get github.com/fsnotify/fsnotify        # File watching
go get golang.org/x/crypto/pbkdf2          # Key derivation
go get github.com/hashicorp/golang-lru     # LRU cache
go get go.etcd.io/bbolt                    # Persistent storage
```

### Optional but Helpful
```bash
go get github.com/spf13/cobra              # CLI framework
go get github.com/spf13/viper              # Configuration
go get github.com/sirupsen/logrus          # Structured logging
go get github.com/stretchr/testify         # Testing utilities
```

## Testing Strategy

### Unit Tests

```go
// peer/base_test.go
func TestPathConversion(t *testing.T) {
    peer := &BasePeer{
        config: PeerConf{BaseDir: "shared/"},
    }
    
    tests := []struct {
        name   string
        input  string
        output string
    }{
        {"simple", "doc.md", "shared/doc.md"},
        {"nested", "a/b/c.md", "shared/a/b/c.md"},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := peer.ToLocalPath(tt.input)
            if result != tt.output {
                t.Errorf("got %s, want %s", result, tt.output)
            }
        })
    }
}
```

### Integration Tests

```go
// integration_test.go
func TestStorageToStorageSync(t *testing.T) {
    // Create temp directories
    dir1, _ := os.MkdirTemp("", "peer1")
    dir2, _ := os.MkdirTemp("", "peer2")
    defer os.RemoveAll(dir1)
    defer os.RemoveAll(dir2)
    
    // Create config
    config := Config{
        Peers: []PeerConf{
            {Type: "storage", Name: "p1", BaseDir: dir1, Group: "test"},
            {Type: "storage", Name: "p2", BaseDir: dir2, Group: "test"},
        },
    }
    
    // Start hub
    hub := NewHub(config)
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    go hub.Start(ctx)
    
    // Write file to dir1
    testFile := filepath.Join(dir1, "test.txt")
    os.WriteFile(testFile, []byte("hello"), 0644)
    
    // Wait for sync
    time.Sleep(2 * time.Second)
    
    // Check file exists in dir2
    syncedFile := filepath.Join(dir2, "test.txt")
    data, err := os.ReadFile(syncedFile)
    if err != nil {
        t.Fatalf("File not synced: %v", err)
    }
    if string(data) != "hello" {
        t.Errorf("Wrong content: %s", data)
    }
}
```

## Performance Considerations

### 1. Buffer Sizes
```go
const (
    ReadBufferSize  = 64 * 1024  // 64KB for file reading
    WriteBufferSize = 64 * 1024  // 64KB for file writing
)
```

### 2. Worker Pools
```go
type WorkerPool struct {
    tasks chan func()
}

func NewWorkerPool(size int) *WorkerPool {
    p := &WorkerPool{
        tasks: make(chan func(), 100),
    }
    for i := 0; i < size; i++ {
        go p.worker()
    }
    return p
}

func (p *WorkerPool) worker() {
    for task := range p.tasks {
        task()
    }
}

func (p *WorkerPool) Submit(task func()) {
    p.tasks <- task
}
```

### 3. Connection Pooling
```go
var httpClient = &http.Client{
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 100,
        IdleConnTimeout:     90 * time.Second,
    },
    Timeout: 30 * time.Second,
}
```

## Common Pitfalls to Avoid

### 1. Don't Copy Mutexes
```go
// Bad
func (p Peer) Method() { /* copies p, including mutex */ }

// Good
func (p *Peer) Method() { /* uses pointer */ }
```

### 2. Range Loop Variable Capture
```go
// Bad
for _, peer := range peers {
    go func() {
        peer.Start()  // Wrong! Uses last peer
    }()
}

// Good
for _, peer := range peers {
    peer := peer  // Create new variable
    go func() {
        peer.Start()
    }()
}

// Or
for _, p := range peers {
    go func(peer Peer) {
        peer.Start()
    }(p)
}
```

### 3. Closing Channels
```go
// Only sender should close
// Don't close if multiple senders

// Bad
go func() {
    ch <- value
    close(ch)  // What if another sender?
}()

// Good
// Single sender closes
go func() {
    for _, v := range values {
        ch <- v
    }
    close(ch)
}()
```

## Next Steps

1. **Set up Go environment**: `go mod init github.com/yourusername/livesync-bridge-go`
2. **Start with Phase 1**: Create project structure and config loading
3. **Test incrementally**: Write tests alongside implementation
4. **Profile early**: Use `go test -bench` and `pprof` to find bottlenecks
5. **Read Go docs**: https://go.dev/doc/ (especially "Effective Go")
