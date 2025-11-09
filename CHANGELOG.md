# Changelog

All notable changes to StreamBus will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### In Progress
- Multi-tenancy with quota management
- Cross-datacenter replication
- Admin CLI tool
- Web management UI
- Kubernetes operator

## [0.5.0] - 2025-01-09

### Added - Phase 4.2: Observability

#### Prometheus Metrics Integration
- Complete Prometheus exporter with 40+ pre-defined metrics
- `StreamBusMetrics` struct covering all broker operations:
  - Broker metrics (uptime, status, connections, topics)
  - Message metrics (produced, consumed, bytes, errors)
  - Performance metrics (produce latency, consume latency, request latency)
  - Consumer group metrics (groups, members, lag)
  - Transaction metrics (active, committed, aborted, timeouts)
  - Storage metrics (used, available, segments, compactions)
  - Network metrics (bytes in/out, requests, errors)
  - Security metrics (authentication, authorization, audit events)
  - Cluster metrics (size, leaders, Raft state)
  - Schema metrics (registered, validations, errors)
- HTTP endpoint for Prometheus scraping (`/metrics`)
- Complete test suite for metrics export
- Working observability example (`examples/observability/`)

#### OpenTelemetry Distributed Tracing
- Complete OpenTelemetry integration package (`pkg/tracing/`)
- Support for multiple exporters:
  - OTLP (gRPC and HTTP)
  - Jaeger (native format)
  - Zipkin (compatible)
  - Stdout (for development)
- Configurable sampling strategies:
  - Always sample
  - Never sample
  - Probability-based sampling
  - Parent-based sampling
- W3C Trace Context propagation
- 40+ StreamBus-specific trace attributes
- Helper functions for common tracing patterns
- Complete test coverage
- Working tracing example (`examples/tracing/`)

#### Complete Observability Stack
- Docker Compose configuration with full monitoring stack:
  - Prometheus for metrics collection (port 9090)
  - Grafana for visualization (port 3000)
  - Jaeger for distributed tracing (port 16686)
  - OpenTelemetry Collector for aggregation (ports 4317/4318)
  - Optional: Zipkin, Loki, Promtail
- Pre-built Grafana dashboard (`streambus-overview.json`) with 9 panels:
  - Broker status and health
  - Broker uptime
  - Active connections
  - Topics count
  - Message throughput (produce/consume rates)
  - Latency percentiles (p95/p99)
  - Network throughput
  - Storage utilization
  - Consumer group lag
- Auto-provisioned datasources (Prometheus, Jaeger, Loki)
- OpenTelemetry Collector configuration with:
  - OTLP receivers (gRPC and HTTP)
  - Batch and memory limiter processors
  - Exporters for Jaeger and Prometheus
  - Complete pipelines for traces, metrics, and logs
- Comprehensive documentation (`dashboards/README.md`) with:
  - Quick start guide
  - Architecture diagrams
  - Configuration examples
  - Custom dashboard creation
  - Alerting examples
  - Performance tuning
  - Production recommendations
  - Troubleshooting guide

### Documentation
- Updated README.md to reflect Phase 3-4 completion
- Updated ROADMAP.md with actual completion dates
- Created CHANGELOG.md
- Updated test counts: 532/536 tests passing (99.3%)
- Marked project status as "Beta" (Phases 1-4.2 complete)

## [0.4.0] - 2025-01-08

### Added - Phase 4.1: Security

#### TLS Encryption
- TLS 1.2+ support for broker-to-broker and client-to-broker communication
- Mutual TLS (mTLS) for client authentication
- Configurable cipher suites and protocol versions
- Certificate validation and management
- Secure configuration loading

#### SASL Authentication
- SASL/PLAIN authentication mechanism
- SASL/SCRAM-SHA-256 with secure password hashing
- SASL/SCRAM-SHA-512 for maximum security
- User credential management
- Authentication failure tracking

#### ACL-based Authorization
- Fine-grained access control for topics and consumer groups
- Resource patterns with wildcards:
  - LITERAL: Exact match
  - PREFIXED: Prefix matching
  - ANY: Wildcard matching
- Operation-level permissions:
  - READ, WRITE, CREATE, DELETE
  - DESCRIBE, ALTER, DESCRIBE_CONFIGS, ALTER_CONFIGS
  - CLUSTER_ACTION, IDEMPOTENT_WRITE
- Principal-based authorization
- ACL storage and replication
- Default deny-all policy

#### Audit Logging
- Comprehensive audit trail for security events:
  - Authentication success/failure
  - Authorization decisions (allow/deny)
  - Configuration changes
  - Admin operations
  - ACL modifications
