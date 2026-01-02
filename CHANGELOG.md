# Changelog

All notable changes to this project will be documented in this file.

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
