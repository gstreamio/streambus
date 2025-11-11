# StreamBus Build Guide

This guide covers building, testing, and releasing StreamBus.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Building from Source](#building-from-source)
- [Running Tests](#running-tests)
- [Docker Development](#docker-development)
- [Local Development](#local-development)
- [CI/CD Pipeline](#cicd-pipeline)
- [Release Process](#release-process)

## Prerequisites

### Required Tools

- **Go 1.21+**: [Download](https://golang.org/dl/)
- **Docker**: [Install](https://docs.docker.com/get-docker/)
- **Docker Compose**: Included with Docker Desktop
- **Make**: Usually pre-installed on Unix systems
- **Git**: Version control

### Optional Tools

- **kubectl**: For Kubernetes deployment testing
- **Helm**: For Helm chart testing
- **golangci-lint**: For code linting
- **kind** or **minikube**: For local Kubernetes testing

## Building from Source

### Quick Start

```bash
# Clone the repository
git clone https://github.com/shawntherrien/streambus.git
cd streambus

# Download dependencies
make deps

# Build all binaries
make build

# Run tests
make test
```

### Build Individual Components

**Broker:**
```bash
make build-broker

# Or manually
CGO_ENABLED=0 go build -o bin/streambus-broker ./cmd/broker
```

**Operator:**
```bash
make build-operator

# Or manually
cd deploy/kubernetes/operator
CGO_ENABLED=0 go build -o ../../../bin/streambus-operator ./main.go
```

### Build with Version Information

```bash
VERSION=v0.6.0 COMMIT=$(git rev-parse --short HEAD) make build
```

### Cross-Platform Builds

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 make build-broker

# Linux ARM64
GOOS=linux GOARCH=arm64 make build-broker

# macOS AMD64
GOOS=darwin GOARCH=amd64 make build-broker

# macOS ARM64 (M1/M2)
GOOS=darwin GOARCH=arm64 make build-broker

# Windows AMD64
GOOS=windows GOARCH=amd64 make build-broker
```

## Running Tests

### Unit Tests

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# View coverage report
open coverage.html
```

### Integration Tests

```bash
# Start test environment
docker-compose up -d broker1

# Run integration tests
make test-integration

# Cleanup
docker-compose down -v
```

### Specific Package Tests

```bash
# Test specific package
go test -v ./pkg/client

# Test with race detector
go test -race ./pkg/client

# Test specific function
go test -v -run TestProducer_Send ./pkg/client
```

### Benchmark Tests

```bash
# Run benchmarks
go test -bench=. ./pkg/...

# With memory allocation stats
go test -bench=. -benchmem ./pkg/...
```

## Docker Development

### Building Docker Images

**Using Make:**
```bash
# Build all images
make docker

# Build specific image
make docker-broker
make docker-operator

# Build and push to registry
make docker-push
```

**Using Docker directly:**
```bash
# Build broker image
docker build -t streambus/broker:dev \
  --build-arg VERSION=dev \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -f Dockerfile .

# Build operator image
docker build -t streambus/operator:dev \
  --build-arg VERSION=dev \
  -f deploy/kubernetes/operator/Dockerfile \
  deploy/kubernetes/operator
```

### Docker Compose

**Start cluster:**
```bash
# Start all services (3 brokers + Prometheus + Grafana)
make compose-up

# Or
docker-compose up -d

# View logs
make compose-logs

# Check status
docker-compose ps
```

**Access services:**
- Broker 1: `localhost:9092`
- Broker 2: `localhost:9192`
- Broker 3: `localhost:9292`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000` (admin/admin)

**Stop cluster:**
```bash
make compose-down
```

### Custom Docker Compose Configuration

Create `.env` file for custom settings:

```env
VERSION=v0.6.0
COMMIT=abc123
BUILD_TIME=2024-01-15T10:00:00Z
```

## Local Development

### Running Broker Locally

**Option 1: Using compiled binary**
```bash
# Build and run
make run

# Or
./bin/streambus-broker --config=config/broker.yaml
```

**Option 2: Using go run**
```bash
make run-dev

# Or
go run ./cmd/broker --config=config/broker.yaml --log-level=debug
```

### Configuration

Create `config/broker.yaml`:

```yaml
server:
  brokerId: 1
  host: localhost
  port: 9092
  httpPort: 8081
  grpcPort: 9093

storage:
  dataDir: /tmp/streambus/data
  raftDir: /tmp/streambus/raft

logging:
  level: debug
  format: text
  output: stdout

metrics:
  enabled: true
  port: 8081

health:
  enabled: true
  livenessPath: /health/live
  readinessPath: /health/ready
```

### Development Workflow

1. **Make code changes**

2. **Format code:**
```bash
make fmt
```

3. **Run linter:**
```bash
make lint
```

4. **Run tests:**
```bash
make test
```

5. **Build and test locally:**
```bash
make build
make run
```

6. **Test with Docker Compose:**
```bash
make docker
make compose-restart
```

## CI/CD Pipeline

### GitHub Actions Workflows

**CI Workflow** (`.github/workflows/ci.yml`):
- Triggers on push to `main` and `dev` branches
- Triggers on pull requests
- Runs linting, tests, builds
- Security scanning

**Release Workflow** (`.github/workflows/release.yml`):
- Triggers on version tags (e.g., `v0.6.0`)
- Builds and publishes Docker images
- Creates GitHub release with binaries
- Packages and publishes Helm charts

**Dev Build Workflow** (`.github/workflows/docker-dev.yml`):
- Triggers on push to `dev` branch
- Builds and pushes dev Docker images

### Running CI Locally

**Using act:**
```bash
# Install act
brew install act  # macOS
# or
curl https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash

# Run CI workflow
act push

# Run specific job
act -j test
```

## Release Process

### Semantic Versioning

StreamBus follows [Semantic Versioning](https://semver.org/):
- **Major** (v1.0.0): Breaking changes
- **Minor** (v0.6.0): New features, backward compatible
- **Patch** (v0.6.1): Bug fixes, backward compatible

### Creating a Release

1. **Update version in code** (if needed)

2. **Commit changes:**
```bash
git add .
git commit -m "Prepare release v0.6.0"
git push origin dev
```

3. **Create and push tag:**
```bash
# Create annotated tag
git tag -a v0.6.0 -m "Release v0.6.0"

# Push tag
git push origin v0.6.0
```

4. **GitHub Actions will automatically:**
   - Run tests
   - Build Docker images for multiple architectures
   - Push images to GitHub Container Registry
   - Build binaries for multiple platforms
   - Create GitHub release with changelog
   - Package and upload Helm charts

5. **Verify release:**
   - Check GitHub Actions workflow status
   - Verify Docker images: `docker pull ghcr.io/shawntherrien/streambus/broker:v0.6.0`
   - Download and test binaries from GitHub release

### Manual Release

If needed, release manually:

```bash
# Build all platforms
for os in linux darwin windows; do
  for arch in amd64 arm64; do
    if [ "$os" = "windows" ] && [ "$arch" = "arm64" ]; then
      continue
    fi
    GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build \
      -ldflags="-w -s -X main.version=v0.6.0" \
      -o bin/streambus-broker-v0.6.0-$os-$arch \
      ./cmd/broker
  done
done

# Build and push Docker images
VERSION=v0.6.0 make docker-push

# Package Helm chart
helm package deploy/kubernetes/helm/streambus-operator
```

## Build Artifacts

### Binary Naming Convention

```
streambus-broker-{version}-{os}-{arch}[.exe]

Examples:
- streambus-broker-v0.6.0-linux-amd64
- streambus-broker-v0.6.0-darwin-arm64
- streambus-broker-v0.6.0-windows-amd64.exe
```

### Docker Image Tags

**Broker:**
```
ghcr.io/shawntherrien/streambus/broker:latest
ghcr.io/shawntherrien/streambus/broker:v0.6.0
ghcr.io/shawntherrien/streambus/broker:v0.6
ghcr.io/shawntherrien/streambus/broker:v0
ghcr.io/shawntherrien/streambus/broker:dev
```

**Operator:**
```
ghcr.io/shawntherrien/streambus/operator:latest
ghcr.io/shawntherrien/streambus/operator:v0.6.0
ghcr.io/shawntherrien/streambus/operator:dev
```

## Troubleshooting

### Build Failures

**Go module issues:**
```bash
# Clean module cache
go clean -modcache

# Re-download dependencies
make deps

# Verify go.sum
go mod verify
```

**Docker build issues:**
```bash
# Clean Docker cache
docker builder prune

# Build without cache
docker build --no-cache -t streambus/broker:dev .
```

### Test Failures

**Clean test cache:**
```bash
go clean -testcache
```

**Run tests verbosely:**
```bash
go test -v -race ./...
```

**Debug specific test:**
```bash
go test -v -run TestSpecificFunction ./pkg/package
```

## Best Practices

1. **Always run tests before committing:**
   ```bash
   make test lint
   ```

2. **Use Make targets for consistency:**
   ```bash
   make build test docker
   ```

3. **Keep dependencies up to date:**
   ```bash
   make deps-update
   ```

4. **Follow Git Flow:**
   - Feature branches from `dev`
   - Pull requests to `dev`
   - Releases from `main`

5. **Version tagging:**
   - Use annotated tags: `git tag -a v0.6.0 -m "Release v0.6.0"`
   - Follow semantic versioning

## Additional Resources

- [Go Documentation](https://golang.org/doc/)
- [Docker Documentation](https://docs.docker.com/)
- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Helm Documentation](https://helm.sh/docs/)
- [Kubernetes Documentation](https://kubernetes.io/docs/)
