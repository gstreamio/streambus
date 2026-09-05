# StreamBus Documentation

Welcome to the StreamBus documentation! This guide will help you understand, deploy, and operate StreamBus.

## 📚 Documentation Structure

### Getting Started

Start here if you're new to StreamBus:

- **[Quick Start Guide](GETTING_STARTED.md)** - Get up and running in minutes
- **[Architecture Overview](ARCHITECTURE.md)** - Understand how StreamBus works
- **[API Reference](api-reference.md)** - Complete client API documentation
- **[Examples](../examples/README.md)** - Complete code examples

### Client SDKs

- **[Go SDK](https://github.com/gstreamio/streambus-sdk)** - Official Go client library
- **[Integration Guide](INTEGRATION_GUIDE.md)** - Integrate StreamBus into your applications

### Operations

Production deployment and operations:

- **[Deployment Guide](DEPLOYMENT.md)** - Deploy StreamBus in production
- **[Operations Guide](operations.md)** - Day-to-day operations
- **[Operational Runbooks](OPERATIONAL_RUNBOOKS.md)** - Common procedures
- **[Production Hardening](PRODUCTION_HARDENING_USAGE.md)** - Reliability features
- **[Monitoring Guide](MONITORING.md)** - Metrics, logging, and observability
- **[Logging](LOGGING.md)** - Structured logging implementation

### Performance

Benchmarks and optimization:

- **[Benchmarks](BENCHMARKS.md)** - Comprehensive performance benchmarks
- **[Performance Guide](PERFORMANCE.md)** - Performance characteristics
- **[Performance Tuning](PERFORMANCE_TUNING.md)** - Optimize for your workload
- **[Performance Optimization](PERFORMANCE_OPTIMIZATION_SUMMARY.md)** - Optimization summary

### Migration

Moving to StreamBus from other platforms:

- **[Migrating from Kafka](migration-from-kafka.md)** - Step-by-step migration guide
- **[Migration Overview](MIGRATION.md)** - General migration strategies

### Architecture & Design

Deep technical documentation:

- **[Architecture Overview](ARCHITECTURE.md)** - High-level system design
- **[Replication](REPLICATION.md)** - Leader-follower replication
- **[Raft Connection Improvements](RAFT_CONNECTION_IMPROVEMENTS.md)** - Consensus optimizations

### Security

- **[Security Guide](SECURITY.md)** - Authentication, authorization, and encryption

### Development

Contributing to StreamBus:

- **[Contributing Guide](../CONTRIBUTING.md)** - How to contribute
- **[Testing Guide](TESTING.md)** - Test strategy and running tests
- **[CLI Guide](CLI.md)** - Command-line tools

### Project Information

- **[Changelog](CHANGELOG.md)** - Version history and changes
- **[FAQ](FAQ.md)** - Frequently asked questions
- **[Production Readiness](PRODUCTION_READINESS.md)** - Production readiness checklist
- **[Production Guide](PRODUCTION.md)** - Production best practices
- **[Quick Start](QUICK_START.md)** - Alternative quick start

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

## 📊 Project Status

**Current Phase**: Beta - Active Development

**Core Components**:
- ✅ LSM-tree storage engine with WAL
- ✅ Raft consensus and cluster coordination
- ✅ Multi-broker replication with automatic failover
- ✅ Binary protocol with producer/consumer clients
- ✅ Consumer groups with automatic rebalancing
- ✅ Transactions and exactly-once semantics
- ✅ TLS/SASL authentication and ACL authorization
- ✅ Prometheus metrics and OpenTelemetry tracing
- ✅ Circuit breakers, health checks, and structured logging

**In Development**:
- Cross-datacenter replication
- Kubernetes operator

## 🆘 Getting Help

- **Questions**: [GitHub Discussions](https://github.com/gstreamio/streambus/discussions)
- **Bug Reports**: [GitHub Issues](https://github.com/gstreamio/streambus/issues)
- **Security Issues**: security@streambus.io
- **Documentation Issues**: [Create an issue](https://github.com/gstreamio/streambus/issues/new?labels=documentation)

## 📝 Documentation Conventions

Throughout the documentation, you'll see these markers:

- ✅ **Complete** - Feature is fully implemented and tested
- 🔄 **In Progress** - Feature is currently being developed
- 📅 **Planned** - Feature is on the roadmap
- ⚠️ **Experimental** - Feature is experimental and may change
- 🚫 **Deprecated** - Feature is deprecated and will be removed

---

**Last Updated**: January 2025

**Questions?** Check the [FAQ](FAQ.md) or ask in [Discussions](https://github.com/gstreamio/streambus/discussions).
