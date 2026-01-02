package util

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGetConfigDir(t *testing.T) {
	configDir := GetConfigDir()

	// Should not be empty
	if configDir == "" {
		t.Error("GetConfigDir returned empty string")
	}

	// Should end with livesync-bridge
	if !strings.HasSuffix(configDir, "livesync-bridge") {
		t.Errorf("GetConfigDir should end with 'livesync-bridge', got: %s", configDir)
	}

	// Platform-specific checks
	switch runtime.GOOS {
	case "linux":
		// Should use XDG_CONFIG_HOME or ~/.config
		if os.Getenv("XDG_CONFIG_HOME") != "" {
			expected := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "livesync-bridge")
			if configDir != expected {
				t.Errorf("Expected %s, got %s", expected, configDir)
			}
		} else {
			if !strings.Contains(configDir, ".config") {
				t.Errorf("Expected path to contain .config, got: %s", configDir)
			}
		}
	case "darwin":
		if !strings.Contains(configDir, "Library/Application Support") {
			t.Errorf("Expected path to contain 'Library/Application Support', got: %s", configDir)
		}
	case "windows":
		if !strings.Contains(configDir, "AppData") {
			t.Errorf("Expected path to contain 'AppData', got: %s", configDir)
		}
	}
}

func TestGetDataDir(t *testing.T) {
	dataDir := GetDataDir()

	// Should not be empty
	if dataDir == "" {
		t.Error("GetDataDir returned empty string")
	}

	// Should end with livesync-bridge
	if !strings.HasSuffix(dataDir, "livesync-bridge") {
		t.Errorf("GetDataDir should end with 'livesync-bridge', got: %s", dataDir)
	}

	// Platform-specific checks
	switch runtime.GOOS {
	case "linux":
		// Should use XDG_DATA_HOME or ~/.local/share
		if os.Getenv("XDG_DATA_HOME") != "" {
			expected := filepath.Join(os.Getenv("XDG_DATA_HOME"), "livesync-bridge")
			if dataDir != expected {
				t.Errorf("Expected %s, got %s", expected, dataDir)
			}
		} else {
			if !strings.Contains(dataDir, ".local/share") {
				t.Errorf("Expected path to contain .local/share, got: %s", dataDir)
			}
		}
	case "darwin":
		if !strings.Contains(dataDir, "Library/Application Support") {
			t.Errorf("Expected path to contain 'Library/Application Support', got: %s", dataDir)
		}
	case "windows":
		if !strings.Contains(dataDir, "AppData") {
			t.Errorf("Expected path to contain 'AppData', got: %s", dataDir)
		}
	}
}

func TestGetDefaultConfigPath(t *testing.T) {
	configPath := GetDefaultConfigPath()

	// Should not be empty
	if configPath == "" {
		t.Error("GetDefaultConfigPath returned empty string")
	}

	// Should end with config.json
	if !strings.HasSuffix(configPath, "config.json") {
		t.Errorf("GetDefaultConfigPath should end with 'config.json', got: %s", configPath)
	}

	// Should contain livesync-bridge
	if !strings.Contains(configPath, "livesync-bridge") {
		t.Errorf("Expected path to contain 'livesync-bridge', got: %s", configPath)
	}
}

func TestGetDefaultDBPath(t *testing.T) {
	dbPath := GetDefaultDBPath()

	// Should not be empty
	if dbPath == "" {
		t.Error("GetDefaultDBPath returned empty string")
	}

	// Should end with .db
	if !strings.HasSuffix(dbPath, "livesync-bridge.db") {
		t.Errorf("GetDefaultDBPath should end with 'livesync-bridge.db', got: %s", dbPath)
	}

	// Should contain livesync-bridge
	if !strings.Contains(dbPath, "livesync-bridge") {
		t.Errorf("Expected path to contain 'livesync-bridge', got: %s", dbPath)
	}
}

func TestConfigAndDataDirsDifferent(t *testing.T) {
	configDir := GetConfigDir()
	dataDir := GetDataDir()

	// On Linux, config and data should be in different locations
	// On macOS/Windows, they might be the same
	if runtime.GOOS == "linux" {
		if configDir == dataDir {
			t.Error("On Linux, config and data directories should be different")
		}
	}
}
