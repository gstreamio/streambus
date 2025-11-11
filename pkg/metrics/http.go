package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// Handler provides HTTP handlers for metrics endpoints
type Handler struct {
	registry *Registry
}

// NewHandler creates a new HTTP handler for metrics
func NewHandler(registry *Registry) *Handler {
	return &Handler{
		registry: registry,
	}
}

// ServeHTTP implements http.Handler interface
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	// Get all metrics
	metrics := h.registry.All()

	// Group metrics by name
	metricsByName := make(map[string][]Metric)
	for _, metric := range metrics {
		metricsByName[metric.Name()] = append(metricsByName[metric.Name()], metric)
	}

	// Sort metric names for consistent output
	names := make([]string, 0, len(metricsByName))
	for name := range metricsByName {
		names = append(names, name)
	}
	sort.Strings(names)

	// Write metrics in Prometheus format
	for _, name := range names {
		metricsGroup := metricsByName[name]
		if len(metricsGroup) == 0 {
			continue
		}

		// Write HELP and TYPE once per metric name
		firstMetric := metricsGroup[0]
		fmt.Fprintf(w, "# HELP %s %s\n", firstMetric.Name(), firstMetric.Help())
		fmt.Fprintf(w, "# TYPE %s %s\n", firstMetric.Name(), firstMetric.Type())

		// Write metric values
		for _, metric := range metricsGroup {
			h.writeMetric(w, metric)
		}
		fmt.Fprintln(w) // Blank line between metrics
	}
}

// writeMetric writes a single metric in Prometheus format
func (h *Handler) writeMetric(w http.ResponseWriter, metric Metric) {
	name := metric.Name()
	labels := metric.Labels()
	labelsStr := formatLabels(labels)

	switch metric.Type() {
	case MetricTypeCounter:
		fmt.Fprintf(w, "%s%s %v\n", name, labelsStr, metric.Value())

	case MetricTypeGauge:
		fmt.Fprintf(w, "%s%s %v\n", name, labelsStr, metric.Value())

	case MetricTypeHistogram:
		h.writeHistogram(w, name, labels, metric)

	case MetricTypeSummary:
		// Summary not yet implemented
		fmt.Fprintf(w, "%s%s %v\n", name, labelsStr, metric.Value())
	}
}

// writeHistogram writes histogram metrics in Prometheus format
func (h *Handler) writeHistogram(w http.ResponseWriter, name string, baseLabels map[string]string, metric Metric) {
	value := metric.Value()
	data, ok := value.(map[string]interface{})
	if !ok {
		return
	}

	buckets, _ := data["buckets"].([]float64)
	counts, _ := data["counts"].([]uint64)
	sum, _ := data["sum"].(float64)
	count, _ := data["count"].(uint64)

	// Write bucket counts (cumulative)
	cumulative := uint64(0)
	for i, bucket := range buckets {
		if i < len(counts) {
			cumulative += counts[i]
		}
		bucketLabels := copyLabels(baseLabels)
		bucketLabels["le"] = formatFloat(bucket)
		fmt.Fprintf(w, "%s_bucket%s %d\n", name, formatLabels(bucketLabels), cumulative)
	}

	// Write +Inf bucket
	if len(counts) > len(buckets) {
		cumulative += counts[len(buckets)]
	}
	infLabels := copyLabels(baseLabels)
	infLabels["le"] = "+Inf"
	fmt.Fprintf(w, "%s_bucket%s %d\n", name, formatLabels(infLabels), cumulative)

	// Write sum and count
	fmt.Fprintf(w, "%s_sum%s %v\n", name, formatLabels(baseLabels), sum)
	fmt.Fprintf(w, "%s_count%s %d\n", name, formatLabels(baseLabels), count)
}

// formatLabels formats labels for Prometheus output
func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	// Sort keys for consistent output
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(k)
		sb.WriteString("=\"")
		sb.WriteString(escapeString(labels[k]))
		sb.WriteString("\"")
	}
	sb.WriteString("}")
	return sb.String()
}

// escapeString escapes special characters in label values
func escapeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// formatFloat formats a float64 for Prometheus output
func formatFloat(f float64) string {
	return fmt.Sprintf("%g", f)
}

// copyLabels creates a copy of a label map
func copyLabels(labels map[string]string) map[string]string {
	result := make(map[string]string, len(labels))
	for k, v := range labels {
		result[k] = v
	}
	return result
}

// RegisterRoutes registers metrics routes on the given mux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/metrics", h)
}
