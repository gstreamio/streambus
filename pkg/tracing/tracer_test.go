package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/propagation"
)

func TestTracer_Extract_Disabled(t *testing.T) {
	config := &Config{
		Enabled:     false,
		ServiceName: "test-service",
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
		Exporter: ExporterConfig{
			Type: ExporterTypeNone,
		},
	}

	tracer, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	// Create a carrier with trace context
	carrier := propagation.MapCarrier{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}

	ctx := context.Background()
	extractedCtx := tracer.Extract(ctx, carrier)

	// Should return the original context when tracer is disabled
	if extractedCtx == nil {
		t.Fatal("Expected context to be returned")
	}
}

func TestTracer_Inject_Disabled(t *testing.T) {
	config := &Config{
		Enabled:     false,
		ServiceName: "test-service",
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
		Exporter: ExporterConfig{
			Type: ExporterTypeNone,
		},
	}

	tracer, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	// Create a carrier
	carrier := propagation.MapCarrier{}

	ctx := context.Background()
	// Should not panic when tracer is disabled
	tracer.Inject(ctx, carrier)

	// Carrier should remain empty when tracer is disabled
	if len(carrier) != 0 {
		t.Error("Expected carrier to be empty when tracer is disabled")
	}
}
