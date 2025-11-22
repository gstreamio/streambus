# StreamBus Testing Status & Plan

**Date**: November 22, 2025  
**Current Coverage**: ~80% (unit tests)  
**Target Coverage**: 85%+ overall, 95%+ for critical paths

## Executive Summary

StreamBus has a comprehensive testing strategy with **550+ passing unit tests** covering core functionality. The project follows a multi-layered approach including unit tests, integration tests, chaos tests, and performance benchmarks. This document outlines the current status and provides a systematic plan to achieve our coverage goals.

## Current Test Status

### ✅ Passing Tests (Unit Tests - Short Mode)

All unit tests are passing with the following coverage by package:

| Package | Coverage | Status | Notes |
|---------|----------|--------|-------|
| `pkg/errors` | 97.5% | ✅ Excellent | Error handling well-tested |
| `pkg/timeout` | 96.7% | ✅ Excellent | Timeout mechanisms solid |
| `pkg/metrics` | 94.7% | ✅ Excellent | Metrics collection comprehensive |
| `pkg/protocol` | 93.7% | ✅ Excellent | Protocol implementation strong |
| `pkg/logging` | 92.9% | ✅ Excellent | Logging infrastructure solid |
| `pkg/resilience` | 91.5% | ✅ Excellent | Resilience patterns well-tested |
| `pkg/profiling` | 91.3% | ✅ Excellent | Profiling tools covered |
| `pkg/health` | 91.4% | ✅ Excellent | Health checks comprehensive |
| `pkg/tenancy` | 90.8% | ✅ Excellent | Multi-tenancy well-tested |
| `pkg/security` | 88.9% | ✅ Good | Security features solid |
| `pkg/replication` | 86.8% | ✅ Good | Replication logic strong |
| `pkg/logger` | 86.1% | ✅ Good | Logger implementation solid |
| `pkg/tracing` | 83.0% | ✅ Good | Tracing infrastructure covered |
| `pkg/storage` | 81.5% | ✅ Good | Storage layer tested |
| `pkg/cluster` | 80.5% | ✅ Good | Cluster coordination tested |
| `pkg/schema` | 78.6% | ⚠️ Needs improvement | Schema registry needs more tests |
| `pkg/consumer/group` | 77.2% | ⚠️ Needs improvement | Consumer groups need coverage |
| `pkg/transaction` | 76.5% | ⚠️ Needs improvement | Transactions need more tests |
| `pkg/server` | 72.7% | ⚠️ Needs improvement | Server handlers need coverage |
| `pkg/metadata` | 71.0% | ⚠️ Needs improvement | Metadata management needs tests |
| `pkg/replication/link` | 65.5% | ⚠️ Needs improvement | Replication links need coverage |
| `pkg/consensus` | 64.0% | ⚠️ Needs improvement | Consensus algorithms need tests |

### 🔧 Recent Fixes

1. **Fixed**: `TestBrokerHealthChecker_CheckAllBrokers` - Updated to handle broker status lifecycle correctly
2. **Fixed**: `TestClusterCoordinator_ConcurrentRebalance` - Made test more robust for timing-dependent behavior

### 📊 Test Distribution

- **Unit Tests**: 550+ tests (fast, isolated)
- **Integration Tests**: Available but skipped in short mode (require running broker)
- **Chaos Tests**: Available but skipped in short mode (resilience testing)
- **Benchmarks**: Available for performance regression detection

## Testing Strategy

### 1. Unit Tests (Current Focus)

**Goal**: Achieve 85%+ coverage on all packages

**Approach**:
- Fast, isolated tests with no external dependencies
- Use mocks for interfaces
- Table-driven tests for comprehensive coverage
- Run with `-short` flag for quick feedback

**Command**: `make test-unit`

### 2. Integration Tests

**Goal**: Validate end-to-end workflows

**Prerequisites**:
- StreamBus broker running on localhost:9092
- **CRITICAL**: No Kafka dependencies (StreamBus is a drop-in replacement)

**Test Scenarios**:
- Producer/Consumer lifecycle
- Multi-partition handling
- Large message handling (1KB to 1MB)
- High throughput (10,000+ messages)
- Message ordering guarantees

