# StreamBus — Product Requirements Document (PRD)

**Version**: 1.0
**Date**: 2026-03-15
**Status**: Beta — Active Development
**License**: Apache 2.0

---

## 1. Vision

StreamBus is a high-performance, distributed streaming platform built entirely in Go, designed as a drop-in replacement for Apache Kafka with dramatically lower latency, memory usage, and operational complexity.

**One-liner**: Sub-millisecond streaming with zero JVM overhead — a single binary that replaces Kafka + ZooKeeper.

---

## 2. Problem Statement

Apache Kafka dominates distributed streaming, but imposes significant costs:

| Pain Point | Kafka Reality | Impact |
|---|---|---|
| **Latency** | 0.5–5 ms per operation | Unacceptable for real-time use cases |
| **Memory** | 2–8 GB JVM heap minimum | Expensive in containerized environments |
| **Cold start** | 15–45 seconds | Slow failover, poor autoscaling |
| **Operations** | JVM tuning + ZooKeeper cluster | High DevOps burden |
| **Complexity** | Multi-process, multi-language | Steep learning curve |

Organizations need a streaming platform that is fast, lightweight, and operationally simple — without sacrificing reliability or feature parity.

---

## 3. Target Users

| Persona | Need |
|---|---|
| **Platform Engineers** | Drop-in Kafka replacement with lower resource footprint and simpler operations |
| **Backend Developers** | Idiomatic Go SDK for event-driven microservices |
| **SREs / DevOps** | Single-binary deployment, Prometheus metrics, health probes, structured logging |
| **Startups / Edge** | Lightweight streaming for resource-constrained environments (IoT, edge, small clusters) |

---

## 4. Core Requirements

### 4.1 Distributed Streaming Engine

| Requirement | Detail |
|---|---|
| **Topics & Partitions** | Named topics with configurable partition count and replication factor |
| **Producer** | Publish messages (key/value + headers) with configurable acks, batching, and retry |
| **Consumer** | Fetch messages by offset with configurable fetch size |
| **Consumer Groups** | Automatic partition assignment and rebalancing (range, round-robin, sticky) |
| **Offset Management** | Server-side offset tracking per consumer group |
| **Ordering** | Per-partition ordering guarantee |

### 4.2 Storage

| Requirement | Detail |
|---|---|
| **LSM-Tree Engine** | Custom write-optimized storage with MemTable → SSTable pipeline |
| **Write-Ahead Log** | Crash recovery via WAL with configurable sync policy |
| **Compaction** | Leveled (default), size-tiered, and time-window strategies |
| **Indexing** | Offset-to-position index for O(log n) seeks |
| **Retention** | Time-based and size-based retention policies |

### 4.3 Cluster Coordination

| Requirement | Detail |
|---|---|
| **Raft Consensus** | Built-in metadata coordination (no ZooKeeper) via etcd/raft |
| **Leader Election** | Automatic leader election per partition |
| **Replication** | Leader-follower with in-sync replica (ISR) tracking |
| **Failover** | Automatic leader failover when broker goes down |
| **Metadata** | Cluster-wide topic/partition/broker registry via Raft log |

### 4.4 Reliability & Transactions

| Requirement | Detail |
|---|---|
| **Exactly-Once** | Transactional producer/consumer with atomic multi-partition writes |
| **Idempotent Producers** | Duplicate detection via producer ID + sequence number |
| **Circuit Breakers** | Fail-fast for unhealthy dependencies with half-open recovery |
| **Error Categorization** | Retriable, transient, fatal, invalid input — with automatic retry strategies |

### 4.5 Security

| Requirement | Detail |
|---|---|
| **Encryption** | TLS/mTLS for all broker-to-broker and client-to-broker traffic |
| **Authentication** | SASL (PLAIN, SCRAM-SHA-256, SCRAM-SHA-512) |
| **Authorization** | ACL-based access control per topic/consumer group |
| **Audit** | Structured audit logging for all security events |

### 4.6 Observability

| Requirement | Detail |
|---|---|
| **Metrics** | 40+ Prometheus metrics (throughput, latency, storage, consumer lag) |
| **Health Checks** | `/health`, `/health/live`, `/health/ready` — Kubernetes-native |
| **Structured Logging** | JSON-formatted logs with component, request ID, and contextual fields |
| **Distributed Tracing** | OpenTelemetry with OTLP, Jaeger, and Zipkin exporters |

### 4.7 Multi-Tenancy

