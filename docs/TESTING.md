# Testing Guide

StreamBus has comprehensive test coverage across all components. This guide explains our testing strategy, how to run tests, and how to write effective tests.

## Test Philosophy

**Quality First**: Every feature is fully tested before being merged

**Test Pyramid**:
- **Unit Tests** (70%): Fast, isolated component tests
- **Integration Tests** (20%): Component interaction tests
- **End-to-End Tests** (10%): Full system tests

**Coverage Goals**:
- **Minimum**: 80% code coverage
- **Target**: 90%+ code coverage
- **Current**: 100% test pass rate (252/252 tests)

---

## Running Tests

### All Tests

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Specific Packages

```bash
# Storage engine tests
go test -v ./pkg/storage/...

# Protocol tests
go test -v ./pkg/protocol/...

# Client tests
go test -v ./pkg/client/...

# Consensus tests
go test -v ./pkg/consensus/...

# Production hardening tests
go test -v ./pkg/resilience/ ./pkg/health/ ./pkg/errors/ ./pkg/metrics/ ./pkg/logging/ ./pkg/timeout/
```

### Integration Tests

```bash
# End-to-end integration tests
go test -v ./test/integration/...

# With longer timeout
go test -v -timeout 60s ./test/integration/...

# Specific integration test
go test -v -run TestEndToEndIntegration ./test/integration/
```

### Using Make

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run integration tests
make test-integration

# Run specific package tests
make test-storage
make test-protocol
make test-client
```

---

## Test Coverage by Component

### Phase 1: Core Platform (49 tests)

**Storage Engine** (27 tests)
```bash
go test -v ./pkg/storage/...
```
- LSM-tree operations
- WAL functionality
- MemTable operations
- SSTable management
- Index lookups
- Compaction

**Protocol Layer** (Full coverage)
```bash
go test -v ./pkg/protocol/...
```
- Message encoding/decoding
- Request/response handling
- CRC validation
- Error handling

**Client Library** (22 tests)
```bash
go test -v ./pkg/client/...
```
- Producer operations
- Consumer operations
- Connection management
- Retry logic
- Batching

### Phase 2: Distributed System (203 tests)

**Raft Consensus** (36 tests)
```bash
go test -v ./pkg/consensus/...
```
- Leader election
- Log replication
- Snapshot management
- Member management
- State persistence

**Metadata Store** (8 tests)
```bash
go test -v ./pkg/metadata/...
```
- Metadata operations
- Version management
- Broker registration
- Partition management

**Cluster Coordination** (10 tests)
```bash
go test -v ./pkg/coordinator/...
```
- Partition assignment
- Rebalancing
- Leader tracking
- Health monitoring

**Integration Tests** (5 tests)
```bash
go test -v ./test/integration/...
```
- End-to-end producer-consumer flow
- Multi-broker scenarios
- Failover testing
- Network partition handling

**Production Hardening** (144 tests)
```bash
# Circuit breakers (14 tests)
go test -v ./pkg/resilience/

# Health checks (18 tests)
go test -v ./pkg/health/

# Error handling (30 tests)
go test -v ./pkg/errors/

# Metrics (29 tests)
go test -v ./pkg/metrics/

# Logging (24 tests)
go test -v ./pkg/logging/

