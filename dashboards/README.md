# StreamBus Observability Stack

Complete observability infrastructure for StreamBus including metrics collection, distributed tracing, and visualization.

## Overview

This directory contains a turnkey observability solution for StreamBus that includes:

- **Prometheus**: Metrics collection and storage
- **Grafana**: Visualization and dashboards
- **Jaeger**: Distributed tracing UI
- **OpenTelemetry Collector**: Telemetry aggregation and routing
- **Zipkin** (optional): Alternative tracing UI
- **Loki/Promtail** (optional): Log aggregation

## Quick Start

### 1. Start the Observability Stack

```bash
cd dashboards
docker-compose up -d
```

This starts the core services (Prometheus, Grafana, Jaeger, OpenTelemetry Collector).

To include optional services (Zipkin, Loki, Promtail):

```bash
docker-compose --profile full up -d
```

### 2. Configure StreamBus to Export Metrics

Start your StreamBus broker with metrics enabled on port 9090:

```go
import "github.com/shawntherrien/streambus/pkg/metrics"

// Create metrics registry
metricsRegistry := metrics.NewRegistry()

// Initialize StreamBus metrics
streambusMetrics := metrics.NewStreamBusMetrics(metricsRegistry)

// Start Prometheus HTTP server
metricsHandler := metrics.NewHandler(metricsRegistry)
http.Handle("/metrics", metricsHandler)
go http.ListenAndServe(":9090", nil)
```

See `examples/observability/` for a complete working example.

### 3. Configure OpenTelemetry Tracing

Export traces to the OpenTelemetry Collector:

```go
import "github.com/shawntherrien/streambus/pkg/tracing"

tracingConfig := &tracing.Config{
    Enabled:        true,
    ServiceName:    "streambus-broker",
    ServiceVersion: "1.0.0",
    Environment:    "production",
    Exporter: tracing.ExporterConfig{
        Type: tracing.ExporterTypeOTLP,
        OTLP: tracing.OTLPConfig{
            Endpoint: "localhost:4317",
            Insecure: true,
        },
    },
    Sampling: tracing.SamplingConfig{
        SamplingRate: 1.0,
        ParentBased:  true,
    },
}

tracer, err := tracing.NewTracer(tracingConfig)
if err != nil {
    log.Fatal(err)
}
defer tracer.Shutdown(context.Background())
```

See `examples/tracing/` for a complete working example.

### 4. Access the UIs

- **Grafana**: http://localhost:3000
  - Username: `admin`
  - Password: `admin`
  - Pre-loaded dashboard: "StreamBus Overview"

- **Prometheus**: http://localhost:9090
  - Query interface for metrics
  - Service discovery status

- **Jaeger**: http://localhost:16686
  - Trace visualization
  - Service dependency graph

- **OpenTelemetry Collector Health**: http://localhost:13133
  - Health check endpoint

## Architecture

```
┌─────────────────┐
│ StreamBus Broker│
│   :9090/metrics │──┐
└─────────────────┘  │
                     │
┌─────────────────┐  │         ┌────────────┐
│ StreamBus Broker│──┼────────▶│ Prometheus │
│   Traces (OTLP) │  │         │   :9090    │
└─────────────────┘  │         └─────┬──────┘
         │           │               │
         │           │               │
         ▼           │               ▼
┌─────────────────┐  │         ┌────────────┐
│  OpenTelemetry  │◀─┘         │  Grafana   │
│    Collector    │            │   :3000    │
│  :4317 / :4318  │            └────────────┘
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│     Jaeger      │
│     :16686      │
└─────────────────┘
```

## Components

### Prometheus

Metrics collection and time-series database.

**Configuration**: `prometheus/prometheus.yml`

**Key scrape targets**:
- `streambus-broker`: Your StreamBus brokers (port 9090)
- `prometheus`: Self-monitoring
- `otel-collector`: Collector metrics
- `grafana`: Grafana metrics
- `jaeger`: Jaeger metrics

**Access**: http://localhost:9090

### Grafana

Visualization and dashboards.

**Configuration**:
- Datasources: `grafana/provisioning/datasources/datasources.yml`
- Dashboards: `grafana/provisioning/dashboards/dashboards.yml`
- Dashboard JSON: `grafana/dashboards/streambus-overview.json`

