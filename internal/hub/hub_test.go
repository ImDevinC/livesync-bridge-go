package hub

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/imdevinc/livesync-bridge/internal/config"
	"github.com/imdevinc/livesync-bridge/internal/peer"
	"github.com/imdevinc/livesync-bridge/internal/storage"
)

// MockPeer is a test implementation of the Peer interface
type MockPeer struct {
	name      string
	peerType  string
	group     string
	ctx       context.Context
	cancel    context.CancelFunc
	puts      []string // Track Put operations
	deletes   []string // Track Delete operations
	mu        sync.Mutex
	startErr  error
	putErr    error
	deleteErr error
}

func NewMockPeer(name, peerType, group string) *MockPeer {
	ctx, cancel := context.WithCancel(context.Background())
	return &MockPeer{
		name:     name,
		peerType: peerType,
		group:    group,
		ctx:      ctx,
		cancel:   cancel,
		puts:     make([]string, 0),
		deletes:  make([]string, 0),
	}
}

func (m *MockPeer) Start() error {
	if m.startErr != nil {
		return m.startErr
	}
	// Block until context is cancelled (simulating real peer)
	<-m.ctx.Done()
	return nil
}

func (m *MockPeer) Stop() error {
	m.cancel()
	return nil
}

func (m *MockPeer) Put(path string, data *peer.FileData) (bool, error) {
	if m.putErr != nil {
		return false, m.putErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.puts = append(m.puts, path)
	return true, nil
}

func (m *MockPeer) Delete(path string) (bool, error) {
	if m.deleteErr != nil {
		return false, m.deleteErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deletes = append(m.deletes, path)
	return true, nil
}

func (m *MockPeer) Get(path string) (*peer.FileData, error) {
	return nil, nil
}

func (m *MockPeer) Name() string             { return m.name }
func (m *MockPeer) Type() string             { return m.peerType }
func (m *MockPeer) Group() string            { return m.group }
func (m *MockPeer) Context() context.Context { return m.ctx }

func (m *MockPeer) GetPuts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.puts))
	copy(result, m.puts)
	return result
}

func (m *MockPeer) GetDeletes() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.deletes))
	copy(result, m.deletes)
	return result
}

func createTestStore(t *testing.T) *storage.Store {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test store: %v", err)
	}
	return store
}

func TestNewHub(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	hub := NewHub(store)
	if hub == nil {
		t.Fatal("Hub is nil")
	}

	if len(hub.GetPeers()) != 0 {
		t.Error("New hub should have no peers")
	}
}

func TestHubRegisterPeer(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	hub := NewHub(store)
	peer1 := NewMockPeer("peer1", "storage", "main")
	peer2 := NewMockPeer("peer2", "couchdb", "main")

	hub.RegisterPeer(peer1)
	hub.RegisterPeer(peer2)

	peers := hub.GetPeers()
	if len(peers) != 2 {
		t.Errorf("Expected 2 peers, got %d", len(peers))
	}
}

func TestHubDispatchToSameGroup(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	hub := NewHub(store)

	// Create peers in same group
	peer1 := NewMockPeer("peer1", "storage", "main")
	peer2 := NewMockPeer("peer2", "couchdb", "main")
	peer3 := NewMockPeer("peer3", "storage", "other")

	hub.RegisterPeer(peer1)
	hub.RegisterPeer(peer2)
	hub.RegisterPeer(peer3)

	// Dispatch from peer1
	data := &peer.FileData{
		MTime: time.Now(),
		Size:  100,
		Data:  []byte("test content"),
	}

	err := hub.Dispatch(peer1, "document.md", data)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// Check peer2 received it (same group)
	peer2Puts := peer2.GetPuts()
	if len(peer2Puts) != 1 || peer2Puts[0] != "document.md" {
		t.Errorf("Peer2 should have received the file, got %v", peer2Puts)
	}

	// Check peer3 did NOT receive it (different group)
	peer3Puts := peer3.GetPuts()
	if len(peer3Puts) != 0 {
		t.Errorf("Peer3 should not have received the file, got %v", peer3Puts)
	}

	// Check peer1 did NOT receive it (source peer)
	peer1Puts := peer1.GetPuts()
	if len(peer1Puts) != 0 {
		t.Errorf("Peer1 (source) should not have received the file, got %v", peer1Puts)
	}
}

