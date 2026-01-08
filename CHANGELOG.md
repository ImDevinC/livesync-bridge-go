# Changelog

All notable changes to this project will be documented in this file.

## [1.1.5](https://github.com/ImDevinC/livesync-bridge-go/compare/v1.1.4...v1.1.5) (2026-01-08)

### Bug Fixes

* implement automatic reconnection for CouchDB changes feed ([712a675](https://github.com/ImDevinC/livesync-bridge-go/commit/712a6755766a4ff24cdf1a2533c6a0048a7512b3))

## [1.1.4](https://github.com/ImDevinC/livesync-bridge-go/compare/v1.1.3...v1.1.4) (2026-01-06)

### Bug Fixes

* resolve deletion sync with empty paths from CouchDB ([980a6f1](https://github.com/ImDevinC/livesync-bridge-go/commit/980a6f1f6fb567834198e8d6c21e8467f8d534bb))

## [1.1.3](https://github.com/ImDevinC/livesync-bridge-go/compare/v1.1.2...v1.1.3) (2026-01-06)

### Bug Fixes

* resolve file conflicts in parent paths during sync ([67fb812](https://github.com/ImDevinC/livesync-bridge-go/commit/67fb812aa4cf1213d23e36740c1e3aa754efbd54))

## [1.1.2](https://github.com/ImDevinC/livesync-bridge-go/compare/v1.1.1...v1.1.2) (2026-01-06)

### Bug Fixes

* resolve directory-file path conflicts during sync ([54490d8](https://github.com/ImDevinC/livesync-bridge-go/commit/54490d81a27f899ffa4643a9a99598d68fd1262d))

## [1.1.1](https://github.com/ImDevinC/livesync-bridge-go/compare/v1.1.0...v1.1.1) (2026-01-02)

### Bug Fixes

* detect and sync deletions during offline periods ([ac62bc4](https://github.com/ImDevinC/livesync-bridge-go/commit/ac62bc4755456c317cef8b7cb7f88ac397f2daf1))

## [1.1.0](https://github.com/ImDevinC/livesync-bridge-go/compare/v1.0.5...v1.1.0) (2026-01-02)

### Features

* add initial sync support for CouchDB peers ([7599123](https://github.com/ImDevinC/livesync-bridge-go/commit/75991230dea2e4eaa8dc5fb608e77425455f2d36))

## [Unreleased]

### Added
- Deletion detection during offline periods
  - Storage peers now detect files deleted while the bridge was offline on startup
  - CouchDB peers now detect documents deleted while the bridge was offline during initial sync
  - New `IteratePrefix` method in storage layer for efficient prefix-based iteration
  - Document sync tracking in CouchDB peers to identify deletions across bridge restarts
- Initial sync support for CouchDB peers
  - Automatically syncs existing documents on first run when no sync sequence is stored
  - Configurable via `initialSync` option in peer configuration (default: `true`)
  - Bidirectional sync based on modification time comparison
  - Progress logging for visibility during sync of large vaults
  - Metadata tracking to avoid re-syncing unchanged documents on subsequent starts
- New `AllDocsIterator` method in CouchDB client for memory-efficient document streaming

### Fixed
- Storage peer now properly syncs file deletions that occurred while bridge was offline
- CouchDB peer now properly syncs document deletions that occurred while bridge was offline
- Deletion metadata is now cleaned up properly when documents are deleted via changes feed

### Changed
- CouchDB peer now syncs existing documents on first startup instead of only monitoring new changes
- Sample configuration updated to include `initialSync` option

## [1.0.5](https://github.com/ImDevinC/livesync-bridge-go/compare/v1.0.4...v1.0.5) (2026-01-02)

### Bug Fixes

* **storagepeer:** add parameter labels to logging calls ([d3488f1](https://github.com/ImDevinC/livesync-bridge-go/commit/d3488f1fea8e98564f6a9b60b92c08d62e1192ee))

## [1.0.4](https://github.com/ImDevinC/livesync-bridge-go/compare/v1.0.3...v1.0.4) (2026-01-02)

### Bug Fixes

* **ci:** auto-trigger release workflow after version ([ba07720](https://github.com/ImDevinC/livesync-bridge-go/commit/ba077205cd35e968962daf1f8210dbf23010fd45))

## [1.0.3](https://github.com/ImDevinC/livesync-bridge-go/compare/v1.0.2...v1.0.3) (2026-01-02)

### Bug Fixes

* **ci:** use RELEASE_TOKEN to trigger binary builds ([29f9174](https://github.com/ImDevinC/livesync-bridge-go/commit/29f917465a5712023d2a85dd7fa25ae54c86ad95))

## [1.0.2](https://github.com/ImDevinC/livesync-bridge-go/compare/v1.0.1...v1.0.2) (2026-01-02)

### Bug Fixes

* **ci:** add tag_name to release upload action ([c1e09f2](https://github.com/ImDevinC/livesync-bridge-go/commit/c1e09f2c4f495a80e34a7e3cc14dca55d63beffe))

## [1.0.1](https://github.com/ImDevinC/livesync-bridge-go/compare/v1.0.0...v1.0.1) (2026-01-02)

### Bug Fixes

* **ci:** update release workflow to upload binaries only ([4706c2a](https://github.com/ImDevinC/livesync-bridge-go/commit/4706c2a74e185abc3a50cfc21b1f47adba363bcc))

## 1.0.0 (2026-01-02)

### Features

* **ci:** add automatic semantic versioning ([196128c](https://github.com/ImDevinC/livesync-bridge-go/commit/196128c516249c1324d9e3fcab7968d8d62b7a72))
* **ci:** add GitHub Actions for CI/CD and releases ([61699cc](https://github.com/ImDevinC/livesync-bridge-go/commit/61699cc3f5dfc4c3060fcc9ad5553a919944a289))
* initial implementation of livesync-bridge in Go ([abf362d](https://github.com/ImDevinC/livesync-bridge-go/commit/abf362d652a098a8cc407e9554e31bc263f4b0a6))

### Bug Fixes

* **ci:** fix golangci-lint and address linting issues ([0de2555](https://github.com/ImDevinC/livesync-bridge-go/commit/0de2555efcf04abbe6102f68f7b48b111ba1d700))
* **ci:** update Go version to 1.25 in workflows ([c90239d](https://github.com/ImDevinC/livesync-bridge-go/commit/c90239dad86b4884b0a21a6c664749f9405052e1))
* **ci:** upgrade golangci-lint action to v4 ([284b309](https://github.com/ImDevinC/livesync-bridge-go/commit/284b309f134634ca4e7a44847a7bb702d46e8c4e))
* **test:** add mutex to mockDispatcher for thread safety ([5313c92](https://github.com/ImDevinC/livesync-bridge-go/commit/5313c92c85350c7ffd931943aec2558520d5e858))
* **test:** add platform-specific timeouts for macOS ([6b149ae](https://github.com/ImDevinC/livesync-bridge-go/commit/6b149ae490f6888b8f5b7a419dc8356a57bbf231))
* **test:** make TestMakeUniqueString more robust ([b138662](https://github.com/ImDevinC/livesync-bridge-go/commit/b138662a304d0e5fe9d3f249136708a3749c2146))

### Documentation

* add contributing and release process documentation ([0567960](https://github.com/ImDevinC/livesync-bridge-go/commit/0567960c061367f15accee48480a842ddb0c3d7f))

This project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html) and uses [Conventional Commits](https://www.conventionalcommits.org/).

## Unreleased

Initial development version with complete Go port of livesync-bridge from TypeScript/Deno.

### Features

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

### Bug Fixes

- Fixed CI workflow golangci-lint version mismatch
- Fixed macOS file watching test failures with platform-specific timeouts
- Fixed race condition in test mock dispatcher
- Fixed flaky Windows test for unique string generation
- Fixed various linting issues (gosimple, errcheck, staticcheck)