- Structured audit log format
- Configurable audit log destination
- Audit log rotation and retention

#### Security Infrastructure
- User and credential store
- ACL store with efficient lookups
- Security context propagation
- Password hashing with salt
- Secure configuration validation
- Complete test suite (147 tests passing)

## [0.3.0] - 2025-01-07

### Added - Phase 3: Advanced Features

#### Consumer Groups (3.1)
- Consumer group management and coordination
- Multiple partition assignment strategies:
  - **Range**: Continuous partition ranges per consumer
  - **RoundRobin**: Even distribution across all topics
  - **Sticky**: Minimize partition movement during rebalancing
- Group coordinator for membership management
- Rebalancing protocol:
  - Join group requests
  - Sync group protocol
  - Heartbeat mechanism
  - Session timeout handling
- Offset management:
  - Offset commit API
  - Offset fetch API
  - Offset storage in metadata
- Group recovery and failover
- Complete test suite with edge cases

#### Transactions (3.2)
- Transactional producer API:
  - `BeginTransaction()`
  - `SendMessage()` with transaction context
  - `SendOffsetsToTransaction()` for atomic offset commits
  - `CommitTransaction()`
  - `AbortTransaction()`
- Transaction coordinator:
  - Transaction ID management
  - Producer ID generation
  - Epoch tracking
  - Transaction state management
- Exactly-once semantics (EOS):
  - Atomic multi-partition writes
  - Read committed isolation level
  - Transaction markers (BEGIN, COMMIT, ABORT)
  - Duplicate detection via sequence numbers
- Idempotent producers:
  - Producer ID assignment
  - Sequence number tracking
  - Duplicate message detection
- Transaction timeout handling
- Transaction recovery after crashes
- Complete test suite

#### Schema Registry (3.3)
- Schema storage and versioning:
  - Version history per subject
  - Schema ID assignment
  - Schema metadata storage
- Multiple format support:
  - Apache Avro
  - Protocol Buffers (Protobuf)
  - JSON Schema
- Schema evolution:
  - Compatibility checking (backward, forward, full, none)
  - Schema validation rules
  - Breaking change detection
- Integration APIs:
  - `RegisterSchema()` for new schemas
  - `GetSchema()` by ID or subject/version
  - `ValidateMessage()` against schema
  - `CheckCompatibility()` for evolution
- Schema versioning strategies
- Complete test suite

## [0.2.6] - 2025-01-05

### Added - Phase 2.6: Production Hardening

#### Circuit Breaker Pattern
- Automatic fail-fast for unhealthy dependencies
- Three states: Closed, Open, Half-Open
- Configurable failure threshold
- Configurable timeout for half-open attempts
- Success threshold for recovery
- State change callbacks
- Metrics tracking (requests, failures, state changes)
- Complete test suite (14 tests)

#### Health Check System
- Comprehensive component health monitoring
- Three health states: Healthy, Degraded, Unhealthy
- Component-level health checks:
  - Storage engine health
  - Network connectivity
  - Cluster coordination
  - Replication status
- Kubernetes-ready endpoints:
  - `/health` - Overall system health
  - `/health/live` - Liveness probe
  - `/health/ready` - Readiness probe with dependency checks
- Dependency health tracking
- Health status caching
- Complete test suite (18 tests)

#### Enhanced Error Handling
- Error categorization system:
  - **Retriable**: Temporary failures, safe to retry
  - **Transient**: Potentially transient issues
  - **Fatal**: Permanent failures, cannot recover
  - **Invalid Input**: Client errors
- Automatic retry with exponential backoff
- Context preservation through error chains
- Detailed error metadata
- Error wrapping with category preservation
- Complete test suite (30 tests)

#### Prometheus Metrics
- Native Prometheus metrics integration
- Counter, Gauge, and Histogram types
- Label support for multi-dimensional metrics
- HTTP handler for `/metrics` endpoint
- Metric families with proper help text
- Histogram buckets for latency tracking
- Thread-safe metric updates
- Complete test suite (29 tests)

#### Structured Logging
- JSON-formatted logs for machine parsing
- Contextual fields:
  - Component identification
  - Operation tracking
  - Request ID tracing
  - Timestamp with nanosecond precision
  - Log level
- Multiple log levels: DEBUG, INFO, WARN, ERROR
- Component-level log filtering
- Context-aware logging
- Error logging with stack traces
- Complete test suite (24 tests)

#### Timeout Management
- Centralized timeout configuration
- Operation-specific timeouts:
  - Request timeout
  - Connection timeout
  - Shutdown timeout
  - Replication timeout
