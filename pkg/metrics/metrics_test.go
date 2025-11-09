package metrics

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricType_String(t *testing.T) {
	tests := []struct {
		metricType MetricType
		expected   string
	}{
		{MetricTypeCounter, "counter"},
		{MetricTypeGauge, "gauge"},
		{MetricTypeHistogram, "histogram"},
		{MetricTypeSummary, "summary"},
		{MetricType(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.metricType.String())
		})
	}
}

func TestCounter(t *testing.T) {
	counter := NewCounter("test_counter", "A test counter", nil)

	assert.Equal(t, "test_counter", counter.Name())
	assert.Equal(t, MetricTypeCounter, counter.Type())
	assert.Equal(t, "A test counter", counter.Help())
	assert.Equal(t, uint64(0), counter.Value())

	// Test Inc
	counter.Inc()
	assert.Equal(t, uint64(1), counter.Value())

	// Test Add
	counter.Add(5)
	assert.Equal(t, uint64(6), counter.Value())

	// Test Reset
	counter.Reset()
	assert.Equal(t, uint64(0), counter.Value())
}

func TestCounter_WithLabels(t *testing.T) {
	labels := map[string]string{"method": "GET", "status": "200"}
	counter := NewCounter("http_requests_total", "Total HTTP requests", labels)

	assert.Equal(t, labels, counter.Labels())
	counter.Inc()
	assert.Equal(t, uint64(1), counter.Value())
}

func TestCounter_Concurrent(t *testing.T) {
	counter := NewCounter("concurrent_counter", "Concurrent test", nil)

	var wg sync.WaitGroup
	goroutines := 100
	increments := 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < increments; j++ {
				counter.Inc()
			}
		}()
	}

	wg.Wait()
	expected := uint64(goroutines * increments)
	assert.Equal(t, expected, counter.Value())
}

func TestGauge(t *testing.T) {
	gauge := NewGauge("test_gauge", "A test gauge", nil)

	assert.Equal(t, "test_gauge", gauge.Name())
	assert.Equal(t, MetricTypeGauge, gauge.Type())
	assert.Equal(t, "A test gauge", gauge.Help())
	assert.Equal(t, 0.0, gauge.Value())

	// Test Set
	gauge.Set(42.5)
	assert.Equal(t, 42.5, gauge.Value())

	// Test Inc
	gauge.Inc()
	assert.Equal(t, 43.5, gauge.Value())

	// Test Dec
	gauge.Dec()
	assert.Equal(t, 42.5, gauge.Value())

	// Test Add
	gauge.Add(7.5)
	assert.Equal(t, 50.0, gauge.Value())

	// Test Sub
	gauge.Sub(10.0)
	assert.Equal(t, 40.0, gauge.Value())

	// Test Reset
	gauge.Reset()
	assert.Equal(t, 0.0, gauge.Value())
}

func TestGauge_NegativeValues(t *testing.T) {
	gauge := NewGauge("temperature", "Temperature gauge", nil)

	gauge.Set(-10.5)
	assert.Equal(t, -10.5, gauge.Value())

	gauge.Sub(5.5)
	assert.Equal(t, -16.0, gauge.Value())
}

func TestHistogram(t *testing.T) {
	buckets := []float64{0.1, 0.5, 1.0, 5.0, 10.0}
	histogram := NewHistogram("test_histogram", "A test histogram", nil, buckets)

	assert.Equal(t, "test_histogram", histogram.Name())
	assert.Equal(t, MetricTypeHistogram, histogram.Type())
	assert.Equal(t, "A test histogram", histogram.Help())

	// Observe some values
	histogram.Observe(0.05)  // bucket 0
	histogram.Observe(0.2)   // bucket 1
	histogram.Observe(0.75)  // bucket 2
	histogram.Observe(3.0)   // bucket 3
	histogram.Observe(7.5)   // bucket 4
	histogram.Observe(15.0)  // +Inf bucket

	value := histogram.Value().(map[string]interface{})
	counts := value["counts"].([]uint64)

	// Check counts (non-cumulative per bucket)
	assert.Equal(t, uint64(1), counts[0]) // 0.05 <= 0.1
	assert.Equal(t, uint64(1), counts[1]) // 0.2 <= 0.5
	assert.Equal(t, uint64(1), counts[2]) // 0.75 <= 1.0
	assert.Equal(t, uint64(1), counts[3]) // 3.0 <= 5.0
	assert.Equal(t, uint64(1), counts[4]) // 7.5 <= 10.0
	assert.Equal(t, uint64(1), counts[5]) // 15.0 > 10.0 (+Inf)

	// Check sum and count
	assert.Equal(t, 26.5, value["sum"].(float64))
	assert.Equal(t, uint64(6), value["count"].(uint64))
}

