package tracing

import (
	"context"
	"errors"
	"testing"
)

func createTestTracer() *Tracer {
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
	tracer, _ := New(config)
	return tracer
}

func TestTracer_InstrumentedOperation_Success(t *testing.T) {
	tracer := createTestTracer()
	ctx := context.Background()

	called := false
	err := tracer.InstrumentedOperation(ctx, "test-operation", func(ctx context.Context) error {
		called = true
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if !called {
		t.Error("Expected function to be called")
	}
}

func TestTracer_InstrumentedOperation_Error(t *testing.T) {
	tracer := createTestTracer()
	ctx := context.Background()

	expectedErr := errors.New("test error")
	err := tracer.InstrumentedOperation(ctx, "test-operation", func(ctx context.Context) error {
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestTracer_TraceProduceMessage(t *testing.T) {
	tracer := createTestTracer()
	ctx := context.Background()

	key := []byte("test-key")
	value := []byte("test-value")

	newCtx, span := tracer.TraceProduceMessage(ctx, "test-topic", 0, key, value)

	if newCtx == nil {
		t.Error("Expected context to be returned")
	}

	if span == nil {
		t.Error("Expected span to be returned")
	}

	// Clean up
	span.End()
}

func TestTracer_TraceConsumeMessage(t *testing.T) {
	tracer := createTestTracer()
	ctx := context.Background()

	newCtx, span := tracer.TraceConsumeMessage(ctx, "test-topic", 0, 100)

	if newCtx == nil {
		t.Error("Expected context to be returned")
	}

	if span == nil {
		t.Error("Expected span to be returned")
	}

	// Clean up
	span.End()
}

func TestTracer_TraceStorageOperation(t *testing.T) {
	tracer := createTestTracer()
	ctx := context.Background()

	newCtx, span := tracer.TraceStorageOperation(ctx, "write", "wal")

	if newCtx == nil {
		t.Error("Expected context to be returned")
	}

	if span == nil {
		t.Error("Expected span to be returned")
	}

	// Clean up
	span.End()
}

func TestTracer_TraceRaftOperation(t *testing.T) {
	tracer := createTestTracer()
	ctx := context.Background()

	newCtx, span := tracer.TraceRaftOperation(ctx, "append", 1, 100)

	if newCtx == nil {
		t.Error("Expected context to be returned")
	}

	if span == nil {
		t.Error("Expected span to be returned")
	}

	// Clean up
	span.End()
}

func TestTracer_TraceNetworkRequest(t *testing.T) {
	tracer := createTestTracer()
	ctx := context.Background()

	newCtx, span := tracer.TraceNetworkRequest(ctx, "fetch", "localhost:9092")

	if newCtx == nil {
		t.Error("Expected context to be returned")
	}

	if span == nil {
		t.Error("Expected span to be returned")
	}

	// Clean up
	span.End()
}

func TestTracer_RecordMetric(t *testing.T) {
	tracer := createTestTracer()
	ctx := context.Background()

	// Should not panic
	tracer.RecordMetric(ctx, "test-metric", 100, "count")
}

func TestTracer_MeasureDuration_Success(t *testing.T) {
	tracer := createTestTracer()
	ctx := context.Background()

	called := false
	err := tracer.MeasureDuration(ctx, "test-operation", func() error {
		called = true
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if !called {
		t.Error("Expected function to be called")
	}
}

func TestTracer_MeasureDuration_Error(t *testing.T) {
	tracer := createTestTracer()
	ctx := context.Background()

	expectedErr := errors.New("operation failed")
	err := tracer.MeasureDuration(ctx, "test-operation", func() error {
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestTracer_TraceWithRetry_Success(t *testing.T) {
	tracer := createTestTracer()
	ctx := context.Background()

	attempts := 0
	err := tracer.TraceWithRetry(ctx, "test-operation", 3, func(ctx context.Context, attempt int) error {
		attempts++
		if attempt == 2 {
			return nil
		}
		return errors.New("temporary error")
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestTracer_TraceWithRetry_AllFailed(t *testing.T) {
	tracer := createTestTracer()
	ctx := context.Background()

	attempts := 0
	expectedErr := errors.New("persistent error")
	err := tracer.TraceWithRetry(ctx, "test-operation", 3, func(ctx context.Context, attempt int) error {
		attempts++
		return expectedErr
	})

	if err == nil {
		t.Error("Expected error after all retries")
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestTracer_TraceWithRetry_ImmediateSuccess(t *testing.T) {
	tracer := createTestTracer()
	ctx := context.Background()

	attempts := 0
	err := tracer.TraceWithRetry(ctx, "test-operation", 3, func(ctx context.Context, attempt int) error {
		attempts++
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", attempts)
	}
}
