# Frequently Asked Questions

Common questions about StreamBus answered.

## General Questions

### What is StreamBus?

StreamBus is a high-performance, distributed streaming platform written in Go. It's designed to be a modern alternative to Apache Kafka with lower latency, smaller memory footprint, and simpler operations.

### Why create another streaming platform?

While Kafka is excellent, it has inherent limitations due to its JVM foundation:
- Stop-the-world GC pauses impact latency
- Large memory footprints (2-8GB typical)
- Complex JVM tuning required
- Slower cold start times (15-45 seconds)

StreamBus eliminates these issues while maintaining Kafka's proven design principles.

### How does StreamBus compare to Kafka?

**StreamBus advantages:**
- 10-100x lower latency (25µs vs 0.5-5ms)
- 95% less memory (<100MB vs 2-8GB)
- 15-45x faster startup (<1s vs 15-45s)
- Single binary deployment (no JVM, no ZooKeeper)
- Simpler operations (minimal configuration)

**Kafka advantages:**
- Higher batch throughput (millions of msgs/sec)
- More mature and battle-tested
- Larger ecosystem of connectors and tools
- Extensive community and enterprise support

### Can I use StreamBus in production today?

**Not yet.** StreamBus is in active development (Phase 2 complete). Core features are functional with 100% test coverage, but the following are not yet implemented:
- Security (TLS, authentication, authorization)
- Consumer groups
- Transactions
- Cross-datacenter replication
- Production-scale testing

We estimate production readiness in **Q3-Q4 2025**.

### Is StreamBus compatible with Kafka?

**No wire-protocol compatibility**. StreamBus uses its own binary protocol optimized for performance. However, we're building:
- Migration tools for data transfer
- Configuration converters
- Client library compatibility layers
- Connector compatibility (Kafka Connect)

See [Migrating from Kafka](migration-from-kafka.md) for details.

### What license is StreamBus released under?

StreamBus is released under the **Apache 2.0 License**, the same license as Apache Kafka. This allows:
- Free commercial use
- Modification and distribution
- Patent protection
- No warranty

See [LICENSE](../LICENSE) for full text.

---

## Technical Questions

### What programming languages are supported?

**Currently:**
- **Go** - Native client library (fully featured)

**Planned** (Q4 2025 - Q1 2026):
- Java
- Python
- Node.js
- Rust
- C#

All clients will use the same binary protocol.

### Does StreamBus support consumer groups?

**Not yet.** Consumer groups are planned for Phase 3 (Q1 2025). The implementation will include:
- Multiple consumers coordinating on the same topic
- Automatic partition rebalancing
- Offset management
- Group membership protocol

### Does StreamBus support transactions?

**Not yet.** Transactions are planned for Phase 3 (Q1-Q2 2025). The implementation will include:
- Transactional producer API
- Atomic writes across partitions
- Exactly-once semantics
- Read committed isolation

### How does replication work?

StreamBus uses a leader-follower replication model:
- Each partition has one leader and N-1 followers
- Producers write to the leader
- Leader replicates to in-sync replicas (ISR)
- Consumers can read from leader or followers
- Raft consensus manages metadata and leader election

