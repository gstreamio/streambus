# Test Coverage Improvement Plan

## Executive Summary

This document outlines the comprehensive plan for improving test coverage across the StreamBus codebase. The goal is to achieve 80%+ coverage across all core packages while maintaining high-quality, maintainable tests.

**Current Overall Coverage: 75.5%**

---

## Current Coverage Status (As of 2025-11-13)

### High Coverage Packages (>90%)
✅ **Excellent Coverage - Maintain Quality**

| Package | Coverage | Status |
|---------|----------|--------|
| pkg/errors | 97.5% | ✅ Excellent |
| pkg/timeout | 96.7% | ✅ Excellent |
| pkg/metrics | 94.7% | ✅ Excellent |
| pkg/protocol | 93.8% | ✅ Excellent |
| pkg/logging | 92.9% | ✅ Excellent |
| pkg/resilience | 91.5% | ✅ Excellent |
| pkg/health | 91.4% | ✅ Excellent |
| pkg/profiling | 91.3% | ✅ Excellent |
| pkg/tenancy | 90.8% | ✅ Excellent |

### Good Coverage Packages (70-90%)
✅ **Good Coverage - Minor Improvements Needed**

| Package | Coverage | Priority | Notes |
|---------|----------|----------|-------|
| pkg/consensus | 74.4% | Medium | Add snapshot/compact tests |
| pkg/consumer/group | 77.2% | Low | Focus on edge cases |
| pkg/logger | 86.1% | Low | Nearly complete |
| pkg/metadata | 76.7% | Medium | FSM edge cases |
| pkg/replication | 82.2% | Low | Good baseline |
| pkg/schema | 78.6% | Low | Validation tests |
| pkg/storage | 81.5% | Low | Good coverage |
| pkg/transaction | 76.5% | Medium | Transaction edge cases |
| pkg/tracing | 70.7% | Medium | Improve spans/context |
| pkg/broker | 70.8% | Medium | Handler error paths |
| pkg/cluster | 70.8% | Medium | Assignment algorithms |
| pkg/server | 72.3% | Medium | Fixed recent bug |

### Needs Improvement (<70%)
⚠️ **Priority Focus Areas**

| Package | Coverage | Priority | Gap Analysis |
|---------|----------|----------|--------------|
| **pkg/client** | 62.4% | 🔴 **High** | Missing partition consumer & pool tests |
| **pkg/security** | 69.0% | 🟡 **High** | Security should be near-perfect |
| **pkg/replication/link** | 52.5% | 🔴 **High** | Stream handler at 0% |

### No Coverage (0%)
🚫 **Not Priority - CLI/Examples**

- All cmd/ packages (CLI tools - integration tested manually)
- All examples/ directories (demonstration code)
- deploy/kubernetes/operator (tested via Kubernetes)

---

## Priority 1: Critical Gaps (Immediate Action)

### 1.1 pkg/replication/link (52.5% → 75%+)

**Current Issues:**
- stream.go: Multiple functions at 0% coverage
  - `Start()` - 20.7%
  - `Stop()` - 0.0%
  - `replicateBatch()` - 0.0%
  - `healthCheckLoop()` - 0.0%
  - `metricsUpdateLoop()` - 0.0%
  - `getTopicsToReplicate()` - 0.0%
  - `startTopicReplication()` - 0.0%

- manager.go: Functions under 70%
  - `StartLink()` - 32.1%
  - `StopLink()` - 33.3%
  - `PauseLink()` - 46.2%
  - `ResumeLink()` - 46.2%
  - `Failover()` - 48.9%
  - `Failback()` - 53.1%
  - `sendFailoverNotification()` - 0.0%
  - `checkAutomaticFailover()` - 32.0%

**Action Items:**
1. **Create Mock Broker Infrastructure**
   - Implement `MockBroker` for testing replication
   - Support configurable responses and failures
   - Simulate network partitions and delays

2. **Add Stream Handler Tests**
   ```go
   // Tests to add:
   - TestStreamHandler_Start
   - TestStreamHandler_Stop
   - TestStreamHandler_Lifecycle
   - TestStreamHandler_ReplicationFlow
   - TestStreamHandler_ErrorRecovery
   - TestStreamHandler_HealthChecks
   - TestStreamHandler_Metrics
   ```

