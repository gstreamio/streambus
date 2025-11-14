# Production Hardening Plan - Phase 2.6

**Date**: 2025-11-08
**Phase**: 2.6 - Production Hardening
**Status**: 🔄 In Progress

## Overview

This document outlines the production hardening strategy for StreamBus Phase 2. The goal is to enhance resilience, reliability, and observability to make the system production-ready.

## Current State Assessment

### ✅ Already Implemented

1. **Retry Logic** (`pkg/metadata/store.go:39-100`)
   - Exponential backoff (50ms → 2s)
   - Max 10 retries
   - Context-aware cancellation
   - Leader detection and waiting

2. **Graceful Shutdown**
   - `RaftNode.Stop()` - orderly shutdown with channel coordination
   - `ClusterCoordinator.Stop()` - waits for goroutines via WaitGroup
   - `HeartbeatService.Stop()` - stops ticker cleanly
   - `BrokerHealthChecker.Stop()` - stops health monitoring

3. **Basic Error Handling**
   - Context cancellation detection
   - Error wrapping with `fmt.Errorf`
   - Logging at key points

### ❌ Needs to be Added

1. **Circuit Breaker Pattern**
   - Prevent cascading failures
   - Automatic recovery detection
   - State tracking (Closed, Open, Half-Open)

2. **Health Check System**
   - Liveness checks (is process alive?)
   - Readiness checks (can serve traffic?)
   - Raft cluster health
   - Component health aggregation

3. **Enhanced Error Handling**
   - Error categories (Retriable, Fatal, Transient)
   - Structured error types
   - Better error propagation

4. **Timeout Management**
   - Configurable timeouts
   - Timeout tuning per operation type
   - Deadline propagation

5. **Observability**
   - Metrics (Prometheus-compatible)
   - Structured logging (JSON format)
   - Tracing support
   - Performance counters

## Implementation Plan

### Week 13: Core Resilience (This Week)

#### Task 1: Circuit Breaker Implementation
**Priority**: High
**Estimated Time**: 2 days

**Scope**:
- Create `pkg/resilience/circuitbreaker.go`
- Implement three-state circuit breaker (Closed, Open, Half-Open)
- Configurable failure threshold and timeout
- Success tracking for recovery
- Integrate with metadata Store operations

**Interface**:
```go
type CircuitBreaker interface {
    Call(ctx context.Context, fn func() error) error
    State() State
    Stats() Stats
}

type State int
const (
    StateClosed State = iota
    StateOpen
    StateHalfOpen
)
```

**Integration Points**:
- Wrap `Store.propose()` calls
- Wrap `ClusterAdapter` operations
- Wrap external service calls

**Success Criteria**:
- Circuit opens after N consecutive failures
- Circuit auto-recovers after timeout
- Half-open state allows test requests
- Metrics exported for circuit state changes

#### Task 2: Health Check System
**Priority**: High
**Estimated Time**: 1 day

**Scope**:
- Create `pkg/health/checker.go`
- Implement health check interface
- Add component-specific health checks
- Create HTTP handler for health endpoints

**Endpoints**:
- `/health/live` - Liveness probe (200 if process alive)
- `/health/ready` - Readiness probe (200 if can serve traffic)
- `/health/raft` - Raft cluster health
- `/health` - Aggregate health status (JSON)

**Health Checks**:
1. **Raft Health**
   - Is node running?
   - Can communicate with leader?
   - Is cluster achieving quorum?
   - Time since last successful commit

2. **Metadata Store Health**
   - Can propose operations?
   - FSM applying entries?
   - Storage accessible?

3. **Broker Registry Health**
   - How many active brokers?
   - Last heartbeat received?
   - Health check service running?

4. **Coordinator Health**
   - Running and not stuck?
   - Last successful rebalance?
   - Any ongoing rebalances?

**Success Criteria**:
- All endpoints return appropriate status codes
- JSON response includes component details
- Health checks don't impact performance
- Configurable check intervals

#### Task 3: Enhanced Error Handling
**Priority**: Medium
**Estimated Time**: 1 day

**Scope**:
- Create `pkg/errors/types.go` with error categories
- Implement error classification utilities
- Add error context enrichment
- Update components to use typed errors

