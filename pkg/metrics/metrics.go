package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// MetricType represents the type of metric
type MetricType int

const (
	// MetricTypeCounter is a cumulative metric that only increases
	MetricTypeCounter MetricType = iota
	// MetricTypeGauge is a metric that can go up or down
	MetricTypeGauge
	// MetricTypeHistogram tracks distributions of values
	MetricTypeHistogram
	// MetricTypeSummary tracks summary statistics
	MetricTypeSummary
)

// String returns the string representation of the metric type
func (mt MetricType) String() string {
	switch mt {
	case MetricTypeCounter:
		return "counter"
	case MetricTypeGauge:
		return "gauge"
	case MetricTypeHistogram:
		return "histogram"
	case MetricTypeSummary:
		return "summary"
	default:
		return "unknown"
	}
}

// Metric represents a single metric
type Metric interface {
	Name() string
	Type() MetricType
	Help() string
	Value() interface{}
	Labels() map[string]string
	Reset()
}

// Counter is a cumulative metric that only increases
type Counter struct {
	name   string
	help   string
	labels map[string]string
	mu     sync.RWMutex
	value  uint64
}

// NewCounter creates a new counter metric
func NewCounter(name, help string, labels map[string]string) *Counter {
	if labels == nil {
		labels = make(map[string]string)
	}
	return &Counter{
		name:   name,
		help:   help,
		labels: labels,
	}
}

// Name returns the metric name
func (c *Counter) Name() string {
	return c.name
}

// Type returns the metric type
func (c *Counter) Type() MetricType {
	return MetricTypeCounter
}

// Help returns the help text
func (c *Counter) Help() string {
	return c.help
}

// Value returns the current value
func (c *Counter) Value() interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.value
}

// Labels returns the metric labels
func (c *Counter) Labels() map[string]string {
	return c.labels
}

// Inc increments the counter by 1
func (c *Counter) Inc() {
	c.Add(1)
}

// Add adds the given value to the counter
func (c *Counter) Add(delta uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value += delta
}

// Reset resets the counter to 0
func (c *Counter) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value = 0
}

// Gauge is a metric that can go up or down
type Gauge struct {
	name   string
	help   string
	labels map[string]string
	mu     sync.RWMutex
	value  float64
}

// NewGauge creates a new gauge metric
func NewGauge(name, help string, labels map[string]string) *Gauge {
	if labels == nil {
		labels = make(map[string]string)
	}
	return &Gauge{
		name:   name,
		help:   help,
		labels: labels,
	}
}

// Name returns the metric name
func (g *Gauge) Name() string {
	return g.name
}

// Type returns the metric type
func (g *Gauge) Type() MetricType {
	return MetricTypeGauge
}

// Help returns the help text
func (g *Gauge) Help() string {
	return g.help
}

// Value returns the current value
func (g *Gauge) Value() interface{} {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.value
}

// Labels returns the metric labels
func (g *Gauge) Labels() map[string]string {
	return g.labels
}

// Set sets the gauge to the given value
func (g *Gauge) Set(value float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value = value
}

// Inc increments the gauge by 1
func (g *Gauge) Inc() {
	g.Add(1)
}

// Dec decrements the gauge by 1
func (g *Gauge) Dec() {
	g.Sub(1)
}

// Add adds the given value to the gauge
func (g *Gauge) Add(delta float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value += delta
}

// Sub subtracts the given value from the gauge
func (g *Gauge) Sub(delta float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value -= delta
}

// Reset resets the gauge to 0
func (g *Gauge) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value = 0
}

// Histogram tracks the distribution of values
type Histogram struct {
	name    string
	help    string
	labels  map[string]string
	buckets []float64
	mu      sync.RWMutex
	counts  []uint64
	sum     float64
	count   uint64
}

// NewHistogram creates a new histogram metric
func NewHistogram(name, help string, labels map[string]string, buckets []float64) *Histogram {
	if labels == nil {
		labels = make(map[string]string)
	}
	if buckets == nil {
		// Default buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
		buckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	}
	sort.Float64s(buckets)
	return &Histogram{
		name:    name,
		help:    help,
		labels:  labels,
		buckets: buckets,
		counts:  make([]uint64, len(buckets)+1), // +1 for +Inf bucket
	}
}

// Name returns the metric name
func (h *Histogram) Name() string {
	return h.name
}

