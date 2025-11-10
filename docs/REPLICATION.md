# StreamBus Cross-Datacenter Replication

StreamBus provides robust cross-datacenter replication capabilities for high availability, disaster recovery, and multi-region deployments.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Replication Types](#replication-types)
- [Configuration](#configuration)
- [API Reference](#api-reference)
- [Management](#management)
- [Monitoring & Metrics](#monitoring--metrics)
- [Failover & Failback](#failover--failback)
- [Best Practices](#best-practices)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

## Overview

Cross-datacenter replication allows you to replicate topic data from one StreamBus cluster to another, enabling:

- **High Availability** - Automatic failover to a backup cluster if the primary fails
- **Disaster Recovery** - Maintain data copies in geographically distributed locations
- **Data Locality** - Replicate data closer to consumers for reduced latency
- **Multi-Region Deployments** - Support global applications with regional clusters
- **Active-Active** - Bidirectional replication for write-anywhere architectures

### Key Features

- **Multiple Topology Support** - Active-passive, active-active, and star topologies
- **Message Filtering** - Replicate only specific messages based on patterns or headers
- **Message Transformation** - Modify messages during replication
- **Compression** - Reduce network bandwidth with configurable compression
- **Throttling** - Control replication throughput to avoid overwhelming networks
- **Exactly-Once Semantics** - Optional exactly-once delivery guarantees
- **Checkpoint Management** - Automatic offset tracking and recovery
- **Health Monitoring** - Real-time health checks and metrics
- **Automatic Failover** - Trigger failover based on lag thresholds

## Architecture

### Components

```
┌─────────────────────────────────────────────────────────────┐
│                    Replication Manager                       │
│  - Link Management                                           │
│  - Health Monitoring                                         │
│  - Failover Coordination                                     │
└─────────────────────┬───────────────────────────────────────┘
                      │
      ┌───────────────┼───────────────┐
      │               │               │
┌─────▼─────┐   ┌────▼─────┐   ┌────▼─────┐
│  Stream   │   │  Stream  │   │  Stream  │
│ Handler 1 │   │Handler 2 │   │Handler N │
└─────┬─────┘   └────┬─────┘   └────┬─────┘
      │              │              │
      ▼              ▼              ▼
┌──────────────────────────────────────────┐
│         Source Cluster                   │
│   Topics: orders, users, inventory       │
└──────────────────────────────────────────┘
      │              │              │
      │   Replication Stream        │
      │              │              │
      ▼              ▼              ▼
┌──────────────────────────────────────────┐
│         Target Cluster                   │
│   Topics: orders, users, inventory       │
└──────────────────────────────────────────┘
```

### Data Flow

1. **Stream Handler** connects to source and target clusters
2. **Consumer** fetches messages from source topics
3. **Filter** optionally filters messages based on rules
4. **Transform** optionally transforms messages
5. **Producer** writes messages to target topics
6. **Checkpoint** periodically saves offset progress
7. **Health Monitor** tracks replication lag and errors

## Replication Types

### Active-Passive

Unidirectional replication from a primary cluster to a standby cluster.

**Use Cases:**
- Disaster recovery
- Hot standby for failover
- Read-only replicas

**Configuration:**
```json
{
  "type": "active-passive",
  "source_cluster": {
    "cluster_id": "us-west-1",
    "brokers": ["broker1:9092", "broker2:9092"]
  },
  "target_cluster": {
    "cluster_id": "us-east-1",
    "brokers": ["broker3:9092", "broker4:9092"]
  }
}
```

### Active-Active

Bidirectional replication between two clusters.

**Use Cases:**
- Multi-region writes
- Geographic distribution
- Load balancing across regions

**Configuration:**
```json
{
  "type": "active-active",
  "source_cluster": {
    "cluster_id": "us-west-1",
    "brokers": ["broker1:9092"]
  },
  "target_cluster": {
    "cluster_id": "us-east-1",
    "brokers": ["broker3:9092"]
  }
}
```

**Note:** Active-active requires careful coordination to avoid circular replication and conflicts.

### Star

Hub-and-spoke topology with one central cluster and multiple edge clusters.

**Use Cases:**
- Central data aggregation
- Edge computing with central coordination
- Hierarchical data distribution

## Configuration

### Replication Link Configuration

```json
{
  "id": "link-1",
  "name": "US-West to US-East Replication",
  "type": "active-passive",

  "source_cluster": {
    "cluster_id": "us-west-1",
    "brokers": ["broker1.us-west.example.com:9092"],
    "connection_timeout": "30s",
    "request_timeout": "10s",
    "max_retries": 3,
    "security": {
      "enable_tls": true,
      "tls_cert_file": "/certs/client.crt",
      "tls_key_file": "/certs/client.key",
      "tls_ca_file": "/certs/ca.crt"
    }
  },

  "target_cluster": {
    "cluster_id": "us-east-1",
    "brokers": ["broker1.us-east.example.com:9092"],
    "connection_timeout": "30s",
    "request_timeout": "10s",
    "max_retries": 3
  },

  "topics": ["orders", "users", "inventory"],
  "topic_prefix": "",

  "config": {
    "max_bytes": 10485760,
    "max_messages": 10000,
    "batch_size": 1000,
    "concurrent_partitions": 10,
    "enable_compression": true,
    "compression_type": "snappy",
    "throttle_rate_bytes_per_sec": 104857600,
    "checkpoint_interval_ms": 5000,
    "enable_exactly_once": false,
    "enable_idempotence": true
  },

  "failover_config": {
    "enabled": true,
    "failover_threshold": 100000,
    "failover_timeout_ms": 60000,
    "max_consecutive_failures": 3,
    "auto_failback": false,
    "notification_webhook": "https://alerts.example.com/failover"
  }
}
```

### Message Filtering

```json
{
  "filter": {
    "enabled": true,
    "include_patterns": [".*important.*", ".*priority:high.*"],
    "exclude_patterns": [".*test.*", ".*debug.*"],
    "filter_by_header": {
      "region": "us-west-1",
      "priority": "high"
    },
    "min_timestamp": "2024-01-01T00:00:00Z"
  }
}
```

### Message Transformation

```json
{
  "transform": {
    "enabled": true,
    "add_headers": {
      "replicated_from": "us-west-1",
      "replication_timestamp": "${now}"
    },
    "remove_headers": ["internal-id", "debug-info"],
    "header_transforms": {
      "region": "us-west-1 -> us-east-1"
    }
  }
}
```

## API Reference

### REST API Endpoints

#### List Replication Links

```
GET /api/v1/replication/links
```

**Response:**
```json
[
  {
    "id": "link-1",
    "name": "US-West to US-East",
    "type": "active-passive",
    "status": "active",
    "created_at": "2024-01-15T10:00:00Z",
    "started_at": "2024-01-15T10:05:00Z",
    "metrics": {
      "total_messages_replicated": 1000000,
      "total_bytes_replicated": 104857600,
      "messages_per_second": 1000.5,
      "bytes_per_second": 102400.2,
      "replication_lag": 500
    },
    "health": {
      "status": "healthy",
      "source_cluster_reachable": true,
      "target_cluster_reachable": true
    }
  }
]
```

#### Create Replication Link

```
POST /api/v1/replication/links
Content-Type: application/json

{
  "name": "New Replication Link",
  "type": "active-passive",
  "source_cluster": {...},
  "target_cluster": {...},
  "topics": ["orders"],
  "config": {...}
}
```

**Response:** `201 Created`

#### Get Replication Link

```
GET /api/v1/replication/links/{linkId}
```

#### Update Replication Link

```
PUT /api/v1/replication/links/{linkId}
Content-Type: application/json

{
  "config": {
    "throttle_rate_bytes_per_sec": 52428800
  }
}
```

**Note:** Link must be paused before updating.

#### Delete Replication Link

```
DELETE /api/v1/replication/links/{linkId}
```

**Note:** Link must be stopped before deletion.

#### Start Replication Link

```
POST /api/v1/replication/links/{linkId}/start
```

#### Stop Replication Link

```
POST /api/v1/replication/links/{linkId}/stop
```

#### Pause Replication Link

```
POST /api/v1/replication/links/{linkId}/pause
```

#### Resume Replication Link

```
POST /api/v1/replication/links/{linkId}/resume
```

#### Get Metrics

```
GET /api/v1/replication/links/{linkId}/metrics
```

**Response:**
```json
{
  "total_messages_replicated": 1500000,
  "total_bytes_replicated": 157286400,
  "messages_per_second": 1200.3,
  "bytes_per_second": 122880.5,
  "replication_lag": 350,
  "average_replication_lag": 400,
  "max_replication_lag": 1200,
  "total_errors": 5,
  "errors_per_second": 0.01,
  "consecutive_failures": 0,
  "uptime_seconds": 3600,
  "partition_metrics": {
    "orders-0": {
      "topic": "orders",
      "partition": 0,
      "source_offset": 100000,
      "target_offset": 99650,
      "lag": 350,
      "messages_replicated": 100000,
      "bytes_replicated": 10485760
    }
  }
}
```

#### Get Health

```
GET /api/v1/replication/links/{linkId}/health
```

**Response:**
```json
{
  "status": "healthy",
  "last_health_check": "2024-01-15T12:30:00Z",
  "source_cluster_reachable": true,
  "target_cluster_reachable": true,
  "replication_lag_healthy": true,
  "error_rate_healthy": true,
  "checkpoint_healthy": true,
  "issues": [],
  "warnings": []
}
```

#### Manual Failover

```
POST /api/v1/replication/links/{linkId}/failover
```

**Response:**
```json
{
  "id": "failover-event-1",
  "link_id": "link-1",
  "type": "failover",
  "reason": "manual",
  "source_cluster_id": "us-west-1",
  "target_cluster_id": "us-east-1",
  "timestamp": "2024-01-15T12:45:00Z",
  "success": true,
  "duration": "5s"
}
```

## Management

### Creating a Replication Link

```bash
curl -X POST http://localhost:8081/api/v1/replication/links \
  -H "Content-Type: application/json" \
  -d @replication-link.json
```

### Starting Replication

```bash
curl -X POST http://localhost:8081/api/v1/replication/links/link-1/start
```

### Monitoring Replication

```bash
# Get metrics
curl http://localhost:8081/api/v1/replication/links/link-1/metrics

# Get health
curl http://localhost:8081/api/v1/replication/links/link-1/health
```

### Pausing Replication

```bash
curl -X POST http://localhost:8081/api/v1/replication/links/link-1/pause
```

## Monitoring & Metrics

### Key Metrics

| Metric | Description | Alert Threshold |
|--------|-------------|-----------------|
| `replication_lag` | Milliseconds behind source | > 60000 (1 minute) |
| `messages_per_second` | Replication throughput | < expected rate |
| `errors_per_second` | Error rate | > 10 |
| `consecutive_failures` | Failed attempts in a row | > 3 |
| `source_cluster_reachable` | Source connectivity | false |
| `target_cluster_reachable` | Target connectivity | false |

### Prometheus Integration

StreamBus exposes replication metrics via the `/metrics` endpoint:

```
# HELP streambus_replication_lag_ms Replication lag in milliseconds
# TYPE streambus_replication_lag_ms gauge
streambus_replication_lag_ms{link_id="link-1",topic="orders",partition="0"} 350

# HELP streambus_replication_messages_total Total messages replicated
# TYPE streambus_replication_messages_total counter
streambus_replication_messages_total{link_id="link-1"} 1500000

# HELP streambus_replication_bytes_total Total bytes replicated
# TYPE streambus_replication_bytes_total counter
streambus_replication_bytes_total{link_id="link-1"} 157286400
```

## Failover & Failback

### Automatic Failover

Configure automatic failover with:

```json
{
  "failover_config": {
    "enabled": true,
    "failover_threshold": 100000,
    "failover_timeout_ms": 60000,
    "max_consecutive_failures": 3,
    "auto_failback": false
  }
}
```

**Trigger Conditions:**
- Replication lag exceeds `failover_threshold`
- Consecutive failures exceed `max_consecutive_failures`
- Source cluster unreachable for `failover_timeout_ms`

### Manual Failover

```bash
curl -X POST http://localhost:8081/api/v1/replication/links/link-1/failover
```

### Failback

After primary recovery, perform failback:

```bash
# 1. Ensure primary is healthy
# 2. Stop writes to target cluster
# 3. Reverse replication direction
# 4. Trigger failback
curl -X POST http://localhost:8081/api/v1/replication/links/link-1/failback
```

## Best Practices

### 1. Network Configuration

- Use dedicated network bandwidth for replication
- Enable compression to reduce bandwidth usage
- Configure throttling to avoid network saturation

### 2. Topic Selection

- Only replicate necessary topics
- Use filtering to reduce unnecessary data transfer
- Consider data sensitivity and compliance requirements

### 3. Monitoring

- Set up alerts for high replication lag
- Monitor error rates and consecutive failures
- Track throughput to ensure it meets requirements

### 4. Security

- Always enable TLS for cross-datacenter links
- Use mutual TLS authentication when possible
- Secure credentials with secrets management

### 5. Testing

- Regularly test failover procedures
- Validate data consistency after failover
- Benchmark replication performance under load

### 6. Capacity Planning

- Calculate required bandwidth: `messages/sec * avg_message_size`
- Account for compression ratio (typically 2-4x)
- Plan for burst traffic and peak loads

## Examples

### Example 1: Basic Active-Passive Setup

```bash
# Create replication link
curl -X POST http://localhost:8081/api/v1/replication/links \
  -H "Content-Type: application/json" \
  -d '{
    "name": "DR Replication",
    "type": "active-passive",
    "source_cluster": {
      "cluster_id": "primary",
      "brokers": ["primary:9092"]
    },
    "target_cluster": {
      "cluster_id": "dr",
      "brokers": ["dr:9092"]
    },
    "topics": ["orders", "users"]
  }'

# Start replication
curl -X POST http://localhost:8081/api/v1/replication/links/{id}/start
```

### Example 2: Filtered Replication

```json
{
  "name": "High Priority Replication",
  "type": "active-passive",
  "topics": ["events"],
  "filter": {
    "enabled": true,
    "filter_by_header": {
      "priority": "high"
    }
  }
}
```

### Example 3: Throttled Replication

```json
{
  "name": "Throttled Replication",
  "type": "active-passive",
  "topics": ["logs"],
  "config": {
    "throttle_rate_bytes_per_sec": 10485760,
    "enable_compression": true,
    "compression_type": "zstd"
  }
}
```

## Troubleshooting

### High Replication Lag

**Symptoms:**
- `replication_lag` metric exceeds threshold
- Messages take long to appear in target cluster

**Solutions:**
1. Increase `concurrent_partitions` for parallel replication
2. Increase `batch_size` to reduce network round trips
3. Enable compression to reduce data transfer time
4. Check network bandwidth and latency
5. Scale up target cluster resources

### Replication Failures

**Symptoms:**
- `consecutive_failures` count increasing
- Errors in replication logs
- Health status shows `unhealthy`

**Solutions:**
1. Check network connectivity between clusters
2. Verify broker addresses are correct
3. Ensure TLS certificates are valid
4. Check authentication credentials
5. Review broker logs for errors

### Circular Replication

**Symptoms:**
- Messages appear multiple times
- Infinite replication loops
- Rapidly growing topics

**Solutions:**
1. Use topic prefixes to distinguish replicated topics
2. Add replication headers and filter them out
3. Ensure proper replication topology design
4. Review active-active configuration

### Performance Issues

**Symptoms:**
- Low throughput (messages/sec)
- High CPU usage on replication processes
- Network saturation

**Solutions:**
1. Tune `batch_size` and `max_bytes` parameters
2. Adjust `concurrent_partitions` for workload
3. Enable compression if network-bound
4. Use throttling to manage bandwidth
5. Scale replication infrastructure

## Support

For additional help:
- **Documentation:** https://docs.streambus.io
- **Issues:** https://github.com/shawntherrien/streambus/issues
- **Community:** https://community.streambus.io
