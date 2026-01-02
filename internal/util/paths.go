package util

import (
	"os"
	"path/filepath"
	"runtime"
)

// GetConfigDir returns the user's configuration directory following platform conventions
// Linux/BSD: $XDG_CONFIG_HOME/livesync-bridge or ~/.config/livesync-bridge
// macOS: ~/Library/Application Support/livesync-bridge
// Windows: %APPDATA%/livesync-bridge
func GetConfigDir() string {
	var baseDir string

	switch runtime.GOOS {
	case "windows":
		baseDir = os.Getenv("APPDATA")
		if baseDir == "" {
			baseDir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
	case "darwin":
		homeDir, _ := os.UserHomeDir()
		baseDir = filepath.Join(homeDir, "Library", "Application Support")
	default: // Linux, BSD, etc.
		baseDir = os.Getenv("XDG_CONFIG_HOME")
		if baseDir == "" {
			homeDir, _ := os.UserHomeDir()
			baseDir = filepath.Join(homeDir, ".config")
		}
	}

	return filepath.Join(baseDir, "livesync-bridge")
}

// GetDataDir returns the user's data directory following platform conventions
// Linux/BSD: $XDG_DATA_HOME/livesync-bridge or ~/.local/share/livesync-bridge
// macOS: ~/Library/Application Support/livesync-bridge
// Windows: %LOCALAPPDATA%/livesync-bridge
func GetDataDir() string {
	var baseDir string

	switch runtime.GOOS {
	case "windows":
		baseDir = os.Getenv("LOCALAPPDATA")
		if baseDir == "" {
			baseDir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
	case "darwin":
		homeDir, _ := os.UserHomeDir()
		baseDir = filepath.Join(homeDir, "Library", "Application Support")
	default: // Linux, BSD, etc.
		baseDir = os.Getenv("XDG_DATA_HOME")
		if baseDir == "" {
			homeDir, _ := os.UserHomeDir()
			baseDir = filepath.Join(homeDir, ".local", "share")
		}
	}

	return filepath.Join(baseDir, "livesync-bridge")
}

// GetDefaultConfigPath returns the default config file path
func GetDefaultConfigPath() string {
	return filepath.Join(GetConfigDir(), "config.json")
}

// GetDefaultDBPath returns the default database file path
func GetDefaultDBPath() string {
	return filepath.Join(GetDataDir(), "livesync-bridge.db")
}
