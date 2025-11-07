# StreamBus Operations Guide

Comprehensive guide for deploying, configuring, and operating StreamBus in production environments.

> **Note**: StreamBus is currently in early development (Milestone 1.2 complete). This guide describes planned production features. Many distributed features (replication, consensus, multi-broker clusters) are not yet implemented.

## Table of Contents

- [Deployment](#deployment)
- [Configuration](#configuration)
- [Monitoring](#monitoring)
- [Backup and Recovery](#backup-and-recovery)
- [Performance Tuning](#performance-tuning)
- [Security](#security)
- [Troubleshooting](#troubleshooting)
- [Maintenance](#maintenance)

---

## Deployment

### System Requirements

#### Minimum Requirements (Development)
- **CPU**: 2 cores
- **Memory**: 512 MB RAM
- **Storage**: 10 GB SSD
- **Network**: 1 Gbps
- **OS**: Linux, macOS, or Windows

#### Recommended Requirements (Production - Planned)
- **CPU**: 8+ cores
- **Memory**: 16+ GB RAM
- **Storage**: 500+ GB NVMe SSD
- **Network**: 10+ Gbps
- **OS**: Linux (kernel 5.10+)

### Installation

#### From Binary

```bash
# Download the latest release
wget https://github.com/shawntherrien/streambus/releases/latest/download/streambus-linux-amd64.tar.gz

# Extract
tar -xzf streambus-linux-amd64.tar.gz

# Install
sudo mv streambus /usr/local/bin/
sudo chmod +x /usr/local/bin/streambus

# Verify installation
streambus --version
```

#### From Source

```bash
# Clone repository
git clone https://github.com/shawntherrien/streambus.git
cd streambus

# Install dependencies
go mod download

# Build
make build

# Install
sudo make install
```

#### Docker

```bash
# Pull image
docker pull streambus/streambus:latest

# Run server
docker run -d \
  --name streambus \
  -p 9092:9092 \
  -v /data/streambus:/var/lib/streambus \
  streambus/streambus:latest
```

### Single Node Deployment

#### Basic Server

```bash
# Start server with default configuration
streambus server

# Start with custom port
streambus server --port 9093

# Start with data directory
streambus server --data-dir /var/lib/streambus
```

#### Systemd Service

Create `/etc/systemd/system/streambus.service`:

```ini
[Unit]
Description=StreamBus Server
After=network.target
Documentation=https://github.com/shawntherrien/streambus

[Service]
Type=simple
User=streambus
Group=streambus
ExecStart=/usr/local/bin/streambus server \
  --config /etc/streambus/server.yaml \
  --data-dir /var/lib/streambus
Restart=always
RestartSec=10
LimitNOFILE=65536

# Resource limits
MemoryLimit=16G
CPUQuota=800%

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
# Create user
sudo useradd -r -s /bin/false streambus

# Create directories
sudo mkdir -p /var/lib/streambus /etc/streambus
sudo chown streambus:streambus /var/lib/streambus

# Enable service
sudo systemctl enable streambus
sudo systemctl start streambus

# Check status
sudo systemctl status streambus
```

### Docker Deployment

#### Docker Compose

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  streambus:
    image: streambus/streambus:latest
    container_name: streambus
    ports:
      - "9092:9092"
    volumes:
      - streambus-data:/var/lib/streambus
      - ./config:/etc/streambus:ro
    environment:
      - STREAMBUS_LOG_LEVEL=info
      - STREAMBUS_DATA_DIR=/var/lib/streambus
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "streambus", "health"]
      interval: 30s
      timeout: 10s
      retries: 3
    deploy:
      resources:
        limits:
          cpus: '4'
          memory: 8G
        reservations:
          cpus: '2'
          memory: 4G

volumes:
  streambus-data:
    driver: local
```

Start:

```bash
docker-compose up -d
```

### Kubernetes Deployment (Planned)

#### StatefulSet

Create `streambus-statefulset.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: streambus
  labels:
    app: streambus
spec:
  ports:
  - port: 9092
    name: client
  clusterIP: None
  selector:
    app: streambus
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: streambus
spec:
  serviceName: streambus
  replicas: 3
  selector:
    matchLabels:
      app: streambus
  template:
    metadata:
      labels:
        app: streambus
    spec:
      containers:
      - name: streambus
        image: streambus/streambus:latest
        ports:
        - containerPort: 9092
          name: client
        volumeMounts:
        - name: data
          mountPath: /var/lib/streambus
        env:
        - name: STREAMBUS_BROKER_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        resources:
          requests:
            memory: "4Gi"
            cpu: "2"
          limits:
            memory: "8Gi"
            cpu: "4"
        livenessProbe:
          exec:
            command:
            - streambus
            - health
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          exec:
            command:
            - streambus
            - health
          initialDelaySeconds: 5
          periodSeconds: 5
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: [ "ReadWriteOnce" ]
      storageClassName: fast-ssd
      resources:
        requests:
          storage: 100Gi
```

Deploy:

```bash
kubectl apply -f streambus-statefulset.yaml
```

---

## Configuration

### Server Configuration

Create `/etc/streambus/server.yaml`:

```yaml
# Server configuration
server:
  # Broker ID (unique per broker)
  broker_id: 1

  # Listen address
  listen: "0.0.0.0:9092"

  # Data directory
  data_dir: "/var/lib/streambus"

  # Log level (debug, info, warn, error)
  log_level: "info"

# Storage configuration
storage:
  # Write-ahead log sync policy (always, periodic, none)
  wal_sync: "periodic"

  # WAL sync interval (if periodic)
  wal_sync_interval: "10ms"

  # MemTable flush threshold (bytes)
  memtable_flush_threshold: 67108864  # 64MB

  # SSTable compaction policy (size-tiered, leveled)
  compaction_policy: "size-tiered"

  # Maximum SSTable file size
  max_sstable_size: 268435456  # 256MB

  # Retention policy
  retention:
    # Delete segments older than this (0 = never delete)
    max_age: "168h"  # 7 days

    # Delete segments when total size exceeds this (0 = no limit)
    max_bytes: 107374182400  # 100GB

# Network configuration
network:
  # Maximum request size (bytes)
  max_request_size: 1048576  # 1MB

  # Connection timeout
  connection_timeout: "30s"

  # Read timeout
  read_timeout: "30s"

  # Write timeout
  write_timeout: "30s"

  # Maximum concurrent connections
  max_connections: 10000

# Performance tuning
performance:
  # Number of I/O threads
  io_threads: 8

  # Network buffer sizes
  socket_send_buffer: 131072    # 128KB
  socket_receive_buffer: 131072 # 128KB

  # Batch processing
  batch_timeout: "10ms"
  max_batch_size: 1000

# Monitoring
monitoring:
  # Enable metrics endpoint
  metrics_enabled: true
  metrics_port: 9090

  # Enable profiling
  profiling_enabled: false
  profiling_port: 6060

# Security (planned)
security:
  # Enable TLS
  tls_enabled: false
  tls_cert: "/etc/streambus/tls/server.crt"
  tls_key: "/etc/streambus/tls/server.key"

  # Enable authentication
  auth_enabled: false
  auth_mechanism: "SCRAM-SHA-256"
```

### Environment Variables

```bash
# Server configuration
export STREAMBUS_BROKER_ID=1
export STREAMBUS_LISTEN="0.0.0.0:9092"
export STREAMBUS_DATA_DIR="/var/lib/streambus"
export STREAMBUS_LOG_LEVEL="info"

# Storage configuration
export STREAMBUS_WAL_SYNC="periodic"
export STREAMBUS_MEMTABLE_FLUSH_THRESHOLD=67108864

# Network configuration
export STREAMBUS_MAX_CONNECTIONS=10000
export STREAMBUS_CONNECTION_TIMEOUT="30s"

# Monitoring
export STREAMBUS_METRICS_ENABLED=true
export STREAMBUS_METRICS_PORT=9090
```

---

## Monitoring

### Health Checks

```bash
# Check server health
curl http://localhost:9092/health

# Check specific broker
streambus health --broker localhost:9092
```

### Metrics (Planned)

StreamBus will expose Prometheus-compatible metrics:

```bash
# Metrics endpoint
curl http://localhost:9090/metrics
```

#### Key Metrics

**Throughput Metrics:**
- `streambus_messages_produced_total` - Total messages produced
- `streambus_messages_consumed_total` - Total messages consumed
- `streambus_bytes_in_total` - Total bytes received
- `streambus_bytes_out_total` - Total bytes sent

**Latency Metrics:**
- `streambus_produce_latency_seconds` - Producer latency histogram
- `streambus_fetch_latency_seconds` - Consumer fetch latency histogram

**Storage Metrics:**
- `streambus_storage_bytes_used` - Disk space used
- `streambus_storage_bytes_available` - Disk space available
- `streambus_wal_size_bytes` - WAL file size
- `streambus_memtable_size_bytes` - MemTable size

**Connection Metrics:**
- `streambus_connections_active` - Active connections
- `streambus_connection_errors_total` - Connection errors

### Logging

#### Log Levels

- `debug` - Detailed debugging information
- `info` - Normal operational messages
- `warn` - Warning messages
- `error` - Error messages

#### Log Format

```json
{
  "time": "2025-01-07T10:30:45Z",
  "level": "info",
  "msg": "Message produced",
  "topic": "events",
  "partition": 0,
  "offset": 12345,
  "size": 1024
}
```

#### Log Aggregation

**Using Fluentd:**

```yaml
<source>
  @type tail
  path /var/log/streambus/*.log
  pos_file /var/log/fluentd/streambus.pos
  tag streambus
  format json
</source>

<match streambus>
  @type elasticsearch
  host elasticsearch
  port 9200
  index_name streambus
</match>
```

---

## Backup and Recovery

### Data Backup

#### Manual Backup

```bash
# Stop server
sudo systemctl stop streambus

# Backup data directory
sudo tar -czf streambus-backup-$(date +%Y%m%d).tar.gz \
  /var/lib/streambus

# Start server
sudo systemctl start streambus
```

#### Continuous Backup (Planned)

```bash
# Enable continuous backup
streambus backup enable \
  --destination s3://my-bucket/streambus-backups \
  --interval 1h

# List backups
streambus backup list

# Restore from backup
streambus backup restore \
  --backup-id 20250107-103045 \
  --target /var/lib/streambus
```

### Disaster Recovery

#### Recovery Procedures

1. **Data Corruption Recovery:**
   ```bash
   # Stop server
   sudo systemctl stop streambus

   # Verify WAL integrity
   streambus wal verify /var/lib/streambus/wal

   # Replay WAL
   streambus wal replay /var/lib/streambus/wal

   # Start server
   sudo systemctl start streambus
   ```

2. **Complete Node Failure (Planned):**
   - Provision new node
   - Restore from backup or replicate from other nodes
   - Rejoin cluster

---

## Performance Tuning

### Operating System Tuning

#### Linux Kernel Parameters

Edit `/etc/sysctl.conf`:

```bash
# Increase file descriptor limits
fs.file-max = 100000

# Network tuning
net.core.somaxconn = 4096
net.core.netdev_max_backlog = 5000
net.ipv4.tcp_max_syn_backlog = 8192
net.ipv4.tcp_syncookies = 1

# Increase TCP buffer sizes
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.tcp_rmem = 4096 87380 16777216
net.ipv4.tcp_wmem = 4096 65536 16777216

# Enable TCP Fast Open
net.ipv4.tcp_fastopen = 3

# Reduce TIME_WAIT sockets
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 30

# Apply changes
sudo sysctl -p
```

#### File Descriptor Limits

Edit `/etc/security/limits.conf`:

```bash
streambus soft nofile 65536
streambus hard nofile 65536
```

#### Disk I/O Scheduler

```bash
# Use deadline or noop scheduler for SSDs
echo deadline | sudo tee /sys/block/nvme0n1/queue/scheduler

# Disable write barriers (if using battery-backed RAID)
echo 0 | sudo tee /sys/block/nvme0n1/queue/add_random
```

### Storage Optimization

#### SSD Configuration

```bash
# Enable TRIM
sudo systemctl enable fstrim.timer

# Mount options for SSDs
# Add to /etc/fstab:
# /dev/nvme0n1 /var/lib/streambus ext4 defaults,noatime,discard 0 2
```

#### Storage Layout

```
/var/lib/streambus/
├── wal/           # Write-ahead log (fast SSD)
├── memtables/     # Active memtables (memory)
├── sstables/      # SSTables (SSD)
└── archived/      # Old segments (slower storage)
```

### Memory Tuning

```yaml
# Increase MemTable size for higher throughput
storage:
  memtable_flush_threshold: 134217728  # 128MB

# Adjust cache sizes
storage:
  block_cache_size: 536870912  # 512MB
  index_cache_size: 268435456  # 256MB
```

### Network Tuning

```yaml
network:
  # Increase buffer sizes for high throughput
  socket_send_buffer: 262144    # 256KB
  socket_receive_buffer: 262144 # 256KB

  # Adjust batch settings
performance:
  batch_timeout: "5ms"    # Lower for lower latency
  max_batch_size: 10000   # Higher for higher throughput
```

---

## Security

> **Note**: Security features are planned for future milestones.

### TLS/SSL Encryption

```yaml
security:
  tls_enabled: true
  tls_cert: "/etc/streambus/tls/server.crt"
  tls_key: "/etc/streambus/tls/server.key"
  tls_ca: "/etc/streambus/tls/ca.crt"
```

Generate certificates:

```bash
# Generate CA
openssl genrsa -out ca-key.pem 4096
openssl req -new -x509 -days 3650 -key ca-key.pem -out ca.pem

# Generate server certificate
openssl genrsa -out server-key.pem 4096
openssl req -new -key server-key.pem -out server.csr
openssl x509 -req -days 365 -in server.csr -CA ca.pem \
  -CAkey ca-key.pem -CAcreateserial -out server.pem
```

### Authentication

```yaml
security:
  auth_enabled: true
  auth_mechanism: "SCRAM-SHA-256"
  users_file: "/etc/streambus/users.yaml"
```

### Authorization (Planned)

```yaml
# Define ACLs
acls:
  - principal: "user:alice"
    operations: ["read", "write"]
    resources: ["topic:events"]

  - principal: "user:bob"
    operations: ["read"]
    resources: ["topic:events", "topic:logs"]
```

### Network Security

```bash
# Firewall rules (iptables)
sudo iptables -A INPUT -p tcp --dport 9092 -s 10.0.0.0/8 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 9092 -j DROP

# Or using firewalld
sudo firewall-cmd --permanent --add-rich-rule='
  rule family="ipv4"
  source address="10.0.0.0/8"
  port protocol="tcp" port="9092" accept'
sudo firewall-cmd --reload
```

---

## Troubleshooting

### Common Issues

#### High Memory Usage

**Symptoms:**
- Server OOM kills
- Slow performance

**Solutions:**
```bash
# Reduce MemTable size
storage:
  memtable_flush_threshold: 33554432  # 32MB

# Reduce cache sizes
storage:
  block_cache_size: 268435456  # 256MB
  index_cache_size: 134217728  # 128MB

# Check for memory leaks
streambus debug heap-profile
```

#### High Disk I/O

**Symptoms:**
- Slow writes
- High disk utilization

**Solutions:**
```bash
# Check disk I/O stats
iostat -x 1

# Adjust compaction strategy
storage:
  compaction_policy: "leveled"

# Increase MemTable size to reduce flushes
storage:
  memtable_flush_threshold: 134217728  # 128MB
```

#### Connection Timeouts

**Symptoms:**
- Clients timing out
- Connection refused errors

**Solutions:**
```bash
# Check server logs
journalctl -u streambus -f

# Increase connection limits
network:
  max_connections: 20000
  connection_timeout: "60s"

# Check network connectivity
telnet localhost 9092

# Check file descriptor limits
ulimit -n
```

#### Slow Queries

**Symptoms:**
- High fetch latency
- Slow consumer performance

**Solutions:**
```bash
# Enable query profiling
streambus profile --duration 60s

# Check indexes
streambus debug index-stats

# Increase fetch buffer size
consumer:
  max_fetch_bytes: 2097152  # 2MB
```

### Debug Tools

#### Server Diagnostics

```bash
# Check server status
streambus status

# View active connections
streambus connections list

# Check topic statistics
streambus topics stats

# Dump configuration
streambus config dump
```

#### Performance Profiling

```bash
# CPU profiling
streambus profile cpu --duration 30s --output cpu.prof

# Memory profiling
streambus profile mem --output mem.prof

# Analyze with pprof
go tool pprof cpu.prof
```

### Log Analysis

```bash
# View error logs
journalctl -u streambus -p err -f

# Search for specific errors
journalctl -u streambus | grep "connection failed"

# Export logs
journalctl -u streambus --since "1 hour ago" > streambus.log
```

---

## Maintenance

### Routine Maintenance

#### Daily Tasks
- Monitor disk space usage
- Check error logs
- Review performance metrics

#### Weekly Tasks
- Review backup status
- Analyze slow queries
- Check for software updates

#### Monthly Tasks
- Storage cleanup and compaction
- Security audit
- Capacity planning review

### Upgrades

#### Rolling Upgrade (Planned for Multi-Broker)

```bash
# Upgrade broker one at a time
for broker in broker-1 broker-2 broker-3; do
  # Stop broker
  ssh $broker "sudo systemctl stop streambus"

  # Upgrade binary
  ssh $broker "sudo streambus upgrade"

  # Start broker
  ssh $broker "sudo systemctl start streambus"

  # Wait for replication to catch up
  sleep 60
done
```

#### Compatibility

Check compatibility before upgrading:

```bash
streambus compatibility check --version 0.2.0
```

### Data Cleanup

```bash
# Manual cleanup of old segments
streambus cleanup --older-than 7d

# Compact storage
streambus compact --force

# Check storage stats
streambus storage stats
```

---

## Best Practices

1. **Always monitor disk space** - Set up alerts for >80% usage
2. **Use SSD storage** - Prefer NVMe SSDs for best performance
3. **Regular backups** - Automate daily backups to remote storage
4. **Monitoring** - Set up comprehensive monitoring and alerting
5. **Load testing** - Test your setup under expected load before production
6. **Capacity planning** - Plan for 2x expected load
7. **Security** - Enable TLS and authentication in production
8. **Documentation** - Document your specific configuration and procedures

---

## See Also

- [Architecture Documentation](ARCHITECTURE.md)
- [API Reference](api-reference.md)
- [Performance Benchmarks](BENCHMARKS.md)
- [Getting Started Guide](GETTING_STARTED.md)
