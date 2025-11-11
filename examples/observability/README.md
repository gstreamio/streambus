# StreamBus Observability Example

This example demonstrates StreamBus's comprehensive observability and monitoring capabilities using Prometheus metrics.

## Features Demonstrated

### 1. Metrics Collection
- **Broker Metrics**: Uptime, status, connections, active requests
- **Message Metrics**: Messages produced/consumed, bytes in/out
- **Performance Metrics**: Produce/consume/replication/commit latencies
- **Consumer Group Metrics**: Groups, members, lag
- **Transaction Metrics**: Active, committed, aborted transactions
- **Storage Metrics**: Used/available bytes, segments, compactions
- **Network Metrics**: Bytes in/out, requests, errors
- **Security Metrics**: Authentication/authorization attempts and failures
- **Cluster Metrics**: Cluster size, leader status, Raft state
- **Schema Metrics**: Registered schemas, validation stats

### 2. Prometheus Integration
- HTTP endpoint exposing metrics in Prometheus format
- Proper metric naming and labeling
- Support for Counter, Gauge, and Histogram metric types
- Real-time metric updates

### 3. Real-World Usage
- Broker lifecycle management
- Message production and consumption
- Metric tracking during operations
- Clean shutdown procedures

## Running the Example

```bash
# From the examples/observability directory
go run main.go
```

The example will:
1. Create a metrics registry
2. Initialize StreamBus-specific metrics
3. Start a Prometheus HTTP server on port 9090
4. Create and start a broker
5. Create a topic with 3 partitions
6. Produce 100 messages while tracking metrics
7. Consume messages while tracking metrics
8. Display a metrics summary
9. Wait for Ctrl+C to shutdown

## Accessing Metrics

While the example is running, you can access metrics at:
```
http://localhost:9090/metrics
```

### Using curl
```bash
curl http://localhost:9090/metrics
```

### Sample Output
```
# HELP streambus_broker_uptime_seconds Broker uptime in seconds
# TYPE streambus_broker_uptime_seconds gauge
streambus_broker_uptime_seconds 45.2

# HELP streambus_messages_produced_total Total number of messages produced
# TYPE streambus_messages_produced_total counter
streambus_messages_produced_total 100

# HELP streambus_produce_latency_seconds Produce request latency in seconds
# TYPE streambus_produce_latency_seconds histogram
streambus_produce_latency_seconds_bucket{le="0.001"} 10
streambus_produce_latency_seconds_bucket{le="0.005"} 85
streambus_produce_latency_seconds_bucket{le="0.01"} 100
streambus_produce_latency_seconds_bucket{le="+Inf"} 100
streambus_produce_latency_seconds_sum 0.425
streambus_produce_latency_seconds_count 100
```

## Prometheus Configuration

To scrape metrics with Prometheus, add this to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'streambus'
    scrape_interval: 15s
    static_configs:
      - targets: ['localhost:9090']
        labels:
          env: 'dev'
          service: 'streambus'
```

## Grafana Dashboard

Once metrics are in Prometheus, you can create Grafana dashboards to visualize:

### Message Throughput
```promql
# Messages per second
rate(streambus_messages_produced_total[1m])
rate(streambus_messages_consumed_total[1m])
```

### Latency Percentiles
```promql
# 95th percentile produce latency
histogram_quantile(0.95, rate(streambus_produce_latency_seconds_bucket[5m]))

# 99th percentile consume latency
histogram_quantile(0.99, rate(streambus_consume_latency_seconds_bucket[5m]))
```

### Consumer Lag
```promql
# Consumer group lag
streambus_consumer_group_lag
```

### Broker Health
```promql
# Broker uptime
streambus_broker_uptime_seconds

# Broker status (0=stopped, 1=starting, 2=running, 3=stopping)
streambus_broker_status

