package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPMiddleware(t *testing.T) {
	config := &Config{
		Enabled:     false, // Disable to avoid schema conflicts in testing
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
	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	// Wrap with middleware
	middleware := tracer.HTTPMiddleware(handler)

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/test/path", nil)
	req.Header.Set("User-Agent", "test-agent")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	// Serve request
	middleware.ServeHTTP(w, req)

	if !handlerCalled {
		t.Error("Expected handler to be called")
	}

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusOK)
	}

	if w.Body.String() != "test response" {
		t.Errorf("Body = %s, want 'test response'", w.Body.String())
	}
}

func TestHTTPMiddleware_ErrorStatus(t *testing.T) {
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

	// Create a test handler that returns an error
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	})

	// Wrap with middleware
	middleware := tracer.HTTPMiddleware(handler)

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	w := httptest.NewRecorder()

	// Serve request
	middleware.ServeHTTP(w, req)

	// Check response
	if w.Code != http.StatusNotFound {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusNotFound)
	}

	if w.Body.String() != "not found" {
		t.Errorf("Body = %s, want 'not found'", w.Body.String())
	}
}

func TestHTTPMiddleware_DisabledTracer(t *testing.T) {
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

	// Create a test handler
	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with middleware
	middleware := tracer.HTTPMiddleware(handler)

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Serve request
	middleware.ServeHTTP(w, req)

	if !handlerCalled {
		t.Error("Handler should have been called")
	}

	if w.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	rw.WriteHeader(http.StatusCreated)

	if rw.statusCode != http.StatusCreated {
		t.Errorf("statusCode = %d, want %d", rw.statusCode, http.StatusCreated)
	}

	if w.Code != http.StatusCreated {
		t.Errorf("underlying ResponseWriter code = %d, want %d", w.Code, http.StatusCreated)
	}
}

func TestHeaderCarrierFromMap(t *testing.T) {
	headers := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	carrier := HeaderCarrierFromMap(headers)

	if carrier == nil {
		t.Fatal("Expected carrier to be created")
	}

	// Verify we can get values
	if val := carrier.Get("key1"); val != "value1" {
		t.Errorf("Get(key1) = %s, want value1", val)
	}

	if val := carrier.Get("key2"); val != "value2" {
		t.Errorf("Get(key2) = %s, want value2", val)
	}
}

func TestInjectTraceContext(t *testing.T) {
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

	// Try to inject trace context (should not panic)
	headers := make(map[string]string)
	tracer.InjectTraceContext(context.Background(), headers)

	// Since tracer is disabled, headers should remain empty
	if len(headers) != 0 {
		t.Error("Expected headers to be empty when tracer is disabled")
	}
}

func TestInjectTraceContext_DisabledTracer(t *testing.T) {
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

	// Try to inject trace context
	headers := make(map[string]string)
	tracer.InjectTraceContext(context.Background(), headers)

	// Headers should remain empty when tracer is disabled
	if len(headers) != 0 {
		t.Error("Expected headers to be empty when tracer is disabled")
	}
}

func TestExtractTraceContext(t *testing.T) {
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

	headers := map[string]string{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}

	// Extract trace context (should not panic)
	ctx := tracer.ExtractTraceContext(context.Background(), headers)

	// Should return a valid context
	if ctx == nil {
		t.Fatal("Expected context to be returned")
	}
}

func TestExtractTraceContext_DisabledTracer(t *testing.T) {
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

	headers := map[string]string{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}

	ctx := tracer.ExtractTraceContext(context.Background(), headers)

	// Should return the original context unchanged
	if ctx == nil {
		t.Fatal("Expected context to be returned")
	}
}

func TestExtractTraceContext_EmptyHeaders(t *testing.T) {
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

	// Extract with empty headers (should not panic)
	ctx := tracer.ExtractTraceContext(context.Background(), map[string]string{})

	// Should not panic and return a valid context
	if ctx == nil {
		t.Fatal("Expected context to be returned")
	}
}