3. **Add Manager Integration Tests**
   ```go
   // Tests to add:
   - TestManager_LinkLifecycle
   - TestManager_AutomaticFailover
   - TestManager_AutomaticFailback
   - TestManager_FailoverNotifications
   - TestManager_HealthMonitoring
   ```

**Estimated Effort:** 3-5 days

---

### 1.2 pkg/client (62.4% → 75%+)

**Current Issues:**
- partition_consumer.go: All methods at 0%
  - `FetchAll()` - 0.0%
  - `FetchRoundRobin()` - 0.0%
  - `SeekPartition()` - 0.0%
  - `SeekAll()` - 0.0%
  - `GetOffsets()` - 0.0%
  - `GetPartitionInfo()` - 0.0%
  - `PollPartitions()` - 0.0%
  - `Close()` - 0.0%
  - `Metrics()` - 0.0%

- pool.go: Critical functions uncovered
  - `markUsed()` - 0.0%
  - `isHealthy()` - 0.0%
  - `Remove()` - 0.0%
  - `cleanup()` - 0.0%
  - `buildTLSConfig()` - 0.0%

**Action Items:**
1. **Create partition_consumer_test.go**
   ```go
   // Tests to add:
   - TestPartitionConsumer_FetchAll
   - TestPartitionConsumer_FetchRoundRobin
   - TestPartitionConsumer_Seek
   - TestPartitionConsumer_Offsets
   - TestPartitionConsumer_PartitionInfo
   - TestPartitionConsumer_Poll
   - TestPartitionConsumer_Lifecycle
   ```

2. **Create pool_test.go**
   ```go
   // Tests to add:
   - TestConnection_Lifecycle
   - TestConnection_HealthCheck
   - TestConnectionPool_GetPut
   - TestConnectionPool_Cleanup
   - TestConnectionPool_Remove
   - TestConnectionPool_TLSConfig
   - TestConnectionPool_Concurrency
   ```

3. **Enhance Integration Tests**
   - Add more end-to-end consumer scenarios
   - Test connection pooling under load
   - Test failover and recovery

**Estimated Effort:** 2-3 days

---

### 1.3 pkg/security (69.0% → 85%+)

**Current Issues:**
Security is a critical component and should have near-perfect coverage.

**Action Items:**
1. **Audit Current Tests**
   - Review existing security tests
   - Identify uncovered code paths

2. **Add Missing Tests**
   ```go
   // Priority tests:
   - TestACL_ComplexPermissions
   - TestACL_EdgeCases
   - TestEncryption_AllAlgorithms
   - TestTLS_ConfigVariations
   - TestSASL_AllMechanisms
   - TestAudit_AllEvents
   ```

3. **Security Vulnerability Testing**
   - Test against OWASP Top 10
   - SQL injection attempts
   - XSS attempts
   - Command injection
   - Path traversal
   - Authentication bypass attempts

**Estimated Effort:** 2-3 days

---

## Priority 2: Improve Good Packages (70-85%)

### 2.1 Packages to Target

Focus on bringing these packages from 70-80% to 85%+:

1. **pkg/server (72.3% → 85%+)**
   - Fixed recent bug in CreateTopic
   - Add more handler error path tests
   - Test concurrent operations
   - Test resource limits

2. **pkg/broker (70.8% → 85%+)**
   - Test admin API edge cases
   - Test tenancy integration
   - Test concurrent broker operations

3. **pkg/cluster (70.8% → 85%+)**
   - Test all assignment strategies
   - Test rebalancing scenarios
   - Test sticky assignment edge cases

4. **pkg/tracing (70.7% → 85%+)**
   - Test span lifecycle
   - Test context propagation
   - Test sampling strategies

**Estimated Effort:** 3-4 days total

---

## Priority 3: Edge Cases and Error Paths

### 3.1 Focus Areas

Even high-coverage packages need edge case testing:

1. **Error Recovery**
   - Test recovery from all error conditions
   - Test partial failure scenarios
   - Test cascading failures

2. **Concurrent Operations**
   - Race condition testing
   - Deadlock prevention testing
   - Thread-safety verification

3. **Resource Limits**
   - Test memory limits
   - Test connection limits
   - Test disk space limits

4. **Network Failures**
   - Test timeouts
   - Test connection drops
   - Test slow networks

**Estimated Effort:** 2-3 days

---

## Priority 4: Testing Infrastructure

### 4.1 Mock Infrastructure

**Required Components:**

