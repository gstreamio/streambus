# StreamBus

<div align="center">

**A next-generation distributed streaming platform built for performance, reliability, and operational simplicity**

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Go Reference](https://pkg.go.dev/badge/github.com/gstreamio/streambus.svg)](https://pkg.go.dev/github.com/gstreamio/streambus)
[![CI](https://github.com/gstreamio/streambus/actions/workflows/ci.yml/badge.svg)](https://github.com/gstreamio/streambus/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Test Coverage](https://img.shields.io/badge/Coverage-87.0%25-brightgreen)](docs/TESTING.md)
[![Production Ready](https://img.shields.io/badge/Status-Beta-blue)](docs/PRODUCTION_READINESS.md)

[Features](#key-capabilities) • [Performance](#performance) • [Quick Start](#quick-start) • [Documentation](#documentation) • [Community](#community)

</div>

---

## Why StreamBus?

StreamBus is a high-performance, distributed streaming platform designed for modern cloud-native applications. If you're dealing with real-time data streams, event-driven architectures, or need a Kafka alternative with better performance characteristics, StreamBus delivers:

- **🚀 10-100x Lower Latency**: Sub-millisecond operations vs multi-millisecond batch processing
- **💰 95% Lower Memory**: <100MB footprint vs multi-GB JVM heaps
- **⚡ Instant Startup**: Cold start in <1 second vs 15-45 second JVM initialization
- **🛠️ Zero Complexity**: Single binary, no JVM tuning, no ZooKeeper dependency
- **🔒 Enterprise-Grade**: Built-in circuit breakers, health checks, structured logging, and metrics

Perfect for microservices, event sourcing, log aggregation, real-time analytics, and IoT data pipelines.

---

## Key Capabilities

### Built for Performance

- **Sub-Millisecond Latency**: ~25µs producer latency, perfect for latency-sensitive applications
- **Memory Efficient**: Runs comfortably in containers with minimal resource allocation
- **Fast Recovery**: Cold start and failover in under 1 second
- **Optimized Storage**: Custom LSM-tree engine with efficient compaction and indexing

### Production Hardened

- **Circuit Breaker Pattern**: Automatic fail-fast with configurable thresholds
- **Health Monitoring**: Kubernetes-ready liveness and readiness probes
- **Structured Logging**: JSON-formatted logs with contextual fields for observability
- **Prometheus Metrics**: Native metrics export for comprehensive monitoring
- **Smart Error Handling**: Categorized errors with automatic retry strategies
- **Timeout Management**: Centralized timeout configuration for consistent behavior
- **Security**: TLS encryption, SASL authentication, ACL-based authorization
- **Audit Logging**: Complete audit trail for security and compliance
- **Distributed Tracing**: OpenTelemetry integration with Jaeger and Zipkin

### Operationally Simple

- **Single Binary Deployment**: No complex setup, no external dependencies
- **Minimal Configuration**: Sensible defaults with configuration validation
- **Container Native**: Optimized for Docker, Kubernetes, and cloud platforms
- **Self-Contained**: No ZooKeeper, no complex coordination layer
- **Easy Troubleshooting**: Comprehensive health checks and diagnostic endpoints

### Developer Friendly

- **Idiomatic Go Client**: Clean, type-safe API with excellent documentation
- **Automatic Batching**: Smart batching for optimal throughput without sacrificing latency
- **Connection Pooling**: Built-in connection management with health checks
- **Retry Logic**: Exponential backoff with configurable retry policies
- **Rich Examples**: Production-ready examples for common use cases
- **Consumer Groups**: Automatic partition rebalancing with multiple strategies
- **Transactions**: Exactly-once semantics with atomic multi-partition writes
- **Schema Registry**: Built-in schema validation for Avro, Protobuf, and JSON Schema
- **Multi-Tenancy**: Resource isolation and quota management for multiple tenants

---

## Use Cases

**Microservices Communication**
- Event-driven architecture between services
- Asynchronous command and query handling
- Service-to-service messaging with guaranteed delivery

**Real-Time Analytics**
- Streaming data ingestion for analytics pipelines
- Low-latency metric collection and aggregation
- Event stream processing for dashboards

**Event Sourcing & CQRS**
- Persistent event store with replay capabilities
- Command and event separation
- Temporal queries and projections

**Log Aggregation**
- Centralized logging from distributed systems
- High-throughput log collection
- Searchable log streams

**IoT Data Processing**
- Sensor data ingestion at scale
- Edge-to-cloud data streaming
- Real-time device telemetry

**Change Data Capture (CDC)**
- Database change streams
- Data synchronization across systems
- Audit trail and compliance logging

---

## Architecture Overview

```
┌─────────────┐       ┌─────────────────────────────────┐       ┌─────────────┐
│             │       │        StreamBus Cluster        │       │             │
│  Producers  │──────▶│   ┌──────────┐   ┌──────────┐   │──────▶│  Consumers  │
│             │       │   │ Broker 1 │   │ Broker 2 │   │       │             │
└─────────────┘       │   │ (Leader) │   │(Follower)│   │       └─────────────┘
                      │   └────┬─────┘   └────┬─────┘   │
                      │        │              │         │
                      │        └──────┬───────┘         │
                      │               ▼                 │
                      │       ┌───────────────┐         │
                      │       │ Raft Consensus│         │
                      │       │   (Metadata)  │         │
                      │       └───────────────┘         │
                      │                                 │
                      │    ┌─────────────────────┐      │
                      │    │  LSM Storage Engine │      │
                      │    │   + Write-Ahead Log │      │
                      │    └─────────────────────┘      │
                      └─────────────────────────────────┘
```

**Core Components:**
- **LSM-Tree Storage**: Write-optimized storage with efficient compaction
- **Raft Consensus**: Leader election and metadata coordination without ZooKeeper
- **Binary Protocol**: Efficient custom protocol for low-latency communication
- **Replication**: Leader-follower topology with in-sync replica tracking
- **Health System**: Comprehensive health checks for all components

---

## Quick Start

### Installation

```bash
# Clone the repository
git clone https://github.com/gstreamio/streambus.git
cd streambus

# Install dependencies
go mod download

# Run tests to verify
go test ./...

# Build the server
go build -o bin/streambus cmd/server/main.go
```

### Running StreamBus

```bash
# Start the server
./bin/streambus --port 9092

# Server starts with:
# - Binary protocol on port 9092
# - Health checks on port 8080
# - Metrics endpoint on port 8080/metrics
```

### Using the Go SDK

**For application development, use the official Go SDK:**

```bash
go get github.com/gstreamio/streambus-sdk
```

```go
package main

import (
    "log"
    "github.com/gstreamio/streambus-sdk/client"
)

func main() {
    // Connect to StreamBus
    config := client.DefaultConfig()
    config.Brokers = []string{"localhost:9092"}

    c, err := client.New(config)
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    // Create a topic
    if err := c.CreateTopic("orders", 3, 1); err != nil {
        log.Fatal(err)
    }

    // Produce messages
    producer := client.NewProducer(c)
    defer producer.Close()

    err = producer.Send("orders", []byte("order-123"), []byte(`{
        "orderId": "123",
        "amount": 99.99,
        "status": "pending"
    }`))
    if err != nil {
        log.Fatal(err)
    }

    // Consume messages
    consumer := client.NewConsumer(c, "orders", 0)
    defer consumer.Close()

    if err := consumer.Seek(0); err != nil {
        log.Fatal(err)
    }

    record, err := consumer.FetchOne()
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Received: %s\n", record.Value)
}
```

**SDK Repository:** [github.com/gstreamio/streambus-sdk](https://github.com/gstreamio/streambus-sdk)

See the [SDK README](https://github.com/gstreamio/streambus-sdk#readme) for complete documentation, examples, and API reference.

---

## Performance

**Test Environment**: Apple M4 Max, 16 cores, Go 1.23

### Key Metrics

| Metric | StreamBus | Typical Kafka | Advantage |
|--------|-----------|---------------|-----------|
| **Producer Latency** | 25 µs | 0.5-5 ms | **20-200x faster** |
| **Memory Footprint** | <100 MB | 2-8 GB | **95% less memory** |
| **Cold Start** | <1 second | 15-45 seconds | **15-45x faster** |
| **GC Pauses** | <1 ms | 10-200 ms | **10-200x less pause time** |
| **Binary Size** | 15 MB | N/A (JVM) | Single binary deployment |

### Detailed Benchmarks

**Client Operations** (End-to-End):
- Producer Send: 25.1 µs/op, ~40,000 msg/s
- Consumer Fetch: 21.8 µs/op, ~46,000 fetch/s

**Storage Layer** (LSM-Tree):
- Write: 1,095 ns/op (single), 5,494 ns/op (batch)
- Read: 140 ns/op from MemTable
- WAL Append: 919 ns/op (buffered), 8.5 ms/op (synced)

**Protocol Layer** (Serialization):
- Encode Produce: 38.6 ns/op
- Decode Produce: 110 ns/op
- Encode Fetch: 21.6 ns/op
- Decode Fetch: 70.5 ns/op

See [docs/BENCHMARKS.md](docs/BENCHMARKS.md) for comprehensive performance analysis.

---

## Production Features

### Observability

**Health Checks**
- `/health` - Comprehensive component health status
- `/health/live` - Kubernetes liveness probe
- `/health/ready` - Kubernetes readiness probe with dependency checks

**Metrics** (Prometheus Integration)
- 40+ broker metrics (uptime, connections, throughput, latency)
- Message metrics (produced, consumed, bytes, errors)
- Storage metrics (used, available, segments, compactions)
- Consumer group metrics (groups, members, lag)
- Security metrics (auth, authz, audit events)
- Native Prometheus exporter on `/metrics` endpoint
- Pre-built Grafana dashboards

**Distributed Tracing** (OpenTelemetry)
- End-to-end request tracing across brokers
- Support for OTLP, Jaeger, Zipkin exporters
- Configurable sampling strategies
- Trace context propagation
- Integration with Grafana and Jaeger

**Structured Logging**
- JSON-formatted logs with contextual fields
- Component-level log filtering
- Request ID tracing
- Error categorization and tracking

**Complete Observability Stack**
- Docker Compose setup with Prometheus, Grafana, Jaeger
- OpenTelemetry Collector for aggregation
- Pre-configured dashboards and alerts
- See `dashboards/` directory for turnkey setup

### Reliability

**Circuit Breakers**
- Automatic fail-fast for unhealthy dependencies
- Configurable failure thresholds
- Half-open testing for recovery
- State change callbacks

**Error Handling**
- Categorized errors (Retriable, Transient, Fatal, Invalid Input)
- Automatic retry with exponential backoff
- Context preservation through error chains
- Detailed error metadata

**Timeout Management**
- Centralized timeout configuration
- Context-based timeout enforcement
- Operation-specific timeout strategies
- Runtime configuration updates

### Deployment

**Container Native**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: streambus
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: streambus
        image: streambus:latest
        ports:
        - containerPort: 9092
          name: protocol
        - containerPort: 8080
          name: health
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 5
        resources:
          requests:
            memory: "128Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "1000m"
```

**Docker**
```bash
# Build
docker build -t streambus:latest .

# Run single broker
docker run -p 9092:9092 -p 8080:8080 streambus:latest

# Run local cluster with Docker Compose
docker-compose up -d

# This starts a 3-broker cluster with:
# - Broker 1: localhost:9092 (leader)
# - Broker 2: localhost:9093
# - Broker 3: localhost:9094
# - Prometheus: localhost:9090
# - Grafana: localhost:3000
```

---

## Documentation

### SDKs & Client Libraries
- **[Go SDK](https://github.com/gstreamio/streambus-sdk)** - Official Go client library (Apache 2.0)
  - Full-featured with producer, consumer, and consumer groups
  - TLS/mTLS and SASL authentication support
  - Transactional messaging with exactly-once semantics
- **Python SDK** - Coming soon (streambus-py-sdk)
- **C# SDK** - Planned (streambus-csharp-sdk)
- **Java SDK** - Planned (streambus-java-sdk)

### Getting Started
- [Quick Start Guide](docs/GETTING_STARTED.md) - Step-by-step tutorial
- [Examples](examples/README.md) - Complete producer and consumer examples
- [API Reference](docs/api-reference.md) - Comprehensive API documentation

### Operations
- [Deployment Guide](docs/operations.md) - Production deployment and operations
- [Configuration Reference](docs/configuration.md) - Complete configuration options
- [Production Hardening](docs/PRODUCTION_HARDENING_USAGE.md) - Reliability and observability features
- [Monitoring](docs/monitoring.md) - Metrics, logging, and health checks

### Architecture
- [Architecture Overview](docs/ARCHITECTURE.md) - System design and components
- [Storage Engine](docs/storage-engine.md) - LSM-tree implementation details
- [Consensus Protocol](docs/consensus.md) - Raft implementation
- [Replication](docs/replication.md) - Leader-follower replication

### Migration
- [Migrating from Kafka](docs/migration-from-kafka.md) - Migration guide and tools
- [Compatibility](docs/kafka-compatibility.md) - Kafka compatibility layer

### Development
- [Contributing Guide](CONTRIBUTING.md) - How to contribute
- [Development Setup](docs/development.md) - Local development environment
- [Testing Guide](docs/TESTING.md) - Test strategy and coverage
- [Benchmarking](docs/BENCHMARKS.md) - Performance benchmarks and methodology

---

## Project Status

**Beta** — active development, with production-ready core components.

### Core Features ✅

**Distributed System**
- LSM-tree storage engine with WAL
- Raft consensus for cluster coordination
- Multi-broker replication with automatic failover
- Binary protocol with producer/consumer clients

**Advanced Streaming**
- Schema registry (Avro/Protobuf/JSON Schema)
- Idempotent producers
- Consumer groups with broker-side coordination (join/sync/heartbeat,
  range/round-robin/sticky assignment, committed offsets persisted across
  restarts)
- Transactional producers with commit/abort markers written durably to every
  participating partition, and consumer offsets committed inside a transaction,
  giving exactly-once semantics within a cluster. Pinning
  `storage.message_format_version` to v2 for a rolling upgrade trades this
  away: v2 has nowhere to carry producer identity, so while it is selected a
  transactional produce is refused outright rather than written without the
  identity `read_committed` needs to hide it after an abort
- read_committed consumer isolation: a fetch never returns a record from a
  transaction still in flight, and an aborted transaction's records stay
  hidden after its marker resolves
- Cross-datacenter replication: links (create, start/stop/pause/resume,
  failover, metrics and health via the admin API) persisted across restarts,
  and a data plane that fetches, filters, transforms and produces records to
  the target cluster, resuming from checkpoints after a restart

**Enterprise Security**
- TLS encryption and SASL authentication
- ACL-based authorization
- Audit logging

**Production Reliability**
- Multi-tenancy with per-tenant quotas and topic ownership
- Circuit breakers and health checks
- Prometheus metrics and OpenTelemetry tracing
- Structured logging
- Grafana dashboards

---

## Why Choose StreamBus?

### vs Apache Kafka

| Feature | StreamBus | Apache Kafka |
|---------|-----------|--------------|
| **Latency** | Sub-millisecond (25µs) | Milliseconds (0.5-5ms) |
| **Memory** | <100 MB | 2-8 GB |
| **Startup** | <1 second | 15-45 seconds |
| **Deployment** | Single binary | JVM + ZooKeeper |
| **Tuning** | Minimal config | Extensive JVM tuning |
| **Dependencies** | None | ZooKeeper required |
| **Language** | Go (modern runtime) | Java/Scala (JVM) |
| **Use Case** | Low-latency, real-time | High-throughput, batch |

**Choose StreamBus if you need:**
- Lower latency and faster response times
- Smaller resource footprint
- Simpler operations and deployment
- Cloud-native, container-friendly architecture
- Modern Go-based development

**Choose Kafka if you need:**
- Massive batch throughput (millions msg/s)
- Extensive ecosystem of connectors
- Battle-tested production maturity
- Large community and support

### vs NATS

| Feature | StreamBus | NATS Streaming |
|---------|-----------|----------------|
| **Persistence** | Full LSM-tree with compaction | Memory-first with overflow |
| **Consensus** | Raft (built-in) | NATS clustering |
| **Storage** | Optimized for disk | Memory-optimized |
| **Replication** | Multi-broker with ISR | NATS JetStream |
| **Use Case** | Durable streaming | Lightweight messaging |

**Choose StreamBus for:** Durable event storage, replay capabilities, large message volumes

**Choose NATS for:** Lightweight pub-sub, minimal latency, ephemeral messaging

---

## Community & Support

### Get Help

- 📖 **Documentation**: [Complete docs](docs/)
- 💬 **Discussions**: [GitHub Discussions](https://github.com/gstreamio/streambus/discussions)
- 🐛 **Issues**: [GitHub Issues](https://github.com/gstreamio/streambus/issues)
- 🔒 **Security**: Report vulnerabilities to security@streambus.io

### Stay Updated

- ⭐ **Star** this repo to follow development
- 👀 **Watch** for release notifications
- 🐦 **Twitter**: [@streambus](https://twitter.com/streambus)
- 📝 **Blog**: [blog.streambus.io](https://blog.streambus.io)

### Contributing

We welcome contributions! StreamBus is open source and community-driven.

```bash
# Fork and clone
git clone https://github.com/YOUR_USERNAME/streambus.git

# Create a feature branch
git checkout -b feature/amazing-feature

# Make changes and test
go test ./...

# Submit a pull request
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.

---

## License

StreamBus is released under the **Apache 2.0 License**. See [LICENSE](LICENSE) for details.

```
Copyright 2025 StreamBus Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
```

---

## Acknowledgments

StreamBus is inspired by the groundbreaking work of:
- **Apache Kafka** - For establishing distributed streaming patterns
- **Raft Consensus** - For elegant distributed consensus
- **LSM-Tree research** - For efficient write-optimized storage
- **Go community** - For an exceptional runtime and ecosystem

Built with ❤️ in Go by developers who believe streaming platforms should be fast, simple, and reliable.

---

<div align="center">

**[Get Started](docs/GETTING_STARTED.md)** • **[View Examples](examples/)** • **[Read Docs](docs/)** • **[Join Community](https://github.com/gstreamio/streambus/discussions)**

⭐ **Star us on GitHub** — it helps!

</div>