func TestHistogram_DefaultBuckets(t *testing.T) {
	histogram := NewHistogram("default_buckets", "Test default buckets", nil, nil)

	value := histogram.Value().(map[string]interface{})
	buckets := value["buckets"].([]float64)

	// Should have default buckets
	assert.NotEmpty(t, buckets)
	assert.Contains(t, buckets, 0.001)
	assert.Contains(t, buckets, 0.01)
	assert.Contains(t, buckets, 0.1)
	assert.Contains(t, buckets, 1.0)
	assert.Contains(t, buckets, 10.0)
}

func TestHistogram_ObserveDuration(t *testing.T) {
	histogram := NewHistogram("request_duration", "Request duration", nil, nil)

	start := time.Now().Add(-100 * time.Millisecond)
	histogram.ObserveDuration(start)

	value := histogram.Value().(map[string]interface{})
	count := value["count"].(uint64)

	assert.Equal(t, uint64(1), count)
	assert.Greater(t, value["sum"].(float64), 0.0)
}

func TestHistogram_Reset(t *testing.T) {
	histogram := NewHistogram("test", "test", nil, []float64{1.0, 5.0})

	histogram.Observe(0.5)
	histogram.Observe(2.0)

	value := histogram.Value().(map[string]interface{})
	assert.Equal(t, uint64(2), value["count"].(uint64))

	histogram.Reset()

	value = histogram.Value().(map[string]interface{})
	assert.Equal(t, uint64(0), value["count"].(uint64))
	assert.Equal(t, 0.0, value["sum"].(float64))
}

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()

	counter := NewCounter("test_counter", "test", nil)
	err := registry.Register(counter)
	require.NoError(t, err)

	// Try to register duplicate
	err = registry.Register(counter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRegistry_Get(t *testing.T) {
	registry := NewRegistry()

	counter := NewCounter("test_counter", "test", map[string]string{"label": "value"})
	err := registry.Register(counter)
	require.NoError(t, err)

	// Get existing metric
	metric, exists := registry.Get("test_counter", map[string]string{"label": "value"})
	assert.True(t, exists)
	assert.Equal(t, counter, metric)

	// Get non-existing metric
	_, exists = registry.Get("nonexistent", nil)
	assert.False(t, exists)
}

func TestRegistry_Unregister(t *testing.T) {
	registry := NewRegistry()

	counter := NewCounter("test_counter", "test", nil)
	err := registry.Register(counter)
	require.NoError(t, err)

	// Unregister
	registry.Unregister("test_counter", nil)

	// Should not exist
	_, exists := registry.Get("test_counter", nil)
	assert.False(t, exists)
}

func TestRegistry_GetOrCreate(t *testing.T) {
	registry := NewRegistry()

	// Test Counter
	counter1 := registry.GetOrCreateCounter("requests", "Request count", nil)
	counter1.Inc()
	counter2 := registry.GetOrCreateCounter("requests", "Request count", nil)
	assert.Equal(t, counter1, counter2)
	assert.Equal(t, uint64(1), counter2.Value())

	// Test Gauge
	gauge1 := registry.GetOrCreateGauge("temperature", "Temperature", nil)
	gauge1.Set(25.5)
	gauge2 := registry.GetOrCreateGauge("temperature", "Temperature", nil)
	assert.Equal(t, gauge1, gauge2)
	assert.Equal(t, 25.5, gauge2.Value())

	// Test Histogram
	hist1 := registry.GetOrCreateHistogram("latency", "Latency", nil, []float64{1, 5, 10})
	hist1.Observe(2.5)
	hist2 := registry.GetOrCreateHistogram("latency", "Latency", nil, []float64{1, 5, 10})
	assert.Equal(t, hist1, hist2)
}

func TestRegistry_All(t *testing.T) {
	registry := NewRegistry()

	counter := NewCounter("counter", "test", nil)
	gauge := NewGauge("gauge", "test", nil)

	registry.Register(counter)
	registry.Register(gauge)

	all := registry.All()
	assert.Len(t, all, 2)
}

func TestRegistry_Reset(t *testing.T) {
	registry := NewRegistry()

	counter := NewCounter("counter", "test", nil)
	gauge := NewGauge("gauge", "test", nil)

	registry.Register(counter)
	registry.Register(gauge)

	counter.Add(5)
	gauge.Set(10.0)

	registry.Reset()

	assert.Equal(t, uint64(0), counter.Value())
	assert.Equal(t, 0.0, gauge.Value())
}

func TestRegistry_Clear(t *testing.T) {
	registry := NewRegistry()

	counter := NewCounter("counter", "test", nil)
	registry.Register(counter)

	assert.Len(t, registry.All(), 1)

	registry.Clear()

	assert.Len(t, registry.All(), 0)
}

func TestMetricKey(t *testing.T) {
	tests := []struct {
		name     string
		metric   string
		labels   map[string]string
		expected string
	}{
		{
			name:     "no labels",
			metric:   "test_metric",
			labels:   nil,
			expected: "test_metric",
		},
		{
			name:     "single label",
			metric:   "test_metric",
			labels:   map[string]string{"env": "prod"},
			expected: "test_metric{env=prod}",
		},
		{
			name:   "multiple labels",
			metric: "test_metric",
			labels: map[string]string{"env": "prod", "region": "us-west"},
			// Labels should be sorted
			expected: "test_metric{env=prod,region=us-west}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := metricKey(tt.metric, tt.labels)
			assert.Equal(t, tt.expected, key)
		})
	}
}

