# StreamBus Logging

## Overview

StreamBus uses structured logging with **Uber's Zap** for high-performance logging. Zap is one of the fastest structured logging libraries for Go, with zero-allocation logging for hot paths.

## Configuration

### Log Levels

Set the log level using the `LOG_LEVEL` environment variable:

```bash
# Production (default) - only INFO and above
export LOG_LEVEL=info

# Development - see DEBUG logs
export LOG_LEVEL=debug

# Critical only
export LOG_LEVEL=error
```

Available levels:
- `debug` - Detailed diagnostic information for development/troubleshooting
- `info` - General informational messages
- `warn` - Warning messages for potentially harmful situations
- `error` - Error messages for failures
- `fatal` - Fatal errors that cause the application to exit

### Log Format

Set the log format using the `LOG_FORMAT` environment variable:

```bash
# JSON format (default) - structured, machine-parseable
export LOG_FORMAT=json

# Console format - human-readable with colors
export LOG_FORMAT=console
```

## Performance Features

### 1. **Log Sampling**
High-frequency logs are automatically sampled:
- First 100 messages per second are logged
- After that, only 10% of messages are logged
- Prevents log flooding while maintaining visibility

### 2. **Zero-Allocation Logging**
Uses strongly-typed fields instead of string formatting:

```go
// Good - zero allocations
logger.Debug("fetch request",
    zap.String("topic", topic),
    zap.Int64("offset", offset))

// Bad - allocates strings
fmt.Printf("fetch request: topic=%s offset=%d", topic, offset)
```

### 3. **Disabled by Default**
Debug logs have zero overhead when the log level is set to `info` or above.

## Usage Examples

### Basic Logging

```go
import (
    "github.com/gstreamio/streambus/pkg/logger"
    "go.uber.org/zap"
)

// Info level - production events
logger.Info("server started", zap.String("address", addr))

// Debug level - development/troubleshooting
logger.Debug("processing request",
    zap.String("topic", topic),
    zap.Int64("offset", offset))

// Error level - failures
logger.Error("failed to connect",
    zap.String("host", host),
    zap.Error(err))
```

### With Context

```go
// Create a logger with common fields
reqLogger := logger.WithFields(
    zap.String("requestID", reqID),
    zap.String("topic", topic),
)

// Use throughout request lifecycle
reqLogger.Debug("fetching messages")
reqLogger.Info("request completed", zap.Int("messageCount", count))
```

## Best Practices

### 1. Use Appropriate Log Levels

- **Debug**: Verbose information for development (request details, state changes)
- **Info**: Important production events (server start, topic creation, recovery)
- **Warn**: Potentially harmful situations (max connections, retries)
- **Error**: Failures that need attention (connection errors, storage failures)

### 2. Use Structured Fields

Always use typed fields instead of string formatting:

```go
// Good
logger.Debug("message received",
    zap.String("topic", topic),
    zap.Uint32("partition", partitionID),
    zap.Int64("offset", offset))

// Bad
logger.Debug(fmt.Sprintf("message received: topic=%s partition=%d offset=%d",
    topic, partitionID, offset))
```

### 3. Avoid Logging in Hot Paths

For very hot code paths (inner loops, per-message processing), consider:
- Logging at DEBUG level only
- Using metrics instead of logs
- Sampling with counters

### 4. Include Relevant Context

Always include enough context to understand what happened:

```go
logger.Error("failed to write message",
    zap.String("topic", topic),
    zap.Uint32("partition", partitionID),
    zap.Int64("offset", offset),
    zap.Error(err))
```

## Production Deployment

### Recommended Configuration

```bash
# Production settings
export LOG_LEVEL=info
export LOG_FORMAT=json

# Start broker
./streambus
```

### Troubleshooting

To enable debug logging temporarily:

```bash
# Enable debug logs
export LOG_LEVEL=debug

# Restart the broker
./streambus
```

### Log Aggregation

JSON format is ideal for log aggregation systems:
- ELK Stack (Elasticsearch, Logstash, Kibana)
- Splunk
- Datadog
- CloudWatch Logs

Example log entry:
```json
{
  "level": "info",
  "ts": 1234567890.123,
  "msg": "server listening",
  "address": ":9092"
}
```

## Performance Impact

### Log Level Impact

| Level | Hot Path Impact | Use Case |
|-------|----------------|----------|
| `debug` | ~50ns per call | Development/troubleshooting |
| `info` | 0ns (disabled debug) | Production |
| `error` | 0ns (disabled debug/info) | Critical only |

### Comparison to fmt.Printf

| Method | Allocations | Speed |
|--------|-------------|-------|
| `fmt.Printf` | ~3-5 allocs | ~500ns |
| `logger.Debug` (disabled) | 0 allocs | ~0ns |
| `logger.Debug` (enabled) | 0 allocs | ~50ns |

## Migration from fmt.Printf

Old code:
```go
fmt.Printf("FETCH REQUEST: topic='%s' partition=%d offset=%d\n",
    topic, partitionID, offset)
```

New code:
```go
logger.Debug("fetch request",
    zap.String("topic", topic),
    zap.Uint32("partition", partitionID),
    zap.Int64("offset", offset))
```

Benefits:
- **10x faster** when enabled
- **Zero overhead** when disabled
- **Structured** for machine parsing
- **Type-safe** prevents formatting errors