1. **MockBroker**
   ```go
   type MockBroker interface {
       Start() error
       Stop() error
       HandleProduce(req *ProduceRequest) *ProduceResponse
       HandleFetch(req *FetchRequest) *FetchResponse
       InjectFailure(failureType FailureType)
       GetMetrics() *BrokerMetrics
   }
   ```

2. **MockClient**
   ```go
   type MockClient interface {
       Send(req *Request) (*Response, error)
       SetBehavior(behavior ClientBehavior)
       InjectLatency(duration time.Duration)
   }
   ```

3. **Test Helpers**
   ```go
   // Helper functions:
   - CreateTestCluster(numBrokers int) *TestCluster
   - CreateTestTopic(name string, partitions int) error
   - ProduceTestMessages(topic string, count int) error
   - ConsumeTestMessages(topic string, count int) ([]*Message, error)
   ```

### 4.2 Test Containers (Optional)

Consider using testcontainers-go for integration tests:

```go
// Example:
func setupIntegrationTest(t *testing.T) *StreamBusContainer {
    ctx := context.Background()
    req := testcontainers.ContainerRequest{
        Image:        "streambus:latest",
        ExposedPorts: []string{"9092/tcp"},
        WaitingFor:   wait.ForLog("Server started"),
    }
    container, err := testcontainers.GenericContainer(ctx,
        testcontainers.GenericContainerRequest{
            ContainerRequest: req,
            Started:          true,
        })
    // ...
}
```

**Estimated Effort:** 2-3 days

---

## Priority 5: Advanced Testing

### 5.1 Property-Based Testing

Use property-based testing for protocol and storage layers:

```go
// Example with gopter:
import "github.com/leanovate/gopter"

func TestProtocol_Properties(t *testing.T) {
    properties := gopter.NewProperties(nil)

    properties.Property("encode/decode roundtrip",
        prop.ForAll(
            func(msg *Message) bool {
                encoded := Encode(msg)
                decoded, err := Decode(encoded)
                return err == nil && reflect.DeepEqual(msg, decoded)
            },
            genMessage(),
        ))

    properties.TestingRun(t)
}
```

**Areas to Apply:**
- Protocol encoding/decoding
- Offset management
- Storage compaction
- Rebalancing algorithms

**Estimated Effort:** 2-3 days

---

### 5.2 Chaos/Fault Injection Tests

Implement systematic fault injection:

```go
// Chaos test framework:
type ChaosScenario struct {
    Name        string
    Setup       func(*TestCluster)
    InjectFault func(*TestCluster)
    Verify      func(*TestCluster) error
}

scenarios := []ChaosScenario{
    {
        Name: "broker-crash-during-write",
        Setup: setupThreeBrokerCluster,
        InjectFault: killRandomBroker,
        Verify: verifyNoDataLoss,
    },
    {
        Name: "network-partition",
        Setup: setupThreeBrokerCluster,
        InjectFault: partitionNetwork,
        Verify: verifyEventualConsistency,
    },
}
```

**Estimated Effort:** 3-4 days

---

### 5.3 Performance/Load Tests

Add performance regression tests:

```go
func BenchmarkProducer_Throughput(b *testing.B) {
    producer := setupProducer(b)
    msg := createTestMessage()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        producer.Send(msg)
    }
    b.StopTimer()

    // Report metrics
    b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "msgs/sec")
}
```

**Estimated Effort:** 2 days

---

## Implementation Timeline

### Phase 1: Critical Gaps (2 weeks)
- Week 1: pkg/replication/link + mock infrastructure
- Week 2: pkg/client + pkg/security

**Target:** Bring all packages to 70%+ coverage

### Phase 2: Good → Great (1 week)
- Bring 70-80% packages to 85%+
- Focus on error paths and edge cases

**Target:** 85%+ coverage on core packages

### Phase 3: Advanced Testing (2 weeks)
- Week 1: Property-based testing + chaos tests
- Week 2: Performance tests + integration test suite

**Target:** Comprehensive test suite with multiple test types

### Phase 4: Maintenance (Ongoing)
- Maintain 85%+ coverage on new code
- Regular test review and cleanup
- Performance regression monitoring

---

## Testing Best Practices

### 1. Test Organization

```
pkg/
  └── component/
      ├── component.go
      ├── component_test.go       # Unit tests
      ├── component_integration_test.go  # Integration tests
      └── component_bench_test.go # Benchmarks
```

