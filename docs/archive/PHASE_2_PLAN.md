# StreamBus Phase 2: Distributed System Implementation Plan

## Executive Summary

Phase 2 transforms StreamBus from a single-node broker into a production-ready distributed streaming platform with multi-broker replication, Raft-based consensus, and automated cluster coordination.

**Timeline**: Months 4-6 (12 weeks)
**Status**: Planning
**Prerequisites**: Phase 1 Complete (Storage Engine, Network Layer, Basic Broker)

## Phase 2 Goals

### Primary Objectives
1. **Multi-Broker Clustering**: Deploy and manage clusters of 3-100 brokers
2. **Data Replication**: Ensure data durability across broker failures
3. **Automatic Failover**: Sub-3-second broker failure recovery
4. **Consensus-Based Metadata**: Raft consensus for cluster metadata management
5. **Production Reliability**: 99.99% uptime with zero data loss

### Success Criteria
- ✅ 3-broker cluster with replication factor 3
- ✅ Leader election completes in < 5 seconds
- ✅ Broker failure recovery in < 3 seconds
- ✅ Zero message loss with acks=all
- ✅ Replication lag < 100ms at 50% capacity
- ✅ All chaos tests passing (broker kills, network partitions)

---

## Milestone 2.1: Raft Consensus (Weeks 1-4)

### Overview
Implement Raft consensus algorithm for distributed metadata management, eliminating the need for external coordination services like ZooKeeper.

### Architecture Decision: Raft Library vs Custom Implementation

**Option 1: etcd/raft Library** (RECOMMENDED)
- **Pros**: Battle-tested, production-ready, saves 4-6 weeks of implementation
- **Cons**: External dependency, learning curve
- **Decision**: Use `go.etcd.io/etcd/raft/v3`

**Option 2: Custom Raft Implementation**
- **Pros**: Full control, no dependencies
- **Cons**: High complexity, requires extensive testing, not production-ready for months

**Recommendation**: Start with etcd/raft library for faster time-to-market.

### Components to Implement

#### 1. Consensus Module (`pkg/consensus`)

```go
// Core interfaces
type ConsensusNode interface {
    Propose(ctx context.Context, data []byte) error
    Start() error
    Stop() error
    IsLeader() bool
    Leader() uint64
    AddNode(nodeID uint64, addr string) error
    RemoveNode(nodeID uint64) error
}

type Snapshot interface {
    Save() ([]byte, error)
    Restore(data []byte) error
}
```

**Files to Create**:
- `pkg/consensus/node.go` - Raft node wrapper
- `pkg/consensus/config.go` - Consensus configuration
- `pkg/consensus/transport.go` - Network transport for Raft messages
- `pkg/consensus/storage.go` - Persistent storage for Raft log
- `pkg/consensus/snapshot.go` - Snapshot management
- `pkg/consensus/fsm.go` - Finite State Machine for metadata
- `pkg/consensus/node_test.go` - Unit tests

#### 2. Metadata State Machine

**Metadata to Manage**:
- Broker registry (ID, address, status, resources)
- Topic metadata (name, partitions, replication factor)
- Partition assignments (partition → leader + replicas)
- ISR (In-Sync Replicas) status per partition
- Consumer group assignments (future)

**Files to Create**:
- `pkg/metadata/types.go` - Metadata structures
- `pkg/metadata/fsm.go` - State machine implementation
- `pkg/metadata/store.go` - In-memory metadata store
- `pkg/metadata/operations.go` - Metadata operations (create topic, assign partition, etc.)
- `pkg/metadata/fsm_test.go` - FSM tests

#### 3. Cluster Membership

**Files to Create**:
- `pkg/cluster/membership.go` - Broker join/leave/health
- `pkg/cluster/config.go` - Cluster configuration
- `pkg/cluster/discovery.go` - Broker discovery (DNS, static, Kubernetes)
- `pkg/cluster/heartbeat.go` - Heartbeat mechanism
- `pkg/cluster/membership_test.go` - Membership tests