func TestDefaultRegistry(t *testing.T) {
	// Clear default registry for clean test
	defaultRegistry.Clear()

	counter := NewCounter("default_counter", "test", nil)
	err := Register(counter)
	require.NoError(t, err)

	metric, exists := defaultRegistry.Get("default_counter", nil)
	assert.True(t, exists)
	assert.Equal(t, counter, metric)

	Unregister("default_counter", nil)
	_, exists = defaultRegistry.Get("default_counter", nil)
	assert.False(t, exists)
}

func TestHTTPHandler_Counter(t *testing.T) {
	registry := NewRegistry()
	counter := NewCounter("http_requests_total", "Total HTTP requests", map[string]string{"method": "GET"})
	counter.Add(42)
	registry.Register(counter)

	handler := NewHandler(registry)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/plain; version=0.0.4; charset=utf-8", w.Header().Get("Content-Type"))

	body := w.Body.String()
	assert.Contains(t, body, "# HELP http_requests_total Total HTTP requests")
	assert.Contains(t, body, "# TYPE http_requests_total counter")
	assert.Contains(t, body, `http_requests_total{method="GET"} 42`)
}

func TestHTTPHandler_Gauge(t *testing.T) {
	registry := NewRegistry()
	gauge := NewGauge("temperature_celsius", "Current temperature", nil)
	gauge.Set(23.5)
	registry.Register(gauge)

	handler := NewHandler(registry)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	body := w.Body.String()
	assert.Contains(t, body, "# HELP temperature_celsius Current temperature")
	assert.Contains(t, body, "# TYPE temperature_celsius gauge")
	assert.Contains(t, body, "temperature_celsius 23.5")
}

