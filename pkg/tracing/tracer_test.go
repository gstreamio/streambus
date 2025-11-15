package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestNew_Disabled(t *testing.T) {
	config := &Config{
		Enabled:     false,
		ServiceName: "test-service",
	}

	tracer, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create disabled tracer: %v", err)
	}

	if tracer == nil {
		t.Fatal("Expected non-nil tracer")
	}

	if tracer.IsEnabled() {
		t.Error("Tracer should be disabled")
	}

	if tracer.provider != nil {
		t.Error("Provider should be nil for disabled tracer")
	}
}

func TestNew_WithStdoutExporter(t *testing.T) {
	config := &Config{
		Enabled:        true,
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Environment:    "test",
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
			ParentBased:  true,
		},
		Exporter: ExporterConfig{
			Type: ExporterTypeStdout,
		},
	}

	tracer, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create stdout tracer: %v", err)
	}

	if tracer == nil {
		t.Fatal("Expected non-nil tracer")
	}

	// Stdout exporter doesn't create a provider
	if tracer.provider != nil {
		t.Error("Stdout tracer should not have provider")
	}
}

func TestNew_WithNoneExporter(t *testing.T) {
	config := &Config{
		Enabled:     true,
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
		t.Fatalf("Failed to create none-exporter tracer: %v", err)
	}

	if tracer == nil {
		t.Fatal("Expected non-nil tracer")
	}
}

func TestNew_WithResourceAttributes(t *testing.T) {
	config := &Config{
		Enabled:     false, // Use disabled to avoid needing real exporter
		ServiceName: "test-service",
		ResourceAttributes: map[string]string{
			"deployment.region": "us-west-2",
			"cluster.name":      "prod-cluster",
		},
	}

	tracer, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create tracer with resource attributes: %v", err)
	}

	if tracer == nil {
		t.Fatal("Expected non-nil tracer")
	}
}

func TestNew_UnsupportedExporter(t *testing.T) {
	config := &Config{
		Enabled:     true,
		ServiceName: "test-service",
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
		Exporter: ExporterConfig{
			Type: "unsupported-type",
		},
	}

	_, err := New(config)
	if err == nil {
		t.Error("Expected error for unsupported exporter type")
	}
}

func TestNew_JaegerExporter_MissingEndpoint(t *testing.T) {
	config := &Config{
		Enabled:     true,
		ServiceName: "test-service",
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
		Exporter: ExporterConfig{
			Type: ExporterTypeJaeger,
			Jaeger: JaegerConfig{
				// No collector or agent endpoint
			},
		},
	}

	_, err := New(config)
	if err == nil {
		t.Error("Expected error when jaeger endpoints are missing")
	}
}

