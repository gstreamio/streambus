package tracing

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Enabled {
		t.Error("Expected tracing to be disabled by default")
	}

	if config.ServiceName != "streambus" {
		t.Errorf("Expected service name 'streambus', got '%s'", config.ServiceName)
	}

	if config.Environment != "development" {
		t.Errorf("Expected environment 'development', got '%s'", config.Environment)
	}

	if config.Sampling.SamplingRate != 1.0 {
		t.Errorf("Expected sampling rate 1.0, got %f", config.Sampling.SamplingRate)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errType error
	}{
		{
			name: "valid config",
			config: &Config{
				Enabled:     true,
				ServiceName: "test-service",
				Exporter: ExporterConfig{
					Type: ExporterTypeOTLP,
					OTLP: OTLPConfig{
						Endpoint: "localhost:4317",
					},
				},
				Sampling: SamplingConfig{
					SamplingRate: 1.0,
				},
			},
			wantErr: false,
		},
		{
			name: "missing service name",
			config: &Config{
				Enabled:     true,
				ServiceName: "",
			},
			wantErr: true,
			errType: ErrInvalidServiceName,
		},
		{
			name: "invalid sampling rate (negative)",
			config: &Config{
				Enabled:     true,
				ServiceName: "test",
				Sampling: SamplingConfig{
					SamplingRate: -0.1,
				},
			},
			wantErr: true,
			errType: ErrInvalidSamplingRate,
		},
		{
			name: "invalid sampling rate (> 1)",
			config: &Config{
				Enabled:     true,
				ServiceName: "test",
				Sampling: SamplingConfig{
					SamplingRate: 1.5,
				},
			},
			wantErr: true,
			errType: ErrInvalidSamplingRate,
		},
		{
			name: "disabled tracing - no validation",
			config: &Config{
				Enabled:     false,
				ServiceName: "", // Invalid but should pass because disabled
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errType != nil && err != tt.errType {
				t.Errorf("Expected error type %v, got %v", tt.errType, err)
			}
		})
	}
}

func TestNewTracer_Disabled(t *testing.T) {
	config := &Config{
		Enabled:     false,
		ServiceName: "test-service",
	}

	tracer, err := NewTracer(config)
	if err != nil {
		t.Fatalf("NewTracer() error = %v", err)
	}

	if tracer == nil {
		t.Fatal("Expected tracer to be non-nil")
	}

	if !tracer.IsEnabled() == config.Enabled {
		t.Error("Tracer enabled state doesn't match config")
	}

	// Should be able to get tracer even when disabled (no-op tracer)
	tr := tracer.Tracer()
	if tr == nil {
		t.Error("Expected non-nil tracer")
	}
}

func TestNewTracer_Stdout(t *testing.T) {
	config := &Config{
		Enabled:     true,
		ServiceName: "test-service",
		Exporter: ExporterConfig{
			Type: ExporterTypeStdout,
		},
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
			ParentBased:  true,
		},
	}

	tracer, err := NewTracer(config)
	if err != nil {
		t.Fatalf("NewTracer() error = %v", err)
	}
	defer tracer.Shutdown(context.Background())

	if tracer == nil {
		t.Fatal("Expected tracer to be non-nil")
	}

	if !tracer.IsEnabled() {
		t.Error("Expected tracer to be enabled")
	}

	tr := tracer.Tracer()
	if tr == nil {
		t.Error("Expected non-nil tracer")
	}

	// Test creating a span
	ctx := context.Background()
	ctx, span := tr.Start(ctx, "test-span")
	span.SetAttributes(AttrBrokerID.Int64(1))
	span.End()
}

func TestNewTracer_OTLP(t *testing.T) {
	// Note: This will fail to connect to OTLP endpoint, but should create tracer
	config := &Config{
		Enabled:     true,
		ServiceName: "test-service",
		Exporter: ExporterConfig{
			Type: ExporterTypeOTLP,
			OTLP: OTLPConfig{
				Endpoint:    "localhost:4317",
				Insecure:    true,
				Timeout:     5 * time.Second,
				Compression: "gzip",
			},
		},
		Sampling: SamplingConfig{
			SamplingRate: 0.5,
			ParentBased:  true,
		},
		ResourceAttributes: map[string]string{
			"region": "us-west-2",
			"env":    "test",
		},
	}

	tracer, err := NewTracer(config)
	if err != nil {
		t.Fatalf("NewTracer() error = %v", err)
	}
	defer tracer.Shutdown(context.Background())

	if !tracer.IsEnabled() {
		t.Error("Expected tracer to be enabled")
	}

	tr := tracer.Tracer()
	if tr == nil {
		t.Error("Expected non-nil tracer")
	}
}