### 2. Test Naming Conventions

```go
// Unit tests:
func TestComponent_Method_Scenario(t *testing.T)

// Table-driven tests:
func TestComponent_Method(t *testing.T) {
    tests := []struct {
        name     string
        input    Input
        expected Output
    }{ /* ... */ }
}

// Integration tests:
func TestIntegration_Scenario(t *testing.T)

// Benchmarks:
func BenchmarkComponent_Method(b *testing.B)
```

### 3. Test Quality Guidelines

- **Isolation:** Tests should not depend on each other
- **Repeatability:** Tests should produce same results every run
- **Speed:** Unit tests should run in < 100ms
- **Clarity:** Test names should describe what they test
- **Coverage:** Aim for edge cases, not just happy path

### 4. Code Review Checklist

- [ ] New code has tests
- [ ] Tests cover happy path
- [ ] Tests cover error paths
- [ ] Tests cover edge cases
- [ ] Tests are isolated
- [ ] Tests are readable
- [ ] No test flakiness
- [ ] Coverage didn't decrease

---

## Coverage Targets by Package Type

| Package Type | Target Coverage | Rationale |
|--------------|----------------|-----------|
| Core Logic | 85-95% | Business-critical code |
| Security | 90-100% | Zero tolerance for bugs |
| Protocol | 90-95% | Correctness is critical |
| Storage | 85-90% | Data integrity critical |
| Client Library | 80-90% | User-facing, high usage |
| Utilities | 75-85% | Supporting code |
| CLI Tools | 60-70% | Integration tested |
| Examples | 0-50% | Demonstration only |

---

## Tools and Infrastructure

### Required Tools

1. **Coverage Analysis**
   ```bash
   # Generate coverage report
   go test -coverprofile=coverage.out ./...
   go tool cover -html=coverage.out -o coverage.html

   # Coverage by function
   go tool cover -func=coverage.out
   ```

2. **Race Detection**
   ```bash
   go test -race ./...
   ```

3. **Memory Profiling**
   ```bash
   go test -memprofile=mem.prof -bench=.
   go tool pprof mem.prof
   ```

4. **Continuous Integration**
   - Run tests on every PR
   - Block merge if coverage decreases
   - Run race detector
   - Run integration tests nightly

### Recommended Libraries

- **Testing:** Standard `testing` package
- **Assertions:** `testify/assert`, `testify/require`
- **Mocking:** `testify/mock`, `gomock`
- **Property Testing:** `gopter`
- **Containers:** `testcontainers-go`
- **Benchmarking:** `benchstat`

---

## Metrics and Reporting

### Coverage Dashboards

Track these metrics over time:

1. **Overall Coverage Trend**
   - Line coverage %
   - Branch coverage %
   - Function coverage %

2. **Per-Package Coverage**
   - Packages below target
   - Packages improving
   - Packages regressing

3. **Test Quality Metrics**
   - Test execution time
   - Flaky test count
   - Test failure rate

4. **Code Quality Metrics**
   - Cyclomatic complexity
   - Code churn in tests
   - Test-to-code ratio

### Weekly Reports

Generate automated reports:

```bash
#!/bin/bash
# Generate coverage report
go test -coverprofile=coverage.out ./pkg/...

# Calculate coverage
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')

# Find packages below target
go tool cover -func=coverage.out | awk '$3 < 80.0 {print $1, $3}'

# Report results
echo "Overall Coverage: $COVERAGE"
```

---

## Success Criteria

### Short-term (1 month)
- [ ] All packages at 70%+ coverage
- [ ] pkg/client at 75%+ coverage
- [ ] pkg/security at 85%+ coverage
- [ ] pkg/replication/link at 70%+ coverage
- [ ] Mock infrastructure in place
- [ ] Zero failing tests

### Medium-term (3 months)
- [ ] Core packages at 85%+ coverage
- [ ] Integration test suite complete
- [ ] Property-based tests implemented
- [ ] Chaos tests running nightly
- [ ] Performance benchmarks tracked
- [ ] CI/CD enforcing coverage

### Long-term (6 months)
- [ ] Overall coverage at 85%+
- [ ] All security code at 95%+
- [ ] Comprehensive test documentation
- [ ] Test maintenance < 10% of dev time
- [ ] Zero flaky tests
- [ ] Full test automation

---

## Recent Improvements (2025-11-13)

