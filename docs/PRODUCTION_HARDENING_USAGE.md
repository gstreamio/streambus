# Production Hardening Usage Guide

This guide provides comprehensive documentation for all production hardening features implemented in StreamBus.

## Table of Contents

1. [Circuit Breaker](#circuit-breaker)
2. [Health Checks](#health-checks)
3. [Error Handling](#error-handling)
4. [Metrics](#metrics)
5. [Structured Logging](#structured-logging)
6. [Timeout Management](#timeout-management)
7. [Integration Examples](#integration-examples)

---

## Circuit Breaker

The circuit breaker pattern prevents cascading failures by failing fast when a service is unhealthy.

### Basic Usage

```go
import "github.com/shawntherrien/streambus/pkg/resilience"

// Create circuit breaker
cb := resilience.NewCircuitBreaker(resilience.DefaultConfig())

// Execute operation with circuit breaker protection
err := cb.Call(ctx, func() error {
    return performRaftOperation()
})

if err != nil {
    if err == resilience.ErrCircuitOpen {
        log.Error("Circuit breaker is open, failing fast")
    }
}
```

### Configuration

```go
config := &resilience.Config{
    FailureThreshold:    5,              // Open after 5 consecutive failures
    SuccessThreshold:    3,              // Close after 3 consecutive successes
    Timeout:             30 * time.Second, // Stay open for 30 seconds
    MaxHalfOpenRequests: 3,              // Allow 3 requests in half-open state
}

cb := resilience.NewCircuitBreaker(config)
```

### State Monitoring

```go
// Register state change callback
cb.OnStateChange(func(from, to resilience.State) {
    log.Infof("Circuit breaker state: %s -> %s", from, to)
    metrics.RecordCircuitBreakerState(to)
})

// Get current statistics
stats := cb.Stats()
fmt.Printf("State: %s, Failures: %d, Successes: %d\n",
    stats.State, stats.TotalFailures, stats.TotalSuccesses)
```

### Integration with Raft

```go
// Wrap Raft proposals with circuit breaker
type RaftNodeWithCircuitBreaker struct {
    node *RaftNode
    cb   *resilience.CircuitBreaker
}

func (r *RaftNodeWithCircuitBreaker) Propose(ctx context.Context, data []byte) error {
    return r.cb.Call(ctx, func() error {
        return r.node.Propose(ctx, data)
    })
}
```

---

## Health Checks

Health checks provide visibility into system health for orchestration platforms like Kubernetes.

### HTTP Endpoints

- **`/health`** - Detailed health status of all components (JSON)
- **`/health/live`** - Liveness probe (is the process alive?)
- **`/health/ready`** - Readiness probe (can accept traffic?)

### Setting Up Health Checks

```go
import "github.com/shawntherrien/streambus/pkg/health"

// Create registry
registry := health.NewRegistry()

// Register component checkers
raftChecker := health.NewRaftNodeHealthChecker("raft", raftNode)
registry.Register(raftChecker)

metadataChecker := health.NewMetadataStoreHealthChecker("metadata", metadataStore)
registry.Register(metadataChecker)

// Start HTTP server
server := health.NewServer(":8080", registry)
go server.Start()
```

### Custom Health Checkers

```go
// Simple checker
checker := health.NewSimpleChecker("database", func(ctx context.Context) health.Check {
    if err := db.Ping(ctx); err != nil {
        return health.Check{
            Status:  health.StatusUnhealthy,
            Message: fmt.Sprintf("database unreachable: %v", err),
        }
    }
    return health.Check{
        Status:  health.StatusHealthy,
        Message: "database connection OK",
    }
})

registry.Register(checker)
```

### Periodic Health Checks

```go
// Check every 10 seconds
periodicChecker := health.NewPeriodicChecker(checker, 10*time.Second)
periodicChecker.Start()
defer periodicChecker.Stop()

registry.Register(periodicChecker)
```

### Kubernetes Integration

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: streambus-broker
spec:
  containers:
  - name: broker
    image: streambus:latest
    livenessProbe:
      httpGet:
        path: /health/live
        port: 8080
      initialDelaySeconds: 30
      periodSeconds: 10
      timeoutSeconds: 2
    readinessProbe:
      httpGet:
        path: /health/ready
        port: 8080
      initialDelaySeconds: 10
      periodSeconds: 5
      timeoutSeconds: 3
```

---

## Error Handling

Structured error handling with categorization for better retry logic and debugging.

### Error Categories

- **Retriable** - Can retry immediately (e.g., not leader)
- **Transient** - Temporary failure, retry with delay (e.g., timeout)
- **Fatal** - Unrecoverable error (e.g., data corruption)
- **InvalidInput** - Client error (e.g., not found, already exists)
- **Unknown** - Unknown error category

### Creating Structured Errors

```go
import "github.com/shawntherrien/streambus/pkg/errors"

// Create categorized errors
err := errors.Retriable("raft.propose", ErrNotLeader).
    WithMessage("node is not the leader").
    WithMetadata("nodeID", nodeID).
    WithMetadata("leaderID", leaderID).
    WithCode("ERR_NOT_LEADER")

// Create transient error
err := errors.Transient("database.query", err).
    WithMessage("query timeout").
    WithMetadata("timeout", "5s")

// Create fatal error
err := errors.Fatal("storage.read", err).
    WithMessage("data corruption detected").
    WithCode("ERR_CORRUPTION")
```

### Error Wrapping

```go
// Wrap errors to add operation context
if err := performOperation(); err != nil {
    return errors.Wrap("metadata.update", err)
}

// Wrap with formatted message
if err := performOperation(); err != nil {
    return errors.Wrapf("metadata.update", err,
        "failed to update metadata version %d", version)
}
```

### Retry Logic Based on Categories

```go
func retryableOperation() error {
    maxRetries := 3
    for i := 0; i <= maxRetries; i++ {
        err := performOperation()
        if err == nil {
            return nil
        }

        // Check if retriable
        if errors.IsRetriable(err) {
            log.Warn("Retriable error, retrying immediately", err)
            continue
        }

        // Check if transient
        if errors.IsTransient(err) {
            log.Warn("Transient error, retrying with backoff", err)
            time.Sleep(time.Duration(1<<i) * time.Second)
            continue
        }

        // Fatal or invalid input - don't retry
        return err
    }
    return errors.New(errors.CategoryRetriable, "operation",
        fmt.Errorf("max retries exceeded"))
}
```

### Multi-Error Handling

```go
// Batch operations
multi := errors.NewMultiError("batch.process")

for _, item := range items {
    if err := processItem(item); err != nil {
        multi.Add(err)
    }
}

if err := multi.ErrorOrNil(); err != nil {
    return err
}
```

---

## Metrics

Prometheus-compatible metrics for monitoring and alerting.

### Metric Types

- **Counter** - Cumulative metric (only increases)
- **Gauge** - Value that can go up or down
- **Histogram** - Distribution of values

### Basic Usage

```go
import "github.com/shawntherrien/streambus/pkg/metrics"

// Create registry (or use default)
registry := metrics.NewRegistry()

// Counter
requestCounter := registry.GetOrCreateCounter(
    "http_requests_total",
    "Total HTTP requests",
    map[string]string{"method": "GET", "status": "200"},
)
requestCounter.Inc()

// Gauge
activeConnections := registry.GetOrCreateGauge(
    "active_connections",
    "Number of active connections",
    nil,
)
activeConnections.Set(42)
activeConnections.Inc()  // Increment
activeConnections.Dec()  // Decrement

// Histogram
requestDuration := registry.GetOrCreateHistogram(
    "http_request_duration_seconds",
    "HTTP request duration",
    map[string]string{"endpoint": "/api/v1"},
    []float64{0.01, 0.05, 0.1, 0.5, 1.0, 5.0},
)
start := time.Now()
// ... perform operation ...
requestDuration.ObserveDuration(start)
```

### Exposing Metrics

```go
// HTTP handler
handler := metrics.NewHandler(registry)
http.Handle("/metrics", handler)

// Or register on existing mux
mux := http.NewServeMux()
handler.RegisterRoutes(mux)
```

### Recommended Metrics

**Consensus Metrics:**
```go
proposalCounter := registry.GetOrCreateCounter(
    "raft_proposals_total",
    "Total Raft proposals",
    map[string]string{"status": "committed"},
)

commitIndex := registry.GetOrCreateGauge(
    "raft_commit_index",
    "Raft commit index",
    nil,
)

leadershipChanges := registry.GetOrCreateCounter(
    "raft_leadership_changes_total",
    "Total leadership changes",
    nil,
)
```

**Metadata Metrics:**
```go
metadataVersion := registry.GetOrCreateGauge(
    "metadata_version",
    "Current metadata version",
    nil,
)

brokerCount := registry.GetOrCreateGauge(
    "metadata_broker_count",
    "Number of registered brokers",
    nil,
)

partitionCount := registry.GetOrCreateGauge(
    "metadata_partition_count",
    "Number of partitions",
    nil,
)
```

**Circuit Breaker Metrics:**
```go
cbState := registry.GetOrCreateGauge(
    "circuit_breaker_state",
    "Circuit breaker state (0=closed, 1=open, 2=half-open)",
    map[string]string{"service": "raft"},
)

cbRequests := registry.GetOrCreateCounter(
    "circuit_breaker_requests_total",
    "Total circuit breaker requests",
    map[string]string{"service": "raft", "result": "success"},
)
```

---

## Structured Logging

JSON-formatted structured logging with contextual fields.

### Log Levels

- **DEBUG** - Detailed debugging information
- **INFO** - Informational messages
- **WARN** - Warning messages
- **ERROR** - Error messages
- **FATAL** - Fatal errors (exits process)

### Basic Usage

```go
import "github.com/shawntherrien/streambus/pkg/logging"

// Create logger
logger := logging.New(&logging.Config{
    Level:        logging.LevelInfo,
    Component:    "broker",
    IncludeFile:  true,
    IncludeTrace: false,
})

// Log messages
logger.Info("Server started", logging.Fields{
    "port": 9092,
    "host": "localhost",
})

logger.Error("Failed to process request", err, logging.Fields{
    "requestID": "abc123",
    "duration":  "5s",
})

// Log with operation context
logger.InfoOp("raft.propose", "Proposing new entry", logging.Fields{
    "entrySize": 1024,
})

logger.ErrorOp("metadata.sync", "Sync failed", err, logging.Fields{
    "retries": 3,
})
```

### Component-Specific Loggers

```go
// Create logger hierarchy
mainLogger := logging.New(&logging.Config{
    Level:     logging.LevelInfo,
    Component: "streambus",
})

raftLogger := mainLogger.WithComponent("raft")
metadataLogger := mainLogger.WithComponent("metadata")

raftLogger.Info("Raft node started")
metadataLogger.Info("Metadata store initialized")
```

### Contextual Fields

```go
// Create logger with default fields
requestLogger := logger.WithFields(logging.Fields{
    "requestID": "abc123",
    "userID":    "user456",
    "sessionID": "xyz789",
})

// All logs from this logger will include these fields
requestLogger.Info("Processing request")
requestLogger.Info("Request completed")
```

### Log Level Control

```go
// Set log level at runtime
logger.SetLevel(logging.LevelDebug)

// Parse level from string (e.g., from config)
level, err := logging.ParseLevel("WARN")
if err == nil {
    logger.SetLevel(level)
}
```

### Global Logger

```go
// Use global logger for convenience
logging.Info("Application started")
logging.Error("Operation failed", err)
logging.InfoOp("database.connect", "Connected to database", logging.Fields{
    "host": "localhost",
    "port": 5432,
})

// Configure global logger
customLogger := logging.New(&logging.Config{
    Level:     logging.LevelDebug,
    Component: "myapp",
})
logging.SetDefaultLogger(customLogger)
```

### JSON Output Format

```json
{
  "timestamp": "2025-01-08T10:30:00Z",
  "level": "INFO",
  "message": "Request completed",
  "component": "api",
  "operation": "http.request",
  "fields": {
    "method": "GET",
    "path": "/api/v1/users",
    "status": 200,
    "duration_ms": 45
  },
  "file": "handler.go",
  "line": 123
}
```

---

## Timeout Management

Centralized timeout configuration for consistent timeout handling.

### Configuration

```go
import "github.com/shawntherrien/streambus/pkg/timeout"

// Use default config
manager, err := timeout.NewManager(timeout.DefaultConfig())

// Use production config (longer timeouts)
manager, err := timeout.NewManager(timeout.ProductionConfig())

// Use test config (shorter timeouts)
manager, err := timeout.NewManager(timeout.TestConfig())

// Custom config
config := &timeout.Config{
    RaftProposal:      10 * time.Second,
    RaftElection:      15 * time.Second,
    HealthCheck:       2 * time.Second,
    GracefulShutdown:  30 * time.Second,
    // ... other timeouts
}
manager, err := timeout.NewManager(config)
```

### Creating Timeout Contexts

```go
// Raft proposal timeout
ctx, cancel := manager.WithRaftProposal(context.Background())
defer cancel()
err := raftNode.Propose(ctx, data)

// Health check timeout
ctx, cancel := manager.WithHealthCheck(context.Background())
defer cancel()
check := healthChecker.Check(ctx)

// Custom timeout
ctx, cancel := manager.WithCustom(context.Background(), 5*time.Second)
defer cancel()
```

### Operation Execution

```go
// Execute with timeout
op := timeout.NewOperation("database.query", 5*time.Second)

err := op.Execute(context.Background(), func(ctx context.Context) error {
    return database.Query(ctx, "SELECT * FROM users")
})

if err != nil {
    log.Error("Operation failed", err)
}
```

### Retry with Timeout

```go
// Execute with retry and exponential backoff
op := timeout.NewOperation("api.request", 3*time.Second)

err := op.ExecuteWithRetry(context.Background(), 3, func(ctx context.Context) error {
    return makeAPIRequest(ctx)
})
```

### Global Timeout Manager

```go
// Use global convenience functions
ctx, cancel := timeout.WithRaftProposal(context.Background())
defer cancel()

ctx, cancel := timeout.WithHealthCheck(context.Background())
defer cancel()

ctx, cancel := timeout.WithRequest(context.Background())
defer cancel()
```

### Runtime Configuration Updates

```go
// Update configuration at runtime
newConfig := &timeout.Config{
    RaftProposal: 15 * time.Second,
    // ... other timeouts
}

if err := manager.UpdateConfig(newConfig); err != nil {
    log.Error("Failed to update timeout config", err)
}
```

---

## Integration Examples

### Complete Raft Node Integration

```go
package main

import (
    "context"
    "github.com/shawntherrien/streambus/pkg/errors"
    "github.com/shawntherrien/streambus/pkg/health"
    "github.com/shawntherrien/streambus/pkg/logging"
    "github.com/shawntherrien/streambus/pkg/metrics"
    "github.com/shawntherrien/streambus/pkg/resilience"
    "github.com/shawntherrien/streambus/pkg/timeout"
)

type HardenedRaftNode struct {
    node      *RaftNode
    cb        *resilience.CircuitBreaker
    logger    *logging.Logger
    metrics   *metrics.Registry
    timeouts  *timeout.Manager
}

func NewHardenedRaftNode() (*HardenedRaftNode, error) {
    // Initialize components
    logger := logging.New(&logging.Config{
        Level:     logging.LevelInfo,
        Component: "raft",
    })

    metricsRegistry := metrics.NewRegistry()

    timeoutManager, err := timeout.NewManager(timeout.ProductionConfig())
    if err != nil {
        return nil, err
    }

    cb := resilience.NewCircuitBreaker(resilience.DefaultConfig())

    // Monitor circuit breaker state
    cb.OnStateChange(func(from, to resilience.State) {
        logger.InfoOp("circuitbreaker.state", "State changed", logging.Fields{
            "from": from.String(),
            "to":   to.String(),
        })

        stateGauge := metricsRegistry.GetOrCreateGauge(
            "circuit_breaker_state",
            "Circuit breaker state",
            map[string]string{"service": "raft"},
        )
        stateGauge.Set(float64(to))
    })

    return &HardenedRaftNode{
        node:     NewRaftNode(),
        cb:       cb,
        logger:   logger,
        metrics:  metricsRegistry,
        timeouts: timeoutManager,
    }, nil
}

func (h *HardenedRaftNode) Propose(ctx context.Context, data []byte) error {
    // Create timeout context
    ctx, cancel := h.timeouts.WithRaftProposal(ctx)
    defer cancel()

    // Record metrics
    counter := h.metrics.GetOrCreateCounter(
        "raft_proposals_total",
        "Total Raft proposals",
        map[string]string{"status": "attempted"},
    )
    counter.Inc()

    histogram := h.metrics.GetOrCreateHistogram(
        "raft_proposal_duration_seconds",
        "Raft proposal duration",
        nil, nil,
    )
    start := time.Now()
    defer histogram.ObserveDuration(start)

    // Execute with circuit breaker
    err := h.cb.Call(ctx, func() error {
        h.logger.DebugOp("raft.propose", "Proposing entry", logging.Fields{
            "size": len(data),
        })

        return h.node.Propose(ctx, data)
    })

    if err != nil {
        // Categorize and log error
        wrappedErr := errors.Wrap("raft.propose", err)
        h.logger.ErrorOp("raft.propose", "Proposal failed", wrappedErr, logging.Fields{
            "category": errors.GetCategory(wrappedErr).String(),
        })

        // Update error metrics
        errorCounter := h.metrics.GetOrCreateCounter(
            "raft_proposals_total",
            "Total Raft proposals",
            map[string]string{
                "status": "failed",
                "category": errors.GetCategory(wrappedErr).String(),
            },
        )
        errorCounter.Inc()

        return wrappedErr
    }

    // Success
    successCounter := h.metrics.GetOrCreateCounter(
        "raft_proposals_total",
        "Total Raft proposals",
        map[string]string{"status": "committed"},
    )
    successCounter.Inc()

    h.logger.InfoOp("raft.propose", "Proposal committed")
    return nil
}
```

### HTTP Server with All Features

```go
func main() {
    // Initialize logging
    logger := logging.New(&logging.Config{
        Level:     logging.LevelInfo,
        Component: "server",
    })

    // Initialize metrics
    metricsRegistry := metrics.NewRegistry()

    // Initialize health checks
    healthRegistry := health.NewRegistry()

    // Register health checkers
    raftChecker := health.NewRaftNodeHealthChecker("raft", raftNode)
    healthRegistry.Register(raftChecker)

    // Setup HTTP server
    mux := http.NewServeMux()

    // Health endpoints
    healthHandler := health.NewHandler(healthRegistry)
    healthHandler.RegisterRoutes(mux)

    // Metrics endpoint
    metricsHandler := metrics.NewHandler(metricsRegistry)
    metricsHandler.RegisterRoutes(mux)

    // Start server
    server := &http.Server{
        Addr:    ":8080",
        Handler: mux,
    }

    logger.Info("Server starting", logging.Fields{
        "addr": server.Addr,
    })

    if err := server.ListenAndServe(); err != nil {
        logger.Fatal("Server failed", err)
    }
}
```

---

## Best Practices

1. **Circuit Breakers**: Use for all external dependencies and consensus operations
2. **Health Checks**: Implement for all critical components
3. **Error Handling**: Always categorize errors for proper retry logic
4. **Metrics**: Export metrics for all operations, especially errors
5. **Logging**: Use structured logging with operation context
6. **Timeouts**: Use centralized timeout management for consistency
7. **Integration**: Combine all features for comprehensive production hardening

---

## Troubleshooting

### Circuit Breaker Always Open
- Check failure threshold configuration
- Verify downstream service health
- Review timeout settings
- Check for request spikes

### Health Checks Failing
- Verify component initialization order
- Check timeout settings
- Review component-specific health logic
- Look for resource exhaustion

### High Error Rates
- Check error categories in logs
- Review retry logic
- Verify timeout configurations
- Check circuit breaker state

### Missing Metrics
- Verify metrics registration
- Check `/metrics` endpoint accessibility
- Review label configurations
- Ensure metrics are being recorded

---

## Performance Considerations

- Circuit breakers add minimal overhead (<1µs per call)
- Health checks are optimized with periodic caching
- Structured logging has negligible impact with async writes
- Metrics collection is lock-free for counters and gauges
- Timeout contexts are standard Go patterns with minimal overhead

All production hardening features are designed for high-performance, production-grade systems.