**Pre-configured datasources**:
- Prometheus (default)
- Jaeger (traces)
- Loki (logs, optional)

**Access**: http://localhost:3000 (admin/admin)

### Jaeger

Distributed tracing UI and storage.

**Features**:
- Trace visualization
- Service dependency graph
- Performance analysis
- Error tracking

**Access**: http://localhost:16686

### OpenTelemetry Collector

Telemetry aggregation and routing.

**Configuration**: `otel-collector/config.yaml`

**Receivers**:
- OTLP gRPC: `:4317`
- OTLP HTTP: `:4318`
- Prometheus scraping: Internal collector metrics

**Processors**:
- Batch: Reduces connection overhead
- Memory limiter: Prevents OOM
- Attributes: Adds/modifies attributes
- Resource: Adds resource attributes

**Exporters**:
- Jaeger: Traces to Jaeger
- Prometheus: Metrics endpoint
- Prometheus Remote Write: To Prometheus
- Logging: Console output (debugging)

**Access**: http://localhost:13133 (health check)

### Zipkin (Optional)

Alternative tracing UI with Zipkin-native format support.

**Access**: http://localhost:9411

**Enable**: Use `--profile full` when starting docker-compose

### Loki + Promtail (Optional)

Log aggregation system integrated with Grafana.

**Loki**: Log storage and query engine
**Promtail**: Log collector and shipper

**Access**: Loki API at http://localhost:3100

**Enable**: Use `--profile full` when starting docker-compose

## Grafana Dashboards

### StreamBus Overview Dashboard

Located at: `grafana/dashboards/streambus-overview.json`

**Panels**:

1. **Broker Status**: Current broker status (Running/Stopped/Error)
2. **Broker Uptime**: Time since broker started
3. **Active Connections**: Real-time connection count
4. **Topics**: Total number of topics
5. **Message Throughput**: Messages produced and consumed per second
6. **Latency Percentiles**: p95 and p99 latencies for produce/consume operations
7. **Network Throughput**: Bytes in and out per second
8. **Storage Utilization**: Percentage of storage used
9. **Consumer Group Lag**: Lag per consumer group

**Time Range**: Last 15 minutes (configurable)
**Refresh**: 5 seconds (configurable)

### Creating Custom Dashboards