**Command**: `make test-integration`

### 3. Chaos Tests

**Goal**: Verify system resilience under fault conditions

**Fault Types**:
- Random latency injection (30% probability)
- Intermittent errors (20% probability)
- Slow network responses (50% probability)
- Connection drops (10% probability)
- Data loss simulation (5% probability)
- Network partitions (manual)

**Command**: `make test-chaos`

### 4. Performance Benchmarks

**Goal**: Detect performance regressions

**Benchmarks**:
- Message encoding/decoding
- Storage layer operations
- Protocol handling
- Client operations

**Command**: `make benchmark`

## Testing Plan - Phase 1: Unit Test Coverage

### Priority 1: Critical Path Coverage (Target: 95%+)

These packages are critical to StreamBus functionality and need the highest coverage:

#### 1.1 Consensus Package (Current: 64.0%)
**Gap**: 31% to target  
**Priority**: HIGH - Critical for cluster coordination

**Action Items**:
- [ ] Add tests for leader election scenarios
- [ ] Test follower synchronization
- [ ] Test log replication edge cases
- [ ] Test network partition recovery
- [ ] Test concurrent vote handling

**Estimated Effort**: 2-3 days

#### 1.2 Replication Link Package (Current: 65.5%)
**Gap**: 29.5% to target  
**Priority**: HIGH - Critical for data durability

**Action Items**:
- [ ] Test link establishment and teardown
- [ ] Test data synchronization
- [ ] Test failure recovery
- [ ] Test backpressure handling
- [ ] Test concurrent replication

**Estimated Effort**: 2-3 days

### Priority 2: Core Functionality (Target: 85%+)

#### 2.1 Metadata Package (Current: 71.0%)
**Gap**: 14% to target  
**Priority**: MEDIUM

**Action Items**:
- [ ] Test metadata CRUD operations
- [ ] Test concurrent access patterns
- [ ] Test metadata persistence
- [ ] Test cache invalidation
- [ ] Test cluster metadata sync

**Estimated Effort**: 1-2 days

#### 2.2 Server Package (Current: 72.7%)
**Gap**: 12.3% to target  
**Priority**: MEDIUM

**Action Items**:
- [ ] Test request handlers
- [ ] Test connection management
- [ ] Test protocol negotiation
- [ ] Test error handling
- [ ] Test graceful shutdown

**Estimated Effort**: 2 days

#### 2.3 Transaction Package (Current: 76.5%)
**Gap**: 8.5% to target  
**Priority**: MEDIUM

**Action Items**:
- [ ] Test transaction lifecycle
- [ ] Test commit/abort scenarios
- [ ] Test concurrent transactions
- [ ] Test transaction timeout
- [ ] Test recovery after crash

**Estimated Effort**: 1-2 days

#### 2.4 Consumer Group Package (Current: 77.2%)
**Gap**: 7.8% to target  
**Priority**: MEDIUM

**Action Items**:
- [ ] Test group coordination
- [ ] Test partition assignment
- [ ] Test rebalancing
- [ ] Test offset management
- [ ] Test member failure handling

**Estimated Effort**: 1-2 days

#### 2.5 Schema Package (Current: 78.6%)
**Gap**: 6.4% to target  
**Priority**: MEDIUM

**Action Items**:
- [ ] Test schema registration
- [ ] Test schema evolution
- [ ] Test compatibility checks
- [ ] Test schema versioning
- [ ] Test schema deletion

**Estimated Effort**: 1 day

### Priority 3: Maintain Excellence (Target: 90%+)

These packages already have good coverage but can be improved:

#### 3.1 Cluster Package (Current: 80.5%)
**Action Items**:
- [ ] Add more edge case tests for broker lifecycle
- [ ] Test coordinator failover scenarios
- [ ] Test assignment strategy edge cases

**Estimated Effort**: 1 day

#### 3.2 Storage Package (Current: 81.5%)
**Action Items**:
- [ ] Test storage corruption recovery
- [ ] Test concurrent read/write patterns
- [ ] Test compaction edge cases

**Estimated Effort**: 1 day

## Testing Plan - Phase 2: Integration Testing

### Prerequisites Setup

