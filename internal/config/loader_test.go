package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configContent := `{
		"peers": [
			{
				"type": "storage",
				"name": "test-storage",
				"group": "main",
				"baseDir": "./test/"
			},
			{
				"type": "couchdb",
				"name": "test-couchdb",
				"group": "main",
				"database": "testdb",
				"username": "admin",
				"password": "pass",
				"url": "http://localhost:5984",
				"passphrase": "secret",
				"baseDir": "shared/"
			}
		]
	}`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Load the config
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validate
	if len(config.Peers) != 2 {
		t.Errorf("Expected 2 peers, got %d", len(config.Peers))
	}

	// Check first peer (storage)
	if config.Peers[0].GetType() != "storage" {
		t.Errorf("Expected first peer type 'storage', got '%s'", config.Peers[0].GetType())
	}
	if config.Peers[0].GetName() != "test-storage" {
		t.Errorf("Expected first peer name 'test-storage', got '%s'", config.Peers[0].GetName())
	}

	// Check second peer (couchdb)
	if config.Peers[1].GetType() != "couchdb" {
		t.Errorf("Expected second peer type 'couchdb', got '%s'", config.Peers[1].GetType())
	}
	if config.Peers[1].GetName() != "test-couchdb" {
		t.Errorf("Expected second peer name 'test-couchdb', got '%s'", config.Peers[1].GetName())
	}

	// Type-specific checks
	couchdbPeer, ok := config.Peers[1].(PeerCouchDBConf)
	if !ok {
		t.Fatal("Failed to cast to PeerCouchDBConf")
	}
	if couchdbPeer.Database != "testdb" {
		t.Errorf("Expected database 'testdb', got '%s'", couchdbPeer.Database)
	}
}

func TestLoadConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		expectError bool
	}{
		{
			name: "no peers",
			config: `{
				"peers": []
			}`,
			expectError: true,
		},
		{
			name: "duplicate names",
			config: `{
				"peers": [
					{
						"type": "storage",
						"name": "test",
						"baseDir": "./test/"
					},
					{
						"type": "storage",
						"name": "test",
						"baseDir": "./test2/"
					}
				]
			}`,
			expectError: true,
		},
		{
			name: "missing name",
			config: `{
				"peers": [
					{
						"type": "storage",
						"baseDir": "./test/"
					}
				]
			}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.json")

			if err := os.WriteFile(configPath, []byte(tt.config), 0644); err != nil {
				t.Fatalf("Failed to create test config: %v", err)
			}

			_, err := LoadConfig(configPath)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestConflictResolutionValidation(t *testing.T) {
	tests := []struct {
		name          string
		config        string
		expectError   bool
		expectedValue string
	}{
		{
			name: "default strategy applied when omitted",
			config: `{
				"peers": [
					{
						"type": "couchdb",
						"name": "test",
						"database": "testdb",
						"username": "admin",
						"password": "pass",
						"url": "http://localhost:5984",
						"passphrase": "secret"
					}
				]
			}`,
			expectError:   false,
			expectedValue: "timestamp-wins",
		},
		{
			name: "valid strategy timestamp-wins",
			config: `{
				"peers": [
					{
						"type": "couchdb",
						"name": "test",
						"database": "testdb",
						"username": "admin",
						"password": "pass",
						"url": "http://localhost:5984",
						"passphrase": "secret",
						"conflictResolution": "timestamp-wins"
					}
				]
			}`,
			expectError:   false,
			expectedValue: "timestamp-wins",
		},
		{
			name: "valid strategy local-wins",
			config: `{
				"peers": [
					{
						"type": "couchdb",
						"name": "test",
						"database": "testdb",
						"username": "admin",
						"password": "pass",
						"url": "http://localhost:5984",
						"passphrase": "secret",
						"conflictResolution": "local-wins"
					}
				]
			}`,
			expectError:   false,
			expectedValue: "local-wins",
		},
		{
			name: "valid strategy remote-wins",
			config: `{
				"peers": [
					{
						"type": "couchdb",
						"name": "test",
						"database": "testdb",
						"username": "admin",
						"password": "pass",
						"url": "http://localhost:5984",
						"passphrase": "secret",
						"conflictResolution": "remote-wins"
					}
				]
			}`,
			expectError:   false,
			expectedValue: "remote-wins",
		},
		{
			name: "valid strategy manual",
			config: `{
				"peers": [
					{
						"type": "couchdb",
						"name": "test",
						"database": "testdb",
						"username": "admin",
						"password": "pass",
						"url": "http://localhost:5984",
						"passphrase": "secret",
						"conflictResolution": "manual"
					}
				]
			}`,
			expectError:   false,
			expectedValue: "manual",
		},
		{
			name: "invalid strategy rejected",
			config: `{
				"peers": [
					{
						"type": "couchdb",
						"name": "test",
						"database": "testdb",
						"username": "admin",
						"password": "pass",
						"url": "http://localhost:5984",
						"passphrase": "secret",
						"conflictResolution": "invalid-strategy"
					}
				]
			}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.json")

			if err := os.WriteFile(configPath, []byte(tt.config), 0644); err != nil {
				t.Fatalf("Failed to create test config: %v", err)
			}

			config, err := LoadConfig(configPath)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// If no error expected, verify the strategy value
			if !tt.expectError && config != nil {
				couchdbPeer, ok := config.Peers[0].(PeerCouchDBConf)
				if !ok {
					t.Fatal("Failed to cast to PeerCouchDBConf")
				}
				if couchdbPeer.ConflictResolution == nil {
					t.Error("ConflictResolution is nil")
				} else if *couchdbPeer.ConflictResolution != tt.expectedValue {
					t.Errorf("Expected conflict resolution '%s', got '%s'", tt.expectedValue, *couchdbPeer.ConflictResolution)
				}
			}
		})
	}
}