### Implementation Tasks (Week by Week)

#### Week 1: Foundation
- [ ] Set up etcd/raft dependency
- [ ] Create `pkg/consensus` package structure
- [ ] Implement basic Raft node wrapper
- [ ] Create transport layer for Raft messages
- [ ] Write unit tests for transport

**Deliverable**: Raft node can start/stop and elect leader in 3-node cluster

#### Week 2: State Machine
- [ ] Design metadata schema (Protocol Buffers)
- [ ] Implement FSM for metadata operations
- [ ] Create in-memory metadata store
- [ ] Add snapshot support
- [ ] Write FSM tests with property-based testing

**Deliverable**: Metadata operations replicated across 3 nodes via Raft

#### Week 3: Persistence & Recovery
- [ ] Implement persistent Raft log storage
- [ ] Add snapshot save/restore
- [ ] Handle broker restart/recovery
- [ ] Implement log compaction
- [ ] Test recovery scenarios

**Deliverable**: Cluster survives broker restarts without data loss

#### Week 4: Integration & Testing
- [ ] Integrate consensus with existing broker
- [ ] Update topic creation to use consensus
- [ ] Add cluster membership APIs
- [ ] Write integration tests (3-node, 5-node clusters)
- [ ] Chaos test: kill leader, verify re-election

**Deliverable**: Fully functional 3-broker cluster with leader election

### Testing Strategy

**Unit Tests**:
- Raft node lifecycle
- FSM state transitions
- Snapshot save/restore
- Transport message encoding/decoding

**Integration Tests**:
- Leader election in 3-node cluster
- Proposal replication
- Node join/leave
- Log compaction
- Recovery from crashes

**Chaos Tests**:
- Kill leader → verify re-election < 5s
- Network partition → verify split-brain prevention
- Kill 2/3 nodes → verify cluster unavailable
- Restart all nodes → verify cluster recovers

---

## Milestone 2.2: Replication Engine (Weeks 5-8)

### Overview
Implement data replication between brokers to ensure durability and fault tolerance for message data.

### Architecture

```
┌──────────────────────────────────────────────────┐
│              Replication Architecture             │
├──────────────────────────────────────────────────┤
│                                                   │
│  Producer → Leader Broker → Follower Brokers     │
│                   │                               │
│                   ├─→ Follower 1 (ISR)           │
│                   ├─→ Follower 2 (ISR)           │
│                   └─→ Follower 3 (catching up)   │
│                                                   │
│  Consumer → Any ISR Broker (read from replica)   │
│                                                   │
└──────────────────────────────────────────────────┘
```

### Components to Implement

#### 1. Replication Protocol

**Files to Create**:
- `pkg/replication/types.go` - Replication types (FetchRequest, FetchResponse)
- `pkg/replication/leader.go` - Leader replication logic
- `pkg/replication/follower.go` - Follower fetch logic
- `pkg/replication/isr.go` - ISR management
- `pkg/replication/fetcher.go` - Background fetcher for followers
- `pkg/replication/config.go` - Replication configuration

#### 2. Partition Leadership

**Files to Create**:
- `pkg/broker/partition.go` - Partition abstraction
- `pkg/broker/leader.go` - Leader-specific operations
- `pkg/broker/follower.go` - Follower-specific operations
- `pkg/broker/assignment.go` - Partition assignment logic

#### 3. High Watermark & ISR

**Concepts**:
- **High Watermark (HW)**: Offset up to which all ISR replicas have caught up
- **Log End Offset (LEO)**: Highest offset in the log
- **ISR**: Replicas within `replica.lag.max.messages` and `replica.lag.time.max.ms`

**Files to Create**:
- `pkg/replication/watermark.go` - High watermark management
- `pkg/replication/lag.go` - Lag tracking and monitoring
- `pkg/replication/isr_manager.go` - ISR shrink/expand logic

### Implementation Tasks (Week by Week)

