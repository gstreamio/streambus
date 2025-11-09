# StreamBus OpenTelemetry Tracing Example

This example demonstrates StreamBus's distributed tracing capabilities using OpenTelemetry.

## Features Demonstrated

### 1. Tracer Configuration
- Service identification (name, version, environment)
- Multiple exporter types (OTLP, Jaeger, Zipkin, Stdout)
- Sampling strategies (always, never, probability-based)
- Resource attributes for context
- Trace context propagation

### 2. Span Creation
- Starting spans for operations
- Parent-child span relationships
- Span kinds (internal, server, client, producer, consumer)
- Span attributes for rich context
- Span events for milestones

### 3. Error Tracking
- Recording errors in traces
- Error attributes
- Setting error status codes
- Debugging with trace context

### 4. StreamBus Integration
- Broker operation tracing
- Message production tracing
- Topic management tracing
- Partition-level tracing
- Transaction tracing

## Running the Example

```bash
# From the examples/tracing directory
go run main.go
```

The example will:
1. Configure OpenTelemetry tracing
2. Initialize a tracer with stdout exporter
3. Create a broker
4. Create a topic with tracing
5. Produce messages with tracing
6. Demonstrate error tracing
7. Show nested span relationships
8. Add custom attributes
9. Record span events
10. Clean shutdown with trace flushing

## Output

The example outputs traces to stdout in JSON format. Each trace includes:
- **TraceID**: Unique identifier for the trace
- **SpanID**: Unique identifier for the span
- **ParentSpanID**: Parent span's ID (for nested spans)
- **Name**: Operation name
- **StartTime** / **EndTime**: Timing information
- **Attributes**: Key-value pairs with context
- **Events**: Important milestones within the span
- **Status**: Success or error status
- **Resource**: Service information

### Sample Trace Output

```json
{
  "Name": "produce-message",
  "SpanContext": {
    "TraceID": "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
    "SpanID": "1a2b3c4d5e6f7g8h",
    "TraceFlags": "01"
  },
  "SpanKind": 3,
  "StartTime": "2025-01-15T10:30:00Z",
  "EndTime": "2025-01-15T10:30:00.005Z",
  "Attributes": [
    {
      "Key": "streambus.topic.name",
      "Value": "traced-events"
    },
    {
      "Key": "streambus.message.key",
      "Value": "key-1"
    },
    {
      "Key": "streambus.message.size",
      "Value": 42
    }
  ],
  "Events": [
    {
      "Name": "message-sent",
      "Timestamp": "2025-01-15T10:30:00.004Z"
    }
  ],
  "Status": {
    "Code": "Ok",
    "Description": "Message sent"
  }
}
```

## Exporter Configurations

### OTLP (OpenTelemetry Collector)

```go
tracingConfig := &tracing.Config{
    Enabled:        true,
    ServiceName:    "streambus",
    ServiceVersion: "1.0.0",
    Environment:    "production",
    Exporter: tracing.ExporterConfig{
        Type: tracing.ExporterTypeOTLP,
        OTLP: tracing.OTLPConfig{
            Endpoint:    "localhost:4317",
            Insecure:    false, // Use TLS in production
            Timeout:     10 * time.Second,
            Compression: "gzip",
            Headers: map[string]string{
                "api-key": "your-api-key",
            },
        },
    },
    Sampling: tracing.SamplingConfig{
        SamplingRate: 0.1, // Sample 10% of traces
        ParentBased:  true,
    },
}
```

### Jaeger

```go
Exporter: tracing.ExporterConfig{
    Type: tracing.ExporterTypeJaeger,
    Jaeger: tracing.JaegerConfig{
        AgentEndpoint: "localhost:6831",
        // OR
        CollectorEndpoint: "http://localhost:14268/api/traces",
    },
}
```

### Zipkin

```go
Exporter: tracing.ExporterConfig{
    Type: tracing.ExporterTypeZipkin,
    Zipkin: tracing.ZipkinConfig{
        Endpoint: "http://localhost:9411/api/v2/spans",
        Timeout:  10 * time.Second,
    },
}
```

### Stdout (Development/Debugging)

```go
Exporter: tracing.ExporterConfig{
    Type: tracing.ExporterTypeStdout,
}
```

## Sampling Strategies

### Always Sample
```go
Sampling: tracing.SamplingConfig{
    SamplingRate: 1.0, // 100%
    ParentBased:  false,
}
```

### Probability-Based Sampling
```go
Sampling: tracing.SamplingConfig{
    SamplingRate: 0.1, // 10%
    ParentBased:  true, // Respect parent's sampling decision
}
```

### Never Sample (Disabled)
```go
Sampling: tracing.SamplingConfig{
    SamplingRate: 0.0,
    ParentBased:  false,
}
```

## Using Traces

### Creating Spans

```go
ctx, span := tracing.StartSpan(ctx, tracer, "operation-name",
    tracing.WithSpanKind(trace.SpanKindClient),
    tracing.WithAttributes(
        tracing.AttrTopicName.String("my-topic"),
        tracing.AttrPartitionID.Int64(0),
    ),
)
defer span.End()

// Do work...

tracing.SetSpanStatus(span, codes.Ok, "Operation successful")
```