### Fixed Issues

1. **pkg/server/topics.go:136** - CreateTopic Idempotency Bug
   - **Issue:** Function returned nil instead of error for duplicate topics
   - **Fix:** Made function idempotent - returns nil if partition count matches, error if different
   - **Test:** Enhanced TestTopicManager_CreateTopic_DuplicateError
   - **Result:** All pkg/server tests now pass

### Coverage Analysis Completed

- Analyzed pkg/replication/link (52.5%)
- Analyzed pkg/client (62.4%)
- Identified specific functions needing tests
- Documented requirements for mock infrastructure

---

## Appendix A: SonarQube Cognitive Complexity

To avoid rewriting methods and maintain clean code principles, we follow SonarQube's cognitive complexity calculation:

### Cognitive Complexity Rules

Cognitive Complexity is calculated by adding 1 for each of:

1. **Flow-breaking structures:**
   - `if`, `else if`, `else`
   - `switch`, `case`
   - `for`, `while`, `do while`
   - `goto`
   - Ternary operators (`? :`)

2. **Nesting penalty:**
   - Each level of nesting adds +1
   - Applies to: if, loops, catch blocks, switches, lambdas

3. **Special cases:**
   - Binary logical operators in different context: +1
   - Recursion: +1 per recursive call

### Complexity Thresholds

| Complexity | Status | Action |
|------------|--------|--------|
| 0-10 | ✅ Good | None needed |
| 11-15 | ⚠️ Warning | Consider refactoring |
| 16-25 | 🔴 Critical | Must refactor |
| 25+ | 🚫 Unacceptable | Block merge |

### Example Calculation

```go
func processOrder(order Order) error {  // Base: 0
    if order.IsValid() {                 // +1 (if)
        for _, item := range order.Items {  // +2 (for + nesting)
            if item.InStock() {              // +3 (if + nesting level 2)
                if item.Price > 100 {        // +4 (if + nesting level 3)
                    applyDiscount(item)
                }
            } else {                         // +3 (else + nesting level 2)
                return errors.New("out of stock")
            }
        }
    }
    return nil
}
// Total Complexity: 13 (Warning level)
```

### Refactoring Strategy

When complexity exceeds threshold:

1. **Extract Methods**
   ```go
   func processOrder(order Order) error {
       if !order.IsValid() {
           return errors.New("invalid order")
       }
       return processItems(order.Items)  // Extracted
   }

   func processItems(items []Item) error {
       for _, item := range items {
           if err := processItem(item); err != nil {  // Extracted
               return err
           }
       }
       return nil
   }
   ```

2. **Early Returns**
   ```go
   // Bad: Nested ifs
   if condition1 {
       if condition2 {
           if condition3 {
               // do work
           }
       }
   }

   // Good: Early returns
   if !condition1 {
       return
   }
   if !condition2 {
       return
   }
   if !condition3 {
       return
   }
   // do work
   ```

3. **Strategy Pattern**
   ```go
   // Replace complex switch/if chains with strategy map
   handlers := map[OrderType]Handler{
       StandardOrder: standardHandler,
       ExpressOrder:  expressHandler,
       BulkOrder:     bulkHandler,
   }
   handler := handlers[order.Type]
   return handler.Process(order)
   ```

---

## Appendix B: Git Flow & Branch Naming

Following the project's git flow process:

### Branch Naming Convention

```
<jira-ticket>-<short-description>
```

Examples:
- `STREAM-123-add-consumer-tests`
- `STREAM-456-fix-replication-bug`
- `STREAM-789-improve-coverage`

### Workflow

1. **Never commit to dev or main directly**
2. **Create feature branch from dev**
   ```bash
   git checkout dev
   git pull origin dev
   git checkout -b STREAM-123-add-tests
   ```

3. **Make changes and commit**
   ```bash
   git add .
   git commit -m "STREAM-123: Add consumer unit tests"
   ```

4. **Push and create PR**
   ```bash
   git push origin STREAM-123-add-tests
   # Create PR: STREAM-123-add-tests → dev
   ```

5. **After approval, merge to dev**
6. **Periodically, dev → main** (release process)

---

## Contact & Questions

For questions about this plan:
- Create a Jira ticket with tag `testing`
- Review with tech lead before starting major test work
- Update this document as testing strategy evolves

---

**Last Updated:** 2025-11-13
**Next Review:** 2025-12-13
