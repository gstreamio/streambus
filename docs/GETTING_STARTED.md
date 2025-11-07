# Getting Started with StreamBus Development

Welcome to StreamBus! This guide will help you get started with the project.

## Project Overview

StreamBus is a next-generation distributed streaming platform written in Go, designed to outperform Apache Kafka in every meaningful metric:

- **Ultra-Low Latency**: Target P99 < 5ms (vs Kafka's 15-25ms)
- **High Throughput**: Target > 3M messages/second
- **Zero GC Pauses**: Eliminate JVM stop-the-world pauses
- **Operational Simplicity**: Single binary, no JVM tuning

## Project Status

🚧 **Phase 1: Foundation (Months 1-3)** - Currently in Progress

We are currently in the initial development phase, focusing on:
- Core storage engine implementation
- Network layer development
- Basic broker functionality

See [PROJECT_PLAN.md](PROJECT_PLAN.md) for the complete roadmap.

## Quick Start

### Prerequisites

- Go 1.23 or later
- Make
- Git

### Clone and Build

```bash
# Clone the repository
git clone https://github.com/shawntherrien/streambus.git
cd streambus

# Install dependencies
make deps

# Build the project
make build

# Run tests (currently minimal)
make test
```

### Project Structure

```
streambus/
├── cmd/                        # Command-line applications
│   ├── broker/                # Broker server
│   └── cli/                   # CLI tool
├── pkg/                       # Public library packages
│   ├── storage/               # Storage engine (LSM, WAL)
│   ├── network/               # Network layer
│   ├── consensus/             # Raft consensus
│   ├── replication/           # Replication engine
│   ├── broker/                # Broker logic
│   ├── consumer/              # Consumer implementation
│   ├── producer/              # Producer implementation
│   ├── protocol/              # Wire protocol
│   └── config/                # Configuration
├── internal/                  # Private application code
│   └── testutil/              # Test utilities
├── docs/                      # Documentation
├── examples/                  # Example applications
├── deploy/                    # Deployment configs
│   ├── kubernetes/            # K8s manifests
│   └── docker/                # Docker configs
├── scripts/                   # Build and utility scripts
├── benchmarks/                # Performance benchmarks
├── config/                    # Configuration files
│   └── broker.yaml           # Broker config example
├── PROJECT_PLAN.md           # Comprehensive project plan
├── ARCHITECTURE.md           # Architecture documentation
├── README.md                 # Project README
├── CONTRIBUTING.md           # Contributing guidelines
├── Makefile                  # Build automation
└── go.mod                    # Go module definition
```

## Development Workflow

### 1. Running the Broker (Placeholder)

The broker is currently a work in progress. You can run the placeholder:

```bash
# Build and run
make build
./bin/streambus-broker --config config/broker.yaml

# Or use make
make run-broker
```

**Note**: The broker doesn't do much yet - it's a skeleton waiting for implementation!

### 2. Using the CLI (Placeholder)

```bash
# Build the CLI
make build

# List topics (placeholder)
./bin/streambus-cli topic list

# Create a topic (placeholder)
./bin/streambus-cli topic create my-topic --partitions 10 --replication-factor 3
```

### 3. Development Commands

```bash
# Run tests
make test

# Run tests with coverage
make test-coverage

# Run linters
make lint

# Format code
make fmt

# Run benchmarks
make benchmark

# Install dev tools
make tools
```

## Next Steps for Contributors

### Phase 1 Focus Areas

We need help with:

1. **Storage Engine** (Priority: HIGH)
   - Implement LSM-tree structure
   - Write-Ahead Log (WAL)
   - Offset indexing
   - Compaction strategies

2. **Network Layer** (Priority: HIGH)
   - TCP server implementation
   - Custom binary protocol
   - Connection pooling
   - Request/response handling

3. **Testing Infrastructure** (Priority: MEDIUM)
   - Benchmark framework
   - Integration test utilities
   - Mock implementations

### How to Contribute

1. Check [PROJECT_PLAN.md](PROJECT_PLAN.md) for current milestones
2. Look for issues tagged `good-first-issue` or `help-wanted`
3. Read [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines
4. Join discussions in GitHub Issues
5. Submit PRs for review

### Learning Resources

#### Understanding Kafka
- [Apache Kafka Documentation](https://kafka.apache.org/documentation/)
- [Kafka Design Principles](https://kafka.apache.org/documentation/#design)
- Our analysis in [PROJECT_PLAN.md](PROJECT_PLAN.md#1-kafka-analysis)

#### Go Best Practices
- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Concurrency Patterns](https://go.dev/blog/pipelines)

#### Distributed Systems
- [Raft Consensus Algorithm](https://raft.github.io/)
- [LSM Trees Explained](https://www.igvita.com/2012/02/06/sstable-and-log-structured-storage-leveldb/)
- Our [ARCHITECTURE.md](ARCHITECTURE.md)

## Architecture Overview

StreamBus uses a modern, high-performance architecture:

```
┌─────────────────────────────────────────────┐
│         Producers & Consumers               │
└───────────────┬─────────────────────────────┘
                │
┌───────────────▼─────────────────────────────┐
│         Load Balancer (Built-in)            │
└───────────────┬─────────────────────────────┘
                │
┌───────────────▼─────────────────────────────┐
│         Broker Cluster                      │
│   ┌─────────┐  ┌─────────┐  ┌─────────┐   │
│   │Broker 1 │  │Broker 2 │  │Broker 3 │   │
│   └─────────┘  └─────────┘  └─────────┘   │
│              Raft Consensus                 │
└─────────────────────────────────────────────┘
```

### Key Components

1. **Storage Engine**: LSM-tree based, optimized for sequential writes
2. **Replication**: Leader-follower with ISR tracking
3. **Consensus**: Raft for metadata (no ZooKeeper)
4. **Network**: Custom binary protocol + gRPC
5. **Clients**: Native Go SDK with multi-language support

See [ARCHITECTURE.md](ARCHITECTURE.md) for details.

## Performance Goals

Our targets vs Kafka:

| Metric | StreamBus Target | Kafka Baseline | Improvement |
|--------|------------------|----------------|-------------|
| P99 Latency | < 5ms | 15-25ms | 5x faster |
| Throughput | > 3M msg/s | 2.1M msg/s | 43% higher |
| Memory | < 4GB | 8-32GB | 75% less |
| Max GC Pause | < 1ms | 50-200ms | 200x faster |

## Milestones

### ✅ Completed
- [x] Project planning and architecture design
- [x] Repository setup
- [x] Initial documentation

### 🔄 In Progress (Month 1)
- [ ] Storage engine implementation
- [ ] Benchmark framework setup
- [ ] Unit test infrastructure

### 📅 Coming Soon (Months 2-3)
- [ ] Network layer
- [ ] Basic broker functionality
- [ ] Integration tests

### 🔮 Future (Months 4+)
- [ ] Distributed cluster support
- [ ] Replication engine
- [ ] Consumer groups
- [ ] Security features
- [ ] Kubernetes operator

## Communication

- **GitHub Issues**: Bug reports, feature requests
- **GitHub Discussions**: Questions, ideas, general discussion
- **Pull Requests**: Code contributions

## Resources

- [PROJECT_PLAN.md](PROJECT_PLAN.md) - Comprehensive 15-month plan
- [ARCHITECTURE.md](ARCHITECTURE.md) - Technical architecture
- [CONTRIBUTING.md](CONTRIBUTING.md) - Contribution guidelines
- [README.md](README.md) - Project overview

## Support

Need help?

1. Check existing documentation
2. Search GitHub Issues
3. Ask in GitHub Discussions
4. Tag maintainers in issues

## License

StreamBus is licensed under the Apache 2.0 License. See [LICENSE](LICENSE).

---

**Ready to contribute?** Start by reading the [PROJECT_PLAN.md](PROJECT_PLAN.md) and picking a task from Milestone 1.1!

**Have questions?** Open a discussion on GitHub!

**Let's build the future of streaming together!** 🚀
