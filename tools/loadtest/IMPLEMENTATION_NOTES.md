# Load Test Tool - Implementation Notes

## Status

Load test tool framework is complete but requires API updates to match current client library.

## Current Implementation

The load test tool (`tools/loadtest/main.go`) provides:
- Configurable producer/consumer threads
- Rate limiting and message sizing
- Real-time statistics reporting
- Multiple test scenarios via wrapper script

## Required API Updates

To make the load test tool functional, the following client API enhancements are needed:

### 1. Simplified Consumer API

**Current**: Complex consumer creation requiring topic + partitionID
**Needed**: Group consumer with automatic partition assignment

```go
// Needed in pkg/client
func (c *Client) NewConsumer(groupID string) (*GroupConsumer, error)

// With Subscribe method
func (gc *GroupConsumer) Subscribe(topic string) error
func (gc *GroupConsumer) Poll(ctx context.Context, timeout time.Duration) ([]Message, error)
```

### 2. Simplified Producer API

**Current**: Requires creating producer via NewProducer(client)
**Needed**: Method on client

```go
// Needed in pkg/client
func (c *Client) NewProducer() (*Producer, error)
```

### 3. Unified Timeout Configuration

**Current**: Multiple timeout fields (ConnectTimeout, ReadTimeout, etc.)
**Needed**: Simpler configuration option

```go
// Add to Config
Timeout time.Duration // Default for all operations
```

## Alternative: Use Existing Benchmarks

Until the client API is enhanced, use the existing benchmark suite for load testing:

```bash
# Throughput testing
go test -bench=BenchmarkE2E_ProducerThroughput -benchtime=60s ./bench

# Latency testing
go test -bench=BenchmarkE2E_ProduceLatency -benchtime=60s ./bench

# Concurrent testing
go test -bench=BenchmarkConcurrent -benchtime=60s ./bench
```

## Future Enhancements

When the client API is updated, additional features to add:

1. **Message Key Distribution**: Various key patterns (random, sequential, hash-based)
2. **Variable Message Sizes**: Random sizes within a range
3. **Burst Patterns**: Configurable burst/pause cycles
4. **Transaction Support**: Transactional producers option
5. **Detailed Metrics**: Percentile latencies, throughput curves
6. **Result Export**: JSON/CSV output for analysis

## See Also

- `bench/` - Comprehensive benchmark suite (functional now)
- `scripts/run-benchmarks.sh` - Benchmark automation
- `docs/PERFORMANCE.md` - Performance tuning guide