1. **Start StreamBus Broker**:
   ```bash
   make run-broker
   # OR
   make run-cluster  # For 3-node cluster
   ```

2. **Verify Broker is Running**:
   ```bash
   lsof -i :9092
   ```

### Integration Test Execution

1. **Run All Integration Tests**:
   ```bash
   make test-integration
   ```

2. **Run Specific Integration Test**:
   ```bash
   go test -v -run TestE2E_ProducerConsumerLifecycle ./tests/integration/
   ```

### Integration Test Coverage Goals

- [ ] Producer/Consumer basic lifecycle
- [ ] Multi-partition message distribution
- [ ] Large message handling (up to 1MB)
- [ ] High throughput scenarios (10K+ msg/s)
- [ ] Message ordering guarantees
- [ ] Offset management
- [ ] Consumer group rebalancing
- [ ] Transaction support
- [ ] Schema registry integration

## Testing Plan - Phase 3: Chaos Engineering

### Chaos Test Scenarios

1. **Random Latency** (30s duration):
   - Inject random delays in operations
   - Verify timeout handling
   - Ensure graceful degradation

2. **Intermittent Errors** (30s duration):
   - Simulate random failures
   - Verify retry logic
   - Test error recovery

3. **Slow Network** (60s duration):
   - Simulate degraded network
   - Test backpressure handling
   - Verify system stability

4. **Combined Chaos** (120s duration):
   - Multiple fault types simultaneously
   - Stress test resilience
   - Verify recovery mechanisms

5. **Network Partition** (manual):
   - Simulate split-brain scenarios
   - Test partition detection
   - Verify cluster recovery

### Chaos Test Execution

```bash
# Run all chaos tests (requires broker)
make test-chaos

# Run specific chaos test
go test -v -timeout 30m -run TestChaos_RandomLatency ./tests/chaos/
```

## Testing Plan - Phase 4: Performance Benchmarking

### Benchmark Execution

1. **Run All Benchmarks**:
   ```bash
   make benchmark
   ```

2. **Run Specific Layer Benchmarks**:
   ```bash
   make benchmark-storage
   make benchmark-protocol
   make benchmark-client
   make benchmark-server
   ```

3. **Generate Benchmark Report**:
   ```bash
   make benchmark-report
   ```

4. **Compare with Baseline**:
   ```bash
   # Set baseline
   make benchmark-baseline
   
   # Run and compare
   make benchmark-compare
   ```

### Performance Targets

- **Message Encoding**: < 1µs per message
- **Storage Write**: < 100µs per message
- **Storage Read**: < 50µs per message
- **Protocol Handling**: < 10µs per request
- **End-to-End Latency**: < 5ms (p99)
- **Throughput**: > 100K msg/s (single partition)

## CI/CD Integration

### GitHub Actions Workflow

The CI pipeline runs on every push and PR:

1. **Unit Tests** (Go 1.21, 1.22 matrix)
2. **Integration Tests** (requires broker)
3. **Chaos Tests** (main branch only)
4. **Linting** (golangci-lint)
5. **Benchmarks** (performance tracking)
6. **Security Scan** (Gosec + CodeQL)
7. **Build** (multi-platform)
8. **Test Summary** (aggregate results)

### Coverage Requirements

- **PR Requirement**: No decrease in coverage
- **Main Branch**: Maintain 85%+ coverage
- **Critical Paths**: Maintain 95%+ coverage

## Tools and Scripts

### Coverage Analysis

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

### Make Targets

```bash
# Quick test commands
make test-unit          # Fast unit tests only
make test-integration   # Integration tests (requires broker)
make test-chaos         # Chaos tests (requires broker)
make test-all           # All test types
make test-coverage      # Tests with HTML coverage report
make coverage-analysis  # Comprehensive coverage analysis

# Benchmark commands
make benchmark          # All benchmarks
make benchmark-full     # Comprehensive benchmark suite
make benchmark-report   # Generate formatted report
make benchmark-compare  # Compare with baseline

# Utility commands
make check-kafka-deps   # Verify no Kafka dependencies
make ci                 # Run CI checks locally
make ci-full            # Run full CI suite
```

## Best Practices

### Writing Tests

