# StreamBus Production Deployment Guide

Complete guide for deploying StreamBus in production environments.

## Table of Contents

- [Pre-Deployment](#pre-deployment)
- [Architecture Planning](#architecture-planning)
- [Hardware Requirements](#hardware-requirements)
- [Installation](#installation)
- [Configuration](#configuration)
- [Security](#security)
- [Monitoring](#monitoring)
- [Backup and Recovery](#backup-and-recovery)
- [Operations](#operations)

## Pre-Deployment

### Requirements Gathering

Before deploying, determine:

1. **Throughput requirements**
   - Messages per second (peak and average)
   - Message sizes
   - Number of topics/partitions

2. **Latency requirements**
   - p50, p95, p99 latency targets
   - Acceptable end-to-end delay

3. **Durability requirements**
   - Replication factor
   - Acknowledgment mode
   - Retention period

4. **Availability requirements**
   - Uptime SLA (99.9%, 99.99%)
   - RTO (Recovery Time Objective)
   - RPO (Recovery Point Objective)

### Capacity Planning

**Example calculation for 10,000 msgs/sec:**

```
Throughput: 10,000 msgs/sec
Message size: 1KB average
Daily volume: 10,000 * 86,400 = 864M messages
Daily storage: 864M * 1KB = 864 GB
Retention: 7 days
Total storage: 864 GB * 7 = 6 TB

With replication factor 3:
Total cluster storage: 6 TB * 3 = 18 TB
Add 50% buffer: 27 TB
```

## Architecture Planning

### Cluster Topologies

**1. Small Cluster (Development/Staging):**
```
┌─────────────┐
│   Broker 1  │  2 cores, 4GB RAM
└─────────────┘
```
- Single broker
- No replication
- Local storage
- Capacity: ~5,000 msgs/sec

**2. Medium Cluster (Production):**
```
┌─────────────┐  ┌─────────────┐  ┌─────────────┐
│   Broker 1  │  │   Broker 2  │  │   Broker 3  │
│  (Leader)   │  │  (Follower) │  │  (Follower) │
└─────────────┘  └─────────────┘  └─────────────┘
```
- 3 brokers (quorum)
- Replication factor: 3
- 4 cores, 8GB RAM each
- SSD storage
- Capacity: ~50,000 msgs/sec

**3. Large Cluster (High-Scale Production):**
```
┌─────┐  ┌─────┐  ┌─────┐  ┌─────┐  ┌─────┐
│ Br1 │  │ Br2 │  │ Br3 │  │ Br4 │  │ Br5 │
└─────┘  └─────┘  └─────┘  └─────┘  └─────┘
  AZ-1      AZ-1     AZ-2     AZ-2     AZ-3
```
- 5+ brokers across 3 availability zones
- Replication factor: 3
- 8+ cores, 16GB+ RAM each
- NVMe SSD storage
- Capacity: 100,000+ msgs/sec

### Network Topology

**Production Network Layout:**
```
                  ┌──────────────┐
                  │ Load Balancer│
                  └──────┬───────┘
                         │
          ┌──────────────┼──────────────┐
          │              │              │
    ┌─────▼────┐   ┌────▼─────┐  ┌────▼─────┐
    │ Broker 1 │   │ Broker 2 │  │ Broker 3 │
    │ Public   │   │ Public   │  │ Public   │
    │ Private  │   │ Private  │  │ Private  │
    └────┬─────┘   └────┬─────┘  └────┬─────┘
         │              │              │
         └──────────────┴──────────────┘
              Private Network (Raft)
```

- Public IPs for client connections
- Private network for inter-broker Raft communication
- Firewall rules for security

## Hardware Requirements

### Minimum Requirements

**Development:**
- 2 CPU cores
- 4 GB RAM
- 50 GB SSD
- 1 Gbps network

**Staging:**
- 4 CPU cores
- 8 GB RAM
- 200 GB SSD
- 10 Gbps network

**Production:**
- 8 CPU cores
- 16 GB RAM
- 1 TB NVMe SSD
- 10 Gbps network

### Storage Sizing

**Formula:**
```
Storage = Daily_Messages * Message_Size * Retention_Days * Replication_Factor * 1.5
```

**Example:**
- 10 million messages/day
- 1 KB average size
- 7 days retention
- Replication factor 3
- Result: 10M * 1KB * 7 * 3 * 1.5 = 315 GB per broker

### Network Bandwidth

**Formula:**
```
Bandwidth = Messages_Per_Second * Message_Size * (1 + Replication_Factor)
```

**Example:**
- 10,000 msgs/sec
- 1 KB messages
- Replication factor 3
- Result: 10,000 * 1KB * 4 = 40 MB/sec = 320 Mbps

Add 50% overhead for protocol and replication: **480 Mbps**

## Installation

### Kubernetes (Recommended)

```bash
# Install operator
helm repo add streambus https://charts.streambus.io
helm install streambus-operator streambus/streambus-operator \
  --namespace streambus-system \
  --create-namespace

# Deploy production cluster
kubectl apply -f - <<EOF
apiVersion: streambus.io/v1alpha1
kind: StreamBusCluster
metadata:
  name: streambus-prod
  namespace: production
spec:
  replicas: 5
  image:
    repository: streambus/broker
    tag: v0.6.0
  resources:
    requests:
      cpu: 4000m
      memory: 8Gi
    limits:
      cpu: 8000m
      memory: 16Gi
  storage:
    class: fast-ssd
    size: 1Ti
    raftSize: 500Gi
  security:
    tls:
      enabled: true
      certSecretName: streambus-tls
    authentication:
      enabled: true
      method: mtls
  observability:
    metrics:
      enabled: true
      serviceMonitor: true
    tracing:
      enabled: true
EOF
```

### Docker (Alternative)

```bash
# Clone repository
git clone https://github.com/shawntherrien/streambus
cd streambus

# Configure
cp docker-compose.yml docker-compose.prod.yml
# Edit docker-compose.prod.yml with production settings

# Deploy
docker-compose -f docker-compose.prod.yml up -d
```

### Bare Metal

```bash
# Install binary
wget https://github.com/shawntherrien/streambus/releases/download/v0.6.0/streambus-broker-v0.6.0-linux-amd64
chmod +x streambus-broker-v0.6.0-linux-amd64
sudo mv streambus-broker-v0.6.0-linux-amd64 /usr/local/bin/streambus-broker

# Create systemd service
sudo tee /etc/systemd/system/streambus.service <<EOF
[Unit]
Description=StreamBus Broker
After=network.target

[Service]
Type=simple
User=streambus
ExecStart=/usr/local/bin/streambus-broker --config=/etc/streambus/broker.yaml
Restart=on-failure
RestartSec=10s
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

# Enable and start
sudo systemctl enable streambus
sudo systemctl start streambus
```

## Configuration

### Production Configuration Template

```yaml
# /etc/streambus/broker.yaml
server:
  brokerId: 1
  host: broker1.example.com
  port: 9092
  httpPort: 8081
  grpcPort: 9093
  maxConnections: 10000

storage:
  dataDir: /data/streambus
  raftDir: /data/streambus/raft
  syncInterval: 1s
  compactionInterval: 1h
  retentionDays: 7

cluster:
  nodes:
    - id: 1
      host: broker1.example.com
      port: 9092
    - id: 2
      host: broker2.example.com
      port: 9092
    - id: 3
      host: broker3.example.com
      port: 9092

raft:
  enabled: true
  heartbeatTimeout: 1s
  electionTimeout: 3s
  snapshotInterval: 3600s
  snapshotThreshold: 100000

security:
  tls:
    enabled: true
    certFile: /etc/streambus/certs/server.crt
    keyFile: /etc/streambus/certs/server.key
    caFile: /etc/streambus/certs/ca.crt
    clientAuthRequired: true

  authentication:
    enabled: true
    method: mtls

logging:
  level: info
  format: json
  output: /var/log/streambus/broker.log
  maxSize: 100MB
  maxBackups: 10
  maxAge: 30

metrics:
  enabled: true
  port: 8081
  path: /metrics
  interval: 30s

health:
  enabled: true
  livenessPath: /health/live
  readinessPath: /health/ready

multiTenancy:
  enabled: true
  defaultQuotas:
    maxTopics: 100
    maxConnections: 1000
    maxBandwidthMBps: 100

observability:
  tracing:
    enabled: true
    endpoint: jaeger-collector:4317
    samplingRate: 0.1
```

## Security

### TLS Configuration

**Generate certificates:**

```bash
# Create CA
openssl genrsa -out ca.key 4096
openssl req -x509 -new -nodes -key ca.key -sha256 -days 3650 -out ca.crt

# Create broker certificate
openssl genrsa -out broker.key 4096
openssl req -new -key broker.key -out broker.csr
openssl x509 -req -in broker.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out broker.crt -days 365 -sha256

# Create Kubernetes secret
kubectl create secret tls streambus-tls \
  --cert=broker.crt \
  --key=broker.key \
  --namespace=production
```

### Firewall Rules

```bash
# Allow client connections
sudo ufw allow 9092/tcp comment 'StreamBus broker'
sudo ufw allow 8081/tcp comment 'StreamBus HTTP'
sudo ufw allow 9093/tcp comment 'StreamBus gRPC'

# Allow inter-broker (from specific IPs only)
sudo ufw allow from 10.0.1.0/24 to any port 9092
sudo ufw allow from 10.0.1.0/24 to any port 9093
```

### Authentication

**mTLS:**
- Most secure
- Requires client certificates
- Recommended for production

**JWT:**
- Token-based authentication
- Good for multi-tenant scenarios
- Easier client management

## Monitoring

### Prometheus Setup

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'streambus'
    static_configs:
      - targets:
          - 'broker1:8081'
          - 'broker2:8081'
          - 'broker3:8081'
    metrics_path: /metrics
    scrape_interval: 30s
```

### Key Metrics to Monitor

**System Health:**
- `streambus_broker_up`: Broker status
- `streambus_raft_leader`: Leadership status
- `streambus_raft_term`: Raft term

**Performance:**
- `streambus_messages_total`: Total messages
- `streambus_message_latency_seconds`: Message latency
- `streambus_throughput_messages_per_second`: Throughput

**Resources:**
- `process_cpu_seconds_total`: CPU usage
- `process_resident_memory_bytes`: Memory usage
- `process_open_fds`: Open file descriptors

**Storage:**
- `streambus_storage_used_bytes`: Storage used
- `streambus_storage_available_bytes`: Storage available

### Alerting Rules

```yaml
# alerts.yml
groups:
  - name: streambus
    rules:
      - alert: BrokerDown
        expr: up{job="streambus"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Broker {{ $labels.instance }} is down"

      - alert: HighLatency
        expr: streambus_message_latency_seconds{quantile="0.99"} > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High latency detected"

      - alert: DiskSpacelow
        expr: streambus_storage_available_bytes / streambus_storage_total_bytes < 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Disk space below 10%"
```

## Backup and Recovery

### Backup Strategy

**What to backup:**
1. Data directory (`/data/streambus`)
2. Raft logs (`/data/streambus/raft`)
3. Configuration files
4. TLS certificates

**Backup schedule:**
- Incremental: Every hour
- Full: Daily
- Retention: 30 days

**Using Velero (Kubernetes):**

```bash
# Install Velero
velero install \
  --provider aws \
  --bucket streambus-backups \
  --backup-location-config region=us-east-1

# Create backup schedule
velero schedule create streambus-daily \
  --schedule="0 2 * * *" \
  --include-namespaces production \
  --selector app.kubernetes.io/name=streambus-broker
```

### Recovery Procedures

**Single broker failure:**
1. Broker automatically removed from cluster
2. Raft elects new leader if needed
3. Replace failed broker
4. Data replicates from other brokers

**Complete cluster failure:**
1. Restore from latest backup
2. Start all brokers
3. Verify Raft cluster formation
4. Verify data integrity

## Operations

### Health Checks

```bash
# Liveness check
curl http://broker1:8081/health/live

# Readiness check
curl http://broker1:8081/health/ready

# Metrics
curl http://broker1:8081/metrics
```

### Rolling Updates

**Kubernetes:**
```bash
# Update image
kubectl set image statefulset/streambus-prod \
  streambus=streambus/broker:v0.7.0 \
  --namespace=production

# Monitor rollout
kubectl rollout status statefulset/streambus-prod --namespace=production
```

### Scaling

**Scale up:**
```bash
# Kubernetes
kubectl scale statefulset streambus-prod --replicas=5 --namespace=production

# Add rebalancing time for data redistribution
```

**Scale down:**
```bash
# Decommission broker first
streambus-cli broker decommission --id=5

# Then scale
kubectl scale statefulset streambus-prod --replicas=4 --namespace=production
```

### Troubleshooting

See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for detailed troubleshooting guide.

## Checklist

Pre-production deployment:

- [ ] Capacity planning completed
- [ ] Hardware provisioned
- [ ] Network configured
- [ ] TLS certificates generated
- [ ] Configuration reviewed
- [ ] Monitoring setup
- [ ] Alerting configured
- [ ] Backup strategy implemented
- [ ] Disaster recovery plan documented
- [ ] Load testing completed
- [ ] Security review completed
- [ ] Documentation updated
- [ ] Team trained
- [ ] Runbooks created

## See Also

- [Performance Tuning Guide](PERFORMANCE_TUNING.md)
- [Security Best Practices](SECURITY.md)
- [Disaster Recovery](DISASTER_RECOVERY.md)
- [Troubleshooting Guide](TROUBLESHOOTING.md)
