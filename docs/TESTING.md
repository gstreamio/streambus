# StreamBus Testing Strategy

This document describes the comprehensive testing strategy for StreamBus, including how to run tests, write new tests, and understand test coverage.

**Current Status**: 81% core library coverage (pkg/ packages) | 550+ tests passing

## Table of Contents

- [Testing Philosophy](#testing-philosophy)
- [Test Types](#test-types)
- [Running Tests](#running-tests)
- [Writing Tests](#writing-tests)
- [Coverage Analysis](#coverage-analysis)
- [CI/CD Pipeline](#cicd-pipeline)
- [Best Practices](#best-practices)
- [Coverage Improvement Roadmap](#coverage-improvement-roadmap)

## Testing Philosophy

StreamBus follows a multi-layered testing approach to ensure reliability, performance, and correctness:

1. **Unit Tests**: Fast, isolated tests for individual components
2. **Integration Tests**: End-to-end tests with real broker instances
3. **Chaos Tests**: Resilience testing with fault injection
4. **Performance Tests**: Benchmarks and load tests
5. **Security Tests**: Static analysis and vulnerability scanning

### ⚠️ CRITICAL RULE: No Kafka Dependencies

**StreamBus tests MUST NEVER depend on Apache Kafka software or competitor messaging systems.**

- ✅ **DO**: Use StreamBus broker for all integration and chaos tests
- ✅ **DO**: Run StreamBus broker on port 9092 (Kafka-compatible drop-in replacement)
- ✅ **DO**: Use StreamBus client libraries exclusively
- ❌ **DON'T**: Import Kafka client libraries (`kafka-go`, `sarama`, etc.) in test code
- ❌ **DON'T**: Run tests against actual Kafka broker instances
- ❌ **DON'T**: Depend on Kafka binaries or Docker images

**Why?** StreamBus is a **drop-in replacement** for Apache Kafka. Tests use port 9092 for Kafka compatibility, but MUST connect to StreamBus broker (not Kafka). This ensures:
- Tests validate StreamBus implementation, not Kafka's
- No dependencies on competitor software
- Validates the drop-in replacement experience
- Tests reflect real-world migration scenarios from Kafka to StreamBus

### Coverage Targets

- **Minimum Coverage**: 85%
- **Critical Paths**: 95%+
- **Package-level**: No package below 70%

## Test Types

### 1. Unit Tests

Unit tests are fast, isolated tests that verify individual components without external dependencies.

**Location**: Alongside source code (`*_test.go`)

**Characteristics**:
- No external dependencies
- Run with `-short` flag
- Use mocks for interfaces
- Fast execution (< 1s total)

**Example**:
```go
func TestMessageEncoding(t *testing.T) {
    msg := &protocol.Message{
        Key:   []byte("test-key"),
        Value: []byte("test-value"),
    }

    encoded, err := protocol.EncodeMessage(msg)
    if err != nil {
        t.Fatalf("Failed to encode: %v", err)
    }

    decoded, err := protocol.DecodeMessage(encoded)
    if err != nil {
        t.Fatalf("Failed to decode: %v", err)
    }

    if !bytes.Equal(decoded.Key, msg.Key) {
        t.Error("Key mismatch")
    }
}
```

### 2. Integration Tests (E2E)

Integration tests verify complete workflows with real broker instances.

**Location**: `tests/integration/`

**Characteristics**:
- Require running broker
- Test complete workflows
- Skip with `-short` flag
- Moderate execution time (seconds to minutes)

**Test Scenarios**:

| Test | Description | Messages | Duration |
|------|-------------|----------|----------|
| ProducerConsumerLifecycle | Basic produce/consume flow | 100 | ~5s |
| MultiPartition | Multi-partition handling | 150 | ~10s |
| LargeMessages | 1KB to 1MB payloads | 4 | ~5s |
| HighThroughput | 10,000 messages | 10,000 | ~30s |
| OrderingGuarantee | Message ordering | 100 | ~5s |

**Example**:
```go
func TestE2E_ProducerConsumerLifecycle(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    // Connect to StreamBus broker (Kafka-compatible port)
    brokers := []string{"localhost:9092"}
    topic := fmt.Sprintf("test-%d", time.Now().Unix())

    // Create client
    cfg := &client.Config{
        Brokers:        brokers,
        ConnectTimeout: 10 * time.Second,
    }
    c, err := client.New(cfg)
    if err != nil {
        t.Skipf("Cannot connect to broker: %v", err)
        return
    }
    defer c.Close()

    // Produce messages
    producer := client.NewProducer(c)
    for i := 0; i < 100; i++ {
        key := []byte(fmt.Sprintf("key-%d", i))
        value := []byte(fmt.Sprintf("message-%d", i))

        err := producer.Send(ctx, topic, 0, key, value)
        if err != nil {
            t.Fatalf("Failed to send message %d: %v", i, err)
        }
    }

    // Consume and verify
    consumer := client.NewConsumer(c, topic, 0)
    // ... consume logic
}
```

### 3. Chaos Tests

Chaos tests inject faults to verify system resilience and error handling.

**Location**: `tests/chaos/`

**Characteristics**:
- Fault injection (latency, errors, partitions)
- Probability-based failures
- Long-running (minutes)
- Skip with `-short` flag

**Fault Types**:

| Fault Type | Description | Default Probability |
|------------|-------------|-------------------|
| Latency | Artificial delays | 30% |
| Error | Simulated errors | 20% |
| SlowResponse | Slow operations | 50% |
| Disconnect | Connection drops | 10% |
| DataLoss | Message drops | 5% |
| Partition | Network splits | Manual |

**Scenarios**:

1. **Random Latency**: Tests timeout handling
2. **Intermittent Errors**: Tests retry logic
3. **Slow Network**: Tests degraded performance
4. **Combined Chaos**: Multiple fault types
5. **Network Partition**: Split-brain scenarios

**Example**:
```go
func TestChaos_RandomLatency(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping chaos test in short mode")
    }

    logger := &SimpleLogger{t: t}
    scenario := NewRandomLatencyScenario(logger)
    scenario.Duration = 30 * time.Second

    testFunc := func(ctx context.Context) error {
        // Create client
        c, _ := client.New(cfg)
        producer := client.NewProducer(c)

        ticker := time.NewTicker(100 * time.Millisecond)
        defer ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                return nil
            case <-ticker.C:
                // Inject latency
                if err := scenario.Injector.InjectLatency(ctx, "produce"); err != nil {
                    continue
                }

                // Attempt produce
                producer.Send(ctx, topic, 0, key, value)
            }
        }
    }

    err := scenario.Run(context.Background(), testFunc)

    // Verify fault injection occurred
    stats := scenario.Injector.GetStats()
    if stats[FaultTypeLatency] == 0 {
        t.Error("No latency faults were injected")
    }
}
```

### 4. Benchmarks

Performance benchmarks measure throughput, latency, and resource usage.

**Location**: `*_bench_test.go`

**Characteristics**:
- Measure performance
- Compare baselines
- Track regressions

**Example**:
```go
func BenchmarkMessageEncoding(b *testing.B) {
    msg := &protocol.Message{
        Key:   []byte("benchmark-key"),
        Value: make([]byte, 1024),
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = protocol.EncodeMessage(msg)
    }
}
```

## Running Tests

### Quick Commands

```bash
# Run unit tests only (fast)
make test-unit

# Run integration tests (requires broker)
make test-integration

# Run chaos tests (requires broker, slow)
make test-chaos

# Run all tests
make test-all

# Run with coverage
make test-coverage

# Run comprehensive coverage analysis
make coverage-analysis
```

### Prerequisites

**For Integration and Chaos Tests**:
```bash
# Start local broker
make run-broker

# Or use Docker Compose
make run-cluster
```

### Test Flags

```bash
# Run only short tests (unit tests)
go test -short ./...

# Run specific test
go test -run TestE2E_ProducerConsumerLifecycle ./tests/integration/

# Run with race detector
go test -race ./...

# Run with verbose output
go test -v ./...

# Run with timeout
go test -timeout 30m ./tests/chaos/
```

### Using the Coverage Script

```bash
# Generate HTML coverage report
./scripts/test-coverage.sh --html

# Test specific package
./scripts/test-coverage.sh -p ./pkg/storage

# Show function-level coverage
./scripts/test-coverage.sh --func

# Generate JSON report for CI
./scripts/test-coverage.sh --format json

# Custom threshold
./scripts/test-coverage.sh --threshold 90.0
```

## Writing Tests

### Unit Test Guidelines

1. **Keep tests fast**: No I/O, no sleep
2. **Use table-driven tests**:
```go
func TestParseTopicName(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid", "my-topic", "my-topic", false},
        {"empty", "", "", true},
        {"invalid chars", "topic@123", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseTopicName(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("ParseTopicName() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("ParseTopicName() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

3. **Use subtests** for better organization
4. **Mock external dependencies**
5. **Test error conditions**

### Integration Test Guidelines

1. **Check for short mode**:
```go
if testing.Short() {
    t.Skip("Skipping integration test in short mode")
}
```

2. **Use unique topic names**:
```go
topic := fmt.Sprintf("test-%s-%d", t.Name(), time.Now().Unix())
```

3. **Clean up resources**:
```go
defer client.Close()
```

4. **Handle broker unavailability**:
```go
c, err := client.New(cfg)
if err != nil {
    t.Skipf("Cannot connect to broker: %v", err)
    return
}
```

5. **Use timeouts**:
```go
timeout := time.After(30 * time.Second)
for {
    select {
    case <-timeout:
        t.Fatal("Test timeout")
    default:
        // Test logic
    }
}
```

### Chaos Test Guidelines

1. **Use predefined scenarios**:
```go
scenario := NewRandomLatencyScenario(logger)
scenario.Duration = 30 * time.Second
```

2. **Track statistics**:
```go
var successCount, errorCount int64
atomic.AddInt64(&successCount, 1)
```

3. **Verify fault injection**:
```go
stats := scenario.Injector.GetStats()
if stats[FaultTypeLatency] == 0 {
    t.Error("No latency faults were injected")
}
```

4. **Use context for cancellation**:
```go
testFunc := func(ctx context.Context) error {
    select {
    case <-ctx.Done():
        return nil
    // ... test logic
    }
}
```

### Benchmark Guidelines

1. **Reset timer** after setup:
```go
b.ResetTimer()
```

2. **Report custom metrics**:
```go
b.ReportMetric(float64(bytes)/float64(b.N), "B/msg")
```

3. **Use RunParallel** for concurrent tests:
```go
b.RunParallel(func(pb *testing.PB) {
    for pb.Next() {
        // Benchmark logic
    }
})
```

## Coverage Analysis

### Coverage Reports

The coverage script generates three types of reports:

1. **Text Summary**: Console output with overall coverage
2. **HTML Report**: Visual coverage with source highlighting
3. **JSON Report**: Machine-readable for CI/CD

### Understanding Coverage

```bash
# Overall coverage
Overall Coverage: 87.3%
Target Coverage:  85.0%

# Package-level breakdown
pkg/protocol                                      89.2%
pkg/storage                                       91.5%
pkg/server                                        85.7%
pkg/client                                        82.3%

# Low-coverage files
pkg/admin/metrics.go                              45.2%
pkg/server/connection_pool.go                     68.9%
```

### Improving Coverage

1. **Identify gaps**:
```bash
./scripts/test-coverage.sh --func | head -20
```

2. **Focus on critical paths** (server, protocol, storage)
3. **Add integration tests** for complex workflows
4. **Use coverage-guided development**

## CI/CD Pipeline

### GitHub Actions Workflow

The CI/CD pipeline runs on every push and pull request:

```yaml
jobs:
  unit-tests:      # Go 1.21, 1.22 matrix
  integration-tests: # Requires broker
  chaos-tests:     # Main branch only
  lint:            # golangci-lint
  benchmarks:      # Performance tracking
  security-scan:   # Gosec + CodeQL
  build:           # Multi-platform builds
  test-summary:    # Aggregate results
```

### Job Dependencies

```
unit-tests ─┬─> integration-tests ──> chaos-tests
            │
            └─> lint ──> build ──> test-summary
```

### Matrix Testing

Tests run on multiple Go versions and platforms:

- **Go versions**: 1.21, 1.22
- **Platforms**: linux, darwin, windows
- **Architectures**: amd64, arm64

### Artifacts

The pipeline uploads:
- Coverage reports
- Benchmark results
- Chaos test logs
- Build binaries

## Best Practices

### 1. Test Isolation

- Each test should be independent
- Use unique resource names
- Clean up after tests
- Don't rely on execution order

### 2. Test Naming

```go
// Unit tests
func TestMessageEncoding(t *testing.T)

// Integration tests
func TestE2E_ProducerConsumerLifecycle(t *testing.T)

// Chaos tests
func TestChaos_RandomLatency(t *testing.T)

// Benchmarks
func BenchmarkMessageEncoding(b *testing.B)
```

### 3. Error Messages

```go
// Bad: Generic error
t.Error("Test failed")

// Good: Descriptive error with context
t.Errorf("Expected status %d, got %d after sending %d messages",
    expectedStatus, gotStatus, messageCount)
```

### 4. Test Data

```go
// Use realistic data
value := []byte(fmt.Sprintf("message-%d-with-realistic-payload", i))

// Not just placeholders
value := []byte("test")
```

### 5. Assertions

```go
// Check error conditions
if err != nil {
    t.Fatalf("Unexpected error: %v", err)
}

// Verify state
if len(messages) != expectedCount {
    t.Errorf("Expected %d messages, got %d", expectedCount, len(messages))
}

// Deep comparisons
if !reflect.DeepEqual(got, want) {
    t.Errorf("Mismatch:\ngot:  %+v\nwant: %+v", got, want)
}
```

### 6. Test Performance

- Unit tests should complete in < 1s total
- Integration tests: < 60s per test
- Chaos tests: Allow 15-30 minutes
- Use `-timeout` to prevent hangs

### 7. Continuous Testing

```bash
# Run tests on file change
find . -name '*.go' | entr -c make test-unit

# Or use a test watcher
gow test -v ./...
```

### 8. Code Review Checklist

- [ ] Tests pass locally
- [ ] Coverage meets threshold (85%)
- [ ] No flaky tests
- [ ] Error cases tested
- [ ] Documentation updated
- [ ] Benchmarks run (if performance-critical)

## Troubleshooting

### Common Issues

**Integration tests fail with "connection refused"**:
```bash
# Start StreamBus broker first (runs on Kafka-compatible port 9092)
make run-broker

# Or check if StreamBus broker is already running
lsof -i :9092
```

**Chaos tests timeout**:
```bash
# Increase timeout
go test -timeout 30m ./tests/chaos/
```

**Coverage below threshold**:
```bash
# Identify gaps
./scripts/test-coverage.sh --func

# Focus on low-coverage files
./scripts/test-coverage.sh | grep "< 70%"
```

**Race detector failures**:
```bash
# Run with race detector
go test -race ./...

# Common fixes:
# - Use sync.Mutex for shared state
# - Use atomic operations for counters
# - Use channels for synchronization
```

## Additional Resources

- [Go Testing Documentation](https://golang.org/pkg/testing/)
- [Table Driven Tests](https://github.com/golang/go/wiki/TableDrivenTests)
- [Advanced Testing Patterns](https://golang.org/doc/tutorial/add-a-test)
- [Chaos Engineering Principles](https://principlesofchaos.org/)
- [Code Coverage Best Practices](https://testing.googleblog.com/2020/08/code-coverage-best-practices.html)

## Coverage Improvement Roadmap

StreamBus ships with **81% core library coverage** (pkg/ packages), which provides solid production-ready quality with a plan to improve coverage to 85%+ in future releases.

## Summary

StreamBus employs a comprehensive testing strategy with:

- **81% current coverage** (core library) with roadmap to 85%+
- **Multi-layered approach**: unit, integration, chaos, performance
- **Automated CI/CD** with matrix testing
- **Chaos engineering** for resilience validation
- **Performance benchmarks** for regression detection
- **Security scanning** with Gosec and CodeQL

StreamBus is production-ready with solid test coverage (81% core library) with a roadmap to achieve 85%+ coverage in future releases.
