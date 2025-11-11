package metrics

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

// PrometheusExporter exports metrics in Prometheus text format
type PrometheusExporter struct {
	registry *Registry
	mu       sync.RWMutex
}

// NewPrometheusExporter creates a new Prometheus exporter
func NewPrometheusExporter(registry *Registry) *PrometheusExporter {
	if registry == nil {
		registry = DefaultRegistry()
	}
	return &PrometheusExporter{
		registry: registry,
	}
}

// Export writes metrics in Prometheus text format to the writer
func (e *PrometheusExporter) Export(w io.Writer) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	metrics := e.registry.All()

	// Group metrics by name
	metricGroups := make(map[string][]Metric)
	for _, metric := range metrics {
		metricGroups[metric.Name()] = append(metricGroups[metric.Name()], metric)
	}

	// Sort metric names for consistent output
	names := make([]string, 0, len(metricGroups))
	for name := range metricGroups {
		names = append(names, name)
	}
	sort.Strings(names)

	// Export each metric group
	for _, name := range names {
		group := metricGroups[name]
		if len(group) == 0 {
			continue
		}

		// Write HELP and TYPE once per metric name
		metric := group[0]
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n", sanitizeMetricName(metric.Name()), metric.Help()); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "# TYPE %s %s\n", sanitizeMetricName(metric.Name()), promType(metric.Type())); err != nil {
			return err
		}

		// Write metric lines
		for _, m := range group {
			if err := e.exportMetric(w, m); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}

	return nil
}

// exportMetric exports a single metric
func (e *PrometheusExporter) exportMetric(w io.Writer, metric Metric) error {
	switch metric.Type() {
	case MetricTypeCounter:
		return e.exportCounter(w, metric.(*Counter))
	case MetricTypeGauge:
		return e.exportGauge(w, metric.(*Gauge))
	case MetricTypeHistogram:
		return e.exportHistogram(w, metric.(*Histogram))
	default:
		return fmt.Errorf("unsupported metric type: %s", metric.Type())
	}
}

// exportCounter exports a counter metric
func (e *PrometheusExporter) exportCounter(w io.Writer, counter *Counter) error {
	labels := formatLabels(counter.Labels())
	value := counter.Value().(uint64)
	_, err := fmt.Fprintf(w, "%s%s %d\n", sanitizeMetricName(counter.Name()), labels, value)
	return err
}

// exportGauge exports a gauge metric
func (e *PrometheusExporter) exportGauge(w io.Writer, gauge *Gauge) error {
	labels := formatLabels(gauge.Labels())
	value := gauge.Value().(float64)
	_, err := fmt.Fprintf(w, "%s%s %g\n", sanitizeMetricName(gauge.Name()), labels, value)
	return err
}

// exportHistogram exports a histogram metric
func (e *PrometheusExporter) exportHistogram(w io.Writer, histogram *Histogram) error {
	value := histogram.Value().(map[string]interface{})
	buckets := value["buckets"].([]float64)
	counts := value["counts"].([]uint64)
	sum := value["sum"].(float64)
	count := value["count"].(uint64)

	name := sanitizeMetricName(histogram.Name())
	labels := histogram.Labels()

	// Export bucket counts
	cumulativeCount := uint64(0)
	for i, bucket := range buckets {
		cumulativeCount += counts[i]
		bucketLabels := copyLabels(labels)
		bucketLabels["le"] = fmt.Sprintf("%g", bucket)
		_, err := fmt.Fprintf(w, "%s_bucket%s %d\n", name, formatLabels(bucketLabels), cumulativeCount)
		if err != nil {
			return err
		}
	}

	// Export +Inf bucket
	cumulativeCount += counts[len(buckets)] // +Inf bucket
	infLabels := copyLabels(labels)
	infLabels["le"] = "+Inf"
	if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n", name, formatLabels(infLabels), cumulativeCount); err != nil {
		return err
	}

	// Export sum
	if _, err := fmt.Fprintf(w, "%s_sum%s %g\n", name, formatLabels(labels), sum); err != nil {
		return err
	}

	// Export count
	_, err := fmt.Fprintf(w, "%s_count%s %d\n", name, formatLabels(labels), count)
	return err
}

// sanitizeMetricName sanitizes a metric name for Prometheus
func sanitizeMetricName(name string) string {
	// Replace invalid characters with underscores
	var sb strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == ':' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	return sb.String()
}

// promType converts internal metric type to Prometheus type
func promType(mt MetricType) string {
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
		return "untyped"
	}
}

