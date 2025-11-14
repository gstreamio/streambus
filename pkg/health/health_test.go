package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_RegisterAndCheckAll(t *testing.T) {
	registry := NewRegistry()

	// Register some checkers
	healthy := NewSimpleChecker("test1", func(ctx context.Context) Check {
		return Check{Status: StatusHealthy, Message: "all good"}
	})

	degraded := NewSimpleChecker("test2", func(ctx context.Context) Check {
		return Check{Status: StatusDegraded, Message: "degraded"}
	})

	registry.Register(healthy)
	registry.Register(degraded)

	// Check all
	ctx := context.Background()
	response := registry.CheckAll(ctx)

	assert.Equal(t, StatusDegraded, response.Status) // Overall should be degraded
	assert.Len(t, response.Checks, 2)
	assert.Equal(t, StatusHealthy, response.Checks["test1"].Status)
	assert.Equal(t, StatusDegraded, response.Checks["test2"].Status)
}

func TestRegistry_CheckOne(t *testing.T) {
	registry := NewRegistry()

	checker := NewSimpleChecker("test", func(ctx context.Context) Check {
		return Check{Status: StatusHealthy, Message: "ok"}
	})
	registry.Register(checker)

	ctx := context.Background()
	check, err := registry.CheckOne(ctx, "test")

	require.NoError(t, err)
	assert.Equal(t, StatusHealthy, check.Status)
}

