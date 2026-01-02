package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// RawConfig is used for JSON unmarshaling
type RawConfig struct {
	Peers []json.RawMessage `json:"peers"`
}

// LoadConfig loads and parses the configuration file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// First parse into raw config to inspect peer types
	var rawConfig RawConfig
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	config := &Config{
		Peers: make([]PeerConf, 0, len(rawConfig.Peers)),
	}

	// Parse each peer based on its type
	for i, rawPeer := range rawConfig.Peers {
		// First decode to get the type field
		var typeCheck struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(rawPeer, &typeCheck); err != nil {
			return nil, fmt.Errorf("failed to determine type for peer %d: %w", i, err)
		}

		var peer PeerConf
		switch typeCheck.Type {
		case "storage":
			var storagePeer PeerStorageConf
			if err := json.Unmarshal(rawPeer, &storagePeer); err != nil {
				return nil, fmt.Errorf("failed to parse storage peer %d: %w", i, err)
			}
			peer = storagePeer
		case "couchdb":
			var couchdbPeer PeerCouchDBConf
			if err := json.Unmarshal(rawPeer, &couchdbPeer); err != nil {
				return nil, fmt.Errorf("failed to parse couchdb peer %d: %w", i, err)
			}
			peer = couchdbPeer
		default:
			return nil, fmt.Errorf("unknown peer type '%s' for peer %d", typeCheck.Type, i)
		}

		config.Peers = append(config.Peers, peer)
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

// validateConfig performs validation on the loaded configuration
func validateConfig(config *Config) error {
	if len(config.Peers) == 0 {
		return fmt.Errorf("no peers configured")
	}

	names := make(map[string]bool)
	for i, peer := range config.Peers {
		// Check for unique names
		name := peer.GetName()
		if name == "" {
			return fmt.Errorf("peer %d has no name", i)
		}
		if names[name] {
			return fmt.Errorf("duplicate peer name: %s", name)
		}
		names[name] = true

		// Type-specific validation
		switch p := peer.(type) {
		case PeerCouchDBConf:
			// BaseDir is optional for CouchDB peers (used for path filtering)
			if p.URL == "" {
				return fmt.Errorf("couchdb peer %s has no URL", name)
			}
			if p.Database == "" {
				return fmt.Errorf("couchdb peer %s has no database", name)
			}
			if p.Username == "" {
				return fmt.Errorf("couchdb peer %s has no username", name)
			}
			if p.Password == "" {
				return fmt.Errorf("couchdb peer %s has no password", name)
			}
		case PeerStorageConf:
			// BaseDir is required for storage peers
			if p.BaseDir == "" {
				return fmt.Errorf("storage peer %s has no baseDir", name)
			}
		default:
			return fmt.Errorf("unknown peer type for peer %s", name)
		}
	}

	return nil
}