func TestHubDispatchDeletion(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	hub := NewHub(store)

	peer1 := NewMockPeer("peer1", "storage", "main")
	peer2 := NewMockPeer("peer2", "couchdb", "main")

	hub.RegisterPeer(peer1)
	hub.RegisterPeer(peer2)

	// Dispatch deletion (nil data)
	err := hub.Dispatch(peer1, "document.md", nil)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// Check peer2 received deletion
	peer2Deletes := peer2.GetDeletes()
	if len(peer2Deletes) != 1 || peer2Deletes[0] != "document.md" {
		t.Errorf("Peer2 should have received deletion, got %v", peer2Deletes)
	}
}

func TestHubGetPeerByName(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	hub := NewHub(store)
	peer1 := NewMockPeer("peer1", "storage", "main")
	hub.RegisterPeer(peer1)

	// Get existing peer
	found, err := hub.GetPeerByName("peer1")
	if err != nil {
		t.Fatalf("Failed to get peer: %v", err)
	}
	if found.Name() != "peer1" {
		t.Errorf("Expected peer1, got %s", found.Name())
	}

	// Get non-existent peer
	_, err = hub.GetPeerByName("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent peer")
	}
}

func TestHubGetPeersByGroup(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	hub := NewHub(store)
	peer1 := NewMockPeer("peer1", "storage", "main")
	peer2 := NewMockPeer("peer2", "couchdb", "main")
	peer3 := NewMockPeer("peer3", "storage", "other")

	hub.RegisterPeer(peer1)
	hub.RegisterPeer(peer2)
	hub.RegisterPeer(peer3)

	// Get peers in "main" group
	mainPeers := hub.GetPeersByGroup("main")
	if len(mainPeers) != 2 {
		t.Errorf("Expected 2 peers in main group, got %d", len(mainPeers))
	}

	// Get peers in "other" group
	otherPeers := hub.GetPeersByGroup("other")
	if len(otherPeers) != 1 {
		t.Errorf("Expected 1 peer in other group, got %d", len(otherPeers))
	}

	// Get peers in non-existent group
	nonePeers := hub.GetPeersByGroup("nonexistent")
	if len(nonePeers) != 0 {
		t.Errorf("Expected 0 peers in nonexistent group, got %d", len(nonePeers))
	}
}

func TestHubStartStop(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	hub := NewHub(store)
	peer1 := NewMockPeer("peer1", "storage", "main")
	hub.RegisterPeer(peer1)

	// Start hub
	err := hub.Start()
	if err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop hub
	err = hub.Stop()
	if err != nil {
		t.Fatalf("Failed to stop hub: %v", err)
	}

	// Context should be cancelled
	select {
	case <-hub.Context().Done():
		// Good
	default:
		t.Error("Hub context should be cancelled after stop")
	}
}

func TestHubCreateDispatcher(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	hub := NewHub(store)
	peer1 := NewMockPeer("peer1", "storage", "main")
	peer2 := NewMockPeer("peer2", "couchdb", "main")

	hub.RegisterPeer(peer1)
	hub.RegisterPeer(peer2)

	// Create dispatcher
	dispatcher := hub.CreateDispatcher()
	if dispatcher == nil {
		t.Fatal("Dispatcher is nil")
	}

	// Use dispatcher
	data := &peer.FileData{
		MTime: time.Now(),
		Size:  100,
		Data:  []byte("test"),
	}

	err := dispatcher(peer1, "doc.md", data)
	if err != nil {
		t.Fatalf("Dispatcher failed: %v", err)
	}

	// Peer2 should have received it
	peer2Puts := peer2.GetPuts()
	if len(peer2Puts) != 1 {
		t.Errorf("Peer2 should have received file via dispatcher")
	}
}

func TestHubMultipleGroups(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	hub := NewHub(store)

	// Group A
	peerA1 := NewMockPeer("a1", "storage", "groupA")
	peerA2 := NewMockPeer("a2", "couchdb", "groupA")

	// Group B
	peerB1 := NewMockPeer("b1", "storage", "groupB")
	peerB2 := NewMockPeer("b2", "couchdb", "groupB")

	hub.RegisterPeer(peerA1)
	hub.RegisterPeer(peerA2)
	hub.RegisterPeer(peerB1)
	hub.RegisterPeer(peerB2)

	data := &peer.FileData{
		MTime: time.Now(),
		Size:  100,
		Data:  []byte("test"),
	}

	// Dispatch from groupA peer
	hub.Dispatch(peerA1, "doc.md", data)

	// Check only groupA peers received it
	if len(peerA2.GetPuts()) != 1 {
		t.Error("PeerA2 should have received the file")
	}
	if len(peerB1.GetPuts()) != 0 {
		t.Error("PeerB1 should not have received the file")
	}
	if len(peerB2.GetPuts()) != 0 {
		t.Error("PeerB2 should not have received the file")
	}
}