#### Week 5: Replication Protocol
- [ ] Define replication RPC protocol
- [ ] Implement FetchRequest/FetchResponse
- [ ] Create follower fetcher background worker
- [ ] Add offset tracking (LEO, HW)
- [ ] Write protocol tests

**Deliverable**: Follower can fetch data from leader

#### Week 6: ISR Management
- [ ] Implement ISR tracking per partition
- [ ] Add replica lag detection
- [ ] Implement ISR shrink/expand logic
- [ ] Add high watermark calculation
- [ ] Test ISR membership changes

**Deliverable**: ISR correctly tracks in-sync replicas

#### Week 7: Leader/Follower Coordination
- [ ] Update producer write path for replication
- [ ] Implement acks=0, acks=1, acks=all
- [ ] Add follower fetch batching
- [ ] Implement log reconciliation on leader change
- [ ] Test replication scenarios

**Deliverable**: Messages replicated to all ISR members before ack

#### Week 8: Failover & Recovery
- [ ] Implement automated leader failover
- [ ] Add replica promotion logic
- [ ] Implement log truncation on follower promotion
- [ ] Add replication lag metrics
- [ ] Chaos test: kill leader, verify promotion

**Deliverable**: Cluster survives leader failure with zero message loss

### Testing Strategy

**Unit Tests**:
- Fetch request/response encoding
- ISR membership logic
- High watermark calculation
- Lag detection

**Integration Tests**:
- 3-broker replication (RF=3)
- Producer with acks=all
- Follower catchup from behind
- ISR shrink/expand
- Leader failover

**Chaos Tests**:
- Kill leader → verify follower promotion
- Kill 2/3 replicas → verify writes fail (no quorum)
- Network partition → verify ISR shrinks
- Slow disk on follower → verify ISR removal

---

## Milestone 2.3: Cluster Coordination (Weeks 9-12)

### Overview
Implement cluster-wide coordination for partition assignment, rebalancing, and topology management.

### Components to Implement

#### 1. Partition Assignment

**Algorithms**:
- **Range Assignment**: Distribute partitions evenly across brokers
- **Rack-Aware Assignment**: Spread replicas across failure domains
- **Resource-Aware Assignment**: Consider broker CPU/disk/network capacity

**Files to Create**:
- `pkg/cluster/assignment.go` - Partition assignment algorithms
- `pkg/cluster/balancer.go` - Rebalancing logic
- `pkg/cluster/constraints.go` - Placement constraints (rack awareness)
- `pkg/cluster/assignment_test.go` - Assignment tests

#### 2. Broker Registration

**Files to Create**:
- `pkg/broker/registration.go` - Broker registration with cluster
- `pkg/broker/metadata.go` - Broker metadata (resources, capabilities)
- `pkg/broker/health.go` - Health check endpoint

#### 3. Partition Migration

**Files to Create**:
- `pkg/cluster/migration.go` - Partition migration logic
- `pkg/cluster/throttle.go` - Bandwidth throttling for migrations
- `pkg/replication/copy.go` - Data copy for migration

### Implementation Tasks (Week by Week)

#### Week 9: Partition Assignment
- [ ] Implement range assignment algorithm
- [ ] Add rack-aware assignment
- [ ] Create assignment CLI tool
- [ ] Add assignment to metadata FSM
- [ ] Test assignment fairness

**Deliverable**: Partitions assigned evenly across brokers

#### Week 10: Broker Registration
- [ ] Implement broker registration on startup
- [ ] Add broker heartbeat mechanism
- [ ] Implement broker deregistration on shutdown
- [ ] Add health check endpoint
- [ ] Test broker lifecycle

**Deliverable**: Brokers dynamically join/leave cluster

#### Week 11: Rebalancing
- [ ] Implement partition rebalancing trigger
- [ ] Add partition migration logic
- [ ] Implement throttled data transfer
- [ ] Add rebalancing progress tracking
- [ ] Test rebalancing scenarios

**Deliverable**: Cluster rebalances when broker added/removed