See [Architecture](ARCHITECTURE.md#replication) for details.

### What consensus algorithm does StreamBus use?

StreamBus uses **Raft consensus** for:
- Metadata management
- Leader election
- Cluster coordination

**Not** for data replication (uses leader-follower like Kafka).

Benefits of Raft:
- No ZooKeeper dependency
- Simpler to understand and operate
- Built-in to every broker
- Fast leader election (<1 second)

### How does storage work?

StreamBus uses an **LSM-tree storage engine** with:
- Write-Ahead Log (WAL) for durability
- MemTable for in-memory writes
- SSTables for on-disk storage
- Background compaction
- Efficient indexed lookups

This provides:
- Fast writes (append-only)
- Efficient compaction
- Good read performance
- Low memory overhead

See [Storage Engine](storage-engine.md) for deep dive.

### What about exactly-once semantics?

Exactly-once semantics are planned for Phase 3 (Q1-Q2 2025) via:
- Idempotent producers
- Transactional writes
- Read committed isolation
- Deduplication

### Can I run StreamBus on Kubernetes?

**Yes!** StreamBus is designed for Kubernetes:
- Single binary deployment
- Native health checks (`/health/live`, `/health/ready`)
- Prometheus metrics export
- Minimal resource requirements
- Fast startup and shutdown

A Kubernetes operator is planned for Q4 2025.

See [Operations Guide](operations.md#kubernetes-deployment) for examples.

---

## Performance Questions

### What performance can I expect?

**Typical performance** (M4 Max, Go 1.23):

| Metric | Performance |
|--------|-------------|
| Producer latency | 25 µs |
| Consumer latency | 22 µs |
| Throughput | ~40,000 msg/s (single-threaded) |
| Memory footprint | <100 MB |
| Cold start | <1 second |
| GC pauses | <1 ms |

See [Benchmarks](BENCHMARKS.md) for detailed analysis.

### How does StreamBus achieve low latency?

Several design choices contribute:
- **Go runtime**: Efficient concurrency, small GC pauses
- **Custom protocol**: Optimized binary encoding
- **LSM storage**: Fast append-only writes
- **Single binary**: No JVM overhead
- **Memory efficiency**: Minimal allocations

### Will StreamBus scale to Kafka's throughput?

**Different optimization goals:**
- **Kafka**: Optimized for batch throughput (millions msg/s)
- **StreamBus**: Optimized for single-message latency (µs range)

For high-throughput batch workloads, Kafka may be more appropriate. For low-latency, real-time processing, StreamBus excels.

### What are the hardware requirements?

**Minimum** (Development):
- 2 CPU cores
- 2 GB RAM
- 10 GB disk

**Recommended** (Production):
- 4-8 CPU cores
- 8-16 GB RAM
- 100+ GB SSD
- 1 Gbps+ network

**Optimal** (High Performance):
- 16+ CPU cores
- 32+ GB RAM
- NVMe SSD
- 10 Gbps+ network

See [Capacity Planning](capacity-planning.md) for sizing guidance.

---

## Operational Questions

### How do I monitor StreamBus?

StreamBus provides comprehensive observability:

**Metrics** (Prometheus):
- Request rates and latencies
- Storage statistics
- Replication lag
- Circuit breaker states
- Resource utilization

**Logs** (JSON):
- Structured logging with contextual fields
- Configurable log levels
- Component and operation tracking
- Error categorization

**Health Checks**:
- HTTP endpoints for liveness and readiness
- Component-specific health status
- Kubernetes-compatible probes

See [Monitoring Guide](monitoring.md) for setup.

### How do I backup StreamBus data?

**Current** (Phase 2):
- File-based backups of data directory
- WAL and SSTable files
- Snapshot support

**Planned** (Q3 2025):
- Automated backup tools
- Cloud storage integration (S3, GCS)
- Point-in-time recovery
- Cross-datacenter replication

See [Operations Guide](operations.md#backup-and-recovery) for procedures.

### How do I upgrade StreamBus?

**Current** (Development):
- Stop all brokers
- Replace binary
- Restart brokers
- Run data migration if needed

**Planned** (Q4 2025):
- Rolling upgrades with zero downtime
- Kubernetes operator for automated upgrades
- Backward compatibility guarantees
- Version compatibility matrix

### What happens during a broker failure?

StreamBus handles broker failures gracefully:

1. **Detection**: Health checks detect failure (<2 seconds)
2. **Leader Election**: Raft elects new leader (<1 second)
3. **Rebalancing**: Partitions reassigned to healthy brokers
4. **Replication**: Data replicated to new followers
5. **Client Retry**: Clients automatically retry with new leader

Total failover time: <3 seconds typically

See [Architecture](ARCHITECTURE.md#fault-tolerance) for details.

---

## Development Questions

### How can I contribute?

We welcome contributions! Here's how:

1. **Start small**: Fix bugs, improve docs, add tests
2. **Discuss first**: Open an issue or discussion before large changes
3. **Follow guidelines**: See [CONTRIBUTING.md](../CONTRIBUTING.md)
4. **Write tests**: All code must have tests
5. **Get reviewed**: Code review is required for all PRs

See [Contributing Guide](../CONTRIBUTING.md) for details.

### What's the development setup?

**Prerequisites**:
- Go 1.23 or later
- Git
- Make (optional)

**Setup**:
```bash
git clone https://github.com/shawntherrien/streambus.git
cd streambus
go mod download
go test ./...
```

See [Development Setup](development.md) for complete guide.

### Where should I start contributing?

**Good first issues**:
- Documentation improvements
- Test coverage gaps
- Bug fixes
- Performance optimizations

**Current needs** (Phase 3):
- Consumer group implementation
- Transaction protocol design
- Schema registry architecture

Check [Issues](https://github.com/shawntherrien/streambus/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22) with "good first issue" label.

### How often are releases made?

**Current** (Development Phase):
- No stable releases yet
- Main branch is continuously updated
- Weekly progress updates

**Future** (Post v1.0):
- Minor releases: Monthly
- Patch releases: As needed
- Major releases: Quarterly

---

## Migration Questions

### How do I migrate from Kafka?

We're building comprehensive migration tools:

**Current**:
- Manual migration scripts
- Configuration mapping guide
- Client code examples

**Planned** (Q2 2025):
- Automated data migration tool
- Dual-write compatibility layer
- Topic and schema migration
- Offset mapping utilities

See [Migration Guide](migration-from-kafka.md) for step-by-step instructions.

### Will I lose data during migration?

**No.** The migration process is designed to be safe:
- Dual-write to both platforms during migration
- Validation tools to verify data consistency
- Rollback procedures if issues arise
- No downtime required

### Can I run StreamBus and Kafka side-by-side?

**Yes!** This is the recommended migration approach:
1. Deploy StreamBus alongside Kafka
2. Migrate producers one-by-one (dual-write)
3. Migrate consumers one-by-one
4. Validate data consistency
5. Decommission Kafka when complete

### What about Kafka connectors?

**Planned** (Q3-Q4 2025):
- Kafka Connect compatibility layer
- Native StreamBus connectors
- Migration tool for existing connectors

Most connectors will work with minimal changes.

---

## Community Questions

### Where can I get help?

**Documentation**: [docs/](README.md)

**Community Support**:
- [GitHub Discussions](https://github.com/shawntherrien/streambus/discussions)
- [GitHub Issues](https://github.com/shawntherrien/streambus/issues)
- [Slack Community](https://streambus.slack.com)

**Commercial Support** (Coming):
- Enterprise support contracts (Q4 2025)
- Training and consulting
- Custom development

### How can I stay updated?

- **Star** the [GitHub repository](https://github.com/shawntherrien/streambus)
- **Watch** for release notifications
- **Follow** [@streambus](https://twitter.com/streambus) on Twitter
- **Subscribe** to the [blog](https://blog.streambus.io)
- **Join** the [Slack community](https://streambus.slack.com)

### Is there a Slack or Discord?

**Slack**: [streambus.slack.com](https://streambus.slack.com) (Coming Soon)

**GitHub Discussions**: [Discussions](https://github.com/shawntherrien/streambus/discussions) (Active Now)

We prefer GitHub Discussions during development to keep everything in one place.

### Who is behind StreamBus?

StreamBus is an open-source project built by developers who believe streaming platforms should be:
- Fast
- Simple
- Reliable
- Cloud-native

We're inspired by Apache Kafka and built upon decades of distributed systems research.

---

## Still Have Questions?

- **Documentation**: Browse the [full docs](README.md)
- **Search Issues**: Check [existing issues](https://github.com/shawntherrien/streambus/issues)
- **Ask the Community**: [GitHub Discussions](https://github.com/shawntherrien/streambus/discussions)
- **Report Bugs**: [GitHub Issues](https://github.com/shawntherrien/streambus/issues/new)

---

**Last Updated**: January 2025

**Didn't find your answer?** [Ask a question](https://github.com/shawntherrien/streambus/discussions/new?category=q-a)