func TestRegisterPeerFactory(t *testing.T) {
	// Clear any existing factories
	peerFactories = make(map[string]PeerFactory)

	called := false
	factory := func(conf config.PeerConf, dispatcher peer.DispatchFunc, store *storage.Store) (peer.Peer, error) {
		called = true
		return NewMockPeer("test", "test", "main"), nil
	}

	RegisterPeerFactory("test", factory)

	// Verify factory was registered
	if _, ok := peerFactories["test"]; !ok {
		t.Error("Factory was not registered")
	}

	// Try to use it
	store := createTestStore(t)
	defer store.Close()

	registeredFactory := peerFactories["test"]
	_, err := registeredFactory(nil, nil, store)
	if err != nil {
		t.Errorf("Factory call failed: %v", err)
	}
	if !called {
		t.Error("Factory was not called")
	}
}

func TestHubDispatchDeletionWithDeletedFlag(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	hub := NewHub(store)

	peer1 := NewMockPeer("peer1", "storage", "main")
	peer2 := NewMockPeer("peer2", "couchdb", "main")

	hub.RegisterPeer(peer1)
	hub.RegisterPeer(peer2)

	// Dispatch deletion with FileData{Deleted: true}
	deletionData := &peer.FileData{
		Deleted: true,
	}
	err := hub.Dispatch(peer1, "document.md", deletionData)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// Check peer2 received deletion (not put)
	peer2Deletes := peer2.GetDeletes()
	if len(peer2Deletes) != 1 || peer2Deletes[0] != "document.md" {
		t.Errorf("Peer2 should have received deletion via Delete(), got deletes=%v", peer2Deletes)
	}

	// Ensure it was NOT dispatched as a Put
	peer2Puts := peer2.GetPuts()
	if len(peer2Puts) != 0 {
		t.Errorf("Peer2 should not have received Put(), got puts=%v", peer2Puts)
	}
}

func TestHubDispatchDeletionWithNil(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	hub := NewHub(store)

	peer1 := NewMockPeer("peer1", "storage", "main")
	peer2 := NewMockPeer("peer2", "couchdb", "main")

	hub.RegisterPeer(peer1)
	hub.RegisterPeer(peer2)

	// Dispatch deletion with nil (original behavior)
	err := hub.Dispatch(peer1, "document.md", nil)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// Check peer2 received deletion
	peer2Deletes := peer2.GetDeletes()
	if len(peer2Deletes) != 1 || peer2Deletes[0] != "document.md" {
		t.Errorf("Peer2 should have received deletion via Delete(), got deletes=%v", peer2Deletes)
	}

	// Ensure it was NOT dispatched as a Put
	peer2Puts := peer2.GetPuts()
	if len(peer2Puts) != 0 {
		t.Errorf("Peer2 should not have received Put(), got puts=%v", peer2Puts)
	}
}

func TestHubDispatchRegularFile(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	hub := NewHub(store)

	peer1 := NewMockPeer("peer1", "storage", "main")
	peer2 := NewMockPeer("peer2", "couchdb", "main")

	hub.RegisterPeer(peer1)
	hub.RegisterPeer(peer2)

	// Dispatch regular file with Deleted=false
	fileData := &peer.FileData{
		MTime:   time.Now(),
		Size:    100,
		Data:    []byte("test content"),
		Deleted: false,
	}
	err := hub.Dispatch(peer1, "document.md", fileData)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// Check peer2 received it as a Put (not Delete)
	peer2Puts := peer2.GetPuts()
	if len(peer2Puts) != 1 || peer2Puts[0] != "document.md" {
		t.Errorf("Peer2 should have received file via Put(), got puts=%v", peer2Puts)
	}

	// Ensure it was NOT dispatched as Delete
	peer2Deletes := peer2.GetDeletes()
	if len(peer2Deletes) != 0 {
		t.Errorf("Peer2 should not have received Delete(), got deletes=%v", peer2Deletes)
	}
}
