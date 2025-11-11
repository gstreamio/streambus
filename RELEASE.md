# StreamBus Release Process

This document describes the release process for StreamBus.

## Release Types

### Major Release (X.0.0)

Breaking changes that are not backward compatible.

**Examples:**
- Protocol changes
- API changes that break existing clients
- Major architectural changes

### Minor Release (0.X.0)

New features that are backward compatible.

**Examples:**
- New features
- Performance improvements
- New APIs that don't break existing functionality

### Patch Release (0.0.X)

Bug fixes and minor improvements.

**Examples:**
- Bug fixes
- Security patches
- Documentation updates

## Pre-Release Checklist

Before creating a release, ensure:

- [ ] All tests pass: `make test`
- [ ] Code is linted: `make lint`
- [ ] Documentation is updated
- [ ] CHANGELOG.md is updated
- [ ] Version numbers are updated in:
  - `deploy/kubernetes/helm/streambus-operator/Chart.yaml`
  - Code version constants (if any)
- [ ] All PRs are merged to `main`
- [ ] No known critical bugs

## Release Process

### 1. Prepare Release Branch

```bash
# Ensure you're on main
git checkout main
git pull origin main

# Create release branch (for major/minor releases)
git checkout -b release/v0.6.0
```

### 2. Update Version Numbers

**Helm Chart** (`deploy/kubernetes/helm/streambus-operator/Chart.yaml`):
```yaml
version: 0.6.0
appVersion: "0.6.0"
```

**Commit changes:**
```bash
git add .
git commit -m "Bump version to v0.6.0"
git push origin release/v0.6.0
```

### 3. Create Pull Request

Create PR from `release/v0.6.0` to `main` for final review.

### 4. Tag Release

After PR is merged:

```bash
# Checkout main
git checkout main
git pull origin main

# Create annotated tag
git tag -a v0.6.0 -m "Release v0.6.0

Features:
- Feature 1
- Feature 2

Bug Fixes:
- Fix 1
- Fix 2

See CHANGELOG.md for details."

# Push tag
git push origin v0.6.0
```

### 5. GitHub Actions Automation

The release workflow (`.github/workflows/release.yml`) will automatically:

1. **Run Tests**: Ensure all tests pass
2. **Build Docker Images**:
   - Multi-architecture (amd64, arm64)
   - Push to GitHub Container Registry
3. **Build Binaries**:
   - Linux, macOS, Windows
   - AMD64 and ARM64 architectures
4. **Create GitHub Release**:
   - Generate changelog
   - Upload binaries
   - Add Docker image info
5. **Package Helm Chart**: Package and upload to release

### 6. Verify Release

**Check GitHub Release:**
- Visit: `https://github.com/shawntherrien/streambus/releases/tag/v0.6.0`
- Verify binaries are attached
- Verify release notes

**Check Docker Images:**
```bash
# Pull and test broker image
docker pull ghcr.io/shawntherrien/streambus/broker:v0.6.0
docker run ghcr.io/shawntherrien/streambus/broker:v0.6.0 --version

# Pull and test operator image
docker pull ghcr.io/shawntherrien/streambus/operator:v0.6.0
```

**Check Helm Chart:**
```bash
# Download chart from release
wget https://github.com/shawntherrien/streambus/releases/download/v0.6.0/streambus-operator-0.6.0.tgz

# Install and test
helm install streambus-operator streambus-operator-0.6.0.tgz \
  --namespace streambus-system \
  --create-namespace
```

### 7. Update Documentation

Update documentation site (if applicable):
- Release notes
- Migration guide
- API documentation

### 8. Announce Release

Announce on:
- GitHub Discussions
- Community Slack
- Twitter/Social media
- Mailing list

## Release Channels

### Stable Releases

Tagged releases without pre-release suffixes:
- `v0.6.0`
- `v0.6.1`
- `v1.0.0`

**Docker tags:**
- `v0.6.0`, `v0.6`, `v0`, `latest`

### Pre-Release (Beta/RC)

Tagged releases with pre-release suffixes:
- `v0.6.0-beta.1`
- `v0.6.0-rc.1`

**Docker tags:**
- `v0.6.0-beta.1`

**GitHub Release**: Marked as pre-release

### Development Builds

Automatic builds from `dev` branch:

**Docker tags:**
- `dev`
- `dev-<commit-sha>`

## Hotfix Process

For critical bugs in production:

### 1. Create Hotfix Branch

```bash
# From main
git checkout main
git checkout -b hotfix/v0.6.1

# Make fix
git add .
git commit -m "Fix critical bug"
git push origin hotfix/v0.6.1
```

### 2. Create PR and Merge

Create PR to `main` with `hotfix` label.

### 3. Tag and Release

```bash
git checkout main
git pull origin main
git tag -a v0.6.1 -m "Hotfix: Critical bug fix"
git push origin v0.6.1
```

### 4. Backport to Dev

```bash
git checkout dev
git cherry-pick <hotfix-commit-sha>
git push origin dev
```

## Manual Release (Emergency)

