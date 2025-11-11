package metrics

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestPrometheusExporter_Export(t *testing.T) {
	registry := NewRegistry()

	// Create various metrics
	counter := registry.GetOrCreateCounter("test_counter", "Test counter", nil)
	counter.Inc()
	counter.Inc()

	gauge := registry.GetOrCreateGauge("test_gauge", "Test gauge", nil)
	gauge.Set(42.5)

	histogram := registry.GetOrCreateHistogram("test_histogram", "Test histogram", nil, []float64{0.1, 0.5, 1.0})
	histogram.Observe(0.25)
	histogram.Observe(0.75)
	histogram.Observe(1.5)

	// Create exporter and export
	exporter := NewPrometheusExporter(registry)
	var buf bytes.Buffer
	err := exporter.Export(&buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	output := buf.String()

	// Verify counter
	if !strings.Contains(output, "# HELP test_counter Test counter") {
		t.Error("Missing counter help text")
	}
	if !strings.Contains(output, "# TYPE test_counter counter") {
		t.Error("Missing counter type")
	}
	if !strings.Contains(output, "test_counter 2") {
		t.Error("Counter value incorrect")
	}

	// Verify gauge
	if !strings.Contains(output, "# HELP test_gauge Test gauge") {
		t.Error("Missing gauge help text")
	}
	if !strings.Contains(output, "# TYPE test_gauge gauge") {
		t.Error("Missing gauge type")
	}
	if !strings.Contains(output, "test_gauge 42.5") {
		t.Error("Gauge value incorrect")
	}

	// Verify histogram
	if !strings.Contains(output, "# HELP test_histogram Test histogram") {
		t.Error("Missing histogram help text")
	}
	if !strings.Contains(output, "# TYPE test_histogram histogram") {
		t.Error("Missing histogram type")
	}
	if !strings.Contains(output, "test_histogram_count 3") {
		t.Error("Histogram count incorrect")
	}
	if !strings.Contains(output, "test_histogram_sum") {
		t.Error("Missing histogram sum")
	}
}

func TestPrometheusExporter_ExportWithLabels(t *testing.T) {
	registry := NewRegistry()

	labels := map[string]string{
		"broker_id": "1",
		"topic":     "test-topic",
	}

	counter := registry.GetOrCreateCounter("messages_total", "Total messages", labels)
	counter.Inc()
	counter.Inc()

	exporter := NewPrometheusExporter(registry)
	var buf bytes.Buffer
	err := exporter.Export(&buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	output := buf.String()

	// Should contain labels in Prometheus format
	if !strings.Contains(output, `broker_id="1"`) {
		t.Error("Missing broker_id label")
	}
	if !strings.Contains(output, `topic="test-topic"`) {
		t.Error("Missing topic label")
	}
	if !strings.Contains(output, "messages_total") {
		t.Error("Missing metric name")
	}
}

func TestPrometheusExporter_SanitizeMetricName(t *testing.T) {
	registry := NewRegistry()

	// Metric name with invalid characters
	counter := registry.GetOrCreateCounter("invalid-metric.name", "Test metric", nil)
	counter.Inc()

	exporter := NewPrometheusExporter(registry)
	var buf bytes.Buffer
	err := exporter.Export(&buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	output := buf.String()

	// Should be sanitized to valid Prometheus name
	if !strings.Contains(output, "invalid_metric_name") {
		t.Error("Metric name not properly sanitized")
	}
}

func TestPrometheusExporter_EmptyRegistry(t *testing.T) {
	registry := NewRegistry()
	exporter := NewPrometheusExporter(registry)

	var buf bytes.Buffer
	err := exporter.Export(&buf)
	if err != nil {
		t.Fatalf("Export should not fail with empty registry: %v", err)
	}

	// Should produce empty output
	if buf.Len() > 0 {
		t.Error("Empty registry should produce no output")
	}
}

func TestPrometheusExporter_HistogramBuckets(t *testing.T) {
	registry := NewRegistry()

	histogram := registry.GetOrCreateHistogram("request_duration", "Request duration", nil, []float64{0.1, 0.5, 1.0, 5.0})
	histogram.Observe(0.05)  // In first bucket
	histogram.Observe(0.3)   // In second bucket
	histogram.Observe(0.8)   // In third bucket
	histogram.Observe(2.0)   // In fourth bucket
	histogram.Observe(10.0)  // In +Inf bucket

	exporter := NewPrometheusExporter(registry)
	var buf bytes.Buffer
	err := exporter.Export(&buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	output := buf.String()

	// Verify bucket counts (cumulative)
	if !strings.Contains(output, `request_duration_bucket{le="0.1"} 1`) {
		t.Error("Bucket 0.1 count incorrect")
	}
	if !strings.Contains(output, `request_duration_bucket{le="0.5"} 2`) {
		t.Error("Bucket 0.5 count incorrect")
	}
	if !strings.Contains(output, `request_duration_bucket{le="1"} 3`) {
		t.Error("Bucket 1.0 count incorrect")
	}
	if !strings.Contains(output, `request_duration_bucket{le="5"} 4`) {
		t.Error("Bucket 5.0 count incorrect")
	}
	if !strings.Contains(output, `request_duration_bucket{le="+Inf"} 5`) {
		t.Error("Bucket +Inf count incorrect")
	}
	if !strings.Contains(output, "request_duration_count 5") {
		t.Error("Total count incorrect")
	}
}

func TestStreamBusMetrics_Creation(t *testing.T) {
	registry := NewRegistry()
	metrics := NewStreamBusMetrics(registry)

	if metrics == nil {
		t.Fatal("Failed to create StreamBusMetrics")
	}

	// Verify broker metrics exist
	if metrics.BrokerUptime == nil {
		t.Error("BrokerUptime metric not created")
	}
	if metrics.BrokerStatus == nil {
		t.Error("BrokerStatus metric not created")
	}
	if metrics.BrokerConnections == nil {
		t.Error("BrokerConnections metric not created")
	}

	// Verify message metrics exist
	if metrics.MessagesProduced == nil {
		t.Error("MessagesProduced metric not created")
	}
	if metrics.MessagesConsumed == nil {
		t.Error("MessagesConsumed metric not created")
	}

	// Verify performance metrics exist
	if metrics.ProduceLatency == nil {
		t.Error("ProduceLatency metric not created")
	}
	if metrics.ConsumeLatency == nil {
		t.Error("ConsumeLatency metric not created")
	}

	// Verify security metrics exist
	if metrics.AuthenticationAttempts == nil {
		t.Error("AuthenticationAttempts metric not created")
	}
	if metrics.AuthorizationChecks == nil {
		t.Error("AuthorizationChecks metric not created")
	}

	// Verify cluster metrics exist
	if metrics.ClusterSize == nil {
		t.Error("ClusterSize metric not created")
	}
}

func TestStreamBusMetrics_Usage(t *testing.T) {
	registry := NewRegistry()
	metrics := NewStreamBusMetrics(registry)

	// Simulate some broker activity
	metrics.BrokerConnections.Set(10)
	metrics.MessagesProduced.Add(100)
	metrics.MessagesConsumed.Add(95)
	metrics.BytesProduced.Add(1024 * 1024) // 1 MB

	metrics.ProduceLatency.Observe(0.005) // 5ms
	metrics.ProduceLatency.Observe(0.010) // 10ms
	metrics.ConsumeLatency.Observe(0.003) // 3ms

	metrics.TopicsTotal.Set(5)
	metrics.PartitionsTotal.Set(20)

	metrics.AuthenticationAttempts.Inc()
	metrics.AuthenticationAttempts.Inc()
	metrics.AuthenticationFailures.Inc()

	// Export metrics
	exporter := NewPrometheusExporter(registry)
	var buf bytes.Buffer
	err := exporter.Export(&buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	output := buf.String()

	// Verify metrics are present
	if !strings.Contains(output, "streambus_broker_connections") {
		t.Error("Missing broker connections metric")
	}
	if !strings.Contains(output, "streambus_messages_produced_total") {
		t.Error("Missing messages produced metric")
	}
	if !strings.Contains(output, "streambus_produce_latency_seconds") {
		t.Error("Missing produce latency metric")
	}
	if !strings.Contains(output, "streambus_authentication_attempts_total") {
		t.Error("Missing authentication attempts metric")
	}
}

func TestStreamBusMetrics_BrokerUptime(t *testing.T) {
	registry := NewRegistry()
	metrics := NewStreamBusMetrics(registry)

	startTime := time.Now()
	time.Sleep(10 * time.Millisecond)

	// Set uptime (in seconds)
	uptime := time.Since(startTime).Seconds()
	metrics.BrokerUptime.Set(uptime)

	exporter := NewPrometheusExporter(registry)
	var buf bytes.Buffer
	err := exporter.Export(&buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "streambus_broker_uptime_seconds") {
		t.Error("Missing broker uptime metric")
	}

	// Uptime should be greater than 0
	if !strings.Contains(output, "streambus_broker_uptime_seconds 0.0") {
		// Should have a non-zero value
		if strings.Contains(output, "streambus_broker_uptime_seconds 0") {
			t.Error("Broker uptime should be greater than 0")
		}
	}
}

func TestStreamBusMetrics_TransactionMetrics(t *testing.T) {
	registry := NewRegistry()
	metrics := NewStreamBusMetrics(registry)

	// Simulate transaction activity
	metrics.TransactionsActive.Set(5)
	metrics.TransactionsCommitted.Add(100)
	metrics.TransactionsAborted.Add(3)
	metrics.TransactionDuration.Observe(0.150) // 150ms

	exporter := NewPrometheusExporter(registry)
	var buf bytes.Buffer
	err := exporter.Export(&buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "streambus_transactions_active") {
		t.Error("Missing active transactions metric")
	}
	if !strings.Contains(output, "streambus_transactions_committed_total") {
		t.Error("Missing committed transactions metric")
	}
	if !strings.Contains(output, "streambus_transactions_aborted_total") {
		t.Error("Missing aborted transactions metric")
	}
	if !strings.Contains(output, "streambus_transaction_duration_seconds") {
		t.Error("Missing transaction duration metric")
	}
}

func TestStreamBusMetrics_StorageMetrics(t *testing.T) {
	registry := NewRegistry()
	metrics := NewStreamBusMetrics(registry)

	// Simulate storage activity
	metrics.StorageUsedBytes.Set(1024 * 1024 * 1024) // 1 GB
	metrics.StorageAvailableBytes.Set(10 * 1024 * 1024 * 1024) // 10 GB
	metrics.SegmentsTotal.Set(150)
	metrics.CompactionsTotal.Add(5)

	exporter := NewPrometheusExporter(registry)
	var buf bytes.Buffer
	err := exporter.Export(&buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "streambus_storage_used_bytes") {
		t.Error("Missing storage used metric")
	}
	if !strings.Contains(output, "streambus_storage_available_bytes") {
		t.Error("Missing storage available metric")
	}
	if !strings.Contains(output, "streambus_segments_total") {
		t.Error("Missing storage segments metric")
	}
	if !strings.Contains(output, "streambus_compactions_total") {
		t.Error("Missing storage compactions metric")
	}
}

func TestStreamBusMetrics_ConsumerGroupMetrics(t *testing.T) {
	registry := NewRegistry()
	metrics := NewStreamBusMetrics(registry)

	// Simulate consumer group activity
	metrics.ConsumerGroups.Set(3)
	metrics.ConsumerGroupMembers.Set(15)
	metrics.ConsumerGroupLag.Set(1000)

	exporter := NewPrometheusExporter(registry)
	var buf bytes.Buffer
	err := exporter.Export(&buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "streambus_consumer_groups_total") {
		t.Error("Missing consumer groups metric")
	}
	if !strings.Contains(output, "streambus_consumer_group_members_total") {
		t.Error("Missing consumer group members metric")
	}
	if !strings.Contains(output, "streambus_consumer_group_lag") {
		t.Error("Missing consumer group lag metric")
	}
}

func TestStreamBusMetrics_SecurityMetrics(t *testing.T) {
	registry := NewRegistry()
	metrics := NewStreamBusMetrics(registry)

	// Simulate security activity
	metrics.AuthenticationAttempts.Add(100)
	metrics.AuthenticationFailures.Add(5)
	metrics.AuthorizationChecks.Add(500)
	metrics.AuthorizationDenials.Add(10)
	metrics.AuditEventsLogged.Add(600)

	exporter := NewPrometheusExporter(registry)
	var buf bytes.Buffer
	err := exporter.Export(&buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "streambus_authentication_attempts_total") {
		t.Error("Missing authentication attempts metric")
	}
	if !strings.Contains(output, "streambus_authentication_failures_total") {
		t.Error("Missing authentication failures metric")
	}
	if !strings.Contains(output, "streambus_authorization_checks_total") {
		t.Error("Missing authorization checks metric")
	}
	if !strings.Contains(output, "streambus_authorization_denials_total") {
		t.Error("Missing authorization denials metric")
	}
	if !strings.Contains(output, "streambus_audit_events_logged_total") {
		t.Error("Missing audit events metric")
	}
}

func TestStreamBusMetrics_ClusterMetrics(t *testing.T) {
	registry := NewRegistry()
	metrics := NewStreamBusMetrics(registry)

	// Simulate cluster activity
	metrics.ClusterSize.Set(3)
	metrics.ClusterLeader.Set(1)
	metrics.RaftTerm.Set(5)
	metrics.RaftCommitIndex.Set(1000)

	exporter := NewPrometheusExporter(registry)
	var buf bytes.Buffer
	err := exporter.Export(&buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "streambus_cluster_size") {
		t.Error("Missing cluster size metric")
	}
	if !strings.Contains(output, "streambus_cluster_leader") {
		t.Error("Missing cluster leader metric")
	}
	if !strings.Contains(output, "streambus_raft_term") {
		t.Error("Missing raft term metric")
	}
	if !strings.Contains(output, "streambus_raft_commit_index") {
		t.Error("Missing raft commit index metric")
	}
}

func TestStreamBusMetrics_SchemaMetrics(t *testing.T) {
	registry := NewRegistry()
	metrics := NewStreamBusMetrics(registry)

	// Simulate schema registry activity
	metrics.SchemasRegistered.Add(25)
	metrics.SchemaValidations.Add(1000)
	metrics.SchemaValidationErrors.Add(15)

	exporter := NewPrometheusExporter(registry)
	var buf bytes.Buffer
	err := exporter.Export(&buf)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "streambus_schemas_registered_total") {
		t.Error("Missing schemas registered metric")
	}
	if !strings.Contains(output, "streambus_schema_validations_total") {
		t.Error("Missing schema validations metric")
	}
	if !strings.Contains(output, "streambus_schema_validation_errors_total") {
		t.Error("Missing schema validation errors metric")
	}
}

func BenchmarkPrometheusExporter_Export(b *testing.B) {
	registry := NewRegistry()
	metrics := NewStreamBusMetrics(registry)

	// Set some values
	metrics.BrokerConnections.Set(100)
	metrics.MessagesProduced.Add(10000)
	metrics.MessagesConsumed.Add(9500)

	exporter := NewPrometheusExporter(registry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_ = exporter.Export(&buf)
	}
}
