package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/imdevinc/livesync-bridge/internal/config"
	"github.com/imdevinc/livesync-bridge/internal/couchdbpeer"
	"github.com/imdevinc/livesync-bridge/internal/hub"
	"github.com/imdevinc/livesync-bridge/internal/peer"
	"github.com/imdevinc/livesync-bridge/internal/storage"
	"github.com/imdevinc/livesync-bridge/internal/storagepeer"
	"github.com/imdevinc/livesync-bridge/internal/util"
)

const (
	envConfigKey = "LSB_CONFIG"
	envDBKey     = "LSB_DATA"
)

var (
	// version is set via ldflags during build
	version = "dev"
)

func main() {
	// Parse CLI flags
	configPath := flag.String("config", "", "Path to configuration file (overrides default)")
	dbPath := flag.String("db", "", "Path to database file (overrides default)")
	reset := flag.Bool("reset", false, "Reset persistent storage (clear all state)")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	// Handle version flag
	if *showVersion {
		fmt.Printf("livesync-bridge version %s\n", version)
		os.Exit(0)
	}

	// Determine config file path with precedence: CLI flag > env var > XDG default
	finalConfigPath := *configPath
	if finalConfigPath == "" {
		// Check environment variable
		if envPath := os.Getenv(envConfigKey); envPath != "" {
			finalConfigPath = envPath
		} else {
			// Use XDG/platform-specific default
			finalConfigPath = util.GetDefaultConfigPath()
		}
	}

	// Determine database path with precedence: CLI flag > env var > XDG default
	finalDBPath := *dbPath
	if finalDBPath == "" {
		// Check environment variable
		if envPath := os.Getenv(envDBKey); envPath != "" {
			finalDBPath = envPath
		} else {
			// Use XDG/platform-specific default
			finalDBPath = util.GetDefaultDBPath()
		}
	}

	// Set up logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	slog.Info("LiveSync Bridge is starting...")
	slog.Info("Configuration", "path", finalConfigPath)
	slog.Info("Database", "path", finalDBPath)

	// Initialize persistent storage
	if err := os.MkdirAll(filepath.Dir(finalDBPath), 0755); err != nil {
		slog.Error("Failed to create data directory", "error", err)
		os.Exit(1)
	}

	store, err := storage.NewStore(finalDBPath)
	if err != nil {
		slog.Error("Failed to initialize storage", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	slog.Info("Persistent storage initialized", "path", finalDBPath)

	// Handle reset flag
	if *reset {
		slog.Warn("Reset flag detected - clearing all persistent storage")
		if err := store.Clear(); err != nil {
			slog.Error("Failed to clear storage", "error", err)
			os.Exit(1)
		}
		slog.Info("Persistent storage cleared successfully")
	}

	// Load configuration
	cfg, err := config.LoadConfig(finalConfigPath)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	slog.Info("Configuration loaded", "peers", len(cfg.Peers))
	for _, peer := range cfg.Peers {
		slog.Info("Peer configured",
			"name", peer.GetName(),
			"type", peer.GetType(),
			"group", peer.GetGroup(),
			"baseDir", peer.GetBaseDir(),
		)
	}

	// Initialize Hub
	h := hub.NewHub(store)

	// Register peer factories
	hub.RegisterPeerFactory("storage", func(peerConf config.PeerConf, dispatcher peer.DispatchFunc, store *storage.Store) (peer.Peer, error) {
		storagePeerConf, ok := peerConf.(config.PeerStorageConf)
		if !ok {
			return nil, fmt.Errorf("invalid storage peer configuration")
		}
		return storagepeer.NewStoragePeer(storagePeerConf, dispatcher, store)
	})

	hub.RegisterPeerFactory("couchdb", func(peerConf config.PeerConf, dispatcher peer.DispatchFunc, store *storage.Store) (peer.Peer, error) {
		couchDBPeerConf, ok := peerConf.(config.PeerCouchDBConf)
		if !ok {
			return nil, fmt.Errorf("invalid CouchDB peer configuration")
		}
		return couchdbpeer.NewCouchDBPeer(couchDBPeerConf, dispatcher, store)
	})

	// Create peers from configuration
	if err := h.CreatePeersFromConfig(cfg); err != nil {
		slog.Error("Failed to create peers", "error", err)
		os.Exit(1)
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the hub
	if err := h.Start(); err != nil {
		slog.Error("Failed to start hub", "error", err)
		os.Exit(1)
	}

	fmt.Println("\nLiveSync Bridge started successfully!")
	fmt.Println("Press Ctrl+C to stop")

	// Wait for shutdown signal
	<-sigChan
	slog.Info("Shutdown signal received")

	// Stop the hub
	if err := h.Stop(); err != nil {
		slog.Error("Error during shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("LiveSync Bridge stopped gracefully")
}