1. Log into Grafana (http://localhost:3000)
2. Click "+" → "Dashboard"
3. Add panels with Prometheus queries
4. Save dashboard JSON to `grafana/dashboards/`

Example queries:

```promql
# Message rate
rate(streambus_messages_produced_total[5m])

# Latency percentile
histogram_quantile(0.95, rate(streambus_produce_latency_seconds_bucket[5m]))

# Storage usage
streambus_storage_used_bytes / (streambus_storage_used_bytes + streambus_storage_available_bytes) * 100

# Consumer lag
streambus_consumer_group_lag_messages
```

## Prometheus Configuration

### Adding More Brokers

Edit `prometheus/prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'streambus-broker'
    scrape_interval: 10s
    static_configs:
      - targets:
          - 'broker-1:9090'
          - 'broker-2:9090'
          - 'broker-3:9090'
        labels:
          service: 'streambus-broker'
```

Reload Prometheus configuration:

```bash
docker-compose exec prometheus killall -HUP prometheus
```

Or use the reload API:

```bash
curl -X POST http://localhost:9090/-/reload
```

### Service Discovery

For dynamic broker discovery, configure Prometheus with service discovery:

```yaml
scrape_configs:
  - job_name: 'streambus-broker'
    consul_sd_configs:
      - server: 'consul:8500'
        services: ['streambus-broker']
```

## OpenTelemetry Collector Configuration

### Custom Exporters

Edit `otel-collector/config.yaml` to add exporters:

```yaml
exporters:
  # Add Datadog exporter
  datadog:
    api:
      key: "${DD_API_KEY}"
      site: datadoghq.com

  # Add New Relic exporter
  otlp/newrelic:
    endpoint: otlp.nr-data.net:4317
    headers:
      api-key: "${NEW_RELIC_LICENSE_KEY}"

service:
  pipelines:
    traces:
      exporters: [otlp/jaeger, datadog, otlp/newrelic]
```

Restart the collector:

```bash
docker-compose restart otel-collector
```

### Sampling Configuration

Adjust sampling in the collector config:

```yaml
processors:
  probabilistic_sampler:
    sampling_percentage: 10  # Sample 10% of traces

service:
  pipelines:
    traces:
      processors: [probabilistic_sampler, memory_limiter, batch]
```

## Alerting

### Prometheus Alerting Rules

Create `prometheus/alerts/streambus-alerts.yml`:

```yaml
groups:
  - name: streambus
    interval: 30s
    rules:
      - alert: BrokerDown
        expr: up{job="streambus-broker"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "StreamBus broker is down"
          description: "Broker {{ $labels.instance }} has been down for more than 1 minute"

      - alert: HighProduceLatency
        expr: histogram_quantile(0.95, rate(streambus_produce_latency_seconds_bucket[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High produce latency detected"
          description: "p95 produce latency is {{ $value }}s on {{ $labels.instance }}"

      - alert: HighConsumerLag
        expr: streambus_consumer_group_lag_messages > 10000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High consumer group lag"
          description: "Consumer group {{ $labels.group }} has lag of {{ $value }} messages"

      - alert: StorageAlmostFull
        expr: streambus_storage_used_bytes / (streambus_storage_used_bytes + streambus_storage_available_bytes) > 0.8
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Storage utilization is high"
          description: "Storage is {{ $value | humanizePercentage }} full on {{ $labels.instance }}"
```

Update `prometheus/prometheus.yml`:

```yaml
rule_files:
  - "alerts/*.yml"

alerting:
  alertmanagers:
    - static_configs:
        - targets:
          - alertmanager:9093
```

### Grafana Alerts

Create alerts in Grafana dashboards:

1. Edit a panel
2. Go to "Alert" tab
3. Create alert rule
4. Configure notification channels (Slack, PagerDuty, email, etc.)

## Troubleshooting

### Broker Metrics Not Appearing

1. **Check broker is exporting metrics**:
   ```bash
   curl http://localhost:9090/metrics
   ```
   Should return Prometheus format metrics.

2. **Check Prometheus can reach broker**:
   ```bash
   docker-compose logs prometheus
   ```
   Look for scrape errors.

3. **Verify Prometheus targets**:
   Visit http://localhost:9090/targets
   Broker target should be "UP"

4. **Check Docker networking**:
   If running broker on host, use `host.docker.internal` instead of `localhost` in prometheus.yml

### Traces Not Appearing in Jaeger

1. **Verify OTLP endpoint is reachable**:
   ```bash
   curl -v http://localhost:4317
   ```

2. **Check OpenTelemetry Collector logs**:
   ```bash
   docker-compose logs otel-collector
   ```
   Look for connection errors.

3. **Verify trace exporter configuration**:
   Ensure broker is configured to send to `localhost:4317` with gRPC.

4. **Check Jaeger is receiving traces**:
   ```bash
   docker-compose logs jaeger
   ```

### Grafana Dashboard Shows No Data

1. **Check datasource connection**:
   - Go to Configuration → Data Sources
   - Test Prometheus connection
   - Should show "Data source is working"

2. **Verify time range**:
   - Ensure time range covers when metrics were collected
   - Try "Last 1 hour" or "Last 5 minutes"

3. **Check Prometheus has data**:
   - Visit http://localhost:9090
   - Query: `streambus_messages_produced_total`
   - Should return results

4. **Inspect panel queries**:
   - Edit panel
   - Check query syntax
   - Use "Query inspector" to see raw response

## Performance Tuning

### Prometheus

**For high cardinality**:
```yaml
global:
  scrape_interval: 30s  # Reduce scrape frequency
storage:
  tsdb:
    retention.time: 15d  # Reduce retention
    retention.size: 50GB
```

**For large clusters**:
```yaml
scrape_configs:
  - job_name: 'streambus-broker'
    scrape_interval: 15s
    scrape_timeout: 10s
    sample_limit: 10000  # Limit samples per scrape
```

### OpenTelemetry Collector

**For high throughput**:
```yaml
processors:
  batch:
    timeout: 1s
    send_batch_size: 10000  # Increase batch size

  memory_limiter:
    limit_mib: 2048  # Increase memory limit
    spike_limit_mib: 512

receivers:
  otlp:
    protocols:
      grpc:
        max_recv_msg_size_mib: 32  # Increase message size
```

### Grafana

**For large dashboards**:
- Reduce refresh rate (30s or 1m)
- Use query caching
- Limit time range
- Use recording rules in Prometheus

## Production Recommendations

1. **Use persistent volumes** for data:
   ```yaml
   volumes:
     - prometheus-data:/prometheus
     - grafana-data:/var/lib/grafana
   ```

2. **Enable authentication** on all services:
   - Prometheus: Use basic auth or OAuth
   - Grafana: LDAP or OAuth integration
   - Jaeger: Deploy behind auth proxy

3. **Use external storage** for long-term data:
   - Prometheus: Configure remote write to Cortex/Thanos
   - Jaeger: Use Elasticsearch or Cassandra backend

4. **Set resource limits**:
   ```yaml
   services:
     prometheus:
       deploy:
         resources:
           limits:
             cpus: '2'
             memory: 4G
   ```

5. **Enable TLS** for all communication:
   - Generate certificates
   - Configure TLS in each service
   - Update collector to use secure connections

6. **Configure retention policies**:
   - Prometheus: 15-30 days retention
   - Jaeger: 7 days retention
   - Loki: 14 days retention

7. **Set up alerting**:
   - Deploy Alertmanager
   - Configure notification channels
   - Create runbooks for alerts

8. **Monitor the monitoring stack**:
   - Alert on Prometheus scrape failures
   - Monitor Grafana availability
   - Track collector pipeline health

## Backup and Disaster Recovery

### Prometheus Data

```bash
# Backup
docker-compose exec prometheus tar czf - /prometheus > prometheus-backup.tar.gz

# Restore
docker-compose stop prometheus
cat prometheus-backup.tar.gz | docker-compose exec -T prometheus tar xzf - -C /
docker-compose start prometheus
```

### Grafana Dashboards

Dashboards are version controlled in `grafana/dashboards/`. To backup:

```bash
# Export all dashboards via API
curl -H "Content-Type: application/json" \
  http://admin:admin@localhost:3000/api/search?type=dash-db | \
  jq -r '.[].uid' | \
  xargs -I {} curl -H "Content-Type: application/json" \
    http://admin:admin@localhost:3000/api/dashboards/uid/{} > dashboard-{}.json
```

## Advanced Topics

### Distributed Tracing Across Services

When StreamBus is part of a larger microservices architecture:

1. **Propagate trace context** in messages:
   ```go
   // Producer adds trace context to headers
   carrier := propagation.MapCarrier{}
   otel.GetTextMapPropagator().Inject(ctx, carrier)

   msg := &Message{
       Headers: carrier,
       // ... other fields
   }
   ```

2. **Consumer extracts trace context**:
   ```go
   carrier := propagation.MapCarrier(msg.Headers)
   ctx := otel.GetTextMapPropagator().Extract(context.Background(), carrier)
   ```

3. **Link traces** across async boundaries using span links:
   ```go
   span := trace.SpanFromContext(producerCtx)
   link := trace.Link{SpanContext: span.SpanContext()}

   ctx, consumerSpan := tracer.Start(ctx, "consume",
       trace.WithLinks(link))
   ```

### Custom Metrics

Add application-specific metrics:

```go
// Create custom metric
customMetric := metricsRegistry.GetOrCreateCounter(
    "streambus_custom_events_total",
    "Total custom events",
    map[string]string{"event_type": "payment"},
)

// Increment
customMetric.Inc()
```

### Log Correlation with Traces

Configure Loki to extract trace IDs from logs:

```yaml
# promtail/config.yaml
pipeline_stages:
  - regex:
      expression: '.*trace_id=(?P<trace_id>\w+).*'
  - labels:
      trace_id:
```

In Grafana, clicking a trace ID in logs will open the trace in Jaeger.

## References

- [Prometheus Documentation](https://prometheus.io/docs/)
- [Grafana Documentation](https://grafana.com/docs/)
- [Jaeger Documentation](https://www.jaegertracing.io/docs/)
- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [StreamBus Metrics Reference](../examples/observability/README.md)
- [StreamBus Tracing Guide](../examples/tracing/README.md)

## Support

For issues with the observability stack:

1. Check service logs: `docker-compose logs <service>`
2. Verify configurations match this documentation
3. Check network connectivity between services
4. Review Prometheus targets: http://localhost:9090/targets
5. Test datasource connections in Grafana

For StreamBus-specific issues, see the main project README.
