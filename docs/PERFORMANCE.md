# StreamBus Performance Tuning Guide

Comprehensive guide to optimizing StreamBus performance for your workload.

## Table of Contents

- [Overview](#overview)
- [Performance Targets](#performance-targets)
- [Configuration Tuning](#configuration-tuning)
- [Storage Optimization](#storage-optimization)
- [Network Optimization](#network-optimization)
- [Memory Management](#memory-management)
- [Zero-Copy Optimizations](#zero-copy-optimizations)
- [Profiling and Diagnostics](#profiling-and-diagnostics)
- [Common Bottlenecks](#common-bottlenecks)

---

## Overview

StreamBus performance depends on multiple factors:

1. **Hardware**: CPU, memory, disk I/O, network bandwidth
2. **Configuration**: Batch sizes, buffer sizes, fsync policy
3. **Workload**: Message size, throughput requirements, latency sensitivity
4. **Deployment**: Single node vs. cluster, replication factor

### Performance Characteristics

| Component | Typical Performance | Notes |
|-----------|---------------------|-------|
| Producer Throughput | 100K+ msgs/s | 1KB messages, batched |
| Consumer Throughput | 100K+ msgs/s | Sequential reads |
| Produce Latency (p99) | < 10ms | Without fsync |
| Consume Latency (p99) | < 5ms | Warm cache |
| Storage Append | 200K+ ops/s | In-memory, no fsync |

---

## Performance Targets

### Throughput Workloads

For high-throughput scenarios:

```yaml
# config/broker-throughput.yaml
storage:
  wal:
    segment_size: 1073741824  # 1GB segments
    fsync_policy: "interval"
    fsync_interval: "1s"       # Batch fsyncs
  memtable:
    max_size: 134217728        # 128MB memtable
    num_immutable: 4           # More immutable memtables

server:
  max_connections: 10000
  read_buffer_size: 65536      # 64KB read buffer
  write_buffer_size: 65536     # 64KB write buffer

producer:
  batch_size: 1000             # Batch 1000 messages
  batch_timeout: "100ms"       # Max 100ms batching delay
  compression: "lz4"           # Fast compression
```

**Expected Results:**
- Throughput: 150K+ msgs/s (1KB messages)
- Latency: p99 < 15ms
- CPU: 60-80% under load
- Memory: 2-4GB

### Latency-Sensitive Workloads

For low-latency scenarios:

```yaml
# config/broker-latency.yaml
storage:
  wal:
    segment_size: 536870912    # 512MB segments
    fsync_policy: "always"     # Immediate durability
  memtable:
    max_size: 67108864         # 64MB memtable
    num_immutable: 2

server:
  read_buffer_size: 8192       # Smaller buffers
  write_buffer_size: 8192

producer:
  batch_size: 10               # Small batches
  batch_timeout: "10ms"        # Quick batching
  compression: "none"          # No compression overhead
```

**Expected Results:**
- Throughput: 50K+ msgs/s
- Latency: p99 < 5ms, p999 < 10ms
- CPU: 40-60% under load

### Balanced Workload

General-purpose configuration:

```yaml
# config/broker.yaml (default)
storage:
  wal:
    segment_size: 536870912    # 512MB
    fsync_policy: "interval"
    fsync_interval: "500ms"    # Balanced
  memtable:
    max_size: 67108864         # 64MB
    num_immutable: 2

producer:
  batch_size: 100
  batch_timeout: "50ms"
  compression: "snappy"        # Good compression/speed ratio
```

---

## Configuration Tuning

### Storage Configuration

#### WAL Settings

**Segment Size:**
```yaml
storage:
  wal:
    segment_size: 536870912    # 512MB (default)
```

- Larger segments (1GB+): Better for high throughput, fewer file handles
- Smaller segments (256MB): Faster log rotation, better for retention policies

**Fsync Policy:**

```yaml
storage:
  wal:
    fsync_policy: "interval"   # Options: always, interval, never
    fsync_interval: "500ms"    # If policy is "interval"
```

| Policy | Durability | Performance | Use Case |
|--------|------------|-------------|----------|
| `always` | Highest | Lowest | Financial, critical data |
| `interval` | Medium | Medium | General purpose |
| `never` | None | Highest | Development, caching |

**Performance Impact:**
- `always`: ~1K writes/s (limited by disk)
- `interval (1s)`: ~100K+ writes/s
- `never`: ~200K+ writes/s

#### MemTable Settings

```yaml
storage:
  memtable:
    max_size: 67108864         # 64MB
    num_immutable: 2           # Number of immutable memtables
```

**Tuning Guidelines:**
- **Larger MemTable**: Fewer compactions, more memory usage
- **More Immutable**: Better write throughput, more memory
- **Recommended**: 64-128MB per memtable, 2-4 immutable

**Memory Usage Calculation:**
```
Total Memory = max_size * (1 + num_immutable) * num_partitions
Example: 64MB * 3 * 10 partitions = 1.92GB
```

### Network Configuration

```yaml
server:
  max_connections: 1000              # Max concurrent connections
  read_buffer_size: 32768            # 32KB (default)
  write_buffer_size: 32768           # 32KB
  read_timeout: "30s"
  write_timeout: "30s"
  idle_timeout: "5m"
```

**Buffer Size Tuning:**
- Small messages (< 1KB): 8-16KB buffers
- Medium messages (1-10KB): 32-64KB buffers
- Large messages (> 10KB): 64-128KB buffers

### Producer Configuration

```yaml
producer:
  batch_size: 100                    # Messages per batch
  batch_timeout: "50ms"              # Max batching delay
  compression: "snappy"              # none, gzip, snappy, lz4
  max_retries: 3
  retry_backoff: "100ms"
```

**Batch Size Impact:**

| Batch Size | Throughput | Latency | Use Case |
|------------|------------|---------|----------|
| 1-10 | Low | Very Low | Latency-sensitive |
| 50-100 | Medium | Low | Balanced |
| 500-1000 | High | Medium | Throughput-focused |
| 5000+ | Very High | High | Bulk ingestion |

**Compression Comparison:**

| Algorithm | Ratio | Speed | CPU Usage |
|-----------|-------|-------|-----------|
| none | 1.0x | Fastest | Lowest |
| lz4 | 2.0x | Fast | Low |
| snappy | 2.5x | Medium | Medium |
| gzip | 3.0x | Slow | High |

### Consumer Configuration

```yaml
consumer:
  fetch_min_bytes: 1024              # Min bytes to fetch
  fetch_max_bytes: 10485760          # 10MB max
  fetch_max_wait: "500ms"            # Max wait for fetch_min_bytes
  max_poll_records: 500              # Records per poll
```

**Tuning for Throughput:**
```yaml
consumer:
  fetch_min_bytes: 10485760          # 10MB min
  fetch_max_bytes: 52428800          # 50MB max
  fetch_max_wait: "100ms"            # Quick polls
  max_poll_records: 5000             # Large batches
```

**Tuning for Latency:**
```yaml
consumer:
  fetch_min_bytes: 1                 # Immediate return
  fetch_max_bytes: 1048576           # 1MB max
  fetch_max_wait: "10ms"             # Very quick
  max_poll_records: 100              # Small batches
```

---

## Storage Optimization

### Disk Selection

**SSD vs. HDD:**

| Metric | SSD | HDD |
|--------|-----|-----|
| Random IOPS | 10K-100K+ | 100-200 |
| Sequential MB/s | 500-3000 | 100-200 |
| Latency | < 1ms | 5-15ms |
| Recommended for | Production | Development/Archive |

**Best Practices:**
1. Use SSDs for WAL and active segments
2. HDFs acceptable for cold storage/archives
3. Separate data and WAL on different drives if possible

### Compaction Tuning

```yaml
storage:
  compaction:
    max_level: 7                     # LSM tree levels
    level0_file_num_compaction_trigger: 4
    level0_slowdown_writes_trigger: 8
    level0_stop_writes_trigger: 12
```

**Impact:**
- Fewer triggers: Less compaction overhead, more disk usage
- More triggers: More compaction overhead, better read performance

### Retention and Cleanup

```yaml
storage:
  retention:
    time: "168h"                     # 7 days
    bytes: 107374182400              # 100GB
  cleanup_interval: "5m"
```

**Performance Tips:**
- Run cleanup during off-peak hours
- Use time-based retention for predictable disk usage
- Monitor segment count and disk usage

---

## Network Optimization

### TCP Tuning

**Linux sysctl settings:**

```bash
# /etc/sysctl.conf

# Increase TCP buffer sizes
net.core.rmem_max = 134217728          # 128MB
net.core.wmem_max = 134217728
net.ipv4.tcp_rmem = 4096 87380 134217728
net.ipv4.tcp_wmem = 4096 65536 134217728

# Increase connection backlog
net.core.somaxconn = 4096
net.ipv4.tcp_max_syn_backlog = 8192

# Enable TCP window scaling
net.ipv4.tcp_window_scaling = 1

# Reduce TIME_WAIT sockets
net.ipv4.tcp_fin_timeout = 30
net.ipv4.tcp_tw_reuse = 1

# Apply changes
sudo sysctl -p
```

### Network Bandwidth

**Calculate Required Bandwidth:**

```
Bandwidth (Mbps) = (Messages/sec * Avg Message Size * 8) / 1,000,000

Example:
100,000 msgs/s * 1KB * 8 = 800 Mbps
With replication factor 3: 2.4 Gbps
```

**Recommendations:**
- 1 Gbps: Up to 50K msgs/s (1KB each)
- 10 Gbps: Up to 500K msgs/s
- Use dedicated network for inter-broker replication

---

## Memory Management

### Memory Requirements

**Calculate Total Memory:**

```
Base Memory:      512MB (broker overhead)
MemTables:        max_size * (1 + num_immutable) * partitions
Block Cache:      storage.block_cache_size
Connection Buffers: (read_buf + write_buf) * max_connections
Replication:      Additional 20-30% for cluster

Example for 10 partitions, 1000 connections:
Base:             512MB
MemTables:        64MB * 3 * 10 = 1.92GB
Block Cache:      512MB
Connections:      64KB * 1000 = 64MB
Replication:      30% = ~900MB
Total:            ~4GB
```

**Recommended Allocations:**
- Small deployment: 4-8GB RAM
- Medium deployment: 16-32GB RAM
- Large deployment: 64-128GB+ RAM

### GC Tuning

**Go GC Environment Variables:**

```bash
# Reduce GC frequency (trade memory for CPU)
export GOGC=200

# Increase memory limit (Go 1.19+)
export GOMEMLIMIT=8GiB

# Run broker
./bin/streambus-broker
```

**Monitoring GC:**

```bash
# Enable GC trace
export GODEBUG=gctrace=1

# View GC statistics
curl http://localhost:8080/debug/vars | jq '.memstats'
```

---

## Zero-Copy Optimizations

### Buffer Pooling

StreamBus uses buffer pools to reduce allocations:

```go
import "github.com/shawntherrien/streambus/pkg/protocol"

// Use pooled buffers
buf := protocol.GetBuffer(4096)
defer protocol.PutBuffer(buf)

// Or use convenience wrapper
err := protocol.WithBuffer(4096, func(buf []byte) error {
    // Use buffer
    return nil
})
```

**Benefits:**
- Reduces GC pressure
- Lower allocation rate
- Better performance under high load

### Message Batching

Batch messages to reduce syscalls:

```go
producer, _ := client.NewProducer()

batch := make([]client.Message, 0, 1000)
for i := 0; i < 1000; i++ {
    batch = append(batch, client.Message{
        Key:   []byte(fmt.Sprintf("key-%d", i)),
        Value: []byte("message"),
    })
}

// Send entire batch in one operation
producer.SendBatch(ctx, "topic", batch)
```

### Direct I/O

For specific workloads, consider direct I/O:

```yaml
storage:
  use_direct_io: true                # Bypass OS page cache
```

**When to Use:**
- Very large working sets (> available RAM)
- Want to avoid double-buffering
- Have fast SSDs

**Trade-offs:**
- May reduce performance for small messages
- Requires proper alignment
- More complex error handling

---

## Profiling and Diagnostics

### CPU Profiling

**Collect CPU Profile:**

```bash
# Using scripts
./scripts/profile-cpu.sh BenchmarkE2E_ProducerThroughput

# Manual
go test -bench=BenchmarkE2E_ProducerThroughput \
    -cpuprofile=cpu.prof \
    -benchtime=60s \
    ./bench
```

**Analyze Profile:**

```bash
# Top CPU consumers
go tool pprof -text -top20 cpu.prof

# Interactive
go tool pprof cpu.prof
(pprof) top20
(pprof) list FunctionName
(pprof) web

# Web UI
go tool pprof -http=:8081 cpu.prof
```

### Memory Profiling

**Collect Memory Profile:**

```bash
# Using scripts
./scripts/profile-memory.sh -a BenchmarkE2E_ProducerThroughput

# Manual
go test -bench=BenchmarkE2E_ProducerThroughput \
    -memprofile=mem.prof \
    -benchtime=60s \
    ./bench
```

**Analyze Allocations:**

```bash
# Top allocation sites
go tool pprof -text -top20 -sample_index=alloc_objects mem.prof

# Allocation sizes
go tool pprof -text -top20 -sample_index=alloc_space mem.prof

# In-use memory
go tool pprof -text -top20 -sample_index=inuse_space mem.prof

# Interactive
go tool pprof mem.prof
(pprof) top20 -cum
(pprof) list FunctionName
```

### Execution Tracing

**Collect Trace:**

```bash
go test -bench=BenchmarkE2E_ProducerThroughput \
    -trace=trace.out \
    -benchtime=10s \
    ./bench

# View trace
go tool trace trace.out
```

**Trace Analysis:**
- View goroutine execution
- Identify blocking operations
- Find GC pauses
- See syscall timing

### Performance Analysis Script

```bash
# Automated analysis
./scripts/analyze-performance.go -profile bench/results/current.txt

# Output
✗ [CRITICAL] BenchmarkE2E_ProducerThroughput: Excessive allocations (1200 allocs/op)
  Impact: High GC pressure, reduced throughput
  Suggestion: Review code for unnecessary allocations. Consider object pooling.

⚠ [WARNING] BenchmarkE2E_ProduceLatency: High p99 latency (120ms)
  Impact: Poor tail latency for latency-sensitive applications
  Suggestion: Investigate tail latency causes. Check for GC pauses or lock contention.
```

---

## Common Bottlenecks

### 1. Disk I/O

**Symptoms:**
- High iowait CPU
- Slow produce latency
- Compaction backlog

**Solutions:**
- Use faster SSDs
- Tune fsync policy
- Increase segment size
- Reduce replication factor

### 2. Network Bandwidth

**Symptoms:**
- High network utilization
- Increasing consumer lag
- Replication delays

**Solutions:**
- Upgrade network (1 Gbps → 10 Gbps)
- Enable compression
- Reduce replication factor
- Add more brokers (horizontal scaling)

### 3. CPU Saturation

**Symptoms:**
- High CPU usage (>90%)
- Increasing request latency
- Thread pool exhaustion

**Solutions:**
- Profile to find hot functions
- Disable compression
- Reduce concurrent connections
- Scale horizontally

### 4. Memory Pressure

**Symptoms:**
- Frequent GC pauses
- OOM errors
- Slow metadata operations

**Solutions:**
- Increase GOGC value
- Reduce memtable sizes
- Limit concurrent connections
- Add more RAM

### 5. Lock Contention

**Symptoms:**
- High CPU but low throughput
- Thread blocked time
- Mutex wait time in profiles

**Solutions:**
- Review locking strategy
- Use lock-free data structures
- Partition data across goroutines
- Profile with mutex profiling

### 6. Message Size Mismatch

**Symptoms:**
- High allocation rate
- Buffer growth/reallocation

**Solutions:**
- Tune buffer sizes to match message sizes
- Use appropriate batch sizes
- Consider message size limits

---

## Benchmarking

### Running Benchmarks

```bash
# All benchmarks
./scripts/run-benchmarks.sh

# Specific suite
./scripts/run-benchmarks.sh throughput

# With profiling
./scripts/run-benchmarks.sh \
    --cpu-profile=cpu.prof \
    --mem-profile=mem.prof

# Save baseline
./scripts/run-benchmarks.sh --save-baseline

# Compare with baseline
./scripts/run-benchmarks.sh --compare
```

### Interpreting Results

```
BenchmarkE2E_ProducerThroughput-8    50000    23456 ns/op    95.2 MB/s    98234 msgs/s    1200 B/op    15 allocs/op
```

**Key Metrics:**
- `ns/op`: Time per operation (lower is better)
- `MB/s`: Throughput in megabytes/sec
- `msgs/s`: Messages per second
- `B/op`: Bytes allocated per operation (lower is better)
- `allocs/op`: Allocations per operation (lower is better)

**Performance Regression Detection:**

```bash
# Run baseline
go test -bench=. ./bench > baseline.txt

# Make changes...

# Compare
go test -bench=. ./bench > new.txt
benchstat baseline.txt new.txt

# Output
name                          old time/op    new time/op    delta
E2E_ProducerThroughput-8        23.5µs ± 2%    18.2µs ± 1%  -22.55%  (p=0.000 n=10+10)

name                          old alloc/op   new alloc/op   delta
E2E_ProducerThroughput-8        1.20kB ± 0%    0.85kB ± 0%  -29.17%  (p=0.000 n=10+10)
```

---

## Best Practices

### 1. Start with Defaults

Begin with default configuration and measure:

```bash
# Start broker with defaults
./bin/streambus-broker --config config/broker.yaml

# Run load test
./scripts/run-load-test.sh

# Collect metrics
curl http://localhost:8080/metrics
```

### 2. Identify Bottleneck

Use profiling to find the limiting factor:

```bash
# Profile under load
./scripts/profile-cpu.sh BenchmarkE2E_ProducerThroughput
./scripts/profile-memory.sh BenchmarkE2E_ProducerThroughput
```

### 3. Tune One Parameter at a Time

Change one setting, measure impact:

```yaml
# Change 1: Increase batch size
producer:
  batch_size: 500  # Was 100

# Measure throughput improvement
# If better, keep change and move to next parameter
```

### 4. Monitor in Production

Use Prometheus/Grafana dashboards:

```yaml
# Key metrics to watch
- streambus_produce_latency_seconds (p99, p999)
- streambus_messages_produced_total (rate)
- streambus_storage_used_bytes
- streambus_network_bytes_in_total (rate)
- go_gc_duration_seconds (p99)
```

### 5. Load Test Before Production

Simulate production workload:

```bash
# Generate load
./scripts/run-load-test.sh \
    --producers 10 \
    --consumers 5 \
    --message-size 1024 \
    --target-rate 100000 \
    --duration 1h

# Monitor metrics during test
watch -n 1 'curl -s http://localhost:8080/metrics | grep streambus_'
```

---

## Related Documentation

- [Benchmarking Guide](../bench/README.md)
- [Monitoring Guide](./MONITORING.md)
- [Architecture Overview](./ARCHITECTURE.md)
- [Operations Guide](./OPERATIONS.md)

---

## Support

- **Issues**: https://github.com/shawntherrien/streambus/issues
- **Performance Questions**: https://github.com/shawntherrien/streambus/discussions
- **Community**: https://discord.gg/streambus
