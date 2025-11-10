package tracing

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestWithAttributes(t *testing.T) {
	config := &spanConfig{}

	attr1 := attribute.String("key1", "value1")
	attr2 := attribute.String("key2", "value2")

	opt := WithAttributes(attr1, attr2)
	opt(config)

	if len(config.attributes) != 2 {
		t.Errorf("Expected 2 attributes, got %d", len(config.attributes))
	}

	if config.attributes[0] != attr1 {
		t.Errorf("First attribute mismatch")
	}

	if config.attributes[1] != attr2 {
		t.Errorf("Second attribute mismatch")
	}
}

func TestWithSpanKind(t *testing.T) {
	config := &spanConfig{}

	opt := WithSpanKind(oteltrace.SpanKindServer)
	opt(config)

	if config.kind != oteltrace.SpanKindServer {
		t.Errorf("Expected SpanKindServer, got %v", config.kind)
	}
}

func TestStartSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(
		trace.WithSyncer(exporter),
	)
	tracer := tp.Tracer("test")

	ctx := context.Background()
	newCtx, span := StartSpan(ctx, tracer, "test-span")

	if newCtx == nil {
		t.Fatal("Expected context to be returned")
	}

	if span == nil {
		t.Fatal("Expected span to be returned")
	}

	span.End()

	// Verify span was created
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(spans))
	}

	if spans[0].Name != "test-span" {
		t.Errorf("Span name = %s, want test-span", spans[0].Name)
	}
}

func TestStartSpan_WithOptions(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(
		trace.WithSyncer(exporter),
	)
	tracer := tp.Tracer("test")

	ctx := context.Background()
	attr := attribute.String("test-key", "test-value")

	_, span := StartSpan(
		ctx,
		tracer,
		"test-span-with-options",
		WithAttributes(attr),
		WithSpanKind(oteltrace.SpanKindProducer),
	)

	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(spans))
	}

	if spans[0].SpanKind != oteltrace.SpanKindProducer {
		t.Errorf("SpanKind = %v, want SpanKindProducer", spans[0].SpanKind)
	}

	// Check attributes
	foundAttr := false
	for _, a := range spans[0].Attributes {
		if a.Key == "test-key" && a.Value.AsString() == "test-value" {
			foundAttr = true
			break
		}
	}

	if !foundAttr {
		t.Error("Expected attribute not found in span")
	}
}

func TestRecordError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(
		trace.WithSyncer(exporter),
	)
	tracer := tp.Tracer("test")

	ctx := context.Background()
	_, span := tracer.Start(ctx, "test-error-span")

	testErr := errors.New("test error")
	RecordError(span, testErr)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(spans))
	}

	if spans[0].Status.Code != codes.Error {
		t.Errorf("Status code = %v, want Error", spans[0].Status.Code)
	}

	if spans[0].Status.Description != "test error" {
		t.Errorf("Status description = %s, want 'test error'", spans[0].Status.Description)
	}

	if len(spans[0].Events) != 1 {
		t.Errorf("Expected 1 event (error recording), got %d", len(spans[0].Events))
	}
}

func TestRecordError_NilSpan(t *testing.T) {
	// Should not panic with nil span
	testErr := errors.New("test error")
	RecordError(nil, testErr)
}

func TestRecordError_NilError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(
		trace.WithSyncer(exporter),
	)
	tracer := tp.Tracer("test")

	ctx := context.Background()
	_, span := tracer.Start(ctx, "test-span")

	// Should not record anything with nil error
	RecordError(span, nil)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(spans))
	}

	// Status should remain unset (not error)
	if spans[0].Status.Code == codes.Error {
		t.Error("Status should not be Error when recording nil error")
	}
}

func TestSetSpanAttributes(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(
		trace.WithSyncer(exporter),
	)
	tracer := tp.Tracer("test")

	ctx := context.Background()
	_, span := tracer.Start(ctx, "test-span")

	attr1 := attribute.String("key1", "value1")
	attr2 := attribute.Int("key2", 42)

	SetSpanAttributes(span, attr1, attr2)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(spans))
	}

	// Check both attributes exist
	attrs := spans[0].Attributes
	foundKey1 := false
	foundKey2 := false

	for _, a := range attrs {
		if a.Key == "key1" && a.Value.AsString() == "value1" {
			foundKey1 = true
		}
		if a.Key == "key2" && a.Value.AsInt64() == 42 {
			foundKey2 = true
		}
	}

	if !foundKey1 {
		t.Error("Attribute key1 not found")
	}

	if !foundKey2 {
		t.Error("Attribute key2 not found")
	}
}

