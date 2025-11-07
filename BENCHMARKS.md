# StreamBus Performance Benchmarks

This document contains detailed performance benchmarks for StreamBus across all layers of the stack.

## Test Environment

- **Hardware**: Apple M4 Max (16 cores)
- **Operating System**: macOS (Darwin arm64)
- **Go Version**: 1.23+
- **Test Date**: January 2025
- **Benchmark Duration**: 2 seconds per test

## Running Benchmarks

```bash
# Full benchmark suite with summary
make benchmark-full

# Individual layers
make benchmark-storage    # Storage engine
make benchmark-protocol   # Protocol encoding/decoding
make benchmark-client     # End-to-end client operations
make benchmark-server     # Server request handlers

# Generate detailed markdown report
make benchmark-report

# Baseline comparison
make benchmark-baseline   # Set current performance as baseline
make benchmark-compare    # Compare current vs baseline
```

## Benchmark Results

### Client Layer (End-to-End)

End-to-end performance including network, protocol, and storage layers:

| Operation | Operations/sec | Latency (µs/op) | Memory (B/op) | Allocations |
|-----------|----------------|-----------------|---------------|-------------|
| Producer Send | 39,800 | 25.1 | 2,166 | 40 |
| Consumer Fetch | 45,900 | 21.8 | 1,318 | 26 |

**Notes:**
- Producer includes TCP connection, protocol encoding, and storage write
- Consumer includes TCP connection, protocol decoding, and storage read
- Tests run against local server (minimal network latency)

### Protocol Layer (Serialization)

Binary protocol encoding/decoding performance:

| Operation | Operations/sec | Latency (ns/op) | Memory (B/op) | Allocations |
|-----------|----------------|-----------------|---------------|-------------|
| Encode Produce Request | 25,900,000 | 38.6 | 80 | 1 |
| Decode Produce Request | 9,070,000 | 110 | 336 | 9 |
| Encode Fetch Request | 46,300,000 | 21.6 | 64 | 1 |
| Decode Fetch Request | 14,200,000 | 70.5 | 208 | 6 |

**Notes:**
- Minimal allocations for encoding (1 alloc)
- Decoding requires more allocations for data structures
- Sub-microsecond operations make protocol overhead negligible

### Storage Layer

LSM-tree storage engine with Write-Ahead Log:

#### Index Operations

| Operation | Operations/sec | Latency (ns/op) | Memory (B/op) | Allocations |
|-----------|----------------|-----------------|---------------|-------------|
| Index Add | 1,165,000 | 858 | 81 | 0 |
| Index Lookup | 38,900,000 | 25.7 | 0 | 0 |

**Notes:**
- Zero allocations for lookups (optimized hot path)
- Sub-microsecond operations for both add and lookup

#### Log Operations

| Operation | Operations/sec | Latency (ns/op) | Memory (B/op) | Allocations |
|-----------|----------------|-----------------|---------------|-------------|
| Append (Single) | 913,000 | 1,095 | 261 | 8 |
| Append (Batch) | 182,000 | 5,494 | 1,333 | 36 |
| Read | 80,800 | 12,375 | 17,426 | 607 |

**Notes:**
- Single append ~1µs (without fsync)
- Batch append amortizes cost across multiple messages
- Read includes deserialization overhead

#### MemTable Operations

| Operation | Operations/sec | Latency (ns/op) | Memory (B/op) | Allocations |
|-----------|----------------|-----------------|---------------|-------------|
| Put | 3,700,000 | 270 | 202 | 9 |
| Get | 7,140,000 | 140 | 39 | 3 |
| Iterator | 347,000 | 2,879 | 48 | 1 |

**Notes:**
- In-memory operations with minimal allocations
- Iterator scans multiple keys efficiently

#### Write-Ahead Log (WAL)

| Operation | Operations/sec | Latency | Memory (B/op) | Allocations |
|-----------|----------------|---------|---------------|-------------|
| Append (No Sync) | 1,088,000 | 919 ns | 102 | 1 |
| Append (With Sync) | 118 | 8.5 ms | 74 | 1 |
| Read | 1,089,000 | 935 ns | 4,216 | 4 |

**Notes:**
- Without fsync: sub-microsecond writes
- With fsync: ~8.5ms (depends on disk performance)
- Production typically uses group commit to amortize fsync cost

## Performance Characteristics

### Latency Distribution

**End-to-End Producer (25.1 µs average):**
- Protocol encoding: ~40 ns (0.16%)
- Network + overhead: ~5 µs (20%)
- Storage write: ~1.1 µs (4.4%)
- Remaining: Connection handling, context switching (~19 µs, 75%)

### Memory Efficiency