#### Week 12: Production Hardening
- [ ] Add comprehensive logging
- [ ] Implement cluster admin APIs
- [ ] Create CLI tools for cluster management
- [ ] Write runbooks for operations
- [ ] Run full chaos test suite

**Deliverable**: Production-ready cluster management

### Testing Strategy

**Unit Tests**:
- Assignment algorithm correctness
- Rebalancing logic
- Health check

**Integration Tests**:
- Broker join/leave
- Partition rebalancing
- Rack-aware placement
- Resource constraints

**Chaos Tests**:
- Add/remove brokers dynamically
- Kill broker during rebalance
- Network partition during migration
- Disk full on broker

---

## Technical Decisions

### 1. Raft Library
**Decision**: Use `go.etcd.io/etcd/raft/v3`
**Rationale**: Production-ready, saves months of development, battle-tested

### 2. Replication Protocol
**Decision**: Custom binary protocol (extend existing StreamBus protocol)
**Rationale**: Optimize for performance, leverage existing protocol infrastructure

### 3. Metadata Storage
**Decision**: In-memory with Raft log + periodic snapshots
**Rationale**: Fast access, Raft ensures durability

### 4. Partition Assignment
**Decision**: Centralized assignment via Raft leader
**Rationale**: Simplicity, strong consistency

### 5. ISR Shrink/Expand
**Decision**: Leader-driven ISR changes, committed via Raft
**Rationale**: Consistent ISR view across cluster

---

## Testing Strategy

### Test Pyramid

```
         /\
        /  \  E2E Tests (10%)
       /────\
      /      \  Integration Tests (30%)
     /────────\
    /          \  Unit Tests (60%)
   /────────────\
```

### Chaos Engineering Scenarios

1. **Leader Failure**
   - Kill leader → verify re-election < 5s
   - Verify no message loss

2. **Network Partition**
   - Partition cluster 2/3 vs 1/3
   - Verify minority unavailable
   - Heal partition → verify cluster recovers

3. **Disk Failure**
   - Simulate disk full
   - Verify broker marked unhealthy
   - Verify partitions migrated

4. **Slow Broker**
   - Inject latency on follower
   - Verify ISR shrink
   - Remove latency → verify ISR expand

5. **Concurrent Failures**
   - Kill 2 brokers simultaneously
   - Verify cluster degradation
   - Restart brokers → verify recovery

### Performance Tests

1. **Replication Throughput**
   - Measure replication lag at various throughputs
   - Target: < 100ms lag at 50% capacity

2. **Failover Latency**
   - Measure time from leader failure to new leader serving
   - Target: < 3 seconds

3. **Metadata Operations**
   - Measure latency of create topic, assign partition
   - Target: < 100ms

---

## Dependencies & Prerequisites

### External Libraries
```bash
go get go.etcd.io/etcd/raft/v3
go get go.etcd.io/etcd/server/v3/etcdserver/api/snap
go get github.com/hashicorp/raft  # Alternative if etcd/raft doesn't fit
```

### Infrastructure
- 3+ Linux VMs or containers for testing
- Docker Compose for local multi-broker testing
- Chaos Mesh or Toxiproxy for chaos testing

### Existing Code (Phase 1)
- Storage engine (LSM, WAL, MemTable)
- Network protocol (binary protocol, TCP server)
- Basic broker (topic management, produce/fetch)
- Client library (producer, consumer)

---

## Risk Mitigation

### Technical Risks

| Risk | Mitigation |
|------|------------|
| Raft library integration complexity | Allocate 1 week for learning, prototyping |
| Replication lag under load | Implement batching, prioritize replication traffic |
| Split-brain scenarios | Extensive testing, quorum enforcement |
| Data corruption during migration | Checksums, verification, rollback capability |

### Schedule Risks

| Risk | Mitigation |
|------|------------|
| Milestone 2.1 slips | Reduce scope: defer rack awareness to 2.3 |
| Testing takes longer | Parallel testing, automate chaos tests |
| Integration issues | Weekly integration checkpoints |

---

