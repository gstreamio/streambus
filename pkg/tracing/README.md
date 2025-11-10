# StreamBus Distributed Tracing

OpenTelemetry-based distributed tracing for StreamBus.

## Features

- OpenTelemetry integration
- Multiple exporters (Jaeger, OTLP)
- Context propagation across services
- HTTP middleware for automatic request tracing
- Instrumentation helpers for common operations
- Configurable sampling rates
- Custom attributes and events

## Quick Start

### Basic Setup

```go
import "github.com/shawntherrien/streambus/pkg/tracing"

// Create tracer
tracer, err := tracing.New(&tracing.Config{
    Enabled:        true,
    ServiceName:    "streambus-broker",
    ServiceVersion: "1.0.0",
    Environment:    "production",
    Exporter:       "jaeger",
    JaegerEndpoint: "http://localhost:14268/api/traces",
    SamplingRate:   1.0, // 100% sampling
})
if err != nil {
    log.Fatal(err)
}
defer tracer.Shutdown(context.Background())
```

### Tracing Operations

```go
// Start a span
ctx, span := tracer.Start(ctx, "my-operation")
defer span.End()

// Add attributes
tracer.SetAttributes(ctx, map[string]interface{}{
    "user_id": 123,
    "action":  "create",
})

// Add events
tracer.AddEvent(ctx, "validation-complete", map[string]interface{}{
    "valid": true,
})

// Record errors
if err != nil {
    tracer.RecordError(ctx, err, map[string]interface{}{
        "error_code": "VALIDATION_FAILED",
    })
}
```

### Instrumented Operations

```go
// Automatically trace with error handling
err := tracer.InstrumentedOperation(ctx, "process-message", func(ctx context.Context) error {
    return processMessage(ctx, msg)
})
```

### Message Tracing

```go
// Trace producer
ctx, span := tracer.TraceProduceMessage(ctx, "events", 0, key, value)
defer span.End()

err := producer.Send(ctx, topic, partition, key, value)

// Trace consumer
ctx, span := tracer.TraceConsumeMessage(ctx, "events", 0, offset)
defer span.End()

messages, err := consumer.Fetch(ctx, offset, maxBytes)
```

### HTTP Middleware

```go
// Add tracing to HTTP handlers
http.Handle("/api/", tracer.HTTPMiddleware(apiHandler))
```

### Retry Tracing

```go
// Trace operations with retries
err := tracer.TraceWithRetry(ctx, "send-request", 3, func(ctx context.Context, attempt int) error {
    return sendRequest(ctx, req)
})
```

## Configuration

### Jaeger

```go
tracer, err := tracing.New(&tracing.Config{
    Enabled:        true,
    ServiceName:    "streambus-broker",
    Exporter:       "jaeger",
    JaegerEndpoint: "http://localhost:14268/api/traces",
    SamplingRate:   0.1, // 10% sampling
})
```

### OTLP (OpenTelemetry Protocol)

```go
tracer, err := tracing.New(&tracing.Config{
    Enabled:      true,
    ServiceName:  "streambus-broker",
    Exporter:     "otlp",
    OTLPEndpoint: "localhost:4317",
    SamplingRate: 1.0,
})
```

### Custom Attributes

```go
tracer, err := tracing.New(&tracing.Config{
    Enabled:     true,
    ServiceName: "streambus-broker",
    Attributes: map[string]string{
        "cluster":    "us-west-1",
        "datacenter": "dc1",
        "version":    "1.0.0",
    },
})
```

## Context Propagation

### Extract from HTTP Headers

```go
// Extract trace context from incoming request
ctx := tracer.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

ctx, span := tracer.Start(ctx, "handle-request")
defer span.End()
```

### Inject into HTTP Headers

```go
// Inject trace context into outgoing request
req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
tracer.Inject(ctx, propagation.HeaderCarrier(req.Header))
```

### Message Headers

```go
// Inject into message headers
headers := make(map[string]string)
tracer.InjectTraceContext(ctx, headers)

// Extract from message headers
ctx = tracer.ExtractTraceContext(ctx, headers)
```

## Deployment

### With Jaeger

```bash
# Run Jaeger all-in-one
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 14268:14268 \
  jaegertracing/all-in-one:latest

# Access UI at http://localhost:16686
```

### With OpenTelemetry Collector

```yaml
# collector-config.yaml
receivers:
  otlp:
    protocols:
      grpc:

exporters:
  jaeger:
    endpoint: jaeger:14250
    tls:
      insecure: true

service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [jaeger]
```

```bash
# Run collector
docker run -d --name otel-collector \
  -p 4317:4317 \
  -v $(pwd)/collector-config.yaml:/etc/otel-collector-config.yaml \
  otel/opentelemetry-collector:latest \
  --config=/etc/otel-collector-config.yaml
```

## Performance Considerations

### Sampling

Use sampling to reduce overhead in production:

```go
// 10% sampling
SamplingRate: 0.1

// Head-based sampling for specific operations
if isImportantOperation(op) {
    span.SetAttributes(attribute.Bool("sampled", true))
}
```

### Async Export

Traces are exported asynchronously in batches to minimize performance impact.

### Overhead

Typical overhead with tracing enabled:
- Latency: < 1ms per span
- Memory: ~1KB per span
- CPU: < 5% with 10% sampling

## Best Practices

1. **Use meaningful span names**: `"producer.send"` not `"send"`
2. **Add relevant attributes**: Include IDs, types, sizes
3. **Record errors**: Always use `RecordError()` for failures
4. **Propagate context**: Pass context through all function calls
5. **Close spans**: Always defer `span.End()`
6. **Sample intelligently**: Use lower rates in production
7. **Monitor overhead**: Track tracing performance impact

## Troubleshooting

### No traces appearing

1. Check tracer is enabled: `tracer.IsEnabled()`
2. Verify exporter endpoint is reachable
3. Check sampling rate (use 1.0 for testing)
4. Review logs for export errors

### High overhead

1. Reduce sampling rate
2. Limit span attributes
3. Avoid tracing hot paths
4. Use batch export (default)

### Context not propagating

1. Ensure context is passed through call chain
2. Verify `Inject()` and `Extract()` are called
3. Check header propagation in HTTP/gRPC

## Examples

See [examples/tracing](../../examples/tracing) for complete examples:
- Basic tracing
- HTTP service tracing
- Message producer/consumer tracing
- Multi-service distributed tracing

## References

- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [Jaeger Documentation](https://www.jaegertracing.io/docs/)
- [Distributed Tracing Best Practices](https://opentelemetry.io/docs/concepts/signals/traces/)
