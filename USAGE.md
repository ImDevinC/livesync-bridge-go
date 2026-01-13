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
  "baseDir": "",
  "initialSync": true
}
```

**Initial Sync**: When `initialSync` is enabled (default: `true`), the CouchDB peer will scan for existing documents on first startup and sync them to other peers in the group. This ensures that files already present in CouchDB are synced when setting up the application on a new computer. Set to `false` to disable this behavior and only sync new changes.

#### Sync Groups

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

### Initial Sync

Both Storage and CouchDB peers support initial synchronization of existing files when the application starts for the first time (or after a database reset).

#### Storage Peer Initial Sync

Storage peers use the `scanOfflineChanges` option (default: `true`) to detect files that were modified while the application was not running:

```json
{
  "type": "storage",
  "name": "local-vault",
  "group": "sync",
  "baseDir": "./vault/",
  "scanOfflineChanges": true
}
```

On startup, the storage peer:
1. Walks the directory tree
2. Compares file modification times and sizes with stored values
3. Dispatches changed files to other peers in the group

#### CouchDB Peer Initial Sync

CouchDB peers use the `initialSync` option (default: `true`) to download existing documents from the remote vault on first startup:

```json
{
  "type": "couchdb",
  "name": "remote-vault",
  "group": "sync",
  "url": "http://localhost:5984",
  "database": "my_vault",
  "passphrase": "encryption-key",
  "initialSync": true
}
```

On first startup (when no sync sequence is stored), the CouchDB peer:
1. Scans all documents in the database
2. Filters by `baseDir` if configured
3. Compares document metadata with stored values
4. Dispatches new or modified documents to other peers in the group
5. Logs progress for visibility (helpful for large vaults)

**Behavior:**
- **First run**: Syncs all existing documents matching the baseDir filter
- **Subsequent runs**: Only processes changes since last run (fast)
- **After reset**: Performs full sync again if database is cleared
- **Performance**: Processes documents in batches with progress logging

**Use Cases:**
- Setting up the application on a new computer with existing remote data
- Recovering from local data loss
- Initial population of a new storage peer from existing CouchDB vault

**Disabling Initial Sync:**
Set `initialSync: false` if you want the peer to only process new changes and ignore existing documents. This is useful when:
- You're confident all data is already synced
- You want faster startup time
- You're testing with partial data

## Conflict Resolution

When multiple devices edit the same file simultaneously, conflicts can occur during synchronization. LiveSync Bridge includes automatic conflict resolution to handle these situations gracefully without user intervention.

### What is a Conflict?

A conflict occurs when:
- Two devices modify the same file and try to sync at nearly the same time
- The remote version has been updated since you last read it
- CouchDB returns a `409 Conflict` error during synchronization

Without conflict resolution, the synchronization would fail and the file would not be synced. With conflict resolution enabled, LiveSync Bridge automatically determines which version should win based on your configured strategy.

### How Conflict Resolution Works

1. **Retry First**: When a conflict is detected, LiveSync Bridge first attempts to retry the operation with the latest revision (up to 3 attempts with exponential backoff)
2. **Resolve on Exhaustion**: If retries are exhausted and the conflict persists, conflict resolution is triggered
3. **Apply Strategy**: The configured strategy determines which version wins
4. **Sync Winner**: The winning version is persisted and synced to all peers in the group

### Configuration

Add the `conflictResolution` field to your CouchDB peer configuration:

```json
{
  "type": "couchdb",
  "name": "remote-vault",
  "group": "sync",
  "url": "http://localhost:5984",
  "database": "my_vault",
  "passphrase": "encryption-key",
  "conflictResolution": "timestamp-wins"
}
```

**Valid values:**
- `timestamp-wins` (default) - Winner chosen by modification timestamp
- `local-wins` - Local version always wins
- `remote-wins` - Remote version always wins
- `manual` - Manual resolution required (returns error)

If not specified, `timestamp-wins` is used as the default.

### Conflict Resolution Strategies

#### timestamp-wins (Default)

The version with the newer modification timestamp wins. This is the most intelligent strategy as it preserves the most recent changes.

**Behavior:**
- Compares `mtime` (modification time) of local and remote versions
- Local wins if local `mtime` > remote `mtime`
- Remote wins if remote `mtime` > local `mtime`
- **Tiebreaker**: Remote wins when timestamps are exactly equal

**Use when:**
- You want automatic resolution based on "last modified wins"
- Multiple users edit files at different times
- You trust modification timestamps to indicate the "latest" version

**Example log:**
```
Resolving conflict path=notes.md strategy=timestamp-wins localRev=2-abc remoteRev=3-def
Comparing timestamps localMTime=2026-01-13T10:30:00Z remoteMTime=2026-01-13T10:28:00Z
Timestamp winner: LOCAL (newer)
Conflict resolved path=notes.md winner=local newRev=4-xyz
```

#### local-wins

Local version always wins, regardless of timestamps. The local changes are forced to the remote server, overwriting the remote version.

**Behavior:**
- Forces local version to remote with remote's revision
- Re-uploads all chunks if file is chunked
- Ignores timestamps and remote content completely

**Use when:**
- This device is the "primary" or authoritative source
- You always want local changes to take precedence
- Testing or development scenarios where local state matters most

**Example log:**
```
Resolving conflict path=config.json strategy=local-wins
Resolving conflict with local-wins strategy path=config.json
Conflict resolved path=config.json winner=local newRev=4-xyz
```

#### remote-wins

Remote version always wins. The local changes are discarded and the remote version is accepted and synced to local peers.

**Behavior:**
- Accepts remote version as-is
- Dispatches remote data to local storage peer(s)
- Local changes are lost

**Use when:**
- The remote server is the "primary" or authoritative source
- You want to prevent local changes from overwriting remote data
- Implementing a "download-only" or "read-mostly" sync mode

**Example log:**
```
Resolving conflict path=template.md strategy=remote-wins
Resolving conflict with remote-wins strategy path=template.md
Accepting remote version and dispatching to local peers path=template.md
Conflict resolved path=config.json winner=remote newRev=3-def
```

#### manual

Manual resolution required. When a conflict occurs, LiveSync Bridge returns an error instead of automatically resolving it.

**Behavior:**
- Returns a descriptive error when conflict is detected
- Synchronization fails for that file
- Requires user intervention to resolve

**Use when:**
- You want full control over conflict resolution
- Conflicts are rare and should be handled case-by-case
- You want to be notified when conflicts occur

**Example log:**
```
Resolving conflict path=important.md strategy=manual
ERROR: manual conflict resolution required for important.md (local: 2-abc, remote: 3-def)
```

### Conflict Resolution for Delete Operations

Conflict resolution also applies to delete operations when the remote file has been modified.

**Strategy behavior for deletes:**

- **timestamp-wins**: Delete proceeds if delete time >= remote modification time; otherwise remote version is preserved
- **local-wins**: Delete always proceeds
- **remote-wins**: Delete is canceled and remote version is preserved
- **manual**: Error returned requiring manual intervention

When a delete is canceled (remote wins), the remote version is automatically dispatched to local peers to ensure the file is preserved locally.

**Example log (delete conflict with timestamp-wins):**
```
Resolving delete conflict docID=notes.md strategy=timestamp-wins
Delete wins by timestamp localMTime=2026-01-13T10:30:00Z remoteMTime=2026-01-13T10:25:00Z docID=notes.md
Document deleted successfully
```

**Example log (delete canceled):**
```
Resolving delete conflict docID=notes.md strategy=remote-wins
Remote version wins by remote-wins strategy, canceling delete docID=notes.md
Accepting remote version (canceling delete) and dispatching to local peers docID=notes.md
```

### Chunked Files

Conflict resolution works seamlessly with chunked files (files larger than the chunk size threshold):

- **timestamp-wins**: Compares timestamps of main document, winner's chunks replace all chunks
- **local-wins**: Deletes all remote chunks and re-uploads local chunks
- **remote-wins**: Preserves remote chunks and dispatches to local peers

All chunks are handled atomically as part of the resolution process.

### Best Practices

1. **Choose the right strategy**: Consider your use case and pick the strategy that matches your needs
2. **Monitor logs**: Watch for conflict resolution messages in the logs to understand when conflicts occur
3. **Test your strategy**: Use the integration tests to validate behavior matches your expectations
4. **Timestamp accuracy**: For `timestamp-wins` strategy, ensure system clocks are synchronized across devices (use NTP)
5. **Backup important data**: While conflict resolution preserves data based on strategy, backups are always recommended
6. **Manual for critical files**: Consider using `manual` strategy for files where conflicts should never be automatically resolved

### Disabling Conflict Resolution

To disable automatic conflict resolution, set `conflictResolution: "manual"`. This will cause synchronization to fail when conflicts occur, requiring you to resolve them manually.

Alternatively, you can omit the `conflictResolution` field entirely to use the default `timestamp-wins` strategy, which provides automatic resolution based on modification timestamps.

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