func TestHTTPHandler_Histogram(t *testing.T) {
	registry := NewRegistry()
	histogram := NewHistogram("request_duration_seconds", "Request duration", nil, []float64{0.1, 0.5, 1.0})
	histogram.Observe(0.05)
	histogram.Observe(0.3)
	histogram.Observe(0.75)
	histogram.Observe(2.0)
	registry.Register(histogram)

	handler := NewHandler(registry)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	body := w.Body.String()
	assert.Contains(t, body, "# HELP request_duration_seconds Request duration")
	assert.Contains(t, body, "# TYPE request_duration_seconds histogram")
	assert.Contains(t, body, `request_duration_seconds_bucket{le="0.1"} 1`)
	assert.Contains(t, body, `request_duration_seconds_bucket{le="0.5"} 2`)
	assert.Contains(t, body, `request_duration_seconds_bucket{le="1"} 3`)
	assert.Contains(t, body, `request_duration_seconds_bucket{le="+Inf"} 4`)
	assert.Contains(t, body, `request_duration_seconds_sum`)
	assert.Contains(t, body, `request_duration_seconds_count 4`)
}

func TestHTTPHandler_MethodNotAllowed(t *testing.T) {
	registry := NewRegistry()
	handler := NewHandler(registry)

	req := httptest.NewRequest(http.MethodPost, "/metrics", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestFormatLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{
			name:     "no labels",
			labels:   nil,
			expected: "",
		},
		{
			name:     "single label",
			labels:   map[string]string{"method": "GET"},
			expected: `{method="GET"}`,
		},
		{
			name:     "multiple labels sorted",
			labels:   map[string]string{"method": "POST", "status": "200"},
			expected: `{method="POST",status="200"}`,
		},
		{
			name:     "label with special characters",
			labels:   map[string]string{"path": "/api/v1"},
			expected: `{path="/api/v1"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatLabels(tt.labels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEscapeString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`simple`, `simple`},
		{`with\backslash`, `with\\backslash`},
		{`with"quote`, `with\"quote`},
		{"with\nnewline", `with\nnewline`},
		{`all\three"types` + "\n", `all\\three\"types\n`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func BenchmarkCounter_Inc(b *testing.B) {
	counter := NewCounter("bench_counter", "benchmark", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.Inc()
	}
}

func BenchmarkGauge_Set(b *testing.B) {
	gauge := NewGauge("bench_gauge", "benchmark", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gauge.Set(float64(i))
	}
}

func BenchmarkHistogram_Observe(b *testing.B) {
	histogram := NewHistogram("bench_histogram", "benchmark", nil, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		histogram.Observe(float64(i) * 0.001)
	}
}

func BenchmarkRegistry_GetOrCreateCounter(b *testing.B) {
	registry := NewRegistry()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.GetOrCreateCounter("bench_counter", "benchmark", nil)
	}
}

func ExampleCounter() {
	counter := NewCounter("requests_total", "Total number of requests", map[string]string{"method": "GET"})

	counter.Inc()
	counter.Add(5)

	fmt.Println(counter.Value())
	// Output: 6
}

func ExampleGauge() {
	gauge := NewGauge("temperature_celsius", "Current temperature in Celsius", nil)

	gauge.Set(22.5)
	gauge.Add(2.5)

	fmt.Println(gauge.Value())
	// Output: 25
}

func ExampleHistogram() {
	histogram := NewHistogram("request_duration_seconds", "Request duration in seconds", nil, []float64{0.1, 0.5, 1.0})

	histogram.Observe(0.05)
	histogram.Observe(0.3)
	histogram.Observe(0.75)

	value := histogram.Value().(map[string]interface{})
	fmt.Println(value["count"])
	// Output: 3
}

func ExampleRegistry() {
	registry := NewRegistry()

	counter := registry.GetOrCreateCounter("operations_total", "Total operations", nil)
	counter.Inc()

	gauge := registry.GetOrCreateGauge("active_connections", "Active connections", nil)
	gauge.Set(42)

	fmt.Println(len(registry.All()))
	// Output: 2
}