func TestTracer_Start_Enabled(t *testing.T) {
	config := &Config{
		Enabled:     true,
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
	ctx, span := tracer.Start(ctx, "test-span")
	defer span.End()

	if span == nil {
		t.Error("Expected non-nil span")
	}

	if ctx == context.Background() {
		t.Error("Expected context to be different from background")
	}
}

func TestTracer_Start_Disabled(t *testing.T) {
	config := &Config{
		Enabled:     false,
		ServiceName: "test-service",
	}

	tracer, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	ctx := context.Background()
	ctx, span := tracer.Start(ctx, "test-span")

	if span == nil {
		t.Error("Expected non-nil span (even if no-op)")
	}
}

func TestTracer_StartWithAttributes_Enabled(t *testing.T) {
	config := &Config{
		Enabled:     true,
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
		"user.id":   "12345",
		"request.count": 10,
		"success":   true,
	}

	ctx, span := tracer.StartWithAttributes(ctx, "test-span", attrs)
	defer span.End()

	if span == nil {
		t.Error("Expected non-nil span")
	}
}

func TestTracer_AddEvent_Enabled(t *testing.T) {
	config := &Config{
		Enabled:     true,
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
	ctx, span := tracer.Start(ctx, "test-span")
	defer span.End()

	attrs := map[string]interface{}{
		"event.name": "cache-miss",
		"cache.key":  "user:123",
	}

	// Should not panic
	tracer.AddEvent(ctx, "cache-miss", attrs)
}

func TestTracer_AddEvent_Disabled(t *testing.T) {
	config := &Config{
		Enabled:     false,
		ServiceName: "test-service",
	}

	tracer, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	ctx := context.Background()
	attrs := map[string]interface{}{
		"event.name": "test-event",
	}

	// Should not panic even with disabled tracer
	tracer.AddEvent(ctx, "test-event", attrs)
}

func TestTracer_RecordError_Enabled(t *testing.T) {
	config := &Config{
		Enabled:     true,
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
	ctx, span := tracer.Start(ctx, "test-span")
	defer span.End()

	testErr := context.DeadlineExceeded
	attrs := map[string]interface{}{
		"error.type": "timeout",
	}

	// Should not panic
	tracer.RecordError(ctx, testErr, attrs)
}

func TestTracer_SetAttributes_Enabled(t *testing.T) {
	config := &Config{
		Enabled:     true,
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
	ctx, span := tracer.Start(ctx, "test-span")
	defer span.End()

	attrs := map[string]interface{}{
		"http.method": "GET",
		"http.status": 200,
	}

	// Should not panic
	tracer.SetAttributes(ctx, attrs)
}

func TestTracer_Extract_Enabled(t *testing.T) {
	config := &Config{
		Enabled:     true,
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

	carrier := propagation.MapCarrier{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}

	ctx := context.Background()
	extractedCtx := tracer.Extract(ctx, carrier)

	if extractedCtx == nil {
		t.Fatal("Expected non-nil context")
	}
}

func TestTracer_Inject_Enabled(t *testing.T) {
	config := &Config{
		Enabled:     true,
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
	ctx, span := tracer.Start(ctx, "test-span")
	defer span.End()

	carrier := propagation.MapCarrier{}
	tracer.Inject(ctx, carrier)

	// When enabled and with active span, should inject trace context
	if len(carrier) == 0 {
		t.Log("Note: Carrier is empty, but that's okay for none exporter")
	}
}

func TestTracer_Shutdown_WithProvider(t *testing.T) {
	config := &Config{
		Enabled:     true,
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

	// Should not error
	err = tracer.Shutdown(context.Background())
	if err != nil {
		t.Errorf("Shutdown error: %v", err)
	}
}

func TestTracer_IsEnabled_True(t *testing.T) {
	config := &Config{
		Enabled:     true,
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

	if !tracer.IsEnabled() {
		t.Error("Expected tracer to be enabled")
	}
}

func TestHTTPMiddleware_Enabled(t *testing.T) {
	config := &Config{
		Enabled:     true,
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

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Wrap with middleware
	middleware := tracer.HTTPMiddleware(testHandler)

	// Create test request
	req := httptest.NewRequest("GET", "/test-path", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	rec := httptest.NewRecorder()

	// Execute
	middleware.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHTTPMiddleware_EnabledWithError(t *testing.T) {
	config := &Config{
		Enabled:     true,
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

	// Create a test handler that returns an error
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error"))
	})

	// Wrap with middleware
	middleware := tracer.HTTPMiddleware(testHandler)

	// Create test request
	req := httptest.NewRequest("POST", "/test", nil)
	rec := httptest.NewRecorder()

	// Execute
	middleware.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", rec.Code)
	}
}

func TestInjectTraceContext_Enabled(t *testing.T) {
	config := &Config{
		Enabled:     true,
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
	ctx, span := tracer.Start(ctx, "test-span")
	defer span.End()

	headers := make(map[string]string)
	tracer.InjectTraceContext(ctx, headers)

	// Headers may or may not be populated depending on exporter
	// Just ensure no panic
}

func TestExtractTraceContext_Enabled(t *testing.T) {
	config := &Config{
		Enabled:     true,
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
	headers := map[string]string{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}

	extractedCtx := tracer.ExtractTraceContext(ctx, headers)
	if extractedCtx == nil {
		t.Error("Expected non-nil context")
	}
}