- Context-based timeout enforcement
- Timeout propagation through call chains
- Runtime configuration updates
- Timeout validation
- Complete test suite (29 tests)

## [0.2.0] - 2025-01-02

### Added - Phase 2: Distributed System

#### Raft Consensus Implementation (2.1)
- Complete Raft consensus protocol
- Leader election with randomized timeouts
- Log replication with consistency checks
- Snapshot management for compaction
- Persistent state management
- Configuration changes
- Complete test suite (36 tests)

#### Metadata Replication (2.2)
- Metadata store with versioning
- Topic metadata management
- Partition metadata tracking
- Broker registry and discovery
- Cluster configuration state
- Metadata replication via Raft
- Complete test suite (8 tests)

#### Cluster Coordination (2.3)
- Partition assignment algorithms
- Leader-follower coordination
- Rebalancing logic
- Broker health tracking
- Partition replica management
- Complete test suite (10 tests)

#### Multi-Broker Replication (2.4)
- Data replication between brokers
- In-sync replica (ISR) tracking
- Leader-follower replication
- High-water mark management
- Replica synchronization
- Failover and recovery
- Integration tested

#### End-to-End Integration (2.5)
- Full cluster operations
- Producer-to-consumer flow
- Multi-broker scenarios
- Failover testing
- Performance validation
- Complete test suite (5 tests)

## [0.1.0] - 2024-12-20

### Added - Phase 1: Core Platform

#### Storage Engine (1.1)
- LSM-tree storage implementation
- Write-Ahead Log (WAL) for durability
- MemTable for in-memory operations
- SSTable for disk persistence
- Bloom filters for efficient lookups
- Compaction strategies
- Index management
- Complete test suite (27 tests)

#### Network Layer (1.2)
- Binary protocol design
- Protocol encoding/decoding
- Message types:
  - Produce requests/responses
  - Fetch requests/responses
  - Topic create/delete
  - Metadata requests
- TCP server with connection handling
- Request routing and error handling
- Connection pooling with health checks
- Producer client with batching
- Consumer client with offset management
- Complete test suite (22 tests)

### Performance
- Producer latency: ~25 µs per operation
- Consumer fetch: ~22 µs per operation
- Storage write: 1,095 ns per operation
- Storage read: 140 ns from MemTable
- Protocol encoding: 38.6 ns (Produce), 21.6 ns (Fetch)
- Protocol decoding: 110 ns (Produce), 70.5 ns (Fetch)

## Project Statistics

### Test Coverage
- **Total Tests**: 536
- **Passing Tests**: 532 (99.3%)
- **Failing Tests**: 4 (rebalancing edge cases being fixed)

### Phases Completed
- ✅ Phase 1: Core Platform (100%)
- ✅ Phase 2: Distributed System (100%)
- ✅ Phase 2.6: Production Hardening (100%)
- ✅ Phase 3: Advanced Features (100%)
- ✅ Phase 4.1: Security (100%)
- ✅ Phase 4.2: Observability (100%)

### Lines of Code (estimated)
- Core implementation: ~25,000 lines
- Test code: ~15,000 lines
- Total: ~40,000 lines of Go code

## Versioning Strategy

StreamBus follows [Semantic Versioning](https://semver.org/):

- **MAJOR** version: Incompatible API changes
- **MINOR** version: New functionality in a backwards compatible manner
- **PATCH** version: Backwards compatible bug fixes

Current status: Pre-1.0 (0.5.0) - API may change between minor versions

## Upgrade Notes

### From 0.4.x to 0.5.x
- No breaking changes
- Add Prometheus metrics configuration to enable monitoring
- Optional: Deploy observability stack from `dashboards/` directory
- Optional: Configure OpenTelemetry tracing in broker config

### From 0.3.x to 0.4.x
- TLS configuration is now required for secure deployments
- SASL authentication can be enabled optionally
- ACL configuration is required if authorization is enabled
- Update client configurations to include security settings

### From 0.2.x to 0.3.x
- Consumer group API is new - existing consumers still work
- Transaction API is optional - non-transactional producers unchanged
- Schema registry is optional - can continue without schemas
- No breaking changes to existing APIs

## Links

- [Project Homepage](https://github.com/shawntherrien/streambus)
- [Documentation](https://github.com/shawntherrien/streambus/tree/main/docs)
- [Issue Tracker](https://github.com/shawntherrien/streambus/issues)
- [Discussions](https://github.com/shawntherrien/streambus/discussions)

---

**Note**: Versions before 1.0.0 are considered development releases. The API may change between minor versions. Production use is recommended only after 1.0.0 release.
