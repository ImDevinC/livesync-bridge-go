# Changelog

All notable changes to this project will be documented in this file.

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