**Error Categories**:
```go
type ErrorCategory int
const (
    CategoryRetriable ErrorCategory = iota  // Can retry
    CategoryTransient                       // Wait and retry
    CategoryFatal                          // Cannot recover
    CategoryInvalidInput                   // Client error
)

type Error struct {
    Category    ErrorCategory
    Op          string
    Err         error
    Metadata    map[string]interface{}
    Timestamp   time.Time
}
```

**Error Examples**:
- `Retriable`: Network timeout, leadership change
- `Transient`: Temporary resource exhaustion
- `Fatal`: Data corruption, invalid state
- `InvalidInput`: Bad request parameters

**Success Criteria**:
- All errors properly categorized
- Error context includes operation details
- Clients can make retry decisions
- Better debugging with error metadata

### Week 14: Observability & Tuning

#### Task 4: Metrics System
**Priority**: High
**Estimated Time**: 2 days

**Scope**:
- Create `pkg/metrics/registry.go`
- Implement Prometheus-compatible metrics
- Add metrics to all critical paths
- Create metrics HTTP handler

**Metrics to Track**:

**Consensus Metrics**:
- `raft_proposals_total` (counter)
- `raft_proposal_duration_seconds` (histogram)
- `raft_leadership_changes_total` (counter)
- `raft_commit_index` (gauge)
- `raft_applied_index` (gauge)
- `raft_peers_count` (gauge)

**Metadata Store Metrics**:
- `metadata_operations_total{operation,status}` (counter)
- `metadata_operation_duration_seconds{operation}` (histogram)
- `metadata_version` (gauge)
- `metadata_brokers_count` (gauge)
- `metadata_partitions_count` (gauge)

**Cluster Coordination Metrics**:
- `cluster_brokers_active` (gauge)
- `cluster_rebalances_total{status}` (counter)
- `cluster_rebalance_duration_seconds` (histogram)
- `cluster_partitions_total` (gauge)
- `cluster_imbalance_score` (gauge)

**Circuit Breaker Metrics**:
- `circuitbreaker_state{name}` (gauge: 0=closed, 1=open, 2=half-open)
- `circuitbreaker_requests_total{name,result}` (counter)
- `circuitbreaker_state_changes_total{name,from,to}` (counter)

**Success Criteria**:
- All metrics exportable at `/metrics`
- Metrics follow Prometheus naming conventions
- Low overhead (<1% performance impact)
- Dashboards can be built from metrics

#### Task 5: Structured Logging
**Priority**: Medium
**Estimated Time**: 1 day

**Scope**:
- Create `pkg/logging/logger.go`
- Implement structured logging interface
- Add contextual logging throughout
- Support JSON and console formats

**Log Levels**:
- DEBUG: Detailed debugging information
- INFO: General informational messages
- WARN: Warning messages (non-critical issues)
- ERROR: Error messages (critical issues)

**Structured Fields**:
- `component`: Which component logged
- `operation`: What operation was being performed
- `node_id`: Raft node ID
- `broker_id`: Broker ID (if applicable)
- `duration_ms`: Operation duration
- `error`: Error message (if any)

**Success Criteria**:
- All log statements use structured logger
- JSON format for production
- Console format for development
- Log levels configurable
- No performance degradation

#### Task 6: Timeout Management
**Priority**: Medium
**Estimated Time**: 1 day

**Scope**:
- Create `pkg/config/timeouts.go`
- Define timeout constants for all operations
- Make timeouts configurable
- Add timeout enforcement

**Timeouts to Configure**:
```go
type Timeouts struct {
    ProposalTimeout     time.Duration  // Default: 5s
    HealthCheckTimeout  time.Duration  // Default: 2s
    RebalanceTimeout    time.Duration  // Default: 30s
    HeartbeatInterval   time.Duration  // Default: 10s
    HeartbeatTimeout    time.Duration  // Default: 30s
    LeaderElectionTimeout time.Duration // Default: 10s
    SnapshotTimeout     time.Duration  // Default: 60s
}
```

**Success Criteria**:
- All operations have explicit timeouts
- Timeouts loaded from configuration
- Context deadlines properly set
- Timeout violations logged

## Testing Strategy