| Requirement | Detail |
|---|---|
| **Tenant Isolation** | Logical isolation of topics, quotas, and ACLs per tenant |
| **Resource Quotas** | Configurable produce/consume rate limits per tenant |

### 4.8 Schema Registry

| Requirement | Detail |
|---|---|
| **Formats** | JSON Schema, Apache Avro, Protocol Buffers |
| **Compatibility** | BACKWARD, FORWARD, FULL (with transitive variants) |
| **Validation** | Server-side schema enforcement on produce |

---

## 5. Wire Protocol

StreamBus uses a **custom binary protocol** on port 9092 (Kafka-compatible port for migration ease).

### Frame Format

```
[Length (4 bytes)] [Header (20 bytes)] [Payload (variable)] [CRC32 (4 bytes)]
```

### Header

| Field | Size | Description |
|---|---|---|
| RequestID | 8 bytes | Unique request identifier |
| Type | 1 byte | Request type enum (Produce, Fetch, etc.) |
| Version | 1 byte | Protocol version |
| Flags | 2 bytes | Bitflags (compression, etc.) |

### Message Format

```
[Offset (8)] [Timestamp (8)] [KeyLen (4)] [Key (var)] [ValueLen (4)] [Value (var)] [HeaderCount (4)] [Headers...]
```

### Request Types

| Code | Type | Description |
|---|---|---|
| 0x01 | Produce | Write message batch to topic/partition |
| 0x02 | Fetch | Read messages from offset |
| 0x03 | GetOffset | Query committed/latest offset |
| 0x04 | CreateTopic | Create topic with partitions/replication |
| 0x05 | DeleteTopic | Remove topic |
| 0x06 | ListTopics | Enumerate topics |
| 0x07 | HealthCheck | Protocol-level health check |
| 0x08–0x12 | Consumer Groups / Transactions | JoinGroup, SyncGroup, Heartbeat, OffsetCommit, etc. |

### Compression

LZ4, Snappy, Zstd supported per message batch (flag in header).

---

## 6. Internal Serialization

The following internal components currently use **JSON** (`encoding/json`) for serialization:

| Component | What's Serialized | Where |
|---|---|---|
| **Metadata / Raft FSM** | Cluster operations (create topic, update leader, register broker) | `pkg/metadata/fsm.go`, `pkg/metadata/store.go` |
| **Raft Snapshots** | Full cluster state snapshots | `pkg/metadata/fsm.go` |
| **Metadata Types** | BrokerInfo, TopicInfo, PartitionInfo structs | `pkg/metadata/types.go` |
| **Health HTTP** | Health check response bodies | `pkg/health/health.go`, `pkg/health/http.go` |
| **Structured Logging** | Log entries (timestamp, level, message, fields) | `pkg/logging/logger.go` |
| **Security / Audit** | Audit log entries, encryption key storage | `pkg/security/audit.go`, `pkg/security/encryption.go` |
| **Schema Validator** | JSON Schema validation | `pkg/schema/validator.go` |
| **Admin API** | HTTP admin endpoints (topic management, etc.) | `pkg/broker/admin_api.go` |
| **Tenancy API** | HTTP tenant management endpoints | `pkg/broker/tenancy_api.go` |

**Note**: The wire protocol (client ↔ broker communication) already uses a custom binary codec — JSON is **not** used on the hot path.

---

## 7. Performance Targets

| Metric | Target | Current |
|---|---|---|
| Producer latency (p50) | < 50 µs | 25 µs |
| Consumer fetch latency (p50) | < 50 µs | 21.8 µs |
| Protocol encode (produce) | < 100 ns | 38.6 ns |
| Protocol decode (produce) | < 200 ns | 110 ns |
| Memory footprint | < 100 MB | < 100 MB |
| Cold start | < 1 second | < 1 second |
| Storage write (single) | < 2 µs | 1,095 ns |
| Storage read (MemTable) | < 500 ns | 140 ns |

---

## 8. Deployment & Configuration

### Deployment Modes

| Mode | Description |
|---|---|
| **Single binary** | `./streambus --port 9092` — zero external dependencies |
| **Docker** | Single-container or docker-compose (3-broker cluster + Prometheus + Grafana) |
| **Kubernetes** | Liveness/readiness probes, CRD-based operator (planned) |

### Configuration

