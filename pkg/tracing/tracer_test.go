package tracing

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "disabled tracer",
			config: &Config{
				Enabled:     false,
				ServiceName: "test-service",
			},
			wantErr: false,
		},
		{
			name: "invalid exporter",
			config: &Config{
				Enabled:     true,
				ServiceName: "test-service",
				Exporter: ExporterConfig{
					Type: "invalid",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracer, err := New(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && tracer == nil {
				t.Error("New() returned nil tracer without error")
			}
		})
	}
}

func TestTracer_Start(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	ctx := context.Background()
	ctx, span := tracer.Start(ctx, "test-operation")
	defer span.End()

	if ctx == nil {
		t.Error("Start() returned nil context")
	}
	if span == nil {
		t.Error("Start() returned nil span")
	}
}

func TestTracer_StartWithAttributes(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	ctx := context.Background()
	attrs := map[string]interface{}{
		"string_attr": "value",
		"int_attr":    42,
		"bool_attr":   true,
	}

	ctx, span := tracer.StartWithAttributes(ctx, "test-operation", attrs)
	defer span.End()

	if ctx == nil {
		t.Error("StartWithAttributes() returned nil context")
	}
	if span == nil {
		t.Error("StartWithAttributes() returned nil span")
	}
}

func TestTracer_InstrumentedOperation(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	tests := []struct {
		name    string
		fn      func(context.Context) error
		wantErr bool
	}{
		{
			name: "successful operation",
			fn: func(ctx context.Context) error {
				return nil
			},
			wantErr: false,
		},
		{
			name: "failed operation",
			fn: func(ctx context.Context) error {
				return errors.New("test error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tracer.InstrumentedOperation(context.Background(), "test-op", tt.fn)
			if (err != nil) != tt.wantErr {
				t.Errorf("InstrumentedOperation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTracer_Inject_Extract(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	ctx := context.Background()
	headers := make(map[string]string)

	// Inject trace context
	tracer.InjectTraceContext(ctx, headers)

	// Extract trace context
	newCtx := tracer.ExtractTraceContext(ctx, headers)

	if newCtx == nil {
		t.Error("ExtractTraceContext() returned nil context")
	}
}

func TestTracer_TraceProduceMessage(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	ctx := context.Background()
	ctx, span := tracer.TraceProduceMessage(ctx, "test-topic", 0, []byte("key"), []byte("value"))
	defer span.End()

	if ctx == nil {
		t.Error("TraceProduceMessage() returned nil context")
	}
	if span == nil {
		t.Error("TraceProduceMessage() returned nil span")
	}
}

func TestTracer_TraceConsumeMessage(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	ctx := context.Background()
	ctx, span := tracer.TraceConsumeMessage(ctx, "test-topic", 0, 100)
	defer span.End()

	if ctx == nil {
		t.Error("TraceConsumeMessage() returned nil context")
	}
	if span == nil {
		t.Error("TraceConsumeMessage() returned nil span")
	}
}

func TestTracer_MeasureDuration(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	ctx := context.Background()
	err = tracer.MeasureDuration(ctx, "test-operation", func() error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	if err != nil {
		t.Errorf("MeasureDuration() unexpected error: %v", err)
	}
}

func TestConvertAttributes(t *testing.T) {
	attrs := map[string]interface{}{
		"string":  "value",
		"int":     42,
		"int64":   int64(100),
		"float64": 3.14,
		"bool":    true,
		"slice":   []string{"a", "b", "c"},
		"other":   struct{}{},
	}

	result := convertAttributes(attrs)

	if len(result) != len(attrs) {
		t.Errorf("convertAttributes() returned %d attributes, expected %d", len(result), len(attrs))
	}
}

func TestTracer_TraceWithRetry(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	tests := []struct {
		name       string
		maxRetries int
		fn         func(ctx context.Context, attempt int) error
		wantErr    bool
	}{
		{
			name:       "succeeds on first attempt",
			maxRetries: 3,
			fn: func(ctx context.Context, attempt int) error {
				return nil
			},
			wantErr: false,
		},
		{
			name:       "succeeds on second attempt",
			maxRetries: 3,
			fn: func(ctx context.Context, attempt int) error {
				if attempt == 1 {
					return errors.New("temporary error")
				}
				return nil
			},
			wantErr: false,
		},
		{
			name:       "fails all attempts",
			maxRetries: 3,
			fn: func(ctx context.Context, attempt int) error {
				return errors.New("persistent error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tracer.TraceWithRetry(context.Background(), "test-operation", tt.maxRetries, tt.fn)
			if (err != nil) != tt.wantErr {
				t.Errorf("TraceWithRetry() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTracer_AddEvent(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	ctx := context.Background()
	ctx, _ = tracer.Start(ctx, "test-operation")

	tracer.AddEvent(ctx, "test-event", map[string]interface{}{
		"key": "value",
	})

	// No return value to check, but function should not panic
}

func TestTracer_RecordError(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	ctx := context.Background()
	ctx, _ = tracer.Start(ctx, "test-operation")

	testErr := errors.New("test error")
	tracer.RecordError(ctx, testErr, map[string]interface{}{
		"error.code": 500,
	})

	// No return value to check, but function should not panic
}

func TestTracer_SetAttributes(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	ctx := context.Background()
	ctx, _ = tracer.Start(ctx, "test-operation")

	tracer.SetAttributes(ctx, map[string]interface{}{
		"attr1": "value1",
		"attr2": 42,
	})

	// No return value to check, but function should not panic
}

func TestTracer_Shutdown(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	err = tracer.Shutdown(context.Background())
	if err != nil {
		t.Errorf("Shutdown() unexpected error: %v", err)
	}
}

func TestTracer_Extract(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	headers := map[string]string{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}

	ctx := tracer.ExtractTraceContext(context.Background(), headers)
	if ctx == nil {
		t.Error("ExtractTraceContext() returned nil context")
	}
}

func TestTracer_Inject(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	ctx := context.Background()
	ctx, _ = tracer.Start(ctx, "test-operation")

	headers := make(map[string]string)
	tracer.InjectTraceContext(ctx, headers)

	// No specific assertion, but function should not panic
}

func TestTracer_IsEnabled(t *testing.T) {
	// Test disabled tracer
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
	})
	if err != nil {
		t.Fatalf("Failed to create disabled tracer: %v", err)
	}

	if tracer.IsEnabled() != false {
		t.Errorf("IsEnabled() = %v, want false", tracer.IsEnabled())
	}
}

func TestTracer_TraceStorageOperation(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	ctx := context.Background()
	ctx, span := tracer.TraceStorageOperation(ctx, "write", "messages")
	defer span.End()

	if ctx == nil {
		t.Error("TraceStorageOperation() returned nil context")
	}
	if span == nil {
		t.Error("TraceStorageOperation() returned nil span")
	}
}

func TestTracer_TraceRaftOperation(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	ctx := context.Background()
	ctx, span := tracer.TraceRaftOperation(ctx, "append", 5, 1234)
	defer span.End()

	if ctx == nil {
		t.Error("TraceRaftOperation() returned nil context")
	}
	if span == nil {
		t.Error("TraceRaftOperation() returned nil span")
	}
}

func TestTracer_TraceNetworkRequest(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	ctx := context.Background()
	ctx, span := tracer.TraceNetworkRequest(ctx, "GET", "localhost:9092")
	defer span.End()

	if ctx == nil {
		t.Error("TraceNetworkRequest() returned nil context")
	}
	if span == nil {
		t.Error("TraceNetworkRequest() returned nil span")
	}
}

func TestTracer_RecordMetric(t *testing.T) {
	tracer, err := New(&Config{
		Enabled:     false,
		ServiceName: "test-service",
	})
	if err != nil {
		t.Fatalf("Failed to create tracer: %v", err)
	}

	ctx := context.Background()
	ctx, span := tracer.Start(ctx, "test-operation")
	defer span.End()

	tracer.RecordMetric(ctx, "test.metric", 42.0, "bytes")

	// No return value to check, but function should not panic
}