1. **Use Table-Driven Tests**:
   ```go
   tests := []struct {
       name    string
       input   string
       want    string
       wantErr bool
   }{
       {"valid", "input", "output", false},
       {"invalid", "", "", true},
   }
   
   for _, tt := range tests {
       t.Run(tt.name, func(t *testing.T) {
           // Test logic
       })
   }
   ```

2. **Test Isolation**:
   - Each test should be independent
   - Use unique resource names
   - Clean up after tests
   - Don't rely on execution order

3. **Error Messages**:
   ```go
   // Good: Descriptive error with context
   t.Errorf("Expected status %d, got %d after sending %d messages",
       expectedStatus, gotStatus, messageCount)
   ```

4. **Skip Integration Tests in Short Mode**:
   ```go
   if testing.Short() {
       t.Skip("Skipping integration test in short mode")
   }
   ```

### Code Review Checklist

- [ ] Tests pass locally
- [ ] Coverage meets threshold (85%+)
- [ ] No flaky tests
- [ ] Error cases tested
- [ ] Documentation updated
- [ ] Benchmarks run (if performance-critical)
- [ ] No Kafka dependencies introduced

## Timeline and Milestones

### Week 1-2: Priority 1 (Critical Paths)
- [ ] Consensus package to 95%+
- [ ] Replication link package to 95%+
- **Deliverable**: Critical path coverage at 95%+

### Week 3-4: Priority 2 (Core Functionality)
- [ ] Metadata package to 85%+
- [ ] Server package to 85%+
- [ ] Transaction package to 85%+
- [ ] Consumer group package to 85%+
- [ ] Schema package to 85%+
- **Deliverable**: All core packages at 85%+

### Week 5: Priority 3 (Excellence)
- [ ] Cluster package to 90%+
- [ ] Storage package to 90%+
- **Deliverable**: High-coverage packages at 90%+

### Week 6: Integration & Chaos
- [ ] Run full integration test suite
- [ ] Run all chaos test scenarios
- [ ] Document any issues found
- **Deliverable**: Integration and chaos tests passing

### Week 7: Performance & Documentation
- [ ] Run comprehensive benchmarks
- [ ] Establish performance baselines
- [ ] Update documentation
- **Deliverable**: Performance benchmarks established

### Week 8: CI/CD & Polish
- [ ] Verify CI/CD pipeline
- [ ] Address any remaining issues
- [ ] Final coverage report
- **Deliverable**: Production-ready test suite

## Success Criteria

### Phase 1 Success (Unit Tests)
- ✅ All unit tests passing
- ✅ Overall coverage ≥ 85%
- ✅ Critical path coverage ≥ 95%
- ✅ No package below 70%

### Phase 2 Success (Integration)
- ✅ All integration tests passing
- ✅ End-to-end workflows validated
- ✅ Multi-node cluster tested
- ✅ No Kafka dependencies

### Phase 3 Success (Chaos)
- ✅ All chaos scenarios passing
- ✅ Resilience validated
- ✅ Recovery mechanisms tested
- ✅ Fault injection working

### Phase 4 Success (Performance)
- ✅ Benchmarks established
- ✅ Performance targets met
- ✅ No regressions detected
- ✅ Baseline documented

## Resources

### Documentation
- [Testing Strategy](docs/TESTING.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Contributing Guidelines](CONTRIBUTING.md)

### External Resources
- [Go Testing Documentation](https://golang.org/pkg/testing/)
- [Table Driven Tests](https://github.com/golang/go/wiki/TableDrivenTests)
- [Chaos Engineering Principles](https://principlesofchaos.org/)
- [Code Coverage Best Practices](https://testing.googleblog.com/2020/08/code-coverage-best-practices.html)

## Next Steps

1. **Immediate** (Today):
   - ✅ Fix failing unit tests
   - ✅ Document current status
   - [ ] Start Priority 1 tests (consensus package)

2. **This Week**:
   - [ ] Complete consensus package tests
   - [ ] Complete replication link tests
   - [ ] Review and merge PRs

3. **Next Week**:
   - [ ] Start Priority 2 tests
   - [ ] Run integration tests
   - [ ] Update CI/CD pipeline

---

**Last Updated**: November 22, 2025  
**Next Review**: November 29, 2025