func TestTracerShutdown(t *testing.T) {
	config := &Config{
		Enabled:     true,
		ServiceName: "test-service",
		Exporter: ExporterConfig{
			Type: ExporterTypeStdout,
		},
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
	}

	tracer, err := NewTracer(config)
	if err != nil {
		t.Fatalf("NewTracer() error = %v", err)
	}

	ctx := context.Background()

	// Shutdown should succeed
	err = tracer.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}

	// Second shutdown should also succeed (idempotent)
	err = tracer.Shutdown(ctx)
	if err != nil {
		t.Errorf("Second Shutdown() error = %v", err)
	}
}

func TestStartSpan(t *testing.T) {
	config := &Config{
		Enabled:     true,
		ServiceName: "test-service",
		Exporter: ExporterConfig{
			Type: ExporterTypeStdout,
		},
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
	}

	tracer, err := NewTracer(config)
	if err != nil {
		t.Fatalf("NewTracer() error = %v", err)
	}
	defer tracer.Shutdown(context.Background())

	ctx := context.Background()
	tr := tracer.Tracer()

	// Test basic span
	ctx, span := StartSpan(ctx, tr, "test-operation")
	if span == nil {
		t.Fatal("Expected non-nil span")
	}
	span.End()

	// Test span with attributes
	ctx, span = StartSpan(ctx, tr, "test-with-attributes",
		WithAttributes(
			AttrBrokerID.Int64(1),
			AttrTopicName.String("test-topic"),
		),
	)
	if span == nil {
		t.Fatal("Expected non-nil span")
	}
	span.End()

	// Test span with kind
	ctx, span = StartSpan(ctx, tr, "test-with-kind",
		WithSpanKind(trace.SpanKindServer),
	)
	if span == nil {
		t.Fatal("Expected non-nil span")
	}
	span.End()
}

func TestRecordError(t *testing.T) {
	config := &Config{
		Enabled:     true,
		ServiceName: "test-service",
		Exporter: ExporterConfig{
			Type: ExporterTypeStdout,
		},
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
	}

	tracer, err := NewTracer(config)
	if err != nil {
		t.Fatalf("NewTracer() error = %v", err)
	}
	defer tracer.Shutdown(context.Background())

	ctx := context.Background()
	tr := tracer.Tracer()

	ctx, span := tr.Start(ctx, "test-error")
	defer span.End()

	testErr := ErrTracerNotInitialized
	RecordError(span, testErr)

	// Span should have error status
	// We can't easily verify this without reflection or integration tests
}

func TestHelperFunctions(t *testing.T) {
	config := &Config{
		Enabled:     true,
		ServiceName: "test-service",
		Exporter: ExporterConfig{
			Type: ExporterTypeStdout,
		},
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
	}

	tracer, err := NewTracer(config)
	if err != nil {
		t.Fatalf("NewTracer() error = %v", err)
	}
	defer tracer.Shutdown(context.Background())

	ctx := context.Background()
	tr := tracer.Tracer()

	ctx, span := tr.Start(ctx, "test-helpers")
	defer span.End()

	// Test SetSpanAttributes
	SetSpanAttributes(span, AttrBrokerID.Int64(1), AttrTopicName.String("test"))

	// Test AddSpanEvent
	AddSpanEvent(span, "test-event", AttrMessageCount.Int(10))

	// Test SetSpanStatus
	SetSpanStatus(span, codes.Ok, "Operation successful")

	// Test context helpers
	extractedSpan := SpanFromContext(ctx)
	if extractedSpan == nil {
		t.Error("Expected non-nil span from context")
	}

	spanCtx := SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		t.Error("Expected valid span context")
	}
}

