# StreamBus Benchmarking Guide

This guide covers performance benchmarking for StreamBus.

## Table of Contents

- [Quick Start](#quick-start)
- [Benchmark Scenarios](#benchmark-scenarios)
- [Running Benchmarks](#running-benchmarks)
- [Interpreting Results](#interpreting-results)
- [Comparing Versions](#comparing-versions)
- [Production Benchmarks](#production-benchmarks)

## Quick Start

```bash
# Run all benchmarks
make benchmark

# Run specific component benchmarks
make benchmark-storage
make benchmark-protocol
make benchmark-client

# Generate benchmark report
make benchmark-report
```

## Benchmark Scenarios

###  1. Storage Layer Benchmarks

Test storage engine performance:

```bash
# Write performance
go test -bench=BenchmarkStorage_Write -benchtime=10s ./pkg/storage

# Read performance
go test -bench=BenchmarkStorage_Read -benchtime=10s ./pkg/storage

# Sequential vs random access
go test -bench=BenchmarkStorage_Sequential -benchtime=10s ./pkg/storage
go test -bench=BenchmarkStorage_Random -benchtime=10s ./pkg/storage
```

### 2. Protocol Benchmarks

Test serialization/deserialization:

```bash
# Message encoding/decoding
go test -bench=BenchmarkProtocol_Encode -benchtime=10s ./pkg/protocol
go test -bench=BenchmarkProtocol_Decode -benchtime=10s ./pkg/protocol

# Compression
go test -bench=BenchmarkProtocol_Compress -benchtime=10s ./pkg/protocol
```

### 3. End-to-End Benchmarks

Test complete message flow:

```bash
# Producer throughput
go test -bench=BenchmarkProducer_Throughput -benchtime=30s ./pkg/client

# Consumer throughput
go test -bench=BenchmarkConsumer_Throughput -benchtime=30s ./pkg/client

# Latency
go test -bench=BenchmarkE2E_Latency -benchtime=30s ./pkg/client
```

### 4. Concurrency Benchmarks

Test under concurrent load:

```bash
# Multiple producers
go test -bench=BenchmarkConcurrent_Producers -benchtime=30s ./pkg/client

# Multiple consumers
go test -bench=BenchmarkConcurrent_Consumers -benchtime=30s ./pkg/client
```

## Running Benchmarks

### Basic Benchmark Run

```bash
go test -bench=. -benchmem ./...
```

### Extended Benchmark Run

```bash
# Run for longer (more accurate)
go test -bench=. -benchmem -benchtime=10s ./...

# Run multiple times
go test -bench=. -benchmem -count=5 ./...

# With CPU profiling
go test -bench=. -benchmem -cpuprofile=cpu.prof ./...

# With memory profiling
go test -bench=. -benchmem -memprofile=mem.prof ./...
```

### Analyzing Profiles

```bash
# CPU profile
go tool pprof cpu.prof
# Then in pprof: top, list, web

# Memory profile
go tool pprof mem.prof

# Generate flamegraph
go tool pprof -http=:8080 cpu.prof
```

## Interpreting Results

### Example Output

```
BenchmarkProducer_Send-8                    50000      25431 ns/op     2048 B/op     12 allocs/op
BenchmarkProducer_SendBatch-8              100000      12234 ns/op     4096 B/op      8 allocs/op
BenchmarkConsumer_Receive-8                 60000      23456 ns/op     1024 B/op     10 allocs/op
```

**Reading the results:**
- `BenchmarkProducer_Send-8`: Benchmark name + GOMAXPROCS
- `50000`: Number of iterations
- `25431 ns/op`: Nanoseconds per operation
- `2048 B/op`: Bytes allocated per operation
- `12 allocs/op`: Allocations per operation

### Performance Targets

**Latency Targets:**
- p50: < 5ms
- p95: < 20ms
- p99: < 50ms
- p99.9: < 100ms

**Throughput Targets:**
- Single producer: > 10,000 msgs/sec
- Single consumer: > 10,000 msgs/sec
- 3-broker cluster: > 100,000 msgs/sec

**Resource Usage:**
- CPU per 10k msgs/sec: < 1 core
- Memory per 10k msgs/sec: < 500MB
- Network overhead: < 10%

## Comparing Versions

### Create Baseline

```bash
# Before changes
git checkout main
make benchmark-baseline
```

### Compare After Changes

```bash
# After changes
git checkout feature-branch
make benchmark-compare
```

### Using benchstat

```bash
# Install benchstat
go install golang.org/x/perf/cmd/benchstat@latest

# Run baseline
go test -bench=. -count=5 ./... > old.txt

# Make changes, then run again
go test -bench=. -count=5 ./... > new.txt

# Compare
benchstat old.txt new.txt
```

**Example comparison output:**
```
name                    old time/op    new time/op    delta
Producer_Send-8           25.4µs ± 2%    18.3µs ± 1%  -27.95%  (p=0.000 n=5+5)
Consumer_Receive-8        23.5µs ± 3%    21.2µs ± 2%   -9.79%  (p=0.008 n=5+5)

name                    old alloc/op   new alloc/op   delta
Producer_Send-8           2.05kB ± 0%    1.54kB ± 0%  -24.88%  (p=0.000 n=5+5)
Consumer_Receive-8        1.02kB ± 0%    0.98kB ± 0%   -3.92%  (p=0.008 n=5+5)
```

## Production Benchmarks

### Load Test Scenarios

**1. Sustained Throughput:**
```bash
go run ./tests/load/cmd/loadtest \
  --scenario throughput \
  --duration 30m \
  --rate 10000 \
  --producers 10 \
  --consumers 10
```

**2. Peak Load:**
```bash
go run ./tests/load/cmd/loadtest \
  --scenario stress \
  --duration 10m \
  --rate 50000 \
  --producers 20 \
  --consumers 20
```

**3. Soak Test:**
```bash
go run ./tests/load/cmd/loadtest \
  --scenario soak \
  --duration 24h \
  --rate 5000
```

**4. Spike Test:**
```bash
go run ./tests/load/cmd/loadtest \
  --scenario spike \
  --duration 5m \
  --rate 1000 \
  --spike-rate 10000 \
  --spike-duration 30s
```

### Hardware Sizing Benchmarks

Test on different hardware configurations:

**Small (development):**
- 2 cores, 4GB RAM
- Expected: 5,000 msgs/sec

**Medium (staging):**
- 4 cores, 8GB RAM
- Expected: 20,000 msgs/sec

**Large (production):**
- 8+ cores, 16GB+ RAM
- Expected: 100,000+ msgs/sec

### Network Performance

Test network latency impact:

```bash
# Local (< 1ms)
go run ./tests/load/cmd/loadtest --brokers localhost:9092

# Same datacenter (< 5ms)
go run ./tests/load/cmd/loadtest --brokers 10.0.1.100:9092

# Cross-region (50-100ms)
go run ./tests/load/cmd/loadtest --brokers 52.1.2.3:9092
```

## Continuous Benchmarking

### GitHub Actions Integration

```yaml
# .github/workflows/benchmark.yml
name: Benchmark

on:
  pull_request:
    branches: [main]

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Run Benchmarks
        run: |
          make benchmark > new.txt

      - name: Download Baseline
        run: |
          gh release download baseline -p benchmarks.txt
          mv benchmarks.txt old.txt

      - name: Compare
        run: |
          benchstat old.txt new.txt > comparison.txt
          cat comparison.txt >> $GITHUB_STEP_SUMMARY

      - name: Comment on PR
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const comparison = fs.readFileSync('comparison.txt', 'utf8');
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: `## Benchmark Results\n\n\`\`\`\n${comparison}\n\`\`\``
            });
```

### Prometheus Integration

Export metrics to Prometheus:

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    benchmarkLatency = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name: "streambus_benchmark_latency_seconds",
            Help: "Benchmark latency",
        },
    )
)
```

## Best Practices

1. **Run on dedicated hardware** - Avoid running benchmarks on shared/busy machines
2. **Disable CPU scaling** - Set CPU governor to performance mode
3. **Multiple runs** - Run benchmarks 5-10 times and take averages
4. **Warm-up** - Allow system to warm up before measuring
5. **Consistent environment** - Use same hardware, OS, Go version for comparisons
6. **Monitor resources** - Track CPU, memory, disk, network during benchmarks
7. **Isolate variables** - Change one thing at a time when comparing

## Troubleshooting

### High Variance in Results

- Run with `-count=10` for more samples
- Check for background processes
- Disable CPU frequency scaling
- Use dedicated test hardware

### Memory Leaks

```bash
# Profile memory over time
go test -bench=. -benchtime=60s -memprofile=mem.prof ./...
go tool pprof -alloc_space mem.prof
```

### CPU Bottlenecks

```bash
# Profile CPU usage
go test -bench=. -cpuprofile=cpu.prof ./...
go tool pprof cpu.prof
# Then: top, list <function>
```

## Benchmarking Checklist

Before running production benchmarks:

- [ ] Dedicated hardware (no other workloads)
- [ ] CPU governor set to performance
- [ ] Sufficient disk space for logs/data
- [ ] Network bandwidth verified
- [ ] Monitoring in place
- [ ] Baseline measurements taken
- [ ] Test plan documented
- [ ] Expected results defined
- [ ] Rollback plan ready

## Resources

- [Go Benchmark Best Practices](https://dave.cheney.net/2013/06/30/how-to-write-benchmarks-in-go)
- [benchstat Documentation](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat)
- [pprof Tutorial](https://blog.golang.org/pprof)
- [Continuous Benchmarking](https://dave.cheney.net/2013/06/30/how-to-write-benchmarks-in-go)

## See Also

- [Load Testing Guide](tests/load/README.md)
- [Performance Tuning](PERFORMANCE_TUNING.md)
- [Production Deployment](docs/PRODUCTION.md)
