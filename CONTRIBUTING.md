# Contributing to LiveSync Bridge

Thank you for your interest in contributing to LiveSync Bridge! This document provides guidelines and instructions for contributing.

## Code of Conduct

Be respectful and constructive in all interactions. We're all here to make the project better.

## How to Contribute

### Reporting Bugs

1. Check if the bug has already been reported in [Issues](https://github.com/ImDevinC/livesync-bridge-go/issues)
2. If not, create a new issue with:
   - Clear title and description
   - Steps to reproduce
   - Expected vs actual behavior
   - Version information (`livesync-bridge --version`)
   - Platform (OS, architecture)
   - Configuration (sanitized, no passwords)
   - Relevant logs

### Suggesting Features

1. Check existing [Issues](https://github.com/ImDevinC/livesync-bridge-go/issues) for similar suggestions
2. Create a new issue with:
   - Clear description of the feature
   - Use cases and benefits
   - Possible implementation approach (if you have ideas)

### Pull Requests

1. Fork the repository
2. Create a feature branch from `main`:
   ```bash
   git checkout -b feature/my-feature
   # or
   git checkout -b fix/my-bugfix
   ```
3. Make your changes
4. Follow the coding standards (see below)
5. Write or update tests
6. Update documentation if needed
7. Commit using Angular commit format (see below)
8. Push to your fork
9. Open a pull request

## Development Setup

### Prerequisites

- Go 1.23 or later
- Git
- (Optional) Docker for CouchDB integration tests

### Clone and Build

```bash
git clone https://github.com/ImDevinC/livesync-bridge-go.git
cd livesync-bridge-go
go build -o bin/livesync-bridge ./cmd/livesync-bridge
```

### Run Tests

```bash
# All tests
go test ./...

# With coverage
go test ./... -cover

# Specific package
go test ./internal/couchdbpeer -v

# Integration tests (requires CouchDB)
docker run -d --name couchdb-test -p 5984:5984 \
  -e COUCHDB_USER=admin \
  -e COUCHDB_PASSWORD=password \
  couchdb:latest
COUCHDB_URL=http://localhost:5984 go test ./internal/couchdbpeer -v
```

## Coding Standards

### Go Style

Follow standard Go conventions:
- Run `gofmt` on all code
- Run `go vet` to catch common issues
- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use meaningful variable names
- Add comments for exported functions and types
- Keep functions focused and concise

### Commit Message Format

We use [Angular commit message format](https://github.com/angular/angular/blob/master/CONTRIBUTING.md#commit):

```
<type>(<scope>): <subject>

<body>
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `style`: Code style (formatting, etc.)
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `test`: Adding or updating tests
- `chore`: Build process, dependencies, etc.

**Scopes:**
- `config`: Configuration system
- `peer`: Peer interface or base
- `storage`: Storage peer
- `couchdb`: CouchDB peer
- `hub`: Hub dispatcher
- `cli`: Command line interface
- `ci`: CI/CD workflows
- `docs`: Documentation

**Examples:**

```
feat(storage): add support for .gitignore patterns

- Implement gitignore pattern matching
- Add tests for ignore patterns
- Update documentation
```

```
fix(couchdb): resolve document conflict retry issue

The retry logic was not properly fetching the latest revision
on each attempt, causing persistent conflicts.
```

**Rules:**
- No line should exceed 100 characters
- Subject uses imperative mood ("add" not "added")
- Body explains what and why, not how
- Reference issues: "Fixes #123" or "Relates to #456"

### Code Organization

- Keep packages focused and cohesive
- Use internal packages for implementation details
- Export only what's necessary
- Write tests alongside code
- Update documentation when changing behavior

### Testing

- Write unit tests for all new code
- Aim for >70% coverage
- Test edge cases and error conditions
- Use table-driven tests when appropriate
- Mock external dependencies
- Integration tests for CouchDB should be optional

Example test:

```go
func TestMyFeature(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {
            name:     "basic case",
            input:    "test",
            expected: "test_result",
            wantErr:  false,
        },
        // more test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := MyFeature(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("MyFeature() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if result != tt.expected {
                t.Errorf("MyFeature() = %v, want %v", result, tt.expected)
            }
        })
    }
}
```

## Documentation

### Code Documentation

- Add package comments for all packages
- Document all exported functions, types, and constants
- Use complete sentences
- Include examples where helpful

```go
// Package peer provides the interface and base implementation for all peer types.
package peer

// Peer defines the interface that all peer implementations must satisfy.
// Peers are responsible for syncing data with their respective backends
// (filesystem, CouchDB, etc.).
type Peer interface {
    // Start initializes the peer and begins monitoring for changes.
    Start() error
    
    // Stop gracefully shuts down the peer.
    Stop() error
}
```

### User Documentation

Update relevant documentation files:
- `README.md` - Project overview
- `USAGE.md` - Usage instructions
- `TESTING.md` - Testing guide
- Package READMEs - API documentation

## Pull Request Process

1. **Before submitting:**
   - Run all tests and ensure they pass
   - Run `gofmt` and `go vet`
   - Update documentation
   - Add/update tests for your changes
   - Commit with proper format

2. **PR Description:**
   - Describe what the PR does
   - Reference related issues
   - Explain any breaking changes
   - Add screenshots for UI changes (if applicable)

3. **Review Process:**
   - Address reviewer feedback
   - Keep commits clean and logical
   - Squash commits if requested
   - Update PR description as needed

4. **After Merge:**
   - Delete your feature branch
   - Update CHANGELOG.md if needed

## CI/CD

All PRs must pass:
- Unit tests on Linux, macOS, Windows
- Linting (golangci-lint)
- Build verification for all platforms

The CI runs automatically on every push to a PR.

## Questions?

- Open an issue for questions
- Check existing documentation
- Review closed issues and PRs for similar topics

## License

By contributing, you agree that your contributions will be licensed under the same license as the project (see LICENSE file).

Thank you for contributing! 🎉
