# StreamBus Project Rules

## Critical Rules

### 🚫 NO KAFKA SOFTWARE DEPENDENCIES

**StreamBus MUST NEVER depend on Apache Kafka software or competitor messaging systems.**

- ❌ **NEVER** import Kafka client libraries (`github.com/segmentio/kafka-go`, `github.com/IBM/sarama`, etc.)
- ❌ **NEVER** test against running Kafka broker instances
- ❌ **NEVER** depend on Kafka binaries or Docker images
- ✅ **ALWAYS** use StreamBus broker for all tests (runs on Kafka-compatible port 9092)
- ✅ **ALWAYS** use StreamBus client libraries exclusively
- ✅ **ALWAYS** ensure tests validate StreamBus functionality, not competitor systems

**Why?** StreamBus is a **drop-in replacement** for Kafka. We use the same port (9092) for easy migration, but tests must ONLY connect to StreamBus broker, never Kafka software.

### Git Flow Process

- **NEVER** commit directly to `dev` or `main` branches
- **ALWAYS** follow Git Flow process with feature branches
- **ALWAYS** use Jira ticket number in branch name: `<jira-number>-<short-description>`
  - Example: `REG-1234-update-some-code-example-branch-name`

### Security & Code Quality

- **ALWAYS** make secure regex patterns (avoid ReDoS vulnerabilities)
- **ALWAYS** follow clean coding principles
- **ALWAYS** check SonarQube cognitive complexity before committing
  - Keep cognitive complexity low to avoid having to rewrite methods
  - Target: < 15 per method

### Testing Standards

- **Minimum coverage**: 85%
- **Critical paths**: 95%+
- **No package** below 70% coverage
- Integration tests MUST use StreamBus broker, not external systems
- See [docs/TESTING.md](../docs/TESTING.md) for complete testing guidelines

### Jira Integration

When asked to create or update tickets:
- Use Jira MCP ACLI server to create/update Jira tickets
- Include ticket number in branch names

## Code Standards

### Go Best Practices

- Follow [Effective Go](https://go.dev/doc/effective_go)
- Use `gofmt` for formatting
- All exported functions must have godoc comments
- Keep functions small and focused
- Use table-driven tests

### Error Handling

- Return errors, don't panic (except for programmer errors)
- Wrap errors with context: `fmt.Errorf("failed to read segment: %w", err)`
- Log errors at appropriate levels

### Concurrency

- Use channels for communication between goroutines
- Protect shared state with proper synchronization
- Always ensure goroutines can exit (no leaks)
- Use `context.Context` for cancellation

## Documentation

- Update documentation when changing behavior
- Keep [docs/TESTING.md](../docs/TESTING.md) current with test changes
- Update [CHANGELOG.md](../CHANGELOG.md) for significant changes
- Maintain godoc comments for all exported APIs

## Pre-Commit Checklist

Before committing, ensure:
- [ ] No Kafka dependencies (port 9092, Kafka imports, etc.)
- [ ] All tests pass (`make test-unit`)
- [ ] Code coverage meets threshold (>85%)
- [ ] Linters pass (`make lint`)
- [ ] Cognitive complexity is reasonable (< 15 per method)
- [ ] Git Flow followed (no direct commits to dev/main)
- [ ] Branch name includes Jira ticket number
- [ ] Documentation updated if needed
- [ ] Secure regex patterns used
