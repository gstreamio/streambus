# StreamBus Load Testing Tool

Comprehensive load testing tool for StreamBus performance validation.

## Overview

The load testing tool simulates realistic production workloads with configurable:
- Number of producers and consumers
- Message sizes and rates
- Batch sizes and compression
- Test duration and reporting intervals

## Quick Start

### Build

```bash
# Build load test tool
go build -o bin/loadtest ./tools/loadtest

# Or use the script (auto-builds)
./scripts/run-load-test.sh
```

### Run Basic Test

```bash
# Simple throughput test
./bin/loadtest -brokers localhost:9092 -rate 10000 -duration 60s

# Using the wrapper script
./scripts/run-load-test.sh
```

## Usage

### Command Line Options

```bash
./bin/loadtest [OPTIONS]

Options:
  -brokers string
        Comma-separated list of broker addresses (default "localhost:9092")
  -topic string
        Topic to use for load test (default "loadtest")
  -producers int
        Number of producer threads (default 1)
  -consumers int
        Number of consumer threads (default 1)
  -message-size int
        Size of each message in bytes (default 1024)
  -rate int
        Target messages per second across all producers (default 1000)
  -duration duration
        Duration of load test (default 1m0s)
  -batch-size int
        Number of messages per batch (default 100)
  -compression string
        Compression: none, gzip, snappy, lz4 (default "none")
  -report-interval duration
        Stats reporting interval (default 10s)
  -consumer-group string
        Consumer group ID (default "loadtest-group")
  -transactions
        Use transactional producers (default false)
```

## Scenarios

### 1. High Throughput

Maximize messages per second:

```bash
./scripts/run-load-test.sh throughput

# Equivalent to:
./bin/loadtest \
  -producers 10 \
  -consumers 5 \
  -rate 100000 \
  -message-size 1024 \
  -batch-size 1000 \
  -compression lz4 \
  -duration 60s
```

**Expected Results:**
- Throughput: 80K-150K msgs/s
- Latency: 10-20ms average
- Resource usage: CPU 60-80%, Memory 2-4GB

### 2. Low Latency

Minimize message latency:

```bash
./scripts/run-load-test.sh latency

# Equivalent to:
./bin/loadtest \
  -producers 5 \
  -consumers 5 \
  -rate 10000 \
  -message-size 100 \
  -batch-size 10 \
  -compression none \
  -duration 60s
```

**Expected Results:**
- Latency: < 5ms average
- Throughput: 8K-15K msgs/s
- Resource usage: CPU 40-60%, Memory 1-2GB

### 3. Mixed Workload

Balanced producer/consumer:

```bash
./scripts/run-load-test.sh mixed

# Equivalent to:
./bin/loadtest \
  -producers 20 \
  -consumers 20 \
  -rate 50000 \
  -message-size 512 \
  -batch-size 100 \
  -compression snappy \
  -duration 60s
```

### 4. Stress Test

Push system to limits:

```bash
./scripts/run-load-test.sh stress

# Equivalent to:
./bin/loadtest \
  -producers 50 \
  -consumers 50 \
  -rate 200000 \
  -message-size 1024 \
  -batch-size 500 \
  -compression lz4 \
  -duration 10m
```

**Purpose:**
- Identify breaking points
- Test error handling
- Validate resource limits
- Measure degradation under extreme load

### 5. Sustained Load

Long-running stability test:

```bash
./scripts/run-load-test.sh sustained

# Equivalent to:
./bin/loadtest \
  -producers 5 \
  -consumers 5 \
  -rate 10000 \
  -duration 1h
```

**Purpose:**
- Memory leak detection
- Long-term stability validation
- Resource usage over time
- Compaction behavior

### 6. Burst Pattern

Simulate bursty traffic:

```bash
./scripts/run-load-test.sh burst

# Run multiple short bursts
for i in {1..10}; do
  ./bin/loadtest -rate 100000 -duration 30s
  sleep 30
done
```

## Custom Scenarios

### Large Messages

```bash
./bin/loadtest \
  -producers 5 \
  -consumers 5 \
  -rate 1000 \
  -message-size 102400 \
  -batch-size 10 \
  -duration 5m
```

### Many Small Messages

```bash
./bin/loadtest \
  -producers 20 \
  -consumers 10 \
  -rate 500000 \
  -message-size 50 \
  -batch-size 5000 \
  -compression lz4 \
  -duration 2m
```

### Imbalanced Producer/Consumer

```bash
# More producers than consumers (backlog builds)
./bin/loadtest \
  -producers 20 \
  -consumers 5 \
  -rate 50000 \
  -duration 10m

# More consumers than producers (fight for messages)
./bin/loadtest \
  -producers 5 \
  -consumers 20 \
  -rate 10000 \
  -duration 5m
```

## Interpreting Results

### Stats Output

```
========================================
Load Test Complete
========================================
Duration:              1m0s
Messages Sent:         600000
Messages Received:     599950
Message Loss:          50 (0.01%)
Errors:                0
----------------------------------------
Send Rate:             10000 msgs/s
Receive Rate:          9999 msgs/s
Send Throughput:       9.77 MB/s
Receive Throughput:    9.77 MB/s
Average Latency:       5.23 ms
========================================
```

### Key Metrics

**Throughput:**
- **Send Rate**: Messages produced per second
- **Receive Rate**: Messages consumed per second
- **Throughput**: Data transfer in MB/s

**Latency:**
- **Average Latency**: Mean time from produce to ack
- Lower is better (< 10ms is good)

**Reliability:**
- **Message Loss**: Difference between sent and received
- Should be 0 for most scenarios
- May be > 0 during shutdown

