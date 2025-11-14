---
layout: default
title: Home
nav_order: 1
description: "StreamBus - A next-generation distributed streaming platform"
permalink: /
---

# StreamBus Documentation
{: .fs-9 }

A next-generation distributed streaming platform built for performance, reliability, and operational simplicity.
{: .fs-6 .fw-300 }

[Get Started](GETTING_STARTED.md){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[View on GitHub](https://github.com/gstreamio/streambus){: .btn .fs-5 .mb-4 .mb-md-0 }

---

## Why StreamBus?

StreamBus is a high-performance, distributed streaming platform designed for modern cloud-native applications.

- **🚀 Ultra-Low Latency** - Sub-millisecond operations
- **💰 Resource Efficient** - <100MB footprint
- **⚡ Fast Startup** - Cold start in <1 second
- **🛠️ Zero Complexity** - Single binary, no JVM tuning
- **🔒 Enterprise-Grade** - Built-in security and reliability

## Quick Navigation

### For Users
- [Getting Started](GETTING_STARTED.md) - Get up and running quickly
- [Go SDK](https://github.com/gstreamio/streambus-sdk) - Official client library
- [Examples](../examples/README.md) - Code examples
- [API Reference](api-reference.md) - Complete API docs

### For Operators
- [Deployment Guide](DEPLOYMENT.md) - Deploy in production
- [Operations Manual](operations.md) - Day-to-day operations
- [Monitoring](MONITORING.md) - Metrics and observability
- [Operational Runbooks](OPERATIONAL_RUNBOOKS.md) - Common procedures

### For Developers
- [Contributing Guide](../CONTRIBUTING.md) - How to contribute
- [Architecture](ARCHITECTURE.md) - System design
- [Testing Guide](TESTING.md) - Test strategy
- [Roadmap](ROADMAP.md) - Future plans

## Key Features

### Built for Performance
- Sub-millisecond latency (~25µs producer latency)
- Memory efficient (<100MB footprint)
- Fast recovery (cold start <1 second)
- Optimized LSM-tree storage engine

### Production Hardened
- Circuit breaker pattern
- Health monitoring (Kubernetes-ready)
- Structured logging
- Prometheus metrics
- OpenTelemetry tracing
- TLS/SASL security

### Operationally Simple
- Single binary deployment
- Minimal configuration
- Container native
- No ZooKeeper dependency

### Developer Friendly
- Idiomatic Go client
- Consumer groups with rebalancing
- Transactions and exactly-once semantics
- Schema registry (Avro, Protobuf, JSON Schema)

## Documentation Sections

### 📚 [Getting Started](GETTING_STARTED.md)
Quick start guide to get StreamBus running

### 🔧 [Operations](operations.md)
Deploy and operate StreamBus in production

### 📊 [Performance](BENCHMARKS.md)
Benchmarks and performance tuning

### 🔄 [Migration](migration-from-kafka.md)
Migrate from Apache Kafka

### 🏗️ [Architecture](ARCHITECTURE.md)
System design and implementation details

### 🔒 [Security](SECURITY.md)
Authentication, authorization, and encryption

### 💻 [Development](TESTING.md)
Contributing and development guide

## Project Status

**Current Phase**: Beta - Active Development

StreamBus has completed core distributed system features and enterprise capabilities:

✅ **Complete**
- LSM-tree storage with WAL
- Raft consensus and cluster coordination
- Multi-broker replication
- Consumer groups and transactions
- TLS/SASL authentication and ACLs
- Prometheus metrics and tracing

🚧 **In Development**
- Cross-datacenter replication
- Kubernetes operator
- Extended test coverage

See the [Roadmap](ROADMAP.md) for detailed plans.

## Getting Help

- 💬 [GitHub Discussions](https://github.com/gstreamio/streambus/discussions) - Ask questions
- 🐛 [GitHub Issues](https://github.com/gstreamio/streambus/issues) - Report bugs
- 📖 [FAQ](FAQ.md) - Frequently asked questions
- 🔒 security@streambus.io - Security issues

## Community

- ⭐ [Star on GitHub](https://github.com/gstreamio/streambus)
- 🐦 [@streambus](https://twitter.com/streambus) on Twitter
- 📝 [Blog](https://blog.streambus.io)

---

**Ready to get started?** Check out the [Quick Start Guide](GETTING_STARTED.md)!
