# StreamBus Monitoring & Observability

Comprehensive guide to monitoring, metrics, and observability for StreamBus.

## Table of Contents

- [Overview](#overview)
- [Metrics](#metrics)
- [Prometheus Integration](#prometheus-integration)
- [Grafana Dashboards](#grafana-dashboards)
- [Health Checks](#health-checks)
- [Alerting](#alerting)
- [Best Practices](#best-practices)

---

## Overview

StreamBus provides comprehensive observability through:
- **Prometheus Metrics**: 40+ metrics covering all aspects of the system
- **HTTP Endpoints**: `/metrics` and `/health` endpoints
- **Structured Logging**: JSON-formatted logs with contextual information
- **Health Checks**: Broker, storage, and cluster health monitoring

---

## Metrics

### Broker Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `streambus_broker_uptime_seconds` | Gauge | Broker uptime in seconds |
| `streambus_broker_status` | Gauge | Broker status (0=stopped, 1=starting, 2=running, 3=stopping) |
| `streambus_broker_connections` | Gauge | Number of active client connections |
| `streambus_broker_active_requests` | Gauge | Number of active requests being processed |

### Message Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `streambus_messages_produced_total` | Counter | Total messages produced |
| `streambus_messages_consumed_total` | Counter | Total messages consumed |
| `streambus_messages_stored_total` | Counter | Total messages stored |
| `streambus_bytes_produced_total` | Counter | Total bytes produced |
| `streambus_bytes_consumed_total` | Counter | Total bytes consumed |
| `streambus_bytes_stored_total` | Counter | Total bytes stored |

### Topic Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `streambus_topics_total` | Gauge | Total number of topics |
| `streambus_partitions_total` | Gauge | Total number of partitions |
| `streambus_replicas_total` | Gauge | Total number of replicas |

### Performance Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `streambus_produce_latency_seconds` | Histogram | Produce request latency distribution |
| `streambus_consume_latency_seconds` | Histogram | Consume request latency distribution |
| `streambus_replication_latency_seconds` | Histogram | Replication latency distribution |
| `streambus_commit_latency_seconds` | Histogram | Commit latency distribution |

**Histogram Buckets**: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s

### Consumer Group Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `streambus_consumer_groups_total` | Gauge | Total number of consumer groups |
| `streambus_consumer_group_members_total` | Gauge | Total number of consumer group members |
| `streambus_consumer_group_lag` | Gauge | Consumer group lag in messages |

### Transaction Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `streambus_transactions_active` | Gauge | Number of active transactions |
| `streambus_transactions_committed_total` | Counter | Total committed transactions |
| `streambus_transactions_aborted_total` | Counter | Total aborted transactions |
| `streambus_transaction_duration_seconds` | Histogram | Transaction duration distribution |

### Storage Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `streambus_storage_used_bytes` | Gauge | Storage space used in bytes |
| `streambus_storage_available_bytes` | Gauge | Storage space available in bytes |
| `streambus_segments_total` | Gauge | Total number of log segments |
| `streambus_compactions_total` | Counter | Total number of compactions performed |

### Network Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `streambus_network_bytes_in_total` | Counter | Total bytes received from network |
| `streambus_network_bytes_out_total` | Counter | Total bytes sent to network |
| `streambus_network_requests_total` | Counter | Total network requests |
| `streambus_network_errors_total` | Counter | Total network errors |

### Security Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `streambus_authentication_attempts_total` | Counter | Total authentication attempts |
| `streambus_authentication_failures_total` | Counter | Total authentication failures |
| `streambus_authorization_checks_total` | Counter | Total authorization checks |
| `streambus_authorization_denials_total` | Counter | Total authorization denials |
| `streambus_audit_events_logged_total` | Counter | Total audit events logged |

### Cluster Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `streambus_cluster_size` | Gauge | Number of brokers in cluster |
| `streambus_cluster_leader` | Gauge | 1 if this broker is leader, 0 otherwise |
| `streambus_raft_term` | Gauge | Current Raft term |
| `streambus_raft_commit_index` | Gauge | Current Raft commit index |

### Schema Registry Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `streambus_schemas_registered_total` | Counter | Total schemas registered |
| `streambus_schema_validations_total` | Counter | Total schema validations |
| `streambus_schema_validation_errors_total` | Counter | Total schema validation errors |

---

## Prometheus Integration

### Configuration

Add StreamBus to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'streambus'
    static_configs:
      - targets: ['localhost:8080']
    scrape_interval: 15s
    scrape_timeout: 10s
    metrics_path: /metrics
```

### Multi-Broker Setup

For a StreamBus cluster with multiple brokers:

```yaml
scrape_configs:
  - job_name: 'streambus-cluster'
    static_configs:
      - targets:
          - 'broker-1:8080'
          - 'broker-2:8080'
          - 'broker-3:8080'
        labels:
          cluster: 'production'
          environment: 'prod'
```

### Service Discovery

Using Kubernetes service discovery:

```yaml
scrape_configs:
  - job_name: 'streambus-k8s'
    kubernetes_sd_configs:
      - role: pod
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app]
        action: keep
        regex: streambus-broker
      - source_labels: [__meta_kubernetes_pod_name]
        target_label: instance
```

### Example Queries

**Message throughput (messages/sec)**:
```promql
rate(streambus_messages_produced_total[5m])
```

**P99 produce latency**:
```promql
histogram_quantile(0.99, rate(streambus_produce_latency_seconds_bucket[5m]))
```

**Storage usage percentage**:
```promql
(streambus_storage_used_bytes / (streambus_storage_used_bytes + streambus_storage_available_bytes)) * 100
```

**Authentication failure rate**:
```promql
rate(streambus_authentication_failures_total[5m]) / rate(streambus_authentication_attempts_total[5m])
```

**Consumer group lag**:
```promql
streambus_consumer_group_lag
```

---

## Grafana Dashboards

### Import Dashboard

1. **Download Dashboard JSON**:
   ```bash
   wget https://raw.githubusercontent.com/shawntherrien/streambus/main/dashboards/streambus-overview.json
   ```

2. **Import in Grafana**:
   - Navigate to Dashboards → Import
   - Upload `streambus-overview.json`
   - Select Prometheus data source
   - Click Import

### Dashboard Panels

The StreamBus Overview dashboard includes:

1. **Message Throughput**: Real-time messages produced/consumed per second
2. **Byte Throughput**: Data transfer rates
3. **P99 Produce Latency**: 99th percentile produce latency gauge
4. **P99 Consume Latency**: 99th percentile consume latency gauge
5. **Broker Status**: Current broker state indicator
6. **Broker Activity**: Active connections and requests
7. **Storage**: Used vs. available storage

### Custom Dashboards

Create custom dashboards for specific use cases:

#### Security Monitoring Dashboard

```json
{
  "title": "StreamBus Security",
  "panels": [
    {
      "title": "Authentication Failures",
      "targets": [{
        "expr": "rate(streambus_authentication_failures_total[5m])"
      }]
    },
    {
      "title": "Authorization Denials",
      "targets": [{
        "expr": "rate(streambus_authorization_denials_total[5m])"
      }]
    }
  ]
}
```

#### Performance Dashboard

Focus on latencies, throughput, and resource utilization.

#### Capacity Planning Dashboard

Track storage growth, message rates, and resource trends.

---

## Health Checks

### HTTP Health Endpoint

StreamBus exposes a health check endpoint at `/health`:

```bash
# Basic health check
curl http://localhost:8080/health

# Response
{
  "status": "healthy",
  "timestamp": "2024-11-10T10:00:00Z",
  "checks": {
    "broker": "ok",
    "storage": "ok",
    "cluster": "ok"
  }
}
```

### Health Check Status Codes

| Status Code | Meaning |
|-------------|---------|
| 200 | Healthy - all checks passing |
| 503 | Unhealthy - one or more checks failing |

### Kubernetes Liveness Probe

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 30
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3
```

### Kubernetes Readiness Probe

```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 5
  timeoutSeconds: 3
  successThreshold: 1
  failureThreshold: 3
```

---

## Alerting

### Prometheus Alert Rules

Create `streambus-alerts.yml`:

```yaml
groups:
  - name: streambus
    interval: 30s
    rules:
      # High produce latency
      - alert: HighProduceLatency
        expr: histogram_quantile(0.99, rate(streambus_produce_latency_seconds_bucket[5m])) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High produce latency detected"
          description: "P99 produce latency is {{ $value }}s"

      # High authentication failure rate
      - alert: HighAuthFailureRate
        expr: rate(streambus_authentication_failures_total[5m]) / rate(streambus_authentication_attempts_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High authentication failure rate"
          description: "{{ $value | humanizePercentage }} of authentications failing"

      # Broker down
      - alert: BrokerDown
        expr: up{job="streambus"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "StreamBus broker is down"
          description: "Broker {{ $labels.instance }} is unreachable"

      # High storage usage
      - alert: HighStorageUsage
        expr: (streambus_storage_used_bytes / (streambus_storage_used_bytes + streambus_storage_available_bytes)) * 100 > 80
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High storage usage"
          description: "Storage is {{ $value }}% full"

      # Consumer group lag
      - alert: HighConsumerLag
        expr: streambus_consumer_group_lag > 10000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High consumer group lag"
          description: "Consumer group has {{ $value }} messages lag"

      # Network errors
      - alert: HighNetworkErrors
        expr: rate(streambus_network_errors_total[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High network error rate"
          description: "{{ $value }} network errors per second"
```

### Alertmanager Configuration

Configure alerting destinations:

```yaml
route:
  group_by: ['alertname', 'cluster']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h
  receiver: 'streambus-team'

receivers:
  - name: 'streambus-team'
    slack_configs:
      - api_url: 'YOUR_SLACK_WEBHOOK'
        channel: '#streambus-alerts'
        title: 'StreamBus Alert'
        text: '{{ range .Alerts }}{{ .Annotations.description }}{{ end }}'

    pagerduty_configs:
      - service_key: 'YOUR_PAGERDUTY_KEY'
        severity: '{{ .CommonLabels.severity }}'
```

---

## Best Practices

### 1. Monitoring Strategy

- **Monitor Golden Signals**: Latency, Traffic, Errors, Saturation
- **Set Baseline**: Establish normal operating metrics
- **Alert on SLOs**: Define Service Level Objectives and alert on violations
- **Use Labels**: Add environment, cluster, and region labels

### 2. Metrics Collection

- **Scrape Interval**: 15-30 seconds for production
- **Retention**: Keep metrics for at least 30 days
- **High Cardinality**: Avoid labels with high cardinality (e.g., request IDs)
- **Aggregation**: Use recording rules for frequently queried aggregations

### 3. Dashboard Design

- **Focus on Key Metrics**: Don't overcrowd dashboards
- **Use Templates**: Create reusable dashboard templates
- **Drill-Down**: Link from overview to detailed dashboards
- **Time Range**: Default to last 6 hours, allow customization

### 4. Alert Fatigue Prevention

- **Alert on Symptoms**: Not on every metric change
- **Proper Thresholds**: Based on historical data and capacity
- **Suppress Noise**: Use `for` clause to avoid flapping alerts
- **Runbooks**: Link alerts to troubleshooting runbooks

### 5. Performance Impact

- **Efficient Queries**: Use rate() and increase() appropriately
- **Recording Rules**: Pre-compute expensive queries
- **Metric Naming**: Follow Prometheus naming conventions
- **Label Usage**: Keep label sets small and consistent

---

## Troubleshooting

### Metrics Not Appearing

**Issue**: Prometheus not scraping metrics

**Solutions**:
1. Check `/metrics` endpoint is accessible:
   ```bash
   curl http://localhost:8080/metrics
   ```

2. Verify Prometheus target status:
   ```
   http://prometheus:9090/targets
   ```

3. Check firewall rules and network connectivity

4. Review Prometheus logs for scrape errors

### High Memory Usage

**Issue**: Broker memory growing over time

**Check**:
```promql
# Memory growth rate
deriv(process_resident_memory_bytes[1h])

# GC frequency
rate(go_gc_duration_seconds_count[5m])
```

**Solutions**:
- Increase log retention policy cleanup frequency
- Reduce connection pool size
- Check for memory leaks in custom code

### Missing Metrics

**Issue**: Some metrics not exported

**Check**:
```bash
# List all available metrics
curl http://localhost:8080/metrics | grep streambus_
```

**Solutions**:
- Ensure metrics are registered in code
- Check metric initialization in broker startup
- Verify no errors in metric collection logic

---

## Integration Examples

### Docker Compose

```yaml
version: '3.8'
services:
  streambus:
    image: streambus:latest
    ports:
      - "9092:9092"
      - "8080:8080"
    volumes:
      - ./data:/data

  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - ./dashboards:/etc/grafana/provisioning/dashboards
```

### Kubernetes

```yaml
apiVersion: v1
kind: Service
metadata:
  name: streambus-metrics
  labels:
    app: streambus
spec:
  ports:
    - name: metrics
      port: 8080
      targetPort: 8080
  selector:
    app: streambus

---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: streambus
spec:
  selector:
    matchLabels:
      app: streambus
  endpoints:
    - port: metrics
      interval: 30s
      path: /metrics
```

---

## Related Documentation

- [Operations Guide](./OPERATIONS.md)
- [Security Guide](./SECURITY.md)
- [CLI Tools](./CLI.md)
- [API Reference](./API.md)

---

## Support

- **Issues**: https://github.com/shawntherrien/streambus/issues
- **Documentation**: https://docs.streambus.io
- **Community**: https://discord.gg/streambus