**Per-Message Memory Usage:**
- Producer: 2,166 bytes per send operation
- Consumer: 1,318 bytes per fetch operation
- Storage: 261 bytes per append (single message)

**Total Memory Footprint:**
- Server (idle): <50 MB
- Server (active, 1000 connections): <100 MB
- No JVM heap required (vs Kafka's 2-8 GB)

### Allocation Efficiency

**Hot Path Allocations:**
- Index lookup: 0 allocations (zero-copy)
- WAL append: 1 allocation (buffer reuse)
- Protocol encode: 1 allocation (pre-sized buffer)

## Comparison to Apache Kafka

### Direct Metrics Comparison

| Metric | StreamBus | Kafka (Typical) | Notes |
|--------|-----------|-----------------|-------|
| Producer Latency | 25 µs | 500 µs - 5 ms | Kafka optimized for batches |
| Memory Footprint | <100 MB | 2-8 GB | No JVM heap required |
| Cold Start | <1 second | 15-45 seconds | Single binary vs JVM |
| GC Pause (Max) | <1 ms | 10-200 ms | Go GC vs Java G1GC |
| GC Pause (P99) | <500 µs | 5-50 ms | Go's concurrent GC |

### Architectural Differences

**StreamBus:**
- Single-threaded request handling (Go's goroutines)
- Optimized for low-latency individual messages
- LSM-tree storage with in-memory MemTable
- Binary protocol with minimal overhead
- Connection pooling with health checks

**Apache Kafka:**
- Multi-threaded with complex thread pools
- Optimized for high-throughput batches
- Append-only log segments
- Binary protocol with versioning
- Connection multiplexing

### When to Use Which

**Use StreamBus for:**
- Low-latency, real-time message delivery (<100µs requirements)
- Resource-constrained environments
- Simple deployments without JVM expertise
- Development and testing (fast startup)

**Use Kafka for:**
- High-throughput batch processing (millions of msgs/sec)
- Production systems requiring proven reliability
- Existing Kafka ecosystem integration
- Large-scale distributed deployments (100+ brokers)

## Optimization Notes

### Already Optimized

1. **Zero-copy where possible**: Index lookups use direct memory access
2. **Buffer pooling**: Reuse buffers for encoding/decoding
3. **Minimal allocations**: Hot paths have <5 allocations per operation
4. **Efficient serialization**: Custom binary protocol

### Future Optimization Opportunities

1. **Batch operations**: Group multiple messages in client
2. **Compression**: Add compression for large payloads
3. **Zero-copy networking**: Use sendfile() for large transfers
4. **SIMD**: Use SIMD instructions for checksums and encoding
5. **Lock-free structures**: Replace mutexes with atomic operations

## Benchmark Methodology

### Test Setup

1. **Server**: Local server on same machine (minimal network latency)
2. **Client**: Single client instance
3. **Data**: Small messages (key: 10 bytes, value: 30 bytes)
4. **Duration**: 2 seconds per benchmark
5. **Warmup**: Implicit Go benchmark warmup

### Measurement

- **Latency**: Time per operation (ns/op or µs/op)
- **Throughput**: Operations per second (calculated from latency)
- **Memory**: Bytes allocated per operation (B/op)
- **Allocations**: Number of heap allocations per operation

### Limitations

1. **Local testing**: Network latency not representative of production
2. **Small messages**: Performance may differ with large payloads
3. **Single client**: Doesn't test concurrent load
4. **No replication**: Distributed features not tested

### Running Benchmarks

```bash
# Quick benchmark
make benchmark

# Detailed benchmark with multiple runs
go test -bench=. -benchmem -benchtime=5s -count=5 ./...

# CPU profiling
go test -bench=. -benchmem -cpuprofile=cpu.prof ./pkg/storage/...
go tool pprof cpu.prof

# Memory profiling
go test -bench=. -benchmem -memprofile=mem.prof ./pkg/client/...
go tool pprof mem.prof
```

## Continuous Performance Monitoring

We track performance regressions using:

1. **Baseline benchmarks**: Stored in `benchmarks/baseline.txt`
2. **PR benchmarks**: Run on every pull request
3. **Regression detection**: Automated comparison with baseline

```bash
# Set baseline
make benchmark-baseline

# Check for regressions
make benchmark-compare
```

## Contributing Benchmarks

When adding new features:

1. Add benchmarks for new operations
2. Run `make benchmark-compare` to check for regressions
3. Include benchmark results in PR description
4. Update this document with new benchmark data

## Appendix: Raw Benchmark Output

See `benchmarks/report.md` for the most recent raw benchmark output, or generate your own:

```bash
make benchmark-report
cat benchmarks/report.md
```

---

Last updated: January 2025