- CLI flags (highest priority) → environment variables (`STREAMBUS_*`) → YAML config → defaults
- Server: broker ID, host, ports (9092 binary, 9093 gRPC, 8080 HTTP)
- Storage: data directory, compaction strategy, retention
- Cluster: Raft data directory, peer list
- Security: TLS certs, SASL config, ACL definitions

---

## 9. SDK & Client Libraries

| SDK | Status |
|---|---|
| **Go SDK** (`streambus-sdk`) | Released — full-featured (producer, consumer, groups, transactions) |
| **Python SDK** | Planned |
| **C# SDK** | Planned |
| **Java SDK** | Planned |

---

## 10. Migration Path

- **Port 9092**: Same default port as Kafka for transparent migration
- **Mirror Maker**: `cmd/mirror-maker/` tool for Kafka → StreamBus data migration
- **Concept Parity**: Topics, partitions, consumer groups, offsets, transactions all map 1:1
- **Migration Guide**: `docs/migration-from-kafka.md`

---

## 11. Quality Requirements

| Metric | Target |
|---|---|
| Unit test coverage (overall) | ≥ 85% |
| Critical path coverage | ≥ 95% |
| Per-package minimum | ≥ 70% |
| Cognitive complexity per method | < 15 |
| Zero Kafka software dependencies | Enforced (`make check-kafka-deps`) |

---

## 12. Current Status & Roadmap

### Completed

- Core streaming engine (produce/consume/topics/partitions)
- LSM-tree storage with WAL
- Storage compaction — all 3 strategies: leveled, size-tiered, time-window (`pkg/storage/compaction.go`)
- Retention enforcement — time-based and size-based with background manager (`pkg/storage/retention.go`)
- Raft consensus and multi-broker replication
- Consumer groups with partition rebalancing — range, round-robin, sticky assignors (`pkg/consumer/group/assignor.go`)
- Transactions and exactly-once semantics
- Schema registry (Avro, Protobuf, JSON Schema) with produce-path enforcement (`pkg/server/schema_handler.go`)
- Schema registration HTTP API (`pkg/broker/schema_api.go`)
- TLS/SASL/ACL security with request-level ACL enforcement (`pkg/server/security_handler.go`)
- Prometheus metrics, OpenTelemetry tracing, structured logging
- Grafana dashboards — overview, performance, consumer groups, storage (`dashboards/grafana/dashboards/`)
- Multi-tenancy
- Go SDK

### In Progress

- Test coverage: 81% → 85%+
- Cross-datacenter replication
- Kubernetes operator
- Additional admin tooling

### Planned

- Python, C#, Java SDKs
- Kafka consumer protocol compatibility layer
- Tiered storage (hot/warm/cold)
- Stream processing DSL

---

## 13. Architecture Diagram

```
┌─────────────┐       ┌──────────────────────────────────────┐       ┌─────────────┐
│  Producers  │       │         StreamBus Cluster            │       │  Consumers  │
│  (Go SDK)   │──────▶│                                      │──────▶│  (Go SDK)   │
└─────────────┘  9092 │  ┌──────────┐    ┌──────────┐       │ 9092  └─────────────┘
                binary │  │ Broker 1 │    │ Broker 2 │       │ binary
                       │  │ (Leader) │◄──▶│(Follower)│       │
                       │  └────┬─────┘    └────┬─────┘       │
                       │       │               │              │
                       │       └───────┬───────┘              │
                       │               ▼                      │
                       │       ┌───────────────┐              │
                       │       │Raft Consensus │              │
                       │       │  (Metadata)   │              │
                       │       └───────────────┘              │
                       │                                      │
                       │  ┌──────────────────────────┐        │
                       │  │   LSM-Tree Storage       │        │
                       │  │  MemTable → WAL → SSTable│        │
                       │  └──────────────────────────┘        │
                       │                                      │
                       │  ┌───────┐ ┌────────┐ ┌──────────┐  │
                       │  │Health │ │Metrics │ │ Tracing  │  │
                       │  │:8080  │ │Prom    │ │ OTel     │  │
                       │  └───────┘ └────────┘ └──────────┘  │
                       └──────────────────────────────────────┘
```

---

## 14. Key Constraints

1. **No Kafka software dependencies** — StreamBus must be fully self-contained
2. **Go-only** — server and core SDK are pure Go, no CGo
3. **Single binary** — no external processes required (no ZooKeeper, no JVM)
4. **Backward compatibility** — protocol versioning must support rolling upgrades
5. **Apache 2.0 license** — all code and dependencies must be compatible