### Recording Errors

```go
err := someOperation()
if err != nil {
    tracing.RecordError(span, err)
    tracing.SetSpanAttributes(span,
        tracing.ErrorAttributes("operation_failed", err.Error(), 500)...,
    )
}
```

### Adding Events

```go
tracing.AddSpanEvent(span, "checkpoint",
    tracing.AttrMessageCount.Int(100),
)
```

### Nested Spans

```go
// Parent span
parentCtx, parentSpan := tracing.StartSpan(ctx, tracer, "batch-operation")
defer parentSpan.End()

// Child spans inherit parent's trace context
for i := 0; i < 10; i++ {
    childCtx, childSpan := tracing.StartSpan(parentCtx, tracer, "item-operation")
    // Do work...
    childSpan.End()
}
```

## StreamBus-Specific Attributes

The tracing package provides helper functions for common attributes:

### Broker Attributes
```go
tracing.BrokerAttributes(brokerID, host, port)
```

### Topic Attributes
```go
tracing.TopicAttributes(name, partitions, replicas)
```

### Partition Attributes
```go
tracing.PartitionAttributes(partitionID, offset)
```

### Message Attributes
```go
tracing.MessageAttributes(key, size, offset)
```

### Consumer Attributes
```go
tracing.ConsumerAttributes(groupID, consumerID)
```

### Transaction Attributes
```go
tracing.TransactionAttributes(txID, status)
```

### Schema Attributes
```go
tracing.SchemaAttributes(schemaID, version, schemaType)
```

## Trace Backends

### Jaeger

1. Start Jaeger:
```bash
docker run -d --name jaeger \
  -p 6831:6831/udp \
  -p 16686:16686 \
  jaegertracing/all-in-one:latest
```

2. View traces at: http://localhost:16686

### Zipkin

1. Start Zipkin:
```bash
docker run -d --name zipkin \
  -p 9411:9411 \
  openzipkin/zipkin
```

2. View traces at: http://localhost:9411

### OpenTelemetry Collector

1. Create `otel-collector-config.yaml`:
```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317

exporters:
  logging:
    loglevel: debug
  jaeger:
    endpoint: jaeger:14250
    tls:
      insecure: true

service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [logging, jaeger]
```

2. Start collector:
```bash
docker run -d --name otel-collector \
  -v $(pwd)/otel-collector-config.yaml:/etc/otel-collector-config.yaml \
  -p 4317:4317 \
  otel/opentelemetry-collector:latest \
  --config=/etc/otel-collector-config.yaml
```

## Best Practices

### 1. Span Naming
- Use clear, descriptive names
- Follow a consistent naming convention
- Example: `broker.produce`, `consumer.poll`, `topic.create`

### 2. Attributes
- Add relevant context for debugging
- Don't add high-cardinality values (like message content)
- Use semantic conventions when possible

### 3. Sampling
- Use high sampling in development (1.0)
- Use lower sampling in production (0.01-0.1)
- Always use parent-based sampling for consistent traces

### 4. Resource Attributes
- Add deployment information (environment, region, datacenter)
- Add version information
- Add instance identifiers

### 5. Error Handling
- Always record errors in spans
- Add error attributes for context
- Set appropriate status codes

### 6. Span Lifecycle
- Always call `span.End()`
- Use `defer span.End()` for automatic cleanup
- Set status before ending span

### 7. Performance
- Minimize span creation overhead
- Use sampling to reduce volume
- Batch span exports

## Trace Analysis

### Finding Slow Operations
Query for spans with high latency:
```
duration > 100ms
```

### Finding Errors
Query for spans with errors:
```
status.code = ERROR
```

### Finding Specific Operations
Query by span name:
```
name = "produce-message"
```

### Finding Traces for a Topic
Query by attribute:
```
streambus.topic.name = "events"
```

## Integration with Monitoring

Combine tracing with metrics and logs for complete observability:

1. **Metrics**: Track aggregate statistics (throughput, latency percentiles)
2. **Traces**: Debug specific requests and identify bottlenecks
3. **Logs**: Detailed event information and debugging

### Trace-Metric Correlation
Use trace IDs in log messages:
```go
logger.Info("Message produced",
    "trace_id", span.SpanContext().TraceID().String(),
    "message_key", key,
)
```

## Troubleshooting

### Traces Not Appearing

1. Check tracer is enabled:
```go
if tracer.IsEnabled() {
    fmt.Println("Tracing is enabled")
}
```

2. Verify exporter configuration
3. Check sampling rate (ensure > 0)
4. Ensure proper shutdown for span flushing

### High Overhead

1. Reduce sampling rate
2. Minimize attributes per span
3. Use batch span processors
4. Check exporter configuration

### Missing Spans

1. Verify parent context propagation
2. Ensure `span.End()` is called
3. Check for context cancellation
4. Verify tracer is initialized before use

## Next Steps

1. Deploy OpenTelemetry Collector
2. Connect to trace backend (Jaeger/Zipkin)
3. Create custom dashboards
4. Set up alerting on trace patterns
5. Integrate with CI/CD for trace analysis