**Errors:**
- Connection failures
- Timeout errors
- Protocol errors
- Should be 0 in healthy system

### During Test Output

```
INFO: Load test stats
  sent=100000
  received=99950
  send_rate=10000 msgs/s
  receive_rate=9995 msgs/s
  send_throughput=9.77 MB/s
  receive_throughput=9.76 MB/s
  avg_latency=5.23 ms
  errors=0
  lag=50
```

**Lag:**
- Difference between messages sent and received
- Normal to have small lag (< 1000)
- Growing lag indicates consumer can't keep up

## Integration with Monitoring

### Prometheus Metrics

While load test runs, monitor broker metrics:

```bash
# In another terminal
watch -n 1 'curl -s http://localhost:8080/metrics | grep -E "(messages_produced|messages_consumed|latency)"'
```

### Grafana Dashboard

Load the StreamBus Overview dashboard to visualize:
- Message throughput over time
- Latency percentiles (p50, p95, p99)
- Broker resource usage
- Consumer lag

## Best Practices

### 1. Start Small

Begin with low rates and gradually increase:

```bash
# Baseline
./bin/loadtest -rate 1000 -duration 1m

# Increase gradually
./bin/loadtest -rate 10000 -duration 1m
./bin/loadtest -rate 50000 -duration 1m
./bin/loadtest -rate 100000 -duration 1m
```

### 2. Warm Up Period

Allow broker to warm up before measuring:

```bash
# Run short warmup first
./bin/loadtest -rate 10000 -duration 10s

# Then run actual test
./bin/loadtest -rate 10000 -duration 5m
```

### 3. Monitor System Resources

Watch CPU, memory, disk, network:

```bash
# In separate terminals
htop                    # CPU and memory
iostat -x 1            # Disk I/O
iftop                  # Network usage
```

### 4. Use Realistic Message Sizes

Match your production workload:

```bash
# If your average message is 500 bytes
./bin/loadtest -message-size 500 -rate 50000

# Add variance with random sizes (future enhancement)
```

### 5. Test Failure Scenarios

Simulate failures during load:

```bash
# Start load test
./bin/loadtest -rate 10000 -duration 10m &

# While running, kill broker
killall streambus-broker

# Restart broker - observe recovery
./bin/streambus-broker --config config/broker.yaml
```

## Continuous Load Testing

### CI/CD Integration

Add to your pipeline:

```yaml
# .github/workflows/load-test.yml
name: Load Test
on: [push]

jobs:
  load-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2

      - name: Start broker
        run: |
          ./bin/streambus-broker --config config/broker.yaml &
          sleep 5

      - name: Run load test
        run: |
          ./scripts/run-load-test.sh -duration 2m

      - name: Check results
        run: |
          # Parse results and fail if throughput < threshold
          # Or if error rate > 1%
```

### Regression Detection

Save baseline results and compare:

```bash
# Save baseline
./bin/loadtest -rate 10000 -duration 5m > baseline.log

# After changes
./bin/loadtest -rate 10000 -duration 5m > current.log

# Compare (manual or scripted)
diff baseline.log current.log
```

## Troubleshooting

### Low Throughput

**Issue**: Not reaching target rate

**Checks:**
```bash
# Check broker CPU
top -p $(pgrep streambus-broker)

# Check network usage
iftop

# Check for errors
journalctl -u streambus-broker -f
```

**Solutions:**
- Increase batch size
- Enable compression
- Add more broker resources
- Use faster storage

### High Latency

**Issue**: Average latency > 100ms

**Checks:**
```bash
# Check fsync policy
grep fsync_policy config/broker.yaml

# Check GC pauses
GODEBUG=gctrace=1 ./bin/streambus-broker
```

**Solutions:**
- Reduce batch size
- Disable compression
- Change fsync policy to "interval"
- Tune GC with GOGC

### Consumer Lag Growing

**Issue**: Lag continuously increasing

**Checks:**
```bash
# Watch lag metric
curl -s http://localhost:8080/metrics | grep consumer_group_lag
```

**Solutions:**
- Add more consumers
- Increase consumer poll size
- Check consumer processing logic
- Verify consumer resources

### Connection Errors

**Issue**: "connection refused" or timeouts

**Checks:**
```bash
# Verify broker is running
netstat -an | grep 9092

# Check connection limit
grep max_connections config/broker.yaml
```

**Solutions:**
- Increase max_connections
- Use connection pooling
- Reduce number of test clients

## Examples

### Example 1: Performance Baseline

```bash
# Establish baseline for your hardware
./scripts/run-load-test.sh throughput > baseline-$(date +%Y%m%d).log
```

### Example 2: Capacity Planning

```bash
# Test increasing load until saturation
for rate in 10000 20000 50000 100000 200000; do
  echo "Testing rate: $rate msgs/s"
  ./bin/loadtest -rate $rate -duration 2m | tee -a capacity-test.log
  sleep 30
done
```

### Example 3: Latency SLA Validation

```bash
# Verify p99 latency < 10ms under moderate load
./bin/loadtest -rate 10000 -duration 10m

# Check if latency meets SLA
# (Parse output and alert if p99 > 10ms)
```

## Related Documentation

- [Performance Tuning Guide](../../docs/PERFORMANCE.md)
- [Benchmarking Guide](../../bench/README.md)
- [Monitoring Guide](../../docs/MONITORING.md)

## Support

- **Issues**: https://github.com/shawntherrien/streambus/issues
- **Discussions**: https://github.com/shawntherrien/streambus/discussions