# Timeouts (29 tests)
go test -v ./pkg/timeout/
```

---

## Writing Tests

### Unit Test Template

```go
func TestComponentName_FeatureName(t *testing.T) {
    // Arrange - Setup test data and dependencies
    component := NewComponent(config)
    testData := prepareTestData()

    // Act - Execute the function under test
    result, err := component.DoSomething(testData)

    // Assert - Verify the results
    assert.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

### Table-Driven Tests

```go
func TestValidation(t *testing.T) {
    tests := []struct {
        name      string
        input     string
        expected  bool
        expectErr bool
    }{
        {
            name:      "valid input",
            input:     "valid",
            expected:  true,
            expectErr: false,
        },
        {
            name:      "invalid input",
            input:     "invalid",
            expected:  false,
            expectErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := Validate(tt.input)
            if tt.expectErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.expected, result)
            }
        })
    }
}
```

### Integration Test Example

```go
func TestIntegration_ProducerConsumer(t *testing.T) {
    // Setup test cluster
    cluster := setupTestCluster(t, 3)
    defer cluster.Shutdown()

    // Create client
    client := createTestClient(t, cluster.BrokerAddresses())
    defer client.Close()

    // Produce messages
    producer := client.NewProducer()
    for i := 0; i < 100; i++ {
        err := producer.Send(ctx, "test-topic", nil, []byte(fmt.Sprintf("msg-%d", i)))
        require.NoError(t, err)
    }

    // Consume messages
    consumer := client.NewConsumer("test-topic", 0)
    consumer.SeekToBeginning()
    messages, err := consumer.Fetch(ctx)
    require.NoError(t, err)
    assert.Len(t, messages, 100)
}
```

### Benchmarks

```go
func BenchmarkOperation(b *testing.B) {
    // Setup
    component := NewComponent(DefaultConfig())

    // Reset timer to exclude setup time
    b.ResetTimer()

    // Run benchmark
    for i := 0; i < b.N; i++ {
        component.DoOperation()
    }
}

// Run benchmarks
go test -bench=. -benchmem ./pkg/storage/
```

---

## Test Utilities

### Test Helpers

```go
// Test cluster setup
func setupTestCluster(t *testing.T, nodeCount int) *TestCluster

// Test client creation
func createTestClient(t *testing.T, brokers []string) *client.Client

// Test data generation
func generateTestMessages(count int) [][]byte

// Cleanup helper
func cleanupTestData(t *testing.T, path string)
```

### Mocks and Stubs

```go
// Mock Raft node
type MockRaftNode struct {
    mock.Mock
}

func (m *MockRaftNode) Propose(ctx context.Context, data []byte) error {
    args := m.Called(ctx, data)
    return args.Error(0)
}

// Use in tests
mockNode := new(MockRaftNode)
mockNode.On("Propose", mock.Anything, mock.Anything).Return(nil)
```

---

## Continuous Integration

### GitHub Actions

Tests run automatically on:
- Every push
- Every pull request
- Nightly builds

```yaml
# .github/workflows/test.yml
name: Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
        with:
          go-version: 1.23
      - run: go test -v -race -coverprofile=coverage.out ./...
      - run: go tool cover -html=coverage.out -o coverage.html
```

### Pre-Commit Hooks

```bash
# Install pre-commit hooks
make install-hooks

# Hooks run:
# - go fmt
# - go vet
# - go test (fast tests only)
# - golangci-lint
```

---

## Test Best Practices

### Do's ✅

- **Write tests first** (TDD) when possible
- **Test one thing** per test function
- **Use descriptive names**: `TestComponent_Method_Scenario`
- **Use table-driven tests** for multiple cases
- **Clean up resources** with `defer` or `t.Cleanup()`
- **Use testify/assert** for readable assertions
- **Mock external dependencies**
- **Test error cases** as thoroughly as success cases
- **Run tests in parallel** when possible (`t.Parallel()`)
- **Keep tests fast** (< 1 second for unit tests)

### Don'ts ❌

- **Don't share state** between tests
- **Don't use sleep** for synchronization (use channels or mocks)
- **Don't test implementation details** (test behavior)
- **Don't ignore test failures**
- **Don't skip flaky tests** (fix them!)
- **Don't write brittle tests** (avoid hardcoded values)
- **Don't test third-party code**
- **Don't write tests without assertions**

---

## Troubleshooting Tests

### Flaky Tests

```bash
# Run tests multiple times to detect flakiness
go test -count=10 ./pkg/somepackage/

# Run with race detector
go test -race ./...
```

### Debugging Failing Tests

```bash
# Run specific test with verbose output
go test -v -run TestSpecificTest ./pkg/somepackage/

# Run with additional logging
go test -v -args -log-level=debug

# Run with debugger (using delve)
dlv test ./pkg/somepackage/ -- -test.run TestSpecificTest
```

### Performance Issues

```bash
# Profile tests
go test -cpuprofile=cpu.out ./pkg/somepackage/
go tool pprof cpu.out

# Memory profiling
go test -memprofile=mem.out ./pkg/somepackage/
go tool pprof mem.out
```

---

## Code Coverage

### Generate Coverage Report

```bash
# Generate coverage
go test -coverprofile=coverage.out ./...

# View in terminal
go tool cover -func=coverage.out

# View in browser
go tool cover -html=coverage.out

# Coverage by package
go test -coverprofile=coverage.out -covermode=count ./...
go tool cover -func=coverage.out | grep -v "total" | sort -k3 -rn
```

### Coverage Goals

- **Critical paths**: 100% coverage
- **Business logic**: 90%+ coverage
- **Infrastructure**: 80%+ coverage
- **Examples/docs**: No coverage requirement

---

## Testing in CI/CD

### Required Checks

All PRs must pass:
- ✅ All tests passing
- ✅ No race conditions
- ✅ Code coverage ≥ 80%
- ✅ Linting passes
- ✅ Benchmarks not regressed

### Nightly Tests

Extended test suite runs nightly:
- Stress tests
- Long-running tests
- Performance benchmarks
- Memory leak detection

---

## Test Organization

```
streambus/
├── pkg/                    # Package tests alongside code
│   ├── storage/
│   │   ├── lsm.go
│   │   └── lsm_test.go    # Unit tests
│   ├── client/
│   │   ├── client.go
│   │   └── client_test.go
│   └── ...
├── test/                   # Integration and E2E tests
│   ├── integration/
│   │   └── e2e_test.go
│   ├── stress/
│   │   └── stress_test.go
│   └── helpers/
│       └── testutil.go
└── examples/               # Example code (not tested in CI)
    └── ...
```

---

## Resources

- [Go Testing Package](https://pkg.go.dev/testing)
- [Testify Documentation](https://github.com/stretchr/testify)
- [Table-Driven Tests](https://github.com/golang/go/wiki/TableDrivenTests)
- [Go Test Comments](https://github.com/golang/go/wiki/TestComments)
- [Testing in Go Best Practices](https://github.com/golang/go/wiki/CodeReviewComments#tests)

---

**Questions about testing?** Ask in [Discussions](https://github.com/shawntherrien/streambus/discussions/categories/development).