func TestAttributeHelpers(t *testing.T) {
	// Test BrokerAttributes
	brokerAttrs := BrokerAttributes(1, "localhost", 9092)
	if len(brokerAttrs) != 3 {
		t.Errorf("Expected 3 broker attributes, got %d", len(brokerAttrs))
	}

	// Test TopicAttributes
	topicAttrs := TopicAttributes("test-topic", 3, 2)
	if len(topicAttrs) != 3 {
		t.Errorf("Expected 3 topic attributes, got %d", len(topicAttrs))
	}

	// Test PartitionAttributes
	partAttrs := PartitionAttributes(0, 100)
	if len(partAttrs) != 2 {
		t.Errorf("Expected 2 partition attributes, got %d", len(partAttrs))
	}

	// Test MessageAttributes
	msgAttrs := MessageAttributes("key1", 1024, 50)
	if len(msgAttrs) != 3 {
		t.Errorf("Expected 3 message attributes, got %d", len(msgAttrs))
	}

	// Test ConsumerAttributes
	consumerAttrs := ConsumerAttributes("group1", "consumer1")
	if len(consumerAttrs) != 2 {
		t.Errorf("Expected 2 consumer attributes, got %d", len(consumerAttrs))
	}

	// Test TransactionAttributes
	txAttrs := TransactionAttributes("tx-123", "committed")
	if len(txAttrs) != 2 {
		t.Errorf("Expected 2 transaction attributes, got %d", len(txAttrs))
	}

	// Test RequestAttributes
	reqAttrs := RequestAttributes(12345, "produce")
	if len(reqAttrs) != 2 {
		t.Errorf("Expected 2 request attributes, got %d", len(reqAttrs))
	}

	// Test SchemaAttributes
	schemaAttrs := SchemaAttributes(1, 2, "avro")
	if len(schemaAttrs) != 3 {
		t.Errorf("Expected 3 schema attributes, got %d", len(schemaAttrs))
	}

	// Test ErrorAttributes
	errAttrs := ErrorAttributes("validation", "invalid input", 400)
	if len(errAttrs) != 3 {
		t.Errorf("Expected 3 error attributes, got %d", len(errAttrs))
	}
}

func TestToResourceAttributes(t *testing.T) {
	config := &Config{
		ServiceName:    "test-service",
		ServiceVersion: "1.2.3",
		Environment:    "production",
		ResourceAttributes: map[string]string{
			"region": "us-west-2",
			"az":     "us-west-2a",
		},
	}

	attrs := config.ToResourceAttributes()

	// Should have at least service.name, service.version, deployment.environment + custom attrs
	if len(attrs) < 5 {
		t.Errorf("Expected at least 5 attributes, got %d", len(attrs))
	}

	// Verify required attributes are present
	foundServiceName := false
	foundServiceVersion := false
	foundEnvironment := false

	for _, attr := range attrs {
		switch string(attr.Key) {
		case "service.name":
			foundServiceName = true
			if attr.Value.AsString() != "test-service" {
				t.Errorf("Expected service.name='test-service', got '%s'", attr.Value.AsString())
			}
		case "service.version":
			foundServiceVersion = true
			if attr.Value.AsString() != "1.2.3" {
				t.Errorf("Expected service.version='1.2.3', got '%s'", attr.Value.AsString())
			}
		case "deployment.environment":
			foundEnvironment = true
			if attr.Value.AsString() != "production" {
				t.Errorf("Expected deployment.environment='production', got '%s'", attr.Value.AsString())
			}
		}
	}

	if !foundServiceName {
		t.Error("service.name attribute not found")
	}
	if !foundServiceVersion {
		t.Error("service.version attribute not found")
	}
	if !foundEnvironment {
		t.Error("deployment.environment attribute not found")
	}
}

func BenchmarkStartSpan(b *testing.B) {
	config := &Config{
		Enabled:     true,
		ServiceName: "benchmark-service",
		Exporter: ExporterConfig{
			Type: ExporterTypeNone,
		},
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
	}

	tracer, err := NewTracer(config)
	if err != nil {
		b.Fatalf("NewTracer() error = %v", err)
	}
	defer tracer.Shutdown(context.Background())

	ctx := context.Background()
	tr := tracer.Tracer()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, span := StartSpan(ctx, tr, "benchmark-span",
			WithAttributes(AttrBrokerID.Int64(1)),
		)
		span.End()
	}
}