// Type returns the metric type
func (h *Histogram) Type() MetricType {
	return MetricTypeHistogram
}

// Help returns the help text
func (h *Histogram) Help() string {
	return h.help
}

// Value returns the histogram data
func (h *Histogram) Value() interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return map[string]interface{}{
		"buckets": h.buckets,
		"counts":  h.counts,
		"sum":     h.sum,
		"count":   h.count,
	}
}

// Labels returns the metric labels
func (h *Histogram) Labels() map[string]string {
	return h.labels
}

// Observe adds a single observation to the histogram
func (h *Histogram) Observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.sum += value
	h.count++

	// Find the appropriate bucket
	for i, bucket := range h.buckets {
		if value <= bucket {
			h.counts[i]++
			return
		}
	}
	// Value exceeds all buckets, put in +Inf bucket
	h.counts[len(h.buckets)]++
}

// ObserveDuration observes a duration in seconds
func (h *Histogram) ObserveDuration(start time.Time) {
	h.Observe(time.Since(start).Seconds())
}

// Reset resets the histogram
func (h *Histogram) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.counts = make([]uint64, len(h.buckets)+1)
	h.sum = 0
	h.count = 0
}

// Registry manages a collection of metrics
type Registry struct {
	mu      sync.RWMutex
	metrics map[string]Metric
}

// NewRegistry creates a new metrics registry
func NewRegistry() *Registry {
	return &Registry{
		metrics: make(map[string]Metric),
	}
}

// Register registers a metric
func (r *Registry) Register(metric Metric) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := metricKey(metric.Name(), metric.Labels())
	if _, exists := r.metrics[key]; exists {
		return fmt.Errorf("metric already registered: %s", key)
	}

	r.metrics[key] = metric
	return nil
}

// Unregister removes a metric from the registry
func (r *Registry) Unregister(name string, labels map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := metricKey(name, labels)
	delete(r.metrics, key)
}

// Get retrieves a metric by name and labels
func (r *Registry) Get(name string, labels map[string]string) (Metric, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := metricKey(name, labels)
	metric, exists := r.metrics[key]
	return metric, exists
}

// GetOrCreateCounter gets or creates a counter metric
func (r *Registry) GetOrCreateCounter(name, help string, labels map[string]string) *Counter {
	if metric, exists := r.Get(name, labels); exists {
		if counter, ok := metric.(*Counter); ok {
			return counter
		}
	}
	counter := NewCounter(name, help, labels)
	_ = r.Register(counter)
	return counter
}

// GetOrCreateGauge gets or creates a gauge metric
func (r *Registry) GetOrCreateGauge(name, help string, labels map[string]string) *Gauge {
	if metric, exists := r.Get(name, labels); exists {
		if gauge, ok := metric.(*Gauge); ok {
			return gauge
		}
	}
	gauge := NewGauge(name, help, labels)
	_ = r.Register(gauge)
	return gauge
}

// GetOrCreateHistogram gets or creates a histogram metric
func (r *Registry) GetOrCreateHistogram(name, help string, labels map[string]string, buckets []float64) *Histogram {
	if metric, exists := r.Get(name, labels); exists {
		if histogram, ok := metric.(*Histogram); ok {
			return histogram
		}
	}
	histogram := NewHistogram(name, help, labels, buckets)
	_ = r.Register(histogram)
	return histogram
}

// All returns all registered metrics
func (r *Registry) All() []Metric {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metrics := make([]Metric, 0, len(r.metrics))
	for _, metric := range r.metrics {
		metrics = append(metrics, metric)
	}
	return metrics
}

// Reset resets all metrics
func (r *Registry) Reset() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, metric := range r.metrics {
		metric.Reset()
	}
}

// Clear removes all metrics
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics = make(map[string]Metric)
}

// metricKey generates a unique key for a metric
func metricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}

	// Sort labels for consistent keys
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(labels[k])
	}
	sb.WriteString("}")
	return sb.String()
}

// Global default registry
var defaultRegistry = NewRegistry()

// DefaultRegistry returns the default global registry
func DefaultRegistry() *Registry {
	return defaultRegistry
}

// Register registers a metric in the default registry
func Register(metric Metric) error {
	return defaultRegistry.Register(metric)
}

// Unregister removes a metric from the default registry
func Unregister(name string, labels map[string]string) {
	defaultRegistry.Unregister(name, labels)
}
