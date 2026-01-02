# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial release of LiveSync Bridge Go port

### Changed

### Deprecated

### Removed

### Fixed

### Security

## [1.0.0] - TBD

### Added
- Complete port from TypeScript/Deno to Go
- XDG directory support (Linux, macOS, Windows)
- Config loading with JSON validation
- CLI with `--config`, `--db`, `--reset`, and `--version` flags
- BoltDB persistent storage for deduplication
- LRU cache for performance optimization
- Retry logic with exponential backoff and jitter
- Hub dispatcher with group-based routing
- Storage peer with fsnotify file watching
- CouchDB peer with E2EE (AES-256-GCM)
- Compression support (gzip)
- File chunking for large files (>100KB, configurable)
- Real-time changes feed monitoring
- CouchDB conflict resolution with automatic retry
- Platform-specific defaults (Linux/macOS/Windows)
- Comprehensive test suite (67+ unit tests, 13 integration tests)
- GitHub Actions CI/CD pipeline
- Multi-platform binary releases (Linux, macOS, Windows - AMD64 & ARM64)
- Complete documentation (README, USAGE, TESTING, API docs)

[Unreleased]: https://github.com/ImDevinC/livesync-bridge-go/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/ImDevinC/livesync-bridge-go/releases/tag/v1.0.0
