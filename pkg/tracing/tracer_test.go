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

func TestTracer_SetAttributes(t *testing.T) {
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

	ctx := context.Background()
	attrs := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
	}

	// Should not panic
	tracer.SetAttributes(ctx, attrs)
}

func TestTracer_Shutdown(t *testing.T) {
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

	err = tracer.Shutdown(context.Background())
	if err != nil {
		t.Errorf("Expected no error on shutdown, got %v", err)
	}
}

func TestTracer_IsEnabled(t *testing.T) {
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

	if tracer.IsEnabled() {
		t.Error("Expected tracer to be disabled")
	}
}

func TestConvertAttributes(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]interface{}
	}{
		{
			name: "string attribute",
			attrs: map[string]interface{}{
				"key": "value",
			},
		},
		{
			name: "int attribute",
			attrs: map[string]interface{}{
				"key": 123,
			},
		},
		{
			name: "int64 attribute",
			attrs: map[string]interface{}{
				"key": int64(456),
			},
		},
		{
			name: "float64 attribute",
			attrs: map[string]interface{}{
				"key": 3.14,
			},
		},
		{
			name: "bool attribute",
			attrs: map[string]interface{}{
				"key": true,
			},
		},
		{
			name: "string slice attribute",
			attrs: map[string]interface{}{
				"key": []string{"a", "b", "c"},
			},
		},
		{
			name: "mixed attributes",
			attrs: map[string]interface{}{
				"str":    "value",
				"int":    123,
				"int64":  int64(456),
				"float":  3.14,
				"bool":   true,
				"slice":  []string{"x", "y"},
				"other":  struct{ Name string }{Name: "test"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertAttributes(tt.attrs)
			if result == nil {
				t.Error("Expected non-nil result")
			}
			if len(result) != len(tt.attrs) {
				t.Errorf("Expected %d attributes, got %d", len(tt.attrs), len(result))
			}
		})
	}
}
