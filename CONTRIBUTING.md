# Contributing to StreamBus

Thank you for your interest in contributing to StreamBus! This document provides guidelines and instructions for contributing.

## Table of Contents
1. [Code of Conduct](#code-of-conduct)
2. [Getting Started](#getting-started)
3. [Development Setup](#development-setup)
4. [How to Contribute](#how-to-contribute)
5. [Coding Standards](#coding-standards)
6. [Testing Guidelines](#testing-guidelines)
7. [Pull Request Process](#pull-request-process)
8. [Community](#community)

---

## Code of Conduct

We are committed to providing a welcoming and inclusive environment. All contributors are expected to:

- Be respectful and considerate
- Welcome newcomers and help them get started
- Focus on what is best for the community
- Show empathy towards other community members

Unacceptable behavior includes harassment, discrimination, or any form of unwelcome conduct.

---

## Getting Started

### Prerequisites

- Go 1.23 or later
- Git
- Make
- Docker (optional, for integration tests)
- Protocol Buffers compiler (for protocol changes)

### Fork and Clone

1. Fork the repository on GitHub
2. Clone your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/streambus.git
   cd streambus
   ```
3. Add upstream remote:
   ```bash
   git remote add upstream https://github.com/shawntherrien/streambus.git
   ```

---

## Development Setup

### Install Dependencies

```bash
# Install Go dependencies
make deps

# Install development tools
make tools
```

### Build the Project

```bash
# Build all binaries
make build

# Run tests
make test

# Run linters
make lint
```

### Run Locally

```bash
# Run a single broker
make run-broker

# Or run a 3-node cluster with Docker Compose
make run-cluster
```

---

## How to Contribute

### Reporting Bugs

If you find a bug, please create an issue with:

- A clear, descriptive title
- Steps to reproduce the bug
- Expected behavior vs actual behavior
- Go version, OS, and StreamBus version
- Any relevant logs or screenshots

### Suggesting Features

Feature requests are welcome! Please provide:

- A clear description of the feature
- Use cases and motivation
- Proposed implementation (if you have ideas)
- Any potential drawbacks or alternatives

### Contributing Code

1. **Find or Create an Issue**: Before starting work, check if an issue exists. If not, create one to discuss the change.

2. **Create a Branch**: Use descriptive branch names:
   ```bash
   git checkout -b feature/your-feature-name
   # or
   git checkout -b fix/your-bug-fix
   ```

3. **Make Changes**: Write your code following our coding standards.

4. **Write Tests**: All new features and bug fixes must include tests.

5. **Run Tests and Linters**:
   ```bash
   make test
   make lint
   ```

6. **Commit Changes**: Use clear, descriptive commit messages:
   ```bash
   git commit -m "feat: add consumer group rebalancing"
   # or
   git commit -m "fix: resolve race condition in storage engine"
   ```

7. **Push to Your Fork**:
   ```bash
   git push origin your-branch-name
   ```

8. **Create Pull Request**: Open a PR against the `main` branch.

---

## Coding Standards

### Go Style Guidelines

We follow standard Go conventions:

- Use `gofmt` for formatting (run `make fmt`)
- Follow [Effective Go](https://go.dev/doc/effective_go)
- Use meaningful variable and function names
- Keep functions focused and small
- Document exported functions, types, and packages

### Code Organization

```
streambus/
├── cmd/              # Command-line applications
│   ├── broker/       # Broker main package
│   └── cli/          # CLI tool main package
├── pkg/              # Public library packages
│   ├── storage/      # Storage engine
│   ├── network/      # Network layer
│   ├── consensus/    # Raft consensus
│   └── ...
├── internal/         # Private application code
└── docs/             # Documentation
```

### Documentation

- All exported functions must have godoc comments
- Complex algorithms should have explanatory comments
- Update documentation when changing behavior

Example:
```go
// AppendBatch writes a batch of messages to the log and returns their offsets.
// This operation is atomic: either all messages are written or none are.
//
// The function blocks until messages are written to the WAL. If acks=all is
// configured, it also waits for replication to ISR before returning.
func (l *Log) AppendBatch(messages []Message) ([]Offset, error) {
    // Implementation
}
```

### Error Handling

- Return errors, don't panic (except for programmer errors)
- Wrap errors with context: `fmt.Errorf("failed to read segment: %w", err)`
- Use custom error types for specific error conditions
- Log errors at appropriate levels

### Concurrency

- Use channels for communication between goroutines
- Protect shared state with `sync.Mutex` or `sync.RWMutex`
- Prefer message passing over shared memory
- Avoid goroutine leaks (always ensure goroutines can exit)
- Use `context.Context` for cancellation

---

## Testing Guidelines

### Unit Tests

- Write tests for all new code
- Aim for >80% code coverage
- Use table-driven tests where appropriate
- Name tests descriptively: `TestStorageEngine_AppendBatch_ReturnsError_WhenDiskFull`

Example:
```go
func TestLog_AppendBatch(t *testing.T) {
    tests := []struct {
        name     string
        messages []Message
        want     []Offset
        wantErr  bool
    }{
        {
            name: "appends single message",
            messages: []Message{{Data: []byte("test")}},
            want: []Offset{0},
            wantErr: false,
        },
        // More test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Integration Tests

- Tag integration tests: `//go:build integration`
- Test realistic scenarios
- Use Docker containers for external dependencies
- Clean up resources in `defer` statements

### Benchmarks

- Write benchmarks for performance-critical code
- Use `b.ReportAllocs()` to track allocations
- Run benchmarks before and after changes

Example:
```go
func BenchmarkLog_AppendBatch(b *testing.B) {
    log := setupTestLog(b)
    defer log.Close()

    messages := generateMessages(100)
    b.ReportAllocs()
    b.ResetTimer()

    for i := 0; i < b.N; i++ {
        _, err := log.AppendBatch(messages)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

### Running Tests

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run integration tests
make test-integration

# Run benchmarks
make benchmark
```

---

## Pull Request Process

### Before Submitting

- [ ] Code follows style guidelines
- [ ] All tests pass (`make test`)
- [ ] Linters pass (`make lint`)
- [ ] Documentation updated
- [ ] CHANGELOG.md updated (for significant changes)
- [ ] Commit messages are clear

### PR Description

Provide a clear description:

```markdown
## Description
Brief description of the change

## Motivation
Why is this change needed?

## Changes
- List of specific changes

## Testing
How was this tested?

## Checklist
- [ ] Tests added/updated
- [ ] Documentation updated
- [ ] Benchmarks run (if performance-related)
```

### Review Process

1. Maintainers will review your PR
2. Address review comments
3. Once approved, a maintainer will merge

### Commit Message Format

We use conventional commits:

- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation changes
- `perf:` Performance improvement
- `refactor:` Code refactoring
- `test:` Test changes
- `chore:` Maintenance tasks

Example:
```
feat: implement consumer group rebalancing

- Add rebalance coordinator
- Implement sticky assignment strategy
- Add integration tests

Closes #123
```

---

## Community

### Communication Channels

- **GitHub Issues**: Bug reports and feature requests
- **GitHub Discussions**: Questions and general discussion
- **Slack**: Real-time chat (coming soon)
- **Mailing List**: Announcements and discussions (coming soon)

### Getting Help

- Check existing issues and documentation
- Ask in GitHub Discussions
- Join our community chat

### Recognition

Contributors will be:
- Listed in CONTRIBUTORS.md
- Mentioned in release notes
- Invited to join the StreamBus community

---

## Development Workflow

### Typical Development Cycle

1. Sync with upstream:
   ```bash
   git fetch upstream
   git checkout main
   git merge upstream/main
   ```

2. Create a feature branch:
   ```bash
   git checkout -b feature/my-feature
   ```

3. Make changes and commit:
   ```bash
   git add .
   git commit -m "feat: add my feature"
   ```

4. Keep branch updated:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

5. Push and create PR:
   ```bash
   git push origin feature/my-feature
   ```

### CI/CD Pipeline

Our CI pipeline runs:
- Unit tests on multiple Go versions
- Integration tests
- Linters and static analysis
- Security scans
- Benchmarks

All checks must pass before merging.

---

## License

By contributing to StreamBus, you agree that your contributions will be licensed under the Apache 2.0 License.

---

## Thank You!

Your contributions make StreamBus better. We appreciate your time and effort!

For questions, reach out via GitHub Issues or Discussions.