If CI/CD is unavailable:

### Build Binaries

```bash
# Set version
VERSION=v0.6.0

# Build for all platforms
make build-linux
make build-darwin

# Or manually
for os in linux darwin windows; do
  for arch in amd64 arm64; do
    GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build \
      -ldflags="-w -s -X main.version=${VERSION}" \
      -o bin/streambus-broker-${VERSION}-$os-$arch \
      ./cmd/broker
  done
done
```

### Build and Push Docker Images

```bash
# Build multi-arch images
docker buildx build --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=v0.6.0 \
  -t ghcr.io/shawntherrien/streambus/broker:v0.6.0 \
  -t ghcr.io/shawntherrien/streambus/broker:latest \
  --push \
  -f Dockerfile .

docker buildx build --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=v0.6.0 \
  -t ghcr.io/shawntherrien/streambus/operator:v0.6.0 \
  -t ghcr.io/shawntherrien/streambus/operator:latest \
  --push \
  -f deploy/kubernetes/operator/Dockerfile \
  deploy/kubernetes/operator
```

### Create GitHub Release

Use GitHub UI or `gh` CLI:

```bash
gh release create v0.6.0 \
  --title "StreamBus v0.6.0" \
  --notes "Release notes here" \
  bin/streambus-broker-*
```

### Package Helm Chart

```bash
# Update version in Chart.yaml
helm package deploy/kubernetes/helm/streambus-operator

# Upload to release
gh release upload v0.6.0 streambus-operator-0.6.0.tgz
```

## Rollback

If a release has critical issues:

### 1. Communicate

Immediately notify users via all channels.

### 2. Revert Docker Tags

```bash
# Re-tag previous version as latest
docker pull ghcr.io/shawntherrien/streambus/broker:v0.5.9
docker tag ghcr.io/shawntherrien/streambus/broker:v0.5.9 \
  ghcr.io/shawntherrien/streambus/broker:latest
docker push ghcr.io/shawntherrien/streambus/broker:latest
```

### 3. Mark GitHub Release

Edit GitHub release and mark as "This release has known issues. Use v0.5.9 instead."

### 4. Create Hotfix

Follow hotfix process to address issues.

## Post-Release Tasks

After a successful release:

- [ ] Monitor error tracking for new issues
- [ ] Check GitHub issues for bug reports
- [ ] Update documentation
- [ ] Plan next release
- [ ] Merge release branch back to dev

## Version Support Policy

### Active Support

Latest major version receives:
- Security fixes
- Critical bug fixes
- Feature updates

### Limited Support

Previous major version receives:
- Security fixes only
- For 6 months after new major version

### End of Life

Versions older than limited support period:
- No updates
- No support

## Security Releases

For security vulnerabilities:

1. **Do not disclose publicly** until patch is ready
2. Prepare patch in private repository/branch
3. Coordinate with security team
4. Release with security advisory
5. Notify users immediately

Security advisory format:
```markdown
# Security Advisory: [CVE-XXXX-XXXX]

**Severity**: Critical/High/Medium/Low
**Affected Versions**: v0.5.0 - v0.6.0
**Patched Version**: v0.6.1

## Description
[Vulnerability description]

## Impact
[Impact description]

## Mitigation
Upgrade to v0.6.1 immediately:
\```bash
docker pull ghcr.io/shawntherrien/streambus/broker:v0.6.1
\```

## Credits
[Reporter credits]
```

## Release Notes Template

```markdown
# StreamBus v0.6.0

## 🎉 What's New

### Features
- Feature 1 (#123)
- Feature 2 (#124)

### Improvements
- Improvement 1 (#125)
- Improvement 2 (#126)

### Bug Fixes
- Fix 1 (#127)
- Fix 2 (#128)

## 📦 Installation

**Docker:**
\```bash
docker pull ghcr.io/shawntherrien/streambus/broker:v0.6.0
\```

**Kubernetes (Helm):**
\```bash
helm repo add streambus https://charts.streambus.io
helm install streambus-operator streambus/streambus-operator --version 0.6.0
\```

**Binary:**
Download from [releases page](https://github.com/shawntherrien/streambus/releases/tag/v0.6.0)

## 📚 Documentation

- [Upgrade Guide](UPGRADE.md)
- [Documentation](https://docs.streambus.io)
- [Examples](examples/)

## 🔄 Breaking Changes

- Breaking change 1
- Breaking change 2

## 📝 Full Changelog

See [CHANGELOG.md](CHANGELOG.md)

## 🙏 Contributors

Thanks to all contributors who made this release possible!
```

## Tools

### Recommended Tools

- **gh**: GitHub CLI for releases
- **docker buildx**: Multi-architecture builds
- **helm**: Helm chart packaging
- **git**: Version control

### Install Tools

```bash
# GitHub CLI
brew install gh

# Docker Buildx
docker buildx install

# Helm
brew install helm
```

## Contact

Questions about releases?
- GitHub Discussions
- Slack: #releases channel
- Email: releases@streambus.io