func TestRegistry_CheckOne_NotFound(t *testing.T) {
	registry := NewRegistry()
	ctx := context.Background()

	_, err := registry.CheckOne(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestRegistry_Unregister(t *testing.T) {
	registry := NewRegistry()

	checker := NewSimpleChecker("test", func(ctx context.Context) Check {
		return Check{Status: StatusHealthy}
	})
	registry.Register(checker)

	// Should exist
	ctx := context.Background()
	_, err := registry.CheckOne(ctx, "test")
	require.NoError(t, err)

	// Unregister
	registry.Unregister("test")

	// Should not exist
	_, err = registry.CheckOne(ctx, "test")
	assert.Error(t, err)
}

func TestRegistry_AggregateStatus(t *testing.T) {
	tests := []struct {
		name     string
		checks   map[string]Check
		expected Status
	}{
		{
			name:     "empty",
			checks:   map[string]Check{},
			expected: StatusUnknown,
		},
		{
			name: "all healthy",
			checks: map[string]Check{
				"test1": {Status: StatusHealthy},
				"test2": {Status: StatusHealthy},
			},
			expected: StatusHealthy,
		},
		{
			name: "one degraded",
			checks: map[string]Check{
				"test1": {Status: StatusHealthy},
				"test2": {Status: StatusDegraded},
			},
			expected: StatusDegraded,
		},
		{
			name: "one unhealthy",
			checks: map[string]Check{
				"test1": {Status: StatusHealthy},
				"test2": {Status: StatusUnhealthy},
			},
			expected: StatusUnhealthy,
		},
		{
			name: "unknown status",
			checks: map[string]Check{
				"test1": {Status: StatusHealthy},
				"test2": {Status: StatusUnknown},
			},
			expected: StatusDegraded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			result := registry.aggregateStatus(tt.checks)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRegistry_Timeout(t *testing.T) {
	registry := NewRegistry()
	registry.SetTimeout(100 * time.Millisecond)

	// Register a slow checker
	slow := NewSimpleChecker("slow", func(ctx context.Context) Check {
		select {
		case <-time.After(1 * time.Second):
			return Check{Status: StatusHealthy}
		case <-ctx.Done():
			return Check{Status: StatusUnhealthy, Message: "timeout"}
		}
	})
	registry.Register(slow)

	ctx := context.Background()
	start := time.Now()
	response := registry.CheckAll(ctx)
	duration := time.Since(start)

	// Should timeout around 100ms, not wait full 1s
	assert.Less(t, duration, 500*time.Millisecond)
	assert.Len(t, response.Checks, 1)
}

func TestLivenessCheck(t *testing.T) {
	check := LivenessCheck()

	assert.Equal(t, "liveness", check.Name)
	assert.Equal(t, StatusHealthy, check.Status)
	assert.Equal(t, "process is alive", check.Message)
}

func TestRegistry_ReadinessCheck(t *testing.T) {
	registry := NewRegistry()

	// Add healthy checker
	healthy := NewSimpleChecker("test", func(ctx context.Context) Check {
		return Check{Status: StatusHealthy}
	})
	registry.Register(healthy)

	ctx := context.Background()
	check := registry.ReadinessCheck(ctx)

	assert.Equal(t, "readiness", check.Name)
	assert.Equal(t, StatusHealthy, check.Status)
	assert.Contains(t, check.Message, "ready")
}

func TestSimpleChecker(t *testing.T) {
	callCount := 0
	checker := NewSimpleChecker("test", func(ctx context.Context) Check {
		callCount++
		return Check{Status: StatusHealthy, Message: "ok"}
	})

	assert.Equal(t, "test", checker.Name())

	ctx := context.Background()
	check := checker.Check(ctx)

	assert.Equal(t, "test", check.Name)
	assert.Equal(t, StatusHealthy, check.Status)
	assert.Equal(t, 1, callCount)
	assert.Greater(t, check.Duration, time.Duration(0))
	assert.False(t, check.Timestamp.IsZero())
}

func TestPeriodicChecker(t *testing.T) {
	var mu sync.Mutex
	callCount := 0
	underlying := NewSimpleChecker("test", func(ctx context.Context) Check {
		mu.Lock()
		defer mu.Unlock()
		callCount++
		return Check{Status: StatusHealthy, Message: "ok"}
	})

	periodic := NewPeriodicChecker(underlying, 50*time.Millisecond)
	assert.Equal(t, "test", periodic.Name())

	// Start periodic checking
	periodic.Start()
	defer periodic.Stop()

	// Wait for a few checks
	time.Sleep(150 * time.Millisecond)

	// Should have run at least 2 times (initial + 2 intervals)
	mu.Lock()
	count := callCount
	mu.Unlock()
	assert.GreaterOrEqual(t, count, 2)

	// Check should return cached result
	ctx := context.Background()
	check := periodic.Check(ctx)
	assert.Equal(t, StatusHealthy, check.Status)

	// Call count shouldn't increase when getting cached result
	previousCount := callCount
	periodic.Check(ctx)
	assert.Equal(t, previousCount, callCount)
}

func TestPeriodicChecker_BeforeFirstRun(t *testing.T) {
	checker := NewSimpleChecker("test", func(ctx context.Context) Check {
		return Check{Status: StatusHealthy}
	})

	periodic := NewPeriodicChecker(checker, 1*time.Hour)

	ctx := context.Background()
	check := periodic.Check(ctx)

	assert.Equal(t, StatusUnknown, check.Status)
	assert.Contains(t, check.Message, "no health check has run")
}

func TestHTTPHandler_Liveness(t *testing.T) {
	registry := NewRegistry()
	handler := NewHandler(registry)

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()

	handler.handleLiveness(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var check Check
	err := json.NewDecoder(w.Body).Decode(&check)
	require.NoError(t, err)
	assert.Equal(t, StatusHealthy, check.Status)
}

func TestHTTPHandler_Readiness(t *testing.T) {
	registry := NewRegistry()

	// Add a healthy checker
	healthy := NewSimpleChecker("test", func(ctx context.Context) Check {
		return Check{Status: StatusHealthy}
	})
	registry.Register(healthy)

	handler := NewHandler(registry)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	handler.handleReadiness(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var check Check
	err := json.NewDecoder(w.Body).Decode(&check)
	require.NoError(t, err)
	assert.Equal(t, StatusHealthy, check.Status)
}

func TestHTTPHandler_Health(t *testing.T) {
	registry := NewRegistry()

	// Add checkers
	healthy := NewSimpleChecker("healthy", func(ctx context.Context) Check {
		return Check{Status: StatusHealthy, Message: "ok"}
	})
	degraded := NewSimpleChecker("degraded", func(ctx context.Context) Check {
		return Check{Status: StatusDegraded, Message: "slow"}
	})

	registry.Register(healthy)
	registry.Register(degraded)

	handler := NewHandler(registry)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.handleHealth(w, req)

	assert.Equal(t, http.StatusOK, w.Code) // Degraded still returns 200

	var response Response
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, StatusDegraded, response.Status)
	assert.Len(t, response.Checks, 2)
}

func TestHTTPHandler_MethodNotAllowed(t *testing.T) {
	registry := NewRegistry()
	handler := NewHandler(registry)

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	w := httptest.NewRecorder()

	handler.handleHealth(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHTTPHandler_StatusCodes(t *testing.T) {
	tests := []struct {
		status       Status
		expectedCode int
	}{
		{StatusHealthy, http.StatusOK},
		{StatusDegraded, http.StatusOK},
		{StatusUnhealthy, http.StatusServiceUnavailable},
		{StatusUnknown, http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			handler := &Handler{}
			code := handler.statusCodeFromHealth(tt.status)
			assert.Equal(t, tt.expectedCode, code)
		})
	}
}

func TestStatus_String(t *testing.T) {
	assert.Equal(t, "healthy", string(StatusHealthy))
	assert.Equal(t, "degraded", string(StatusDegraded))
	assert.Equal(t, "unhealthy", string(StatusUnhealthy))
	assert.Equal(t, "unknown", string(StatusUnknown))
}

func TestResponse_JSON(t *testing.T) {
	response := &Response{
		Status:    StatusHealthy,
		Timestamp: time.Now(),
		Checks: map[string]Check{
			"test": {
				Name:      "test",
				Status:    StatusHealthy,
				Duration:  10 * time.Millisecond,
				Timestamp: time.Now(),
			},
		},
	}

	data, err := json.Marshal(response)
	require.NoError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "healthy", decoded["status"])
	assert.Contains(t, decoded, "total_duration_ms")
	assert.Contains(t, decoded, "checks")
}

func BenchmarkRegistry_CheckAll(b *testing.B) {
	registry := NewRegistry()

	for i := 0; i < 10; i++ {
		checker := NewSimpleChecker("test", func(ctx context.Context) Check {
			return Check{Status: StatusHealthy}
		})
		registry.Register(checker)
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.CheckAll(ctx)
	}
}

func BenchmarkSimpleChecker_Check(b *testing.B) {
	checker := NewSimpleChecker("test", func(ctx context.Context) Check {
		return Check{Status: StatusHealthy}
	})

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.Check(ctx)
	}
}