### Unit Tests
- Circuit breaker state transitions
- Health check logic
- Error categorization
- Metrics recording
- Timeout enforcement

### Integration Tests
- Circuit breaker with real Raft operations
- Health checks during failures
- Metrics accuracy under load
- Logging output validation
- Graceful shutdown with in-flight operations

### Chaos Tests
- Random node failures
- Network partitions
- Slow operations
- Resource exhaustion
- Leader election during operations

## Success Metrics

### Reliability
- [ ] Circuit breaker prevents cascading failures
- [ ] System recovers automatically from transient failures
- [ ] Graceful shutdown with zero data loss
- [ ] No goroutine leaks

### Observability
- [ ] All critical operations have metrics
- [ ] Logs provide actionable debugging information
- [ ] Health checks accurately reflect system state
- [ ] Dashboards show key system metrics

### Performance
- [ ] Overhead < 1% for metrics and logging
- [ ] Health checks respond in < 100ms
- [ ] Circuit breaker decision in < 1ms
- [ ] No performance regression from hardening

## Configuration Example

```yaml
# config.yaml
resilience:
  circuit_breaker:
    failure_threshold: 5
    success_threshold: 2
    timeout: 30s
    max_requests_half_open: 3

  timeouts:
    proposal: 5s
    health_check: 2s
    rebalance: 30s
    heartbeat_interval: 10s
    heartbeat_timeout: 30s

health:
  check_interval: 5s
  http:
    enabled: true
    port: 8080
    bind_address: "0.0.0.0"

metrics:
  enabled: true
  port: 9090
  path: "/metrics"

logging:
  level: "info"
  format: "json"  # or "console"
  output: "stdout"
```

## Deployment Considerations

### Kubernetes Deployment
```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: streambus
    image: streambus:latest
    ports:
    - containerPort: 8080  # Health checks
      name: health
    - containerPort: 9090  # Metrics
      name: metrics
    livenessProbe:
      httpGet:
        path: /health/live
        port: health
      initialDelaySeconds: 30
      periodSeconds: 10
      timeoutSeconds: 2
    readinessProbe:
      httpGet:
        path: /health/ready
        port: health
      initialDelaySeconds: 10
      periodSeconds: 5
      timeoutSeconds: 2
    resources:
      limits:
        memory: "1Gi"
        cpu: "1000m"
      requests:
        memory: "512Mi"
        cpu: "500m"
```

### Monitoring Setup
- Prometheus scrapes `/metrics` endpoint
- Grafana dashboards for visualization
- Alert manager for critical alerts
- Log aggregation (ELK/Loki)

## Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| Performance overhead from metrics | Medium | Benchmark and optimize hot paths |
| Circuit breaker too aggressive | High | Configurable thresholds, monitoring |
| Health checks give false positives | High | Multiple check types, grace periods |
| Logging overhead | Medium | Async logging, configurable levels |
| Shutdown timeout issues | Medium | Configurable shutdown timeout |

## Timeline

### Week 13 (This Week)
- Day 1-2: Circuit breaker implementation
- Day 3: Health check system
- Day 4: Enhanced error handling
- Day 5: Testing and documentation

### Week 14
- Day 1-2: Metrics system
- Day 3: Structured logging
- Day 4: Timeout management
- Day 5: Integration testing and tuning

## Next Steps

1. ✅ Create production hardening plan (this document)
2. ⏳ Implement circuit breaker pattern
3. ⏳ Add health check endpoints
4. ⏳ Enhance error handling
5. ⏳ Add metrics system
6. ⏳ Implement structured logging
7. ⏳ Configure timeouts
8. ⏳ Test all hardening features
9. ⏳ Document operational procedures

## References

- [Circuit Breaker Pattern](https://martinfowler.com/bliki/CircuitBreaker.html)
- [Health Check API Design](https://tools.ietf.org/id/draft-inadarei-api-health-check-06.html)
- [Prometheus Best Practices](https://prometheus.io/docs/practices/naming/)
- [Structured Logging Best Practices](https://www.thoughtworks.com/insights/blog/microservices/structured-logging-microservices)

---

**Last Updated**: 2025-11-08
**Status**: 🔄 In Progress
**Next Review**: 2025-11-15
