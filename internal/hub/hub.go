package hub

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/imdevinc/livesync-bridge/internal/config"
	"github.com/imdevinc/livesync-bridge/internal/peer"
	"github.com/imdevinc/livesync-bridge/internal/storage"
)

// Hub is the central coordinator that manages peers and routes changes
type Hub struct {
	peers  []peer.Peer
	store  *storage.Store
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
	wg     sync.WaitGroup
}

// NewHub creates a new hub instance
func NewHub(store *storage.Store) *Hub {
	ctx, cancel := context.WithCancel(context.Background())
	return &Hub{
		peers:  make([]peer.Peer, 0),
		store:  store,
		ctx:    ctx,
		cancel: cancel,
	}
}

// RegisterPeer adds a peer to the hub
func (h *Hub) RegisterPeer(p peer.Peer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.peers = append(h.peers, p)
	slog.Info("Peer registered",
		"name", p.Name(),
		"type", p.Type(),
		"group", p.Group(),
	)
}

// Start begins all peer operations
func (h *Hub) Start() error {
	h.mu.RLock()
	peers := make([]peer.Peer, len(h.peers))
	copy(peers, h.peers)
	h.mu.RUnlock()

	slog.Info("Starting hub", "peers", len(peers))

	// Start each peer in its own goroutine
	for _, p := range peers {
		h.wg.Add(1)
		go func(peer peer.Peer) {
			defer h.wg.Done()

			slog.Info("Starting peer", "name", peer.Name())
			if err := peer.Start(); err != nil {
				slog.Error("Peer failed", "name", peer.Name(), "error", err)
			}
		}(p)
	}

	slog.Info("Hub started successfully")
	return nil
}

// Stop gracefully stops all peers
func (h *Hub) Stop() error {
	slog.Info("Stopping hub")

	// Cancel context to signal all peers to stop
	h.cancel()

	h.mu.RLock()
	peers := make([]peer.Peer, len(h.peers))
	copy(peers, h.peers)
	h.mu.RUnlock()

	// Stop each peer
	for _, p := range peers {
		if err := p.Stop(); err != nil {
			slog.Error("Error stopping peer", "name", p.Name(), "error", err)
		}
	}

	// Wait for all goroutines to finish
	h.wg.Wait()

	slog.Info("Hub stopped")
	return nil
}

// Dispatch routes a change from a source peer to all other peers in the same group
func (h *Hub) Dispatch(source peer.Peer, path string, data *peer.FileData) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Determine source group
	sourceGroup := ""
	if source != nil {
		sourceGroup = source.Group()
	}

	// Track if we dispatched to any peers
	dispatched := 0
	errors := make([]error, 0)

	// Dispatch to all peers in the same group (excluding source)
	for _, targetPeer := range h.peers {
		// Skip if this is the source peer
		if source != nil && targetPeer.Name() == source.Name() {
			continue
		}

		// Skip if not in same group
		if targetPeer.Group() != sourceGroup {
			continue
		}

		// Dispatch the change
		var success bool
		var err error

		if data == nil {
			// Deletion
			success, err = targetPeer.Delete(path)
		} else {
			// Put/Update
			success, err = targetPeer.Put(path, data)
		}

		if err != nil {
			slog.Error("Dispatch failed",
				"target", targetPeer.Name(),
				"path", path,
				"error", err,
			)
			errors = append(errors, fmt.Errorf("%s: %w", targetPeer.Name(), err))
		} else if success {
			dispatched++
			operation := "updated"
			if data == nil {
				operation = "deleted"
			}
			slog.Debug("Dispatched successfully",
				"source", getSourceName(source),
				"target", targetPeer.Name(),
				"path", path,
				"operation", operation,
			)
		}
	}

	slog.Debug("Dispatch complete",
		"source", getSourceName(source),
		"path", path,
		"dispatched", dispatched,
		"errors", len(errors),
	)

	// Return combined errors if any
	if len(errors) > 0 {
		return fmt.Errorf("dispatch errors: %v", errors)
	}

	return nil
}

// GetPeers returns all registered peers (thread-safe copy)
func (h *Hub) GetPeers() []peer.Peer {
	h.mu.RLock()
	defer h.mu.RUnlock()

	peers := make([]peer.Peer, len(h.peers))
	copy(peers, h.peers)
	return peers
}

// GetPeerByName finds a peer by name
func (h *Hub) GetPeerByName(name string) (peer.Peer, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, p := range h.peers {
		if p.Name() == name {
			return p, nil
		}
	}

	return nil, fmt.Errorf("peer not found: %s", name)
}

// GetPeersByGroup returns all peers in a specific group
func (h *Hub) GetPeersByGroup(group string) []peer.Peer {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]peer.Peer, 0)
	for _, p := range h.peers {
		if p.Group() == group {
			result = append(result, p)
		}
	}
	return result
}

// Context returns the hub's context
func (h *Hub) Context() context.Context {
	return h.ctx
}

// CreateDispatcher creates a dispatch function for a peer to use
// This is a factory method that binds the hub's Dispatch method
func (h *Hub) CreateDispatcher() peer.DispatchFunc {
	return func(source peer.Peer, path string, data *peer.FileData) error {
		return h.Dispatch(source, path, data)
	}
}

// getSourceName safely gets the source peer name
func getSourceName(source peer.Peer) string {
	if source == nil {
		return "external"
	}
	return source.Name()
}

// PeerFactory is a function that creates a peer from configuration
type PeerFactory func(conf config.PeerConf, dispatcher peer.DispatchFunc, store *storage.Store) (peer.Peer, error)

// peerFactories maps peer types to their factory functions
var peerFactories = make(map[string]PeerFactory)

// RegisterPeerFactory registers a factory for creating peers of a specific type
func RegisterPeerFactory(peerType string, factory PeerFactory) {
	peerFactories[peerType] = factory
}

// CreatePeersFromConfig creates all peers from configuration
func (h *Hub) CreatePeersFromConfig(cfg *config.Config) error {
	dispatcher := h.CreateDispatcher()

	for i, peerConf := range cfg.Peers {
		peerType := peerConf.GetType()

		factory, ok := peerFactories[peerType]
		if !ok {
			return fmt.Errorf("no factory registered for peer type '%s' (peer %d: %s)",
				peerType, i, peerConf.GetName())
		}

		p, err := factory(peerConf, dispatcher, h.store)
		if err != nil {
			return fmt.Errorf("failed to create peer %s: %w", peerConf.GetName(), err)
		}

		h.RegisterPeer(p)
	}

	return nil
}