// StreamBusMetrics holds all StreamBus-specific metrics
type StreamBusMetrics struct {
	// Broker metrics
	BrokerUptime           *Gauge
	BrokerStatus           *Gauge
	BrokerConnections      *Gauge
	BrokerActiveRequests   *Gauge

	// Message metrics
	MessagesProduced       *Counter
	MessagesConsumed       *Counter
	MessagesStored         *Counter
	BytesProduced          *Counter
	BytesConsumed          *Counter
	BytesStored            *Counter

	// Topic metrics
	TopicsTotal            *Gauge
	PartitionsTotal        *Gauge
	ReplicasTotal          *Gauge

	// Performance metrics
	ProduceLatency         *Histogram
	ConsumeLatency         *Histogram
	ReplicationLatency     *Histogram
	CommitLatency          *Histogram

	// Consumer group metrics
	ConsumerGroups         *Gauge
	ConsumerGroupMembers   *Gauge
	ConsumerGroupLag       *Gauge

	// Transaction metrics
	TransactionsActive     *Gauge
	TransactionsCommitted  *Counter
	TransactionsAborted    *Counter
	TransactionDuration    *Histogram

	// Storage metrics
	StorageUsedBytes       *Gauge
	StorageAvailableBytes  *Gauge
	SegmentsTotal          *Gauge
	CompactionsTotal       *Counter

	// Network metrics
	NetworkBytesIn         *Counter
	NetworkBytesOut        *Counter
	NetworkRequestsTotal   *Counter
	NetworkErrorsTotal     *Counter

	// Security metrics
	AuthenticationAttempts *Counter
	AuthenticationFailures *Counter
	AuthorizationChecks    *Counter
	AuthorizationDenials   *Counter
	AuditEventsLogged      *Counter

	// Cluster metrics
	ClusterSize            *Gauge
	ClusterLeader          *Gauge
	RaftTerm               *Gauge
	RaftCommitIndex        *Gauge

	// Schema registry metrics
	SchemasRegistered      *Counter
	SchemaValidations      *Counter
	SchemaValidationErrors *Counter

	registry *Registry
}

// NewStreamBusMetrics creates a new StreamBus metrics collector
func NewStreamBusMetrics(registry *Registry) *StreamBusMetrics {
	if registry == nil {
		registry = DefaultRegistry()
	}

	m := &StreamBusMetrics{
		registry: registry,
	}

	// Initialize all metrics
	m.initBrokerMetrics()
	m.initMessageMetrics()
	m.initTopicMetrics()
	m.initPerformanceMetrics()
	m.initConsumerGroupMetrics()
	m.initTransactionMetrics()
	m.initStorageMetrics()
	m.initNetworkMetrics()
	m.initSecurityMetrics()
	m.initClusterMetrics()
	m.initSchemaMetrics()

	return m
}

func (m *StreamBusMetrics) initBrokerMetrics() {
	m.BrokerUptime = m.registry.GetOrCreateGauge(
		"streambus_broker_uptime_seconds",
		"Broker uptime in seconds",
		nil,
	)
	m.BrokerStatus = m.registry.GetOrCreateGauge(
		"streambus_broker_status",
		"Broker status (0=stopped, 1=starting, 2=running, 3=stopping)",
		nil,
	)
	m.BrokerConnections = m.registry.GetOrCreateGauge(
		"streambus_broker_connections",
		"Number of active client connections",
		nil,
	)
	m.BrokerActiveRequests = m.registry.GetOrCreateGauge(
		"streambus_broker_active_requests",
		"Number of active requests",
		nil,
	)
}

func (m *StreamBusMetrics) initMessageMetrics() {
	m.MessagesProduced = m.registry.GetOrCreateCounter(
		"streambus_messages_produced_total",
		"Total number of messages produced",
		nil,
	)
	m.MessagesConsumed = m.registry.GetOrCreateCounter(
		"streambus_messages_consumed_total",
		"Total number of messages consumed",
		nil,
	)
	m.MessagesStored = m.registry.GetOrCreateCounter(
		"streambus_messages_stored_total",
		"Total number of messages stored",
		nil,
	)
	m.BytesProduced = m.registry.GetOrCreateCounter(
		"streambus_bytes_produced_total",
		"Total bytes produced",
		nil,
	)
	m.BytesConsumed = m.registry.GetOrCreateCounter(
		"streambus_bytes_consumed_total",
		"Total bytes consumed",
		nil,
	)
	m.BytesStored = m.registry.GetOrCreateCounter(
		"streambus_bytes_stored_total",
		"Total bytes stored",
		nil,
	)
}

func (m *StreamBusMetrics) initTopicMetrics() {
	m.TopicsTotal = m.registry.GetOrCreateGauge(
		"streambus_topics_total",
		"Total number of topics",
		nil,
	)
	m.PartitionsTotal = m.registry.GetOrCreateGauge(
		"streambus_partitions_total",
		"Total number of partitions",
		nil,
	)
	m.ReplicasTotal = m.registry.GetOrCreateGauge(
		"streambus_replicas_total",
		"Total number of replicas",
		nil,
	)
}

func (m *StreamBusMetrics) initPerformanceMetrics() {
	m.ProduceLatency = m.registry.GetOrCreateHistogram(
		"streambus_produce_latency_seconds",
		"Produce request latency in seconds",
		nil,
		[]float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
	)
	m.ConsumeLatency = m.registry.GetOrCreateHistogram(
		"streambus_consume_latency_seconds",
		"Consume request latency in seconds",
		nil,
		[]float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
	)
	m.ReplicationLatency = m.registry.GetOrCreateHistogram(
		"streambus_replication_latency_seconds",
		"Replication latency in seconds",
		nil,
		[]float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
	)
	m.CommitLatency = m.registry.GetOrCreateHistogram(
		"streambus_commit_latency_seconds",
		"Commit latency in seconds",
		nil,
		[]float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
	)
}

