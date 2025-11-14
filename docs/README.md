# StreamBus Documentation

Welcome to the StreamBus documentation. This guide will help you understand, deploy, and operate StreamBus.

## 📚 Documentation Structure

### Getting Started

Start here if you're new to StreamBus:

- **[Quick Start Guide](GETTING_STARTED.md)** - Get up and running in 5 minutes
- **[Architecture Overview](ARCHITECTURE.md)** - Understand how StreamBus works
- **[API Reference](api-reference.md)** - Complete client API documentation

### Operations

Production deployment and operations:

- **[Operations Guide](operations.md)** - Deploy and operate StreamBus in production
- **[Production Hardening](PRODUCTION_HARDENING_USAGE.md)** - Reliability and observability features
- **[Configuration Reference](configuration.md)** - All configuration options
- **[Monitoring Guide](monitoring.md)** - Metrics, logging, and alerting
- **[Troubleshooting Guide](troubleshooting.md)** - Common issues and solutions

### Performance

Benchmarks and optimization:

- **[Benchmarks](BENCHMARKS.md)** - Comprehensive performance benchmarks
- **[Performance Tuning](performance-tuning.md)** - Optimize for your workload
- **[Capacity Planning](capacity-planning.md)** - Size your cluster

### Migration

Moving to StreamBus from other platforms:

- **[Migrating from Kafka](migration-from-kafka.md)** - Step-by-step migration guide
- **[Compatibility Layer](kafka-compatibility.md)** - Kafka compatibility features
- **[Migration Tools](migration-tools.md)** - Data migration utilities

### Architecture & Design

Deep technical documentation:

- **[Architecture Overview](ARCHITECTURE.md)** - High-level system design
- **[Storage Engine](storage-engine.md)** - LSM-tree implementation
- **[Consensus Protocol](consensus.md)** - Raft consensus details
- **[Replication](replication.md)** - Leader-follower replication
- **[Network Protocol](protocol.md)** - Binary protocol specification

### Development

Contributing to StreamBus:

- **[Contributing Guide](../CONTRIBUTING.md)** - How to contribute
- **[Development Setup](development.md)** - Local development environment
- **[Testing Guide](TESTING.md)** - Test strategy and running tests
- **[Code Style Guide](code-style.md)** - Go coding conventions
- **[Release Process](release-process.md)** - How we release versions

### Project Information

- **[Project Plan](PROJECT_PLAN.md)** - Detailed project roadmap
- **[Roadmap](ROADMAP.md)** - Feature timeline and priorities
- **[Changelog](CHANGELOG.md)** - Version history and changes
- **[FAQ](FAQ.md)** - Frequently asked questions

## 🚀 Quick Links

### For Users
- [Install StreamBus](GETTING_STARTED.md#installation)
- [Producer Example](../examples/producer/main.go)
- [Consumer Example](../examples/consumer/main.go)
- [Client Configuration](api-reference.md#configuration)

### For Operators
- [Deploy on Kubernetes](operations.md#kubernetes-deployment)
- [Configure Health Checks](PRODUCTION_HARDENING_USAGE.md#health-checks)
- [Setup Monitoring](monitoring.md#prometheus-integration)
- [Backup and Recovery](operations.md#backup-and-recovery)

### For Developers
- [Build from Source](development.md#building)
- [Run Tests](TESTING.md#running-tests)
- [Submit a PR](../CONTRIBUTING.md#pull-requests)
- [Architecture Decisions](architecture-decisions/)

## 📊 Current Status

**Version**: Development (Phase 2 Complete)

**Test Coverage**: 252 tests passing (100%)

**Components**:
- ✅ Storage Engine (LSM-tree + WAL)
- ✅ Network Protocol (Binary protocol)
- ✅ Client Library (Producer + Consumer)
- ✅ Raft Consensus
- ✅ Metadata Replication
- ✅ Cluster Coordination
- ✅ Production Hardening (Circuit breakers, health checks, metrics, logging)
- 🔄 Consumer Groups (In Progress)
- 🔄 Transactions (Planned)
- 🔄 Security (Planned)

See [ROADMAP.md](ROADMAP.md) for detailed timeline.

## 🆘 Getting Help

- **Documentation Issues**: [Create an issue](https://github.com/shawntherrien/streambus/issues/new?labels=documentation)
- **Questions**: [GitHub Discussions](https://github.com/shawntherrien/streambus/discussions)
- **Bug Reports**: [GitHub Issues](https://github.com/shawntherrien/streambus/issues)
- **Security Issues**: security@streambus.io

## 📝 Documentation Conventions

Throughout the documentation, you'll see these markers:

- ✅ **Complete** - Feature is fully implemented and tested
- 🔄 **In Progress** - Feature is currently being developed
- 📅 **Planned** - Feature is on the roadmap
- ⚠️ **Experimental** - Feature is experimental and may change
- 🚫 **Deprecated** - Feature is deprecated and will be removed

## 🗂️ Archive

Historical milestone reports and session summaries are available in [archive/](archive/).

---

**Last Updated**: January 2025

**Questions?** Check the [FAQ](FAQ.md) or ask in [Discussions](https://github.com/shawntherrien/streambus/discussions).
