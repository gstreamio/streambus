# Changelog

All notable changes to StreamBus will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Phase 2.6 - Production Hardening (January 2025)

#### Added
- **Circuit Breaker Pattern**: Fault tolerance with three-state circuit breaker
  - Configurable failure/success thresholds
  - Timeout and max half-open requests
  - State change callbacks for monitoring
  - Thread-safe concurrent access
  - 14 comprehensive tests

- **Health Check System**: Kubernetes-ready health monitoring
  - `/health` - Full component health details
  - `/health/live` - Liveness probe
  - `/health/ready` - Readiness probe
  - Component-specific checkers (Raft, metadata, broker, coordinator)
  - Periodic health checks with result caching
  - 18 comprehensive tests

- **Enhanced Error Handling**: Categorized errors for better retry logic
  - Five error categories (Retriable, Transient, Fatal, InvalidInput, Unknown)
  - Structured Error type with metadata and timestamps
  - Automatic error classification
  - Context enrichment with operation names
  - MultiError for batch operations
  - 30 comprehensive tests

- **Metrics System**: Prometheus-compatible metrics
  - Counter, Gauge, and Histogram metric types
  - Thread-safe metrics registry
  - `/metrics` HTTP endpoint
  - Default and custom histogram buckets
  - 29 comprehensive tests

- **Structured Logging**: JSON-formatted logging with contextual fields
  - Five log levels (DEBUG, INFO, WARN, ERROR, FATAL)
  - Component and operation tracking
  - Optional stack traces and file/line information
  - Configurable log levels at runtime
  - 24 comprehensive tests

- **Timeout Management**: Centralized timeout configuration
  - Three config presets (Default, Production, Test)
  - Context-based timeout utilities
  - Operation execution with retry and exponential backoff
  - Runtime configuration updates
  - 29 comprehensive tests

#### Test Coverage
- Added 144 new tests for production hardening features
- **Total**: 252 tests passing (100% coverage)
- All components fully tested with comprehensive test suites

### Phase 2.5 - End-to-End Integration (January 2025)

#### Added
- End-to-end integration testing framework
- Multi-broker cluster integration tests
- Producer-to-consumer flow validation
- Failover and recovery scenarios
- Network partition handling tests

#### Test Coverage
- Added 5 integration tests
- Full cluster operation validation
- 100% pass rate

### Phase 2: Distributed System (December 2024 - January 2025)

#### Milestone 2.4: Multi-Broker Replication

##### Added
- Data replication between brokers
- In-sync replica (ISR) tracking
- Replication protocol implementation
- Leader-follower coordination for data
- Replication lag monitoring

##### Changed
- Enhanced broker coordination for replication
- Improved failover handling with replica awareness

#### Milestone 2.3: Cluster Coordination

##### Added
- Partition assignment engine
- Rebalancing strategies
- Leader tracking across partitions
- Cluster health monitoring
- Broker availability detection

##### Test Coverage
- Added 10 coordinator tests
- Rebalancing algorithm validation
- Partition assignment testing

#### Milestone 2.2: Metadata Replication

##### Added
- Metadata store with versioning
- Cluster state management
- Broker registration and discovery
- Partition metadata replication
- Version-based conflict resolution

##### Test Coverage
- Added 8 metadata store tests
- Replication validation
- Version management testing

#### Milestone 2.1: Raft Consensus

##### Added
- Complete Raft consensus implementation
- Leader election with randomized timeouts
- Log replication with batching
- Snapshot management and compaction
- Member management (add/remove nodes)
- State machine interface
- Persistent storage for logs and state

##### Test Coverage
- Added 36 Raft tests
- Leader election scenarios
- Log replication validation
- Snapshot creation and restoration
- Network partition handling

##### Performance
- Sub-millisecond leader election
- Efficient log batching
- Optimized snapshot compression

### Phase 1: Core Platform (November - December 2024)

#### Milestone 1.2: Network Layer

##### Added
- Custom binary protocol implementation
- Protocol encoding/decoding with CRC validation
- TCP server with connection pooling
- Request routing and error handling
- Producer client with batching
- Consumer client with offset management
- Connection health checks
- Automatic retry with exponential backoff

##### Test Coverage
- Added 22 client tests
- 100% protocol layer coverage
- End-to-end client-server validation

##### Performance
- Producer latency: 25.1 µs/op
- Consumer latency: 21.8 µs/op
- Protocol encoding: 38.6 ns/op
- Protocol decoding: 110 ns/op

#### Milestone 1.1: Storage Engine

##### Added
- LSM-tree storage implementation
- Write-Ahead Log (WAL) for durability
- MemTable with sorted key-value storage
- SSTable file format with indexing
- Background compaction
- Snapshot support
- Configurable flush and compaction thresholds

##### Test Coverage
- Added 27 storage tests
- 100% storage engine coverage
- Performance benchmarks

##### Performance
- Write: 1,095 ns/op (single), 5,494 ns/op (batch)
- Read: 140 ns/op from MemTable
- WAL append: 919 ns/op (buffered)
- Index lookup: 25.7 ns/op

---

## Release Schedule

- **v0.1.0** (Q1 2025): Developer Preview - Phase 1 & 2 complete
- **v0.2.0** (Q2 2025): Consumer Groups & Transactions
- **v0.3.0** (Q3 2025): Security & Multi-tenancy
- **v0.4.0** (Q4 2025): Beta Release
- **v1.0.0** (Q1 2026): Production Release

---

## Version History

### Development Phase (Current)

All changes are in active development. No stable releases yet.

- **Phase 1 Complete**: Storage engine and network layer
- **Phase 2 Complete**: Distributed system with Raft consensus
- **Phase 3 In Progress**: Advanced features (consumer groups, transactions)

---

## Migration Notes

### From Development Builds

StreamBus is currently in development. When v1.0 is released, we will provide:
- Migration guides for breaking changes
- Backward compatibility notes
- Data migration tools

---

## Security

Security vulnerabilities should be reported to security@streambus.io.

See [SECURITY.md](../SECURITY.md) for our security policy.

---

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) for how to contribute to StreamBus.

---

**Questions about releases?** Check the [Roadmap](ROADMAP.md) or ask in [Discussions](https://github.com/shawntherrien/streambus/discussions).
