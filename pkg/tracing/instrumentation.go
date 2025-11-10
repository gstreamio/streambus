package tracing

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// InstrumentedOperation runs an operation with tracing
func (t *Tracer) InstrumentedOperation(ctx context.Context, operationName string, fn func(context.Context) error) error {
	ctx, span := t.Start(ctx, operationName)
	defer span.End()

	err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return err
}

// TraceProduceMessage traces message production
func (t *Tracer) TraceProduceMessage(ctx context.Context, topic string, partition uint32, key, value []byte) (context.Context, trace.Span) {
	return t.StartWithAttributes(ctx, "producer.send", map[string]interface{}{
		"messaging.system":               "streambus",
		"messaging.operation":            "send",
		"messaging.destination":          topic,
		"messaging.destination_partition": partition,
		"messaging.message_payload_size_bytes": len(value),
	})
}

// TraceConsumeMessage traces message consumption
func (t *Tracer) TraceConsumeMessage(ctx context.Context, topic string, partition uint32, offset int64) (context.Context, trace.Span) {
	return t.StartWithAttributes(ctx, "consumer.receive", map[string]interface{}{
		"messaging.system":                "streambus",
		"messaging.operation":             "receive",
		"messaging.destination":           topic,
		"messaging.destination_partition": partition,
		"messaging.offset":                offset,
	})
}

// TraceStorageOperation traces storage layer operations
func (t *Tracer) TraceStorageOperation(ctx context.Context, operation, component string) (context.Context, trace.Span) {
	return t.StartWithAttributes(ctx, fmt.Sprintf("storage.%s", operation), map[string]interface{}{
		"component":  component,
		"operation":  operation,
		"layer":      "storage",
	})
}

// TraceRaftOperation traces Raft consensus operations
func (t *Tracer) TraceRaftOperation(ctx context.Context, operation string, term, index int64) (context.Context, trace.Span) {
	return t.StartWithAttributes(ctx, fmt.Sprintf("raft.%s", operation), map[string]interface{}{
		"raft.operation": operation,
		"raft.term":      term,
		"raft.index":     index,
		"component":      "raft",
	})
}

// TraceNetworkRequest traces network requests between brokers
func (t *Tracer) TraceNetworkRequest(ctx context.Context, operation, targetBroker string) (context.Context, trace.Span) {
	return t.StartWithAttributes(ctx, fmt.Sprintf("network.%s", operation), map[string]interface{}{
		"network.operation":      operation,
		"network.peer.address":   targetBroker,
		"network.protocol.name":  "streambus",
	})
}

// RecordMetric records a metric as a span event
func (t *Tracer) RecordMetric(ctx context.Context, metricName string, value interface{}, unit string) {
	t.AddEvent(ctx, metricName, map[string]interface{}{
		"metric.name":  metricName,
		"metric.value": value,
		"metric.unit":  unit,
	})
}

// MeasureDuration measures operation duration and records it
func (t *Tracer) MeasureDuration(ctx context.Context, operation string, fn func() error) error {
	start := time.Now()
	err := fn()
	duration := time.Since(start)

	t.RecordMetric(ctx, fmt.Sprintf("%s.duration", operation), duration.Milliseconds(), "ms")

	if err != nil {
		t.RecordError(ctx, err, map[string]interface{}{
			"operation": operation,
			"duration_ms": duration.Milliseconds(),
		})
	}

	return err
}

// TraceWithRetry traces an operation with retry logic
func (t *Tracer) TraceWithRetry(ctx context.Context, operation string, maxRetries int, fn func(ctx context.Context, attempt int) error) error {
	ctx, span := t.Start(ctx, operation)
	defer span.End()

	span.SetAttributes(convertAttributes(map[string]interface{}{
		"retry.max_attempts": maxRetries,
	})...)

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		attemptCtx, attemptSpan := t.StartWithAttributes(ctx, fmt.Sprintf("%s.attempt", operation), map[string]interface{}{
			"retry.attempt": attempt,
		})

		err := fn(attemptCtx, attempt)
		
		if err == nil {
			attemptSpan.SetStatus(codes.Ok, "")
			attemptSpan.End()
			span.SetStatus(codes.Ok, fmt.Sprintf("succeeded on attempt %d", attempt))
			return nil
		}

		lastErr = err
		attemptSpan.RecordError(err)
		attemptSpan.SetStatus(codes.Error, err.Error())
		attemptSpan.End()

		if attempt < maxRetries {
			t.AddEvent(ctx, "retry", map[string]interface{}{
				"attempt": attempt,
				"error":   err.Error(),
			})
		}
	}

	span.RecordError(lastErr)
	span.SetStatus(codes.Error, fmt.Sprintf("failed after %d attempts", maxRetries))
	return fmt.Errorf("operation failed after %d attempts: %w", maxRetries, lastErr)
}
