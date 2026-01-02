# LiveSync Bridge Usage Guide

## Default Locations

LiveSync Bridge follows platform conventions for storing configuration and data files.

### Linux/BSD
- **Config**: `~/.config/livesync-bridge/config.json`
  - Override with `$XDG_CONFIG_HOME` environment variable
- **Database**: `~/.local/share/livesync-bridge/livesync-bridge.db`
  - Override with `$XDG_DATA_HOME` environment variable

### macOS
- **Config**: `~/Library/Application Support/livesync-bridge/config.json`
- **Database**: `~/Library/Application Support/livesync-bridge/livesync-bridge.db`

### Windows
- **Config**: `%APPDATA%\livesync-bridge\config.json`
- **Database**: `%LOCALAPPDATA%\livesync-bridge\livesync-bridge.db`

## Overriding Defaults

You can override the default locations in three ways (in order of precedence):

### 1. Command Line Flags (Highest Priority)

```bash
./livesync-bridge --config /path/to/config.json --db /path/to/database.db
```

### 2. Environment Variables

```bash
export LSB_CONFIG=/path/to/config.json
export LSB_DATA=/path/to/database.db
./livesync-bridge
```

### 3. Platform Defaults (Lowest Priority)

If no flags or environment variables are set, the platform-specific defaults are used.

## First Time Setup

### 1. Create Config Directory

The application will create directories automatically, but you can also create them manually:

```bash
# Linux/BSD
mkdir -p ~/.config/livesync-bridge

# macOS
mkdir -p ~/Library/Application\ Support/livesync-bridge

# Windows (PowerShell)
New-Item -ItemType Directory -Force -Path "$env:APPDATA\livesync-bridge"
```

### 2. Create Configuration File

Create a `config.json` file in the config directory:

**Linux/BSD:** `~/.config/livesync-bridge/config.json`  
**macOS:** `~/Library/Application Support/livesync-bridge/config.json`  
**Windows:** `%APPDATA%\livesync-bridge\config.json`

Example configuration:

```json
{
  "peers": [
    {
      "type": "storage",
      "name": "local-vault",
      "group": "sync",
      "baseDir": "~/Documents/my-vault"
    },
    {
      "type": "couchdb",
      "name": "remote-vault",
      "group": "sync",
      "url": "http://localhost:5984",
      "username": "admin",
      "password": "password",
      "database": "my_vault",
      "passphrase": "my-encryption-key",
      "enableCompression": true
    }
  ]
}
```

### 3. Run the Bridge

```bash
# Using defaults
./livesync-bridge

# With custom config
./livesync-bridge --config /path/to/config.json

# Reset database (clear all sync state)
./livesync-bridge --reset
```

## Common Scenarios

### Scenario 1: Testing with Local Config

During development or testing, you might want to use a local config file:

```bash
./livesync-bridge --config ./configs/test-config.json --db ./data/test.db
```

### Scenario 2: Multiple Profiles

Run different instances with different configs:

```bash
# Work profile
./livesync-bridge --config ~/.config/livesync-bridge/work.json --db ~/.local/share/livesync-bridge/work.db

# Personal profile
./livesync-bridge --config ~/.config/livesync-bridge/personal.json --db ~/.local/share/livesync-bridge/personal.db
```

### Scenario 3: Using Environment Variables

Set environment variables in your shell profile for convenience:

```bash
# Add to ~/.bashrc or ~/.zshrc
export LSB_CONFIG="$HOME/.config/livesync-bridge/config.json"
export LSB_DATA="$HOME/.local/share/livesync-bridge/livesync-bridge.db"
```

### Scenario 4: Docker/Container Deployment

Mount config and data directories as volumes:

```bash
docker run -v ~/.config/livesync-bridge:/config \
           -v ~/.local/share/livesync-bridge:/data \
           livesync-bridge --config /config/config.json --db /data/livesync-bridge.db
```

## Configuration File Options

See [configs/config.sample.json](configs/config.sample.json) for detailed configuration options.

### Peer Types

#### Storage Peer (Filesystem)

```json
{
  "type": "storage",
  "name": "my-local-vault",
  "group": "sync",
  "baseDir": "/path/to/vault",
  "scanOfflineChanges": true
}
```

#### CouchDB Peer (Remote Vault)

```json
{
  "type": "couchdb",
  "name": "my-remote-vault",
  "group": "sync",
  "url": "http://localhost:5984",
  "username": "admin",
  "password": "password",
  "database": "vault_db",
  "passphrase": "encryption-key",
  "enableCompression": true,
  "customChunkSize": 102400,
  "baseDir": ""
}
```

### Sync Groups

Peers in the same `group` synchronize with each other. You can have multiple groups for different sync requirements.

Example - Two separate sync groups:

```json
{
  "peers": [
    {
      "type": "storage",
      "name": "work-local",
      "group": "work",
      "baseDir": "~/work-vault"
    },
    {
      "type": "couchdb",
      "name": "work-remote",
      "group": "work",
      "database": "work_vault",
      "..."
    },
    {
      "type": "storage",
      "name": "personal-local",
      "group": "personal",
      "baseDir": "~/personal-vault"
    },
    {
      "type": "couchdb",
      "name": "personal-remote",
      "group": "personal",
      "database": "personal_vault",
      "..."
    }
  ]
}
```

## Troubleshooting

### Check Current Paths

Run with any invalid flag to see the default paths being used:

```bash
./livesync-bridge --help
```

The application will show where it's looking for config and database files in the log output.

### Clear Sync State

If sync gets stuck or you want to start fresh:

```bash
./livesync-bridge --reset
```

This clears the internal database that tracks processed files and changes.

### Verify Configuration

The application validates configuration on startup and will show detailed error messages if there are issues.

### Enable Debug Logging

The application uses structured logging with `DEBUG` level enabled by default. Check the logs for detailed information about what's happening.

## Advanced Usage

### Custom Database Location

You can place the database anywhere:

```bash
./livesync-bridge --db /mnt/external/sync-state.db
```

### Read-Only Config Directory

If your config directory is read-only, place the database elsewhere:

```bash
./livesync-bridge --config /etc/livesync-bridge/config.json --db ~/sync-state.db
```

### Running as System Service

See your platform's documentation for creating system services:
- Linux: systemd unit files
- macOS: launchd plist files
- Windows: Windows Service or Task Scheduler

Example systemd unit (Linux):

```ini
[Unit]
Description=LiveSync Bridge
After=network.target

[Service]
Type=simple
User=youruser
ExecStart=/usr/local/bin/livesync-bridge
Restart=always

[Install]
WantedBy=multi-user.target
```

The service will use the XDG defaults for the user running it.
