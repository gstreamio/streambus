# StreamBus Benchmarks

Comprehensive benchmarking suite for StreamBus performance testing.

## Overview

This directory contains organized benchmark suites covering:

- **Throughput Benchmarks**: End-to-end and storage layer throughput testing
- **Latency Benchmarks**: Produce, consume, and round-trip latency distribution
- **Concurrent Benchmarks**: Multi-producer/consumer and contention scenarios

## Quick Start

### Run All Benchmarks

```bash
# Run all benchmarks (requires running broker)
go test -bench=. -benchmem -benchtime=10s ./bench

# Run with short mode (skips integration benchmarks)
go test -bench=. -benchmem -short ./bench
```

### Run Specific Benchmark Suites

```bash
# Throughput only
go test -bench=BenchmarkE2E_ProducerThroughput -benchmem ./bench

# Latency only
go test -bench=BenchmarkE2E_ProduceLatency -benchmem ./bench

# Concurrent operations
go test -bench=BenchmarkConcurrent -benchmem ./bench
```

### Run with Automation Script

```bash
# Run all benchmarks with baseline comparison
../scripts/run-benchmarks.sh

# Run specific suite
../scripts/run-benchmarks.sh throughput

# Save new baseline
../scripts/run-benchmarks.sh --save-baseline
```

## Prerequisites

### Start a Test Broker

Most benchmarks require a running StreamBus broker:

```bash
# Start broker with test configuration
./bin/streambus-broker --config config/broker.yaml

# Or use Docker
docker run -p 9092:9092 -p 8080:8080 streambus:latest
```

### Unit Benchmarks Only

To run only unit-level benchmarks (no broker required):

```bash
go test -bench=BenchmarkStorage -benchmem -short ./bench
```

## Benchmark Suites

### Throughput Benchmarks

Measure messages/sec and MB/sec throughput:

```bash
go test -bench=BenchmarkE2E_ProducerThroughput -benchmem -benchtime=30s ./bench
go test -bench=BenchmarkE2E_ConsumerThroughput -benchmem -benchtime=30s ./bench
go test -bench=BenchmarkStorage_AppendThroughput -benchmem -benchtime=30s ./bench
```

**Metrics Reported:**
- `MB/s` - Megabytes per second throughput
- `msgs/s` - Messages per second throughput
- Memory allocations (allocs/op, B/op)

**Test Cases:**
- Small messages (100B) - single thread and batched
- Medium messages (1KB) - single thread and batched
- Large messages (10KB) - single thread and batched
- Concurrent workloads (10 threads)

### Latency Benchmarks

Measure latency distribution (p50, p95, p99, p999):

```bash
go test -bench=BenchmarkE2E_ProduceLatency -benchmem ./bench
go test -bench=BenchmarkE2E_ConsumeLatency -benchmem ./bench
go test -bench=BenchmarkE2E_RoundTripLatency -benchmem ./bench
```

**Metrics Reported:**
- `avg_µs` / `avg_ms` - Average latency
- `p50_µs` / `p50_ms` - Median latency (50th percentile)
- `p95_µs` / `p95_ms` - 95th percentile latency
- `p99_µs` / `p99_ms` - 99th percentile latency
- `p999_µs` - 99.9th percentile latency

**Test Cases:**
- Small messages (100B)
- Medium messages (1KB)
- Large messages (10KB)
- Extra-large messages (100KB)

### Concurrent Benchmarks

Measure performance under concurrent load:

```bash
go test -bench=BenchmarkConcurrent_MultiProducer -benchmem ./bench
go test -bench=BenchmarkConcurrent_MultiConsumer -benchmem ./bench
go test -bench=BenchmarkConcurrent_MixedWorkload -benchmem ./bench
go test -bench=BenchmarkConcurrent_ProducerContention -benchmem ./bench
```

**Metrics Reported:**
- `MB/s` - Throughput under concurrent load
- `msgs/s` - Message rate under concurrent load
- `total_msgs` - Total messages processed
- `produced` / `consumed` - Counts for mixed workloads

**Test Cases:**
- 10, 50, 100 concurrent producers
- 5, 10 concurrent consumers
- Mixed producer/consumer workloads
- Contention scenarios (5-100 producers, single topic)

## Understanding Results

### Interpreting Throughput

```
BenchmarkE2E_ProducerThroughput/MediumMsg_Batched-8    100000    11234 ns/op    95.2 MB/s    98234 msgs/s
```

- `100000` - Number of iterations
- `11234 ns/op` - Nanoseconds per operation
- `95.2 MB/s` - Throughput in megabytes per second
- `98234 msgs/s` - Messages per second
- `-8` - Number of CPU cores used (GOMAXPROCS)

### Interpreting Latency

