# Release Process

This document explains how to create a new release of LiveSync Bridge.

## Prerequisites

- All tests passing on main branch
- All changes committed and pushed
- CHANGELOG.md updated with release notes

## Creating a Release

### 1. Update CHANGELOG.md

Update the `[Unreleased]` section in CHANGELOG.md with the new version number and date:

```markdown
## [1.0.0] - 2026-01-15

### Added
- Feature 1
- Feature 2

### Fixed
- Bug fix 1
```

Commit the CHANGELOG update:

```bash
git add CHANGELOG.md
git commit -m "docs(changelog): prepare release v1.0.0"
git push
```

### 2. Create and Push a Version Tag

```bash
# Create an annotated tag
git tag -a v1.0.0 -m "Release v1.0.0"

# Push the tag to trigger the release workflow
git push origin v1.0.0
```

### 3. GitHub Actions Workflow

The release workflow will automatically:
1. Run all tests
2. Build binaries for 6 platforms:
   - Linux (AMD64, ARM64)
   - macOS (AMD64, ARM64)
   - Windows (AMD64, ARM64)
3. Generate SHA256 checksums
4. Create a GitHub Release with:
   - All binaries
   - Checksums file
   - Auto-generated release notes
   - Installation instructions

### 4. Verify the Release

1. Go to https://github.com/ImDevinC/livesync-bridge-go/releases
2. Verify the release was created
3. Check that all binaries are attached
4. Verify checksums are correct:
   ```bash
   wget https://github.com/ImDevinC/livesync-bridge-go/releases/download/v1.0.0/checksums.txt
   wget https://github.com/ImDevinC/livesync-bridge-go/releases/download/v1.0.0/livesync-bridge-linux-amd64
   sha256sum -c checksums.txt
   ```
5. Test a binary:
   ```bash
   chmod +x livesync-bridge-linux-amd64
   ./livesync-bridge-linux-amd64 --version
   # Should output: livesync-bridge version v1.0.0
   ```

## Release Versioning

Follow [Semantic Versioning](https://semver.org/):

- **MAJOR** (v1.0.0 → v2.0.0): Incompatible API changes
- **MINOR** (v1.0.0 → v1.1.0): New features, backward compatible
- **PATCH** (v1.0.0 → v1.0.1): Bug fixes, backward compatible

## Pre-releases

For beta or release candidate versions:

```bash
git tag -a v1.0.0-beta.1 -m "Release v1.0.0-beta.1"
git push origin v1.0.0-beta.1
```

The release will be marked as a pre-release automatically if the tag contains `-alpha`, `-beta`, or `-rc`.

## Hotfix Releases

For urgent bug fixes:

1. Create a hotfix branch from the release tag:
   ```bash
   git checkout -b hotfix/v1.0.1 v1.0.0
   ```

2. Make the fix and commit:
   ```bash
   git commit -m "fix(peer): resolve sync deadlock issue"
   ```

3. Create a pull request to main

4. After merge, tag the release:
   ```bash
   git checkout main
   git pull
   git tag -a v1.0.1 -m "Release v1.0.1"
   git push origin v1.0.1
   ```

## Troubleshooting

### Release Workflow Failed

1. Check the [Actions tab](https://github.com/ImDevinC/livesync-bridge-go/actions)
2. Review the workflow logs
3. Common issues:
   - Tests failing: Fix tests and create a new tag
   - Build errors: Fix code and create a new tag
   - Permission issues: Check repository settings

### Incorrect Version in Binary

The version is set via ldflags during build. Verify the workflow is using the correct tag:

```yaml
-ldflags="-s -w -X main.version=${{ steps.get_version.outputs.VERSION }}"
```

### Deleting a Release

If you need to delete a release:

```bash
# Delete the tag locally
git tag -d v1.0.0

# Delete the tag remotely
git push origin :refs/tags/v1.0.0

# Delete the GitHub Release via web UI
# Go to Releases > Select release > Delete
```

Then create a new release with a new version number.

## Release Checklist

- [ ] All tests passing on main
- [ ] CHANGELOG.md updated
- [ ] Version tag created and pushed
- [ ] GitHub Release created successfully
- [ ] All binaries attached to release
- [ ] Checksums verified
- [ ] Installation instructions accurate
- [ ] Version flag works in binaries
- [ ] Announcement posted (if applicable)

## Distribution Channels

Consider publishing to:
- [x] GitHub Releases (automatic)
- [ ] Homebrew (create formula)
- [ ] apt/yum repositories
- [ ] Docker Hub
- [ ] Snap Store
- [ ] Flatpak

See distribution-specific documentation for details.