## Success Metrics

### Technical Metrics
- ✅ 3-broker cluster: leader election < 5s
- ✅ Replication lag < 100ms at 50% capacity
- ✅ Failover time < 3s
- ✅ Zero message loss with acks=all
- ✅ 100% chaos tests passing

### Code Quality
- ✅ Unit test coverage > 80%
- ✅ Integration tests for all critical paths
- ✅ Chaos tests for all failure scenarios
- ✅ Zero critical bugs in issue tracker

### Documentation
- ✅ Architecture documentation
- ✅ API documentation
- ✅ Operator runbooks
- ✅ Migration guide from single-broker

---

## Deliverables Checklist

### Milestone 2.1: Raft Consensus
- [ ] `pkg/consensus` module with Raft integration
- [ ] Metadata state machine
- [ ] Cluster membership
- [ ] Leader election working
- [ ] 20+ unit tests
- [ ] 5+ integration tests
- [ ] Architecture documentation

### Milestone 2.2: Replication Engine
- [ ] `pkg/replication` module
- [ ] ISR management
- [ ] High watermark tracking
- [ ] Automated failover
- [ ] 30+ unit tests
- [ ] 10+ integration tests
- [ ] Replication lag metrics

### Milestone 2.3: Cluster Coordination
- [ ] `pkg/cluster` module
- [ ] Partition assignment algorithms
- [ ] Broker registration/health
- [ ] Partition rebalancing
- [ ] CLI tools for cluster management
- [ ] 20+ unit tests
- [ ] 10+ chaos tests
- [ ] Operator runbooks

---

## Next Steps

### Week 1 (Immediate)
1. Review and approve this plan
2. Set up development environment for 3-broker testing
3. Add etcd/raft dependency
4. Create `pkg/consensus` package structure
5. Begin Raft node wrapper implementation

### Decision Points
- **End of Week 4**: Go/no-go on Raft integration (if issues, consider alternatives)
- **End of Week 8**: Re-evaluate replication performance (if lag > target, optimize)
- **End of Week 12**: Production readiness assessment

---

## Appendix: File Structure

```
pkg/
├── consensus/
│   ├── node.go              # Raft node wrapper
│   ├── config.go            # Consensus configuration
│   ├── transport.go         # Raft message transport
│   ├── storage.go           # Persistent Raft log
│   ├── snapshot.go          # Snapshot management
│   ├── fsm.go               # Finite State Machine
│   └── node_test.go         # Tests
├── metadata/
│   ├── types.go             # Metadata structures
│   ├── fsm.go               # FSM implementation
│   ├── store.go             # In-memory store
│   ├── operations.go        # Metadata operations
│   └── fsm_test.go          # Tests
├── replication/
│   ├── types.go             # Replication types
│   ├── leader.go            # Leader replication
│   ├── follower.go          # Follower fetch logic
│   ├── isr.go               # ISR management
│   ├── fetcher.go           # Background fetcher
│   ├── watermark.go         # High watermark
│   ├── lag.go               # Lag tracking
│   └── config.go            # Configuration
├── broker/
│   ├── partition.go         # Partition abstraction
│   ├── leader.go            # Leader operations
│   ├── follower.go          # Follower operations
│   ├── assignment.go        # Partition assignment
│   ├── registration.go      # Broker registration
│   ├── metadata.go          # Broker metadata
│   └── health.go            # Health checks
└── cluster/
    ├── membership.go        # Cluster membership
    ├── config.go            # Cluster configuration
    ├── discovery.go         # Broker discovery
    ├── heartbeat.go         # Heartbeat mechanism
    ├── assignment.go        # Partition assignment
    ├── balancer.go          # Rebalancing logic
    ├── constraints.go       # Placement constraints
    ├── migration.go         # Partition migration
    └── throttle.go          # Bandwidth throttling
```

---

**Document Version**: 1.0
**Created**: 2025-11-07
**Status**: Draft - Awaiting Approval
**Estimated Completion**: 12 weeks from approval