```
BenchmarkE2E_ProduceLatency/MediumMsg_1KB-8    50000    avg_µs:450    p50_µs:380    p95_µs:890    p99_µs:1250    p999_µs:2100
```

- Lower latency is better
- p50 (median) represents typical latency
- p99 and p999 represent tail latency (important for SLAs)
- Watch for large gaps between p50 and p99 (indicates variability)

### Comparing Results

Use `benchcmp` or `benchstat` to compare benchmark runs:

```bash
# Install benchstat
go install golang.org/x/perf/cmd/benchstat@latest

# Run benchmarks twice and compare
go test -bench=. -benchmem ./bench > old.txt
# Make changes...
go test -bench=. -benchmem ./bench > new.txt
benchstat old.txt new.txt
```

## Profiling

### CPU Profiling

```bash
go test -bench=BenchmarkE2E_ProducerThroughput -cpuprofile=cpu.prof ./bench
go tool pprof cpu.prof
```

### Memory Profiling

```bash
go test -bench=BenchmarkE2E_ProducerThroughput -memprofile=mem.prof ./bench
go tool pprof mem.prof
```

### Trace Analysis

```bash
go test -bench=BenchmarkE2E_ProduceLatency -trace=trace.out ./bench
go tool trace trace.out
```

## Performance Targets

### Expected Performance (Single Node)

| Metric | Target | Notes |
|--------|--------|-------|
| Producer Throughput | 100K+ msgs/s | 1KB messages, batched |
| Consumer Throughput | 100K+ msgs/s | 1KB messages |
| Produce Latency (p99) | < 10ms | Without fsync |
| Consume Latency (p99) | < 5ms | Warm cache |
| Storage Append | 200K+ ops/s | In-memory, no fsync |

### Factors Affecting Performance

**Network:**
- Latency between client and broker
- Bandwidth limitations
- TCP configuration

**Storage:**
- Disk type (SSD vs. HDD)
- Fsync policy (FsyncAlways, FsyncInterval, FsyncNever)
- Compaction settings

**Configuration:**
- Batch size
- Compression
- Replication factor
- Number of partitions

**Hardware:**
- CPU cores
- Available memory
- Disk I/O capacity

## Continuous Benchmarking

### CI/CD Integration

Add to your CI pipeline:

```yaml
# GitHub Actions example
- name: Run Benchmarks
  run: |
    go test -bench=. -benchmem -benchtime=10s ./bench > bench-results.txt

- name: Compare with Baseline
  run: |
    benchstat bench-baseline.txt bench-results.txt
```

### Regression Detection

Track performance over time:

```bash
# Save baseline
go test -bench=. -benchmem ./bench > baseline-v0.7.0.txt

# After changes, compare
go test -bench=. -benchmem ./bench > current.txt
benchstat baseline-v0.7.0.txt current.txt

# Look for significant regressions (>10% slower)
```

## Troubleshooting

### "Cannot connect to broker"

Ensure broker is running:
```bash
./bin/streambus-broker --config config/broker.yaml
```

Or skip integration tests:
```bash
go test -bench=. -short ./bench
```

### Inconsistent Results

Benchmarks can vary due to:
- System load (close other programs)
- Thermal throttling (ensure cooling)
- Network conditions
- Disk activity

**Best Practices:**
- Run multiple times and average
- Use consistent hardware
- Minimize background processes
- Use `-benchtime=30s` or more for stability

### Out of Memory

Reduce benchmark iterations:
```bash
go test -bench=. -benchtime=1s ./bench
```

Or increase available memory:
```bash
# Increase Go heap
GOGC=400 go test -bench=. ./bench
```

## Contributing

When adding new benchmarks:

1. Follow naming convention: `Benchmark<Category>_<Operation>`
2. Use `b.ResetTimer()` after setup
3. Use `b.ReportAllocs()` to track allocations
4. Report custom metrics with `b.ReportMetric()`
5. Add `if testing.Short()` check for integration tests
6. Document what the benchmark measures

Example:
```go
func BenchmarkMyFeature(b *testing.B) {
    if testing.Short() {
        b.Skip("Skipping integration benchmark")
    }

    // Setup
    setup()

    b.ResetTimer()
    b.ReportAllocs()

    for i := 0; i < b.N; i++ {
        // Benchmark code
    }

    b.ReportMetric(customMetric, "unit")
}
```

## Related Documentation

- [Performance Tuning Guide](../docs/PERFORMANCE.md)
- [Testing Guide](../docs/TESTING.md)
- [Architecture Overview](../docs/ARCHITECTURE.md)

## Support

- **Issues**: https://github.com/shawntherrien/streambus/issues
- **Discussions**: https://github.com/shawntherrien/streambus/discussions
