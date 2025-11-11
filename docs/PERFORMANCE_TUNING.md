# StreamBus Performance Tuning Guide

Comprehensive guide for optimizing StreamBus performance.

## Table of Contents

- [Quick Wins](#quick-wins)
- [Broker Tuning](#broker-tuning)
- [Client Tuning](#client-tuning)
- [Storage Optimization](#storage-optimization)
- [Network Optimization](#network-optimization)
- [OS Tuning](#os-tuning)
- [Monitoring Performance](#monitoring-performance)

## Quick Wins

Start with these high-impact optimizations:

### 1. Enable Batching

**Producer:**
```go
producer := client.NewProducer(&client.ProducerConfig{
    BatchSize: 1000,           // Batch up to 1000 messages
    BatchTimeout: 10ms,        // Or every 10ms
    Compression: "lz4",        // Enable compression
})
```

**Impact:** 5-10x throughput improvement

### 2. Tune Acknowledgments

```go
// For maximum throughput (at-most-once)
producer := client.NewProducer(&client.ProducerConfig{
    Acks: client.AcksLeader,  // Only wait for leader
})

// For durability (at-least-once)
producer := client.NewProducer(&client.ProducerConfig{
    Acks: client.AcksAll,     // Wait for all replicas
})
```

### 3. Increase Partitions

More partitions = more parallelism:

```bash
# Create topic with many partitions
streambus-cli topic create my-topic --partitions 20
```

**Rule of thumb:** `partitions >= max(producers, consumers)`

### 4. Use SSD Storage

NVMe SSD vs HDD can provide 100x improvement for random I/O.

## Broker Tuning

### Memory Configuration

**Heap size:**
```yaml
# Set to 50-75% of available RAM
resources:
  limits:
    memory: 16Gi
  requests:
    memory: 12Gi
```

**Page cache:**
- Leave 25-50% of RAM for OS page cache
- Critical for read performance

### Thread Pool Sizing

```yaml
server:
  workerThreads: 8        # CPU cores
  ioThreads: 4            # CPU cores / 2
  maxConnections: 10000   # Based on load
```

### Storage Configuration

```yaml
storage:
  # Sync frequency
  syncInterval: 1s        # Lower = more durable, less throughput

  # Compaction
  compactionInterval: 1h  # Adjust based on write rate
  compactionThreshold: 0.3 # Compact when 30% dead data

  # Caching
  cacheSize: 512MB        # Message cache
  indexCacheSize: 128MB   # Index cache
```

### Replication

```yaml
cluster:
  replicationFactor: 3    # Balance durability vs performance

raft:
  heartbeatTimeout: 1s    # Lower = faster failover, more overhead
  electionTimeout: 3s     # 3x heartbeat timeout
  maxAppendEntries: 1000  # Batch size for Raft logs
```

## Client Tuning

### Producer Optimization

**High Throughput:**
```go
config := &client.ProducerConfig{
    // Batching
    BatchSize:    10000,
    BatchTimeout: 100 * time.Millisecond,

    // Compression
    Compression: "lz4",  // Fast compression

    // Buffering
    BufferSize: 100000,  // Large buffer

    // Async sends
    Async: true,

    // Acknowledgments
    Acks: client.AcksLeader,  // Faster but less durable

    // Retries
    Retries:      3,
    RetryBackoff: 100 * time.Millisecond,
}
```

**Low Latency:**
```go
config := &client.ProducerConfig{
    // Small batches
    BatchSize:    10,
    BatchTimeout: 1 * time.Millisecond,

    // No compression
    Compression: "none",

    // Sync sends
    Async: false,

    // All acks
    Acks: client.AcksAll,
}
```

**Balanced:**
```go
config := &client.ProducerConfig{
    BatchSize:    1000,
    BatchTimeout: 10 * time.Millisecond,
    Compression:  "lz4",
    Async:        true,
    Acks:         client.AcksAll,
}
```

### Consumer Optimization

```go
config := &client.ConsumerConfig{
    // Fetch size
    MinFetchSize: 1024,      // 1 KB minimum
    MaxFetchSize: 10485760,  // 10 MB maximum

    // Fetch wait
    MaxWaitTime: 500 * time.Millisecond,

    // Prefetching
    PrefetchCount: 1000,     // Prefetch messages

    // Parallelism
    Concurrency: 10,         // Process 10 messages concurrently

    // Auto-commit
    AutoCommit:         true,
    AutoCommitInterval: 1 * time.Second,
}
```

### Connection Pooling

```go
// Reuse connections
clientPool := client.NewClientPool(&client.PoolConfig{
    MaxConns:     100,
    MinConns:     10,
    MaxIdleTime:  5 * time.Minute,
    DialTimeout:  5 * time.Second,
})
```

## Storage Optimization

### Disk Configuration

**File system:**
```bash
# Use XFS or ext4
mkfs.xfs -f /dev/nvme0n1

# Mount with noatime
mount -o noatime /dev/nvme0n1 /data
```

**/etc/fstab:**
```
/dev/nvme0n1  /data  xfs  noatime,nodiratime  0  0
```

### Storage Layout

```
/data/streambus/
├── data/         # Message data (large, sequential)
├── raft/         # Raft logs (small, random)
└── index/        # Indexes (random access)
```

**Best practice:** Separate disks for data and Raft logs

### I/O Scheduler

```bash
# For SSD, use none/noop
echo none > /sys/block/nvme0n1/queue/scheduler

# For HDD, use deadline
echo deadline > /sys/block/sda/queue/scheduler
```

### Read-Ahead

```bash
# Increase read-ahead for sequential reads
blockdev --setra 8192 /dev/nvme0n1
```

## Network Optimization

### TCP Tuning

```bash
# /etc/sysctl.conf

# Increase buffer sizes
net.core.rmem_max = 134217728
net.core.wmem_max = 134217728
net.ipv4.tcp_rmem = 4096 87380 67108864
net.ipv4.tcp_wmem = 4096 65536 67108864

# Increase connection backlog
net.core.somaxconn = 4096
net.ipv4.tcp_max_syn_backlog = 8192

# Enable TCP fast open
net.ipv4.tcp_fastopen = 3

# Disable slow start after idle
net.ipv4.tcp_slow_start_after_idle = 0

# Apply
sysctl -p
```

### Jumbo Frames

```bash
# If your network supports it (datacenter)
ip link set eth0 mtu 9000
```

### Connection Pooling

```yaml
server:
  keepAliveTime: 60s
  keepAliveTimeout: 20s
  maxIdleConns: 100
  idleTimeout: 90s
```

## OS Tuning

### File Descriptors

```bash
# /etc/security/limits.conf
streambus soft nofile 65536
streambus hard nofile 65536

# /etc/sysctl.conf
fs.file-max = 2097152
```

### Memory

```bash
# /etc/sysctl.conf

# Swappiness
vm.swappiness = 1  # Minimize swapping

# Dirty pages
vm.dirty_ratio = 15
vm.dirty_background_ratio = 5

# Transparent huge pages (disable for predictable latency)
echo never > /sys/kernel/mm/transparent_hugepage/enabled
```

### CPU

```bash
# Disable CPU frequency scaling
for i in /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor; do
    echo performance > $i
done
```

### NUMA

For multi-socket systems:

```bash
# Pin broker to specific NUMA node
numactl --cpunodebind=0 --membind=0 streambus-broker
```

## Monitoring Performance

### Key Metrics

**Latency:**
```promql
# p99 latency
histogram_quantile(0.99,
  rate(streambus_message_latency_seconds_bucket[5m]))

# Check if > 100ms
histogram_quantile(0.99,
  rate(streambus_message_latency_seconds_bucket[5m])) > 0.1
```

**Throughput:**
```promql
# Messages per second
rate(streambus_messages_total[5m])

# Check if below target
rate(streambus_messages_total[5m]) < 10000
```

**Resource Usage:**
```promql
# CPU usage
rate(process_cpu_seconds_total{job="streambus"}[5m])

# Memory usage
process_resident_memory_bytes{job="streambus"}

# Disk I/O
rate(node_disk_io_time_seconds_total[5m])
```

### Profiling

**CPU profiling:**
```bash
# Enable profiling
curl http://broker:8081/debug/pprof/profile?seconds=30 > cpu.prof

# Analyze
go tool pprof cpu.prof
```

**Memory profiling:**
```bash
# Heap profile
curl http://broker:8081/debug/pprof/heap > heap.prof

# Analyze
go tool pprof heap.prof
```

**Continuous profiling:**
```bash
# Pyroscope integration
pyroscope exec streambus-broker \
  --application-name streambus \
  --server-address http://pyroscope:4040
```

## Performance Patterns

### Pattern 1: High Throughput, Relaxed Latency

**Use case:** Log aggregation, analytics

**Configuration:**
- Large batch sizes (10,000)
- Async sends
- Leader acknowledgment only
- LZ4 compression
- Many partitions

**Expected:** 100,000+ msgs/sec, 10-50ms latency

### Pattern 2: Low Latency, Moderate Throughput

**Use case:** Real-time notifications, trading systems

**Configuration:**
- Small batch sizes (10-100)
- Sync sends or small async batches
- All acknowledgments
- No compression
- Fewer partitions (less coordination)

**Expected:** 10,000-50,000 msgs/sec, <5ms latency

### Pattern 3: Balanced

**Use case:** General-purpose messaging

**Configuration:**
- Medium batch sizes (1,000)
- Async sends with small timeout
- All acknowledgments
- LZ4 compression
- Moderate partitions

**Expected:** 50,000+ msgs/sec, 5-20ms latency

## Troubleshooting Performance

### High Latency

**Symptoms:** p99 latency > 100ms

**Checks:**
1. Network latency between client and broker
2. Disk I/O latency (check `iostat`)
3. CPU usage (check for saturation)
4. GC pauses (check GC logs)
5. Large batch sizes causing delay

**Fixes:**
- Reduce batch timeout
- Use faster storage (NVMe SSD)
- Scale horizontally (add brokers)
- Tune GC settings

### Low Throughput

**Symptoms:** Messages/sec below target

**Checks:**
1. Producer batch size (too small?)
2. Number of partitions (too few?)
3. Network bandwidth (saturated?)
4. Disk I/O (bottleneck?)
5. CPU usage (saturated?)

**Fixes:**
- Increase batch size
- Add more partitions
- Enable compression
- Scale horizontally

### High CPU Usage

**Symptoms:** CPU > 80%

**Checks:**
1. Message rate
2. Compression overhead
3. TLS overhead
4. Inefficient code paths

**Fixes:**
- Profile with pprof
- Optimize hot paths
- Scale horizontally
- Use hardware acceleration (AES-NI for TLS)

### High Memory Usage

**Symptoms:** Memory growing unbounded

**Checks:**
1. Message buffering (too large?)
2. Memory leaks (profile with pprof)
3. Cache sizes (too large?)

**Fixes:**
- Reduce buffer sizes
- Fix memory leaks
- Tune cache sizes
- Restart broker (temporary)

## Benchmarking

Always benchmark your specific workload:

```bash
# Run load test
go run ./tests/load/cmd/loadtest \
  --scenario throughput \
  --duration 10m \
  --rate 10000 \
  --producers 10 \
  --consumers 10

# Compare results
make benchmark-compare
```

## Checklist

Performance tuning checklist:

- [ ] Baseline metrics captured
- [ ] Bottleneck identified (CPU, memory, disk, network)
- [ ] Batch sizes tuned
- [ ] Compression configured
- [ ] Partitions optimized
- [ ] OS parameters tuned
- [ ] Storage optimized
- [ ] Network optimized
- [ ] Load tested
- [ ] Results documented

## See Also

- [Benchmarking Guide](../BENCHMARKING.md)
- [Production Deployment](PRODUCTION.md)
- [Load Testing](../tests/load/README.md)