# Active connections
streambus_broker_connections
```

### Storage Utilization
```promql
# Storage used percentage
(streambus_storage_used_bytes / (streambus_storage_used_bytes + streambus_storage_available_bytes)) * 100
```

## Available Metrics

### Broker Metrics
- `streambus_broker_uptime_seconds` - Broker uptime in seconds
- `streambus_broker_status` - Broker status (0-3)
- `streambus_broker_connections` - Active client connections
- `streambus_broker_active_requests` - Active requests

### Message Metrics
- `streambus_messages_produced_total` - Total messages produced
- `streambus_messages_consumed_total` - Total messages consumed
- `streambus_messages_stored_total` - Total messages stored
- `streambus_bytes_produced_total` - Total bytes produced
- `streambus_bytes_consumed_total` - Total bytes consumed
- `streambus_bytes_stored_total` - Total bytes stored

### Performance Metrics (Histograms)
- `streambus_produce_latency_seconds` - Produce request latency
- `streambus_consume_latency_seconds` - Consume request latency
- `streambus_replication_latency_seconds` - Replication latency
- `streambus_commit_latency_seconds` - Commit latency

### Topic Metrics
- `streambus_topics_total` - Total number of topics
- `streambus_partitions_total` - Total number of partitions
- `streambus_replicas_total` - Total number of replicas

### Consumer Group Metrics
- `streambus_consumer_groups_total` - Total consumer groups
- `streambus_consumer_group_members_total` - Consumer group members
- `streambus_consumer_group_lag` - Consumer group lag

### Transaction Metrics
- `streambus_transactions_active` - Active transactions
- `streambus_transactions_committed_total` - Committed transactions
- `streambus_transactions_aborted_total` - Aborted transactions
- `streambus_transaction_duration_seconds` - Transaction duration

### Storage Metrics
- `streambus_storage_used_bytes` - Storage space used
- `streambus_storage_available_bytes` - Storage space available
- `streambus_segments_total` - Total segments
- `streambus_compactions_total` - Total compactions

### Network Metrics
- `streambus_network_bytes_in_total` - Total bytes received
- `streambus_network_bytes_out_total` - Total bytes sent
- `streambus_network_requests_total` - Total network requests
- `streambus_network_errors_total` - Total network errors

### Security Metrics
- `streambus_authentication_attempts_total` - Authentication attempts
- `streambus_authentication_failures_total` - Authentication failures
- `streambus_authorization_checks_total` - Authorization checks
- `streambus_authorization_denials_total` - Authorization denials
- `streambus_audit_events_logged_total` - Audit events logged

### Cluster Metrics
- `streambus_cluster_size` - Number of brokers
- `streambus_cluster_leader` - 1 if leader, 0 otherwise
- `streambus_raft_term` - Current Raft term
- `streambus_raft_commit_index` - Raft commit index

### Schema Registry Metrics
- `streambus_schemas_registered_total` - Total registered schemas
- `streambus_schema_validations_total` - Total schema validations
- `streambus_schema_validation_errors_total` - Schema validation errors

## Alerting Examples

### High Produce Latency
```yaml
- alert: HighProduceLatency
  expr: histogram_quantile(0.99, rate(streambus_produce_latency_seconds_bucket[5m])) > 0.1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "High produce latency detected"
    description: "99th percentile produce latency is {{ $value }}s"
```

### High Consumer Lag
```yaml
- alert: HighConsumerLag
  expr: streambus_consumer_group_lag > 1000
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "High consumer group lag"
    description: "Consumer group lag is {{ $value }} messages"
```

### Authentication Failures
```yaml
- alert: HighAuthenticationFailures
  expr: rate(streambus_authentication_failures_total[5m]) > 10
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "High authentication failure rate"
    description: "{{ $value }} authentication failures per second"
```

### Storage Almost Full
```yaml
- alert: StorageAlmostFull
  expr: (streambus_storage_used_bytes / (streambus_storage_used_bytes + streambus_storage_available_bytes)) > 0.85
  for: 10m
  labels:
    severity: warning
  annotations:
    summary: "Storage is almost full"
    description: "Storage is {{ $value | humanizePercentage }} full"
```

## Best Practices

1. **Metric Naming**: All metrics follow Prometheus naming conventions
2. **Labels**: Use labels for dimensions (broker_id, topic, partition, etc.)
3. **Cardinality**: Be careful with high-cardinality labels
4. **Histograms**: Use appropriate buckets for latency measurements
5. **Scrape Interval**: 15-30 seconds is typical for most use cases
6. **Retention**: Configure Prometheus retention based on your needs

## Monitoring Stack

For a complete monitoring solution, consider:

1. **Prometheus**: Time-series database and metrics collection
2. **Grafana**: Visualization and dashboards
3. **Alertmanager**: Alert routing and management
4. **Node Exporter**: Host-level metrics
5. **Blackbox Exporter**: Endpoint monitoring

## Next Steps

1. Set up Prometheus to scrape StreamBus metrics
2. Create custom Grafana dashboards
3. Configure alerting rules
4. Add OpenTelemetry for distributed tracing
5. Integrate with your existing monitoring infrastructure
