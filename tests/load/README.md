# StreamBus Load Testing

Comprehensive load testing suite for StreamBus.

## Overview

This directory contains load testing tools and scenarios to validate StreamBus performance under various workloads.

## Quick Start

```bash
# Start a test cluster
make run-cluster

# Run basic load test
go run ./tests/load/cmd/loadtest --duration 1m --rate 1000

# Run stress test
go run ./tests/load/cmd/loadtest --duration 5m --rate 10000 --scenario stress
```

## Test Scenarios

### 1. Throughput Test

Measure maximum message throughput:

```bash
go run ./tests/load/cmd/loadtest \
  --scenario throughput \
  --duration 5m \
  --producers 10 \
  --message-size 1KB
```

### 2. Latency Test

Measure end-to-end latency:

```bash
go run ./tests/load/cmd/loadtest \
  --scenario latency \
  --duration 5m \
  --rate 1000 \
  --percentiles 50,95,99,99.9
```

### 3. Stress Test

Test system under extreme load:

```bash
go run ./tests/load/cmd/loadtest \
  --scenario stress \
  --duration 10m \
  --producers 50 \
  --consumers 50 \
  --rate 50000
```

### 4. Soak Test

Long-running stability test:

```bash
go run ./tests/load/cmd/loadtest \
  --scenario soak \
  --duration 24h \
  --rate 1000
```

### 5. Partition Test

Test partitioning performance:

```bash
go run ./tests/load/cmd/loadtest \
  --scenario partition \
  --partitions 100 \
  --producers 20
```

### 6. Consumer Group Test

Test consumer group rebalancing:

```bash
go run ./tests/load/cmd/loadtest \
  --scenario consumer-group \
  --consumers 10 \
  --rebalance-interval 1m
```

## Configuration

### Command Line Options

```
--broker              Broker address (default: localhost:9092)
--scenario            Test scenario (default: throughput)
--duration            Test duration (default: 1m)
--rate                Target message rate (msgs/sec)
--producers           Number of producer threads
--consumers           Number of consumer threads
--message-size        Message size (supports KB, MB)
--partitions          Number of partitions
--batch-size          Producer batch size
--acks                Producer acknowledgment mode
--compression         Compression algorithm
--output              Output format (json, csv, text)
```

### Configuration File

Create `loadtest.yaml`:

```yaml
brokers:
  - localhost:9092
  - localhost:9192
  - localhost:9292

scenario: throughput

producers:
  count: 10
  rate: 10000
  messageSize: 1KB
  batchSize: 100
  compression: lz4
  acks: all

consumers:
  count: 10
  groupId: load-test-group

test:
  duration: 5m
  warmup: 30s
  rampUp: 1m

topics:
  - name: load-test
    partitions: 10
    replicationFactor: 3

metrics:
  enabled: true
  interval: 10s
  percentiles: [50, 90, 95, 99, 99.9]
```

Run with config:

```bash
go run ./tests/load/cmd/loadtest --config loadtest.yaml
```

## Metrics

### Reported Metrics

**Throughput:**
- Messages/sec produced
- Messages/sec consumed
- Bytes/sec produced
- Bytes/sec consumed

**Latency:**
- End-to-end latency (p50, p95, p99, p99.9)
- Producer latency
- Consumer lag

**Reliability:**
- Success rate
- Error rate
- Lost messages

**Resources:**
- CPU usage
- Memory usage
- Network I/O
- Disk I/O

### Output Example

```
========================================
StreamBus Load Test Results
========================================

Scenario:       Throughput Test
Duration:       5m0s
Brokers:        3
Topics:         1
Partitions:     10

Producers:
  Count:        10
  Messages:     3,000,000
  Throughput:   10,000 msgs/sec
  Bandwidth:    10 MB/sec
  Success Rate: 99.99%

Consumers:
  Count:        10
  Messages:     3,000,000
  Throughput:   10,000 msgs/sec
  Bandwidth:    10 MB/sec
  Lag:          < 100ms

Latency (ms):
  Min:          0.5
  p50:          2.1
  p90:          4.5
  p95:          6.2
  p99:          12.3
  p99.9:        45.6
  Max:          123.4

Errors:
  Producer:     12 (0.0004%)
  Consumer:     0 (0.00%)

Resources:
  CPU (avg):    45%
  Memory:       2.1 GB
  Network In:   10 MB/sec
  Network Out:  10 MB/sec
========================================
```

## Advanced Usage

### Custom Test Script

```go
package main

import (
    "github.com/shawntherrien/streambus/tests/load"
)

func main() {
    test := load.NewLoadTest(&load.Config{
        Brokers: []string{"localhost:9092"},
        Duration: 5 * time.Minute,
    })

    // Add custom scenario
    test.AddScenario("custom", func(ctx context.Context) error {
        // Your test logic here
        return nil
    })

    results := test.Run()
    results.Print()
}
```

### Integration with CI/CD

```yaml
# .github/workflows/load-test.yml
- name: Run Load Test
  run: |
    docker-compose up -d
    go run ./tests/load/cmd/loadtest \
      --duration 2m \
      --rate 5000 \
      --output json > results.json

- name: Check Performance
  run: |
    # Fail if p99 latency > 100ms
    cat results.json | jq '.latency.p99 < 100' | grep true
```

## Benchmarking

### Compare Versions

```bash
# Baseline
go run ./tests/load/cmd/loadtest --duration 5m > baseline.txt

# After changes
go run ./tests/load/cmd/loadtest --duration 5m > current.txt

# Compare
go run ./tests/load/cmd/compare baseline.txt current.txt
```

### Continuous Benchmarking

Store results in time-series database for tracking:

```bash
# Export to Prometheus
go run ./tests/load/cmd/loadtest \
  --prometheus-push http://localhost:9091

# Export to InfluxDB
go run ./tests/load/cmd/loadtest \
  --influxdb-url http://localhost:8086
```

## Best Practices

1. **Warm-up period**: Always include 30s-1m warmup before measuring
2. **Steady state**: Run for at least 5 minutes to reach steady state
3. **Realistic data**: Use message sizes and patterns from production
4. **Multiple runs**: Run tests 3-5 times and average results
5. **Monitor resources**: Track CPU, memory, disk, network on all nodes
6. **Isolation**: Run on dedicated hardware without other workloads

## Troubleshooting

### High Latency

- Check network latency between producers/consumers and brokers
- Verify disk I/O performance
- Check for CPU throttling
- Review batch sizes and compression settings

### Low Throughput

- Increase number of partitions
- Increase batch size
- Enable compression
- Use async acknowledgments (if acceptable)
- Check network bandwidth

### Consumer Lag

- Increase number of consumers
- Optimize consumer processing logic
- Check consumer group rebalancing frequency
- Review partition assignment strategy

## See Also

- [Benchmarking Guide](../../BENCHMARKING.md)
- [Performance Tuning](../../docs/performance-tuning.md)
- [Production Deployment](../../docs/production-deployment.md)