func TestSetSpanAttributes_NilSpan(t *testing.T) {
	// Should not panic with nil span
	attr := attribute.String("key", "value")
	SetSpanAttributes(nil, attr)
}

func TestAddSpanEvent(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(
		trace.WithSyncer(exporter),
	)
	tracer := tp.Tracer("test")

	ctx := context.Background()
	_, span := tracer.Start(ctx, "test-span")

	attr := attribute.String("event-key", "event-value")
	AddSpanEvent(span, "test-event", attr)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(spans))
	}

	events := spans[0].Events
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].Name != "test-event" {
		t.Errorf("Event name = %s, want test-event", events[0].Name)
	}

	foundAttr := false
	for _, a := range events[0].Attributes {
		if a.Key == "event-key" && a.Value.AsString() == "event-value" {
			foundAttr = true
			break
		}
	}

	if !foundAttr {
		t.Error("Event attribute not found")
	}
}

func TestAddSpanEvent_NilSpan(t *testing.T) {
	// Should not panic with nil span
	attr := attribute.String("key", "value")
	AddSpanEvent(nil, "test-event", attr)
}

func TestSetSpanStatus(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(
		trace.WithSyncer(exporter),
	)
	tracer := tp.Tracer("test")

	ctx := context.Background()
	_, span := tracer.Start(ctx, "test-span")

	SetSpanStatus(span, codes.Error, "operation failed")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("Expected 1 span, got %d", len(spans))
	}

	if spans[0].Status.Code != codes.Error {
		t.Errorf("Status code = %v, want Error", spans[0].Status.Code)
	}

	if spans[0].Status.Description != "operation failed" {
		t.Errorf("Status description = %s, want 'operation failed'", spans[0].Status.Description)
	}
}

func TestSetSpanStatus_NilSpan(t *testing.T) {
	// Should not panic with nil span
	SetSpanStatus(nil, codes.Error, "test")
}

func TestBrokerAttributes(t *testing.T) {
	attrs := BrokerAttributes(123, "localhost", 9092)

	if len(attrs) != 3 {
		t.Fatalf("Expected 3 attributes, got %d", len(attrs))
	}

	if attrs[0].Key != AttrBrokerID || attrs[0].Value.AsInt64() != 123 {
		t.Error("BrokerID attribute incorrect")
	}

	if attrs[1].Key != AttrBrokerHost || attrs[1].Value.AsString() != "localhost" {
		t.Error("BrokerHost attribute incorrect")
	}

	if attrs[2].Key != AttrBrokerPort || attrs[2].Value.AsInt64() != 9092 {
		t.Error("BrokerPort attribute incorrect")
	}
}

func TestTopicAttributes(t *testing.T) {
	attrs := TopicAttributes("test-topic", 10, 3)

	if len(attrs) != 3 {
		t.Fatalf("Expected 3 attributes, got %d", len(attrs))
	}

	if attrs[0].Key != AttrTopicName || attrs[0].Value.AsString() != "test-topic" {
		t.Error("TopicName attribute incorrect")
	}

	if attrs[1].Key != AttrTopicPartitions || attrs[1].Value.AsInt64() != 10 {
		t.Error("TopicPartitions attribute incorrect")
	}

	if attrs[2].Key != AttrTopicReplicas || attrs[2].Value.AsInt64() != 3 {
		t.Error("TopicReplicas attribute incorrect")
	}
}

func TestPartitionAttributes(t *testing.T) {
	attrs := PartitionAttributes(5, 100)

	if len(attrs) != 2 {
		t.Fatalf("Expected 2 attributes, got %d", len(attrs))
	}

	if attrs[0].Key != AttrPartitionID || attrs[0].Value.AsInt64() != 5 {
		t.Error("PartitionID attribute incorrect")
	}

	if attrs[1].Key != AttrPartitionOffset || attrs[1].Value.AsInt64() != 100 {
		t.Error("PartitionOffset attribute incorrect")
	}
}

func TestMessageAttributes(t *testing.T) {
	attrs := MessageAttributes("msg-key", 1024, 500)

	if len(attrs) != 3 {
		t.Fatalf("Expected 3 attributes, got %d", len(attrs))
	}

	if attrs[0].Key != AttrMessageKey || attrs[0].Value.AsString() != "msg-key" {
		t.Error("MessageKey attribute incorrect")
	}

	if attrs[1].Key != AttrMessageSize || attrs[1].Value.AsInt64() != 1024 {
		t.Error("MessageSize attribute incorrect")
	}

	if attrs[2].Key != AttrMessageOffset || attrs[2].Value.AsInt64() != 500 {
		t.Error("MessageOffset attribute incorrect")
	}
}

