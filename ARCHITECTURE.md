# StreamBus Architecture

## Table of Contents
1. [Overview](#overview)
2. [Design Principles](#design-principles)
3. [System Architecture](#system-architecture)
4. [Component Details](#component-details)
5. [Data Flow](#data-flow)
6. [Performance Optimizations](#performance-optimizations)
7. [Reliability & Fault Tolerance](#reliability--fault-tolerance)

---

## Overview

StreamBus is designed as a distributed, log-based streaming platform optimized for high throughput and low latency. The architecture draws inspiration from Apache Kafka while introducing modern improvements leveraging Go's concurrency model and performance characteristics.

### Key Architectural Goals
- **Performance**: P99 latency < 5ms, throughput > 3M msg/s
- **Reliability**: 99.99% uptime, zero message loss
- **Simplicity**: Single binary, no external dependencies for basic operation
- **Scalability**: Horizontal scaling from 1 to 1000+ brokers
- **Observability**: Built-in metrics, tracing, and logging

---

## Design Principles

### 1. Zero-Copy I/O
Minimize data copying between user space and kernel space using:
- `sendfile()` syscall for disk-to-network transfers
- Memory-mapped files for hot data
- Direct buffer access where possible

### 2. Append-Only Log Structure
All data is written sequentially to an append-only log:
- Enables high write throughput (sequential writes)
- Simplifies replication (just replay the log)
- Natural fit for event streaming use cases

### 3. Immutable Messages
Once written, messages are never modified:
- Simplifies concurrent access (no locking for reads)
- Enables caching strategies
- Facilitates replay and debugging

### 4. Partitioning for Parallelism
Topics are divided into partitions:
- Enables parallel writes and reads
- Scales linearly with partition count
- Maintains ordering within partitions

### 5. Replication for Durability
Data is replicated across multiple brokers:
- Configurable replication factor (typically 3)
- Leader-follower topology
- Automatic failover on leader failure

### 6. Consensus for Coordination
Raft consensus for cluster metadata:
- No external coordinator (unlike Kafka's ZooKeeper)
- Strong consistency guarantees
- Built-in leader election

---

## System Architecture

### High-Level Architecture

```
┌───────────────────────────────────────────────────────────────┐
│                      Client Layer                              │
├───────────────────────────────────────────────────────────────┤
│  Producer Clients          Consumer Clients         Admin      │
│  - Go SDK                  - Go SDK                - CLI       │
│  - Java SDK                - Java SDK              - Web UI    │
│  - Python SDK              - Python SDK            - API       │
└───────────────┬───────────────────┬────────────────┬───────────┘
                │                   │                │
                │                   │                │
┌───────────────▼───────────────────▼────────────────▼───────────┐
│                      Network Layer                              │
├───────────────────────────────────────────────────────────────┤
│  - Load Balancer (Built-in)                                   │
│  - Connection Pool                                             │
│  - Request Router                                              │
│  - Protocol Handler (Binary + gRPC)                           │
└───────────────┬───────────────────────────────────────────────┘
                │
                │
┌───────────────▼───────────────────────────────────────────────┐
│                      Broker Cluster                            │
├───────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐       │
│  │  Broker 1   │    │  Broker 2   │    │  Broker 3   │       │
│  ├─────────────┤    ├─────────────┤    ├─────────────┤       │
│  │ Partitions  │◄──►│ Partitions  │◄──►│ Partitions  │       │
│  │ - Leader    │    │ - Leader    │    │ - Leader    │       │
│  │ - Follower  │    │ - Follower  │    │ - Follower  │       │
│  ├─────────────┤    ├─────────────┤    ├─────────────┤       │
│  │ Storage     │    │ Storage     │    │ Storage     │       │
│  │ - LSM Tree  │    │ - LSM Tree  │    │ - LSM Tree  │       │
│  │ - WAL       │    │ - WAL       │    │ - WAL       │       │
│  │ - Index     │    │ - Index     │    │ - Index     │       │
│  └─────────────┘    └─────────────┘    └─────────────┘       │
│                                                                 │
│         ┌───────────────────────────────────────┐             │
│         │   Raft Consensus Layer (Metadata)    │             │
│         │   - Leader Election                   │             │
│         │   - Log Replication                   │             │
│         │   - Cluster State                     │             │
│         └───────────────────────────────────────┘             │
│                                                                 │
└─────────────────────────────────────────────────────────────┘
                │
                │
┌───────────────▼───────────────────────────────────────────────┐
│                    Storage Layer                               │
├───────────────────────────────────────────────────────────────┤
│  - Local Disk (NVMe/SSD)                                      │
│  - Tiered Storage (S3, GCS, Azure Blob)                       │
│  - Backup & Recovery                                           │
└───────────────────────────────────────────────────────────────┘
```

---

## Component Details

### 1. Broker

The broker is the core component responsible for:
- Accepting producer writes
- Serving consumer reads
- Managing partitions
- Coordinating replication
- Handling metadata operations

#### Key Subsystems

**Request Handler**
```go
type Broker struct {
    config        *Config
    storage       Storage
    replicator    *Replicator
    metadata      *MetadataManager
    networkServer *NetworkServer
    raftNode      *raft.Node
}

func (b *Broker) HandleProduce(req *ProduceRequest) (*ProduceResponse, error) {
    // 1. Validate request
    // 2. Append to local log (WAL)
    // 3. Replicate to followers (if leader)
    // 4. Wait for acks based on policy
    // 5. Return response
}

func (b *Broker) HandleFetch(req *FetchRequest) (*FetchResponse, error) {
    // 1. Validate request
    // 2. Check read authorization
    // 3. Fetch from storage (cache or disk)
    // 4. Return messages
}
```

### 2. Storage Engine

The storage engine is responsible for durable, efficient data storage.

#### LSM Tree Structure

```
┌───────────────────────────────────────────────────────────────┐
│                       Memory                                   │
├───────────────────────────────────────────────────────────────┤
│  MemTable (Active)        MemTable (Immutable)                │
│  - Skip List              - Skip List                          │
│  - Lock-Free Inserts      - Sealed for Flush                   │
└───────────────┬─────────────────┬─────────────────────────────┘
                │                 │
                │                 │ Flush
                │                 ▼
┌───────────────▼───────────────────────────────────────────────┐
│                       Disk                                     │
├───────────────────────────────────────────────────────────────┤
│  Level 0 (Hot - Recent)                                       │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐                       │
│  │ SST 001 │  │ SST 002 │  │ SST 003 │                       │
│  └─────────┘  └─────────┘  └─────────┘                       │
│                       │                                        │
│                       │ Compaction                             │
│                       ▼                                        │
│  Level 1 (Warm)                                               │
│  ┌─────────┐  ┌─────────┐                                    │
│  │ SST 101 │  │ SST 102 │                                    │
│  └─────────┘  └─────────┘                                    │
│                       │                                        │
│                       │ Compaction                             │
│                       ▼                                        │
│  Level 2 (Cold)                                               │
│  ┌─────────┐                                                  │
│  │ SST 201 │                                                  │
│  └─────────┘                                                  │
└───────────────────────────────────────────────────────────────┘
```

#### Storage Interface

```go
type Storage interface {
    // Write operations
    Append(partition int, messages []Message) ([]Offset, error)

    // Read operations
    Read(partition int, offset Offset, maxBytes int) ([]Message, error)
    ReadRange(partition int, startOffset, endOffset Offset) ([]Message, error)

    // Metadata operations
    GetHighWaterMark(partition int) (Offset, error)
    GetLogStartOffset(partition int) (Offset, error)

    // Maintenance
    Compact(partition int) error
    Flush() error
    Close() error
}
```

### 3. Replication Engine

Handles data replication across brokers for durability.

#### Replication Protocol

```
Leader                                    Follower
  │                                          │
  │  1. Produce Request                     │
  │◄─────────────────────────────           │
  │                                          │
  │  2. Write to WAL                        │
  │────────────►                             │
  │                                          │
  │  3. Replicate (FetchRequest)            │
  │─────────────────────────────────────────►│
  │                                          │
  │                         4. Fetch from WAL│
  │                         ◄────────────────│
  │                                          │
  │  5. FetchResponse (with messages)       │
  │◄─────────────────────────────────────────│
  │                                          │
  │                         6. Write to WAL  │
  │                         ────────────────►│
  │                                          │
  │  7. Update ISR (if caught up)           │
  │─────────────────────────────────────────►│
  │                                          │
  │  8. Produce Response (to client)        │
  │─────────────────────────────►            │
```

#### ISR Management

```go
type InSyncReplicas struct {
    leader   BrokerID
    replicas []BrokerID
    mu       sync.RWMutex
}

func (isr *InSyncReplicas) Add(broker BrokerID) {
    isr.mu.Lock()
    defer isr.mu.Unlock()

    if !contains(isr.replicas, broker) {
        isr.replicas = append(isr.replicas, broker)
    }
}

func (isr *InSyncReplicas) Remove(broker BrokerID) {
    isr.mu.Lock()
    defer isr.mu.Unlock()

    isr.replicas = removeElement(isr.replicas, broker)
}

func (isr *InSyncReplicas) CheckReplicaLag(broker BrokerID, lag int64) bool {
    return lag < maxReplicaLagBytes
}
```

### 4. Consumer Groups

Coordinate multiple consumers reading from the same topic.

#### Consumer Group Protocol

```
Coordinator                               Consumer 1            Consumer 2
     │                                       │                    │
     │  1. JoinGroup                         │                    │
     │◄──────────────────────────────────────┤                    │
     │                                       │                    │
     │  2. JoinGroup                         │                    │
     │◄───────────────────────────────────────────────────────────┤
     │                                       │                    │
     │  3. Elect Leader (Consumer 1)        │                    │
     │                                       │                    │
     │  4. SyncGroup (with assignment)      │                    │
     │──────────────────────────────────────►│                    │
     │                                       │                    │
     │  5. SyncGroup (with assignment)      │                    │
     │───────────────────────────────────────────────────────────►│
     │                                       │                    │
     │                                       │  6. Start consuming│
     │                                       │◄──────────────────►│
     │                                       │                    │
     │  7. Heartbeat                         │                    │
     │◄──────────────────────────────────────┤                    │
     │                                       │                    │
     │  8. Heartbeat                         │                    │
     │◄───────────────────────────────────────────────────────────┤
```

### 5. Metadata Management (Raft)

Uses Raft consensus for cluster metadata.

#### Metadata State Machine

```go
type MetadataStateMachine struct {
    topics      map[string]*Topic
    partitions  map[PartitionID]*Partition
    brokers     map[BrokerID]*Broker
    mu          sync.RWMutex
}

func (m *MetadataStateMachine) Apply(entry *raft.LogEntry) interface{} {
    m.mu.Lock()
    defer m.mu.Unlock()

    switch entry.Type {
    case CreateTopic:
        return m.applyCreateTopic(entry.Data)
    case DeleteTopic:
        return m.applyDeleteTopic(entry.Data)
    case UpdatePartition:
        return m.applyUpdatePartition(entry.Data)
    case RegisterBroker:
        return m.applyRegisterBroker(entry.Data)
    }

    return nil
}
```

---

## Data Flow

### Producer Write Path

```
Producer
    │
    │ 1. Send Produce Request
    ▼
Load Balancer
    │
    │ 2. Route to Leader Broker
    ▼
Broker (Leader)
    │
    │ 3. Validate & Assign Offset
    ▼
Write-Ahead Log
    │
    │ 4. Fsync (if required)
    ▼
Replication Engine
    │
    │ 5. Replicate to Followers
    ▼
Followers
    │
    │ 6. Write to WAL
    ▼
Acknowledge to Leader
    │
    │ 7. Update High Water Mark
    ▼
Response to Producer
```

### Consumer Read Path

```
Consumer
    │
    │ 1. Send Fetch Request
    ▼
Load Balancer
    │
    │ 2. Route to Leader or Follower
    ▼
Broker
    │
    │ 3. Check Cache
    ▼
Cache Hit? ──Yes──► Return from Cache
    │
    │ No
    ▼
Storage Engine
    │
    │ 4. Read from Disk
    ▼
Index Lookup
    │
    │ 5. Find Segment
    ▼
Read Segment File
    │
    │ 6. Decompress (if needed)
    ▼
Return Messages to Consumer
```

---

## Performance Optimizations

### 1. Batching
- Group multiple messages into a single request
- Reduces network overhead and increases throughput
- Configurable batch size and linger time

### 2. Compression
- Compress batches of messages
- Supported codecs: LZ4, Snappy, Zstd
- Reduces network and disk I/O

### 3. Zero-Copy
- Use `sendfile()` for disk-to-network transfers
- Avoid copying data between buffers
- Memory-mapped files for frequently accessed data

### 4. Caching
- LRU cache for hot partition data
- OS page cache for recent segments
- Negative cache for non-existent offsets

### 5. Parallel Processing
- Multiple goroutines per partition
- Work-stealing scheduler
- Lock-free data structures

### 6. Network Optimizations
- Connection pooling
- Request pipelining
- TCP_NODELAY for latency-sensitive operations

---

## Reliability & Fault Tolerance

### Failure Scenarios

#### 1. Broker Failure
- **Detection**: Heartbeat timeout (3-5 seconds)
- **Response**:
  - Raft elects new leader for affected partitions
  - Followers promoted to leaders
  - Clients automatically discover new leaders
- **Recovery Time**: < 3 seconds

#### 2. Network Partition
- **Detection**: Raft quorum loss
- **Response**:
  - Majority partition continues operating
  - Minority partition becomes read-only
  - Automatic reconciliation after partition heals
- **Data Loss**: None (quorum-based commits)

#### 3. Disk Failure
- **Detection**: I/O errors during read/write
- **Response**:
  - Mark disk as failed
  - Stop serving affected partitions
  - Rely on replicas on other brokers
- **Data Loss**: None (if replication factor > 1)

#### 4. Message Corruption
- **Detection**: Checksum mismatch
- **Response**:
  - Return error to client
  - Log corruption event
  - Trigger replica reconciliation
- **Prevention**: CRC32C checksums on all messages

### Disaster Recovery

#### Backup Strategy
1. **Continuous Replication**: Cross-region replicas
2. **Periodic Snapshots**: Daily incremental snapshots to S3/GCS
3. **WAL Archiving**: Archive WAL segments to object storage

#### Recovery Procedures
1. **Broker Recovery**: Restore from local disk or replicas
2. **Cluster Recovery**: Restore metadata from Raft snapshots
3. **Data Recovery**: Restore from object storage backups

---

## Conclusion

StreamBus architecture is designed for maximum performance, reliability, and operational simplicity. By leveraging Go's strengths and modern distributed systems patterns, we achieve superior performance compared to JVM-based solutions while maintaining strong consistency and fault tolerance guarantees.

For implementation details, see the source code in `/pkg` directory and individual component documentation.