func (m *StreamBusMetrics) initConsumerGroupMetrics() {
	m.ConsumerGroups = m.registry.GetOrCreateGauge(
		"streambus_consumer_groups_total",
		"Total number of consumer groups",
		nil,
	)
	m.ConsumerGroupMembers = m.registry.GetOrCreateGauge(
		"streambus_consumer_group_members_total",
		"Total number of consumer group members",
		nil,
	)
	m.ConsumerGroupLag = m.registry.GetOrCreateGauge(
		"streambus_consumer_group_lag",
		"Consumer group lag in messages",
		nil,
	)
}

func (m *StreamBusMetrics) initTransactionMetrics() {
	m.TransactionsActive = m.registry.GetOrCreateGauge(
		"streambus_transactions_active",
		"Number of active transactions",
		nil,
	)
	m.TransactionsCommitted = m.registry.GetOrCreateCounter(
		"streambus_transactions_committed_total",
		"Total number of committed transactions",
		nil,
	)
	m.TransactionsAborted = m.registry.GetOrCreateCounter(
		"streambus_transactions_aborted_total",
		"Total number of aborted transactions",
		nil,
	)
	m.TransactionDuration = m.registry.GetOrCreateHistogram(
		"streambus_transaction_duration_seconds",
		"Transaction duration in seconds",
		nil,
		[]float64{0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0},
	)
}

func (m *StreamBusMetrics) initStorageMetrics() {
	m.StorageUsedBytes = m.registry.GetOrCreateGauge(
		"streambus_storage_used_bytes",
		"Storage space used in bytes",
		nil,
	)
	m.StorageAvailableBytes = m.registry.GetOrCreateGauge(
		"streambus_storage_available_bytes",
		"Storage space available in bytes",
		nil,
	)
	m.SegmentsTotal = m.registry.GetOrCreateGauge(
		"streambus_segments_total",
		"Total number of segments",
		nil,
	)
	m.CompactionsTotal = m.registry.GetOrCreateCounter(
		"streambus_compactions_total",
		"Total number of compactions",
		nil,
	)
}

func (m *StreamBusMetrics) initNetworkMetrics() {
	m.NetworkBytesIn = m.registry.GetOrCreateCounter(
		"streambus_network_bytes_in_total",
		"Total bytes received from network",
		nil,
	)
	m.NetworkBytesOut = m.registry.GetOrCreateCounter(
		"streambus_network_bytes_out_total",
		"Total bytes sent to network",
		nil,
	)
	m.NetworkRequestsTotal = m.registry.GetOrCreateCounter(
		"streambus_network_requests_total",
		"Total number of network requests",
		nil,
	)
	m.NetworkErrorsTotal = m.registry.GetOrCreateCounter(
		"streambus_network_errors_total",
		"Total number of network errors",
		nil,
	)
}

func (m *StreamBusMetrics) initSecurityMetrics() {
	m.AuthenticationAttempts = m.registry.GetOrCreateCounter(
		"streambus_authentication_attempts_total",
		"Total number of authentication attempts",
		nil,
	)
	m.AuthenticationFailures = m.registry.GetOrCreateCounter(
		"streambus_authentication_failures_total",
		"Total number of authentication failures",
		nil,
	)
	m.AuthorizationChecks = m.registry.GetOrCreateCounter(
		"streambus_authorization_checks_total",
		"Total number of authorization checks",
		nil,
	)
	m.AuthorizationDenials = m.registry.GetOrCreateCounter(
		"streambus_authorization_denials_total",
		"Total number of authorization denials",
		nil,
	)
	m.AuditEventsLogged = m.registry.GetOrCreateCounter(
		"streambus_audit_events_logged_total",
		"Total number of audit events logged",
		nil,
	)
}

func (m *StreamBusMetrics) initClusterMetrics() {
	m.ClusterSize = m.registry.GetOrCreateGauge(
		"streambus_cluster_size",
		"Number of brokers in the cluster",
		nil,
	)
	m.ClusterLeader = m.registry.GetOrCreateGauge(
		"streambus_cluster_leader",
		"1 if this broker is the leader, 0 otherwise",
		nil,
	)
	m.RaftTerm = m.registry.GetOrCreateGauge(
		"streambus_raft_term",
		"Current Raft term",
		nil,
	)
	m.RaftCommitIndex = m.registry.GetOrCreateGauge(
		"streambus_raft_commit_index",
		"Current Raft commit index",
		nil,
	)
}

func (m *StreamBusMetrics) initSchemaMetrics() {
	m.SchemasRegistered = m.registry.GetOrCreateCounter(
		"streambus_schemas_registered_total",
		"Total number of schemas registered",
		nil,
	)
	m.SchemaValidations = m.registry.GetOrCreateCounter(
		"streambus_schema_validations_total",
		"Total number of schema validations",
		nil,
	)
	m.SchemaValidationErrors = m.registry.GetOrCreateCounter(
		"streambus_schema_validation_errors_total",
		"Total number of schema validation errors",
		nil,
	)
}