func TestConsumerAttributes(t *testing.T) {
	attrs := ConsumerAttributes("group-1", "consumer-1")

	if len(attrs) != 2 {
		t.Fatalf("Expected 2 attributes, got %d", len(attrs))
	}

	if attrs[0].Key != AttrConsumerGroup || attrs[0].Value.AsString() != "group-1" {
		t.Error("ConsumerGroup attribute incorrect")
	}

	if attrs[1].Key != AttrConsumerID || attrs[1].Value.AsString() != "consumer-1" {
		t.Error("ConsumerID attribute incorrect")
	}
}

func TestTransactionAttributes(t *testing.T) {
	attrs := TransactionAttributes("tx-123", "committed")

	if len(attrs) != 2 {
		t.Fatalf("Expected 2 attributes, got %d", len(attrs))
	}

	if attrs[0].Key != AttrTransactionID || attrs[0].Value.AsString() != "tx-123" {
		t.Error("TransactionID attribute incorrect")
	}

	if attrs[1].Key != AttrTransactionStatus || attrs[1].Value.AsString() != "committed" {
		t.Error("TransactionStatus attribute incorrect")
	}
}

func TestRequestAttributes(t *testing.T) {
	attrs := RequestAttributes(456, "PUBLISH")

	if len(attrs) != 2 {
		t.Fatalf("Expected 2 attributes, got %d", len(attrs))
	}

	if attrs[0].Key != AttrRequestID || attrs[0].Value.AsInt64() != 456 {
		t.Error("RequestID attribute incorrect")
	}

	if attrs[1].Key != AttrRequestType || attrs[1].Value.AsString() != "PUBLISH" {
		t.Error("RequestType attribute incorrect")
	}
}

func TestSchemaAttributes(t *testing.T) {
	attrs := SchemaAttributes(789, 2, "avro")

	if len(attrs) != 3 {
		t.Fatalf("Expected 3 attributes, got %d", len(attrs))
	}

	if attrs[0].Key != AttrSchemaID || attrs[0].Value.AsInt64() != 789 {
		t.Error("SchemaID attribute incorrect")
	}

	if attrs[1].Key != AttrSchemaVersion || attrs[1].Value.AsInt64() != 2 {
		t.Error("SchemaVersion attribute incorrect")
	}

	if attrs[2].Key != AttrSchemaType || attrs[2].Value.AsString() != "avro" {
		t.Error("SchemaType attribute incorrect")
	}
}

func TestErrorAttributes(t *testing.T) {
	attrs := ErrorAttributes("InvalidRequest", "bad input", 400)

	if len(attrs) != 3 {
		t.Fatalf("Expected 3 attributes, got %d", len(attrs))
	}

	if attrs[0].Key != AttrErrorType || attrs[0].Value.AsString() != "InvalidRequest" {
		t.Error("ErrorType attribute incorrect")
	}

	if attrs[1].Key != AttrErrorMessage || attrs[1].Value.AsString() != "bad input" {
		t.Error("ErrorMessage attribute incorrect")
	}

	if attrs[2].Key != AttrErrorCode || attrs[2].Value.AsInt64() != 400 {
		t.Error("ErrorCode attribute incorrect")
	}
}

func TestContextWithSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(
		trace.WithSyncer(exporter),
	)
	tracer := tp.Tracer("test")

	ctx := context.Background()
	_, span := tracer.Start(ctx, "test-span")

	newCtx := ContextWithSpan(context.Background(), span)

	if newCtx == nil {
		t.Fatal("Expected context to be returned")
	}

	// Verify we can retrieve the span from the new context
	retrievedSpan := oteltrace.SpanFromContext(newCtx)
	if retrievedSpan != span {
		t.Error("Retrieved span does not match original span")
	}
}

func TestSpanFromContext(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(
		trace.WithSyncer(exporter),
	)
	tracer := tp.Tracer("test")

	ctx := context.Background()
	ctx, span := tracer.Start(ctx, "test-span")

	retrievedSpan := SpanFromContext(ctx)

	if retrievedSpan != span {
		t.Error("Retrieved span does not match original span")
	}
}

func TestSpanContextFromContext(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(
		trace.WithSyncer(exporter),
	)
	tracer := tp.Tracer("test")

	ctx := context.Background()
	ctx, span := tracer.Start(ctx, "test-span")

	spanCtx := SpanContextFromContext(ctx)

	if !spanCtx.IsValid() {
		t.Error("Expected valid span context")
	}

	// Verify it matches the span's context by comparing trace and span IDs
	originalCtx := span.SpanContext()
	if spanCtx.TraceID() != originalCtx.TraceID() {
		t.Error("Trace ID does not match")
	}

	if spanCtx.SpanID() != originalCtx.SpanID() {
		t.Error("Span ID does not match")
	}
}
