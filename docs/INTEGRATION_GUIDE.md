# StreamBus Integration Guide

**Last Updated**: 2025-11-08
**Status**: Phase 2 Integration

## Overview

This guide demonstrates how to integrate all StreamBus components into a complete, production-ready system. It covers the flow from broker registration through Raft consensus to partition assignment and rebalancing.

## Component Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                       Application Layer                         │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐     │
│  │   Broker     │    │   Producer   │    │   Consumer   │     │
│  │  Instance    │    │    Client    │    │    Client    │     │
│  └──────────────┘    └──────────────┘    └──────────────┘     │
└────────┬──────────────────────┬──────────────────┬─────────────┘
         │                      │                  │
┌────────┴──────────────────────┴──────────────────┴─────────────┐
│                    Cluster Coordination Layer                   │
│  ┌──────────────────┐         ┌──────────────────┐            │
│  │ BrokerRegistry   │◄────────│ClusterCoordinator│            │
│  │ - Registration   │         │ - Assign         │            │
│  │ - Heartbeats     │         │ - Rebalance      │            │
│  │ - Health checks  │         │ - Partition mgmt │            │
│  └─────────┬────────┘         └──────────┬───────┘            │
│            │                              │                     │
│            │  ┌───────────────────────────┴──────┐            │
│            │  │ AssignmentStrategy                │            │
│            │  │ - RoundRobin / Range / Sticky     │            │
│            │  └───────────────────────────────────┘            │
└────────────┼───────────────────────────────────────────────────┘
             │
┌────────────┴───────────────────────────────────────────────────┐
│                    Metadata Adapter Layer                       │
│  ┌────────────────────────────────────────────────────┐        │
│  │        ClusterMetadataStore (Adapter)              │        │
│  │  - Implements cluster.MetadataStore                │        │
│  │  - Type conversion (cluster ↔ metadata)           │        │
│  │  - Broker operations → Raft                        │        │
│  │  - Partition operations → Raft                     │        │
│  └──────────────────────┬─────────────────────────────┘        │
└─────────────────────────┼──────────────────────────────────────┘
                          │
┌─────────────────────────┴──────────────────────────────────────┐
│                   Consensus & Storage Layer                     │
│  ┌──────────────┐       ┌──────────────┐                       │
│  │ metadata.    │       │ Raft FSM     │                       │
│  │ Store        │◄──────│              │                       │
│  │ - Brokers    │       │ - Apply ops  │                       │
│  │ - Topics     │       │ - Snapshots  │                       │
│  │ - Partitions │       │ - State mgmt │                       │
│  └──────┬───────┘       └──────┬───────┘                       │
│         │                      │                                │
│         └──────────┬───────────┘                                │
│                    │                                             │
│  ┌─────────────────┴─────────────────┐                         │
│  │      Raft Consensus Layer         │                         │
│  │  - Leader election                │                         │
│  │  - Log replication                │                         │
│  │  - Commit consensus               │                         │
│  └───────────────────────────────────┘                         │
└─────────────────────────────────────────────────────────────────┘
```

## Integration Steps

### Step 1: Initialize Raft Cluster

```go
// Create Raft nodes for 3-node cluster
peers := []consensus.Peer{
    {ID: 1, Addr: "localhost:19001"},
    {ID: 2, Addr: "localhost:19002"},
    {ID: 3, Addr: "localhost:19003"},
}

// Node configuration
config := consensus.DefaultConfig()
config.NodeID = 1 // Change for each node
config.DataDir = "/var/lib/streambus/node1"
config.Peers = peers

// Create FSM for metadata
fsm := metadata.NewFSM()

// Create consensus node
node, err := consensus.NewNode(config, fsm)
if err != nil {
    log.Fatal(err)
}

// Set bind address
node.SetBindAddr(peers[0].Addr)

// Start node
if err := node.Start(); err != nil {
    log.Fatal(err)
}
```

### Step 2: Create Metadata Store

```go
// Create metadata store wrapping FSM and consensus
metadataStore := metadata.NewStore(fsm, node)

// Wait for leader election
for {
    if metadataStore.IsLeader() {
        log.Printf("Node %d is leader", config.NodeID)
        break
    }
    time.Sleep(100 * time.Millisecond)
}
```

### Step 3: Create Cluster Metadata Adapter

```go
// Create adapter that bridges cluster to metadata layer
clusterStore := metadata.NewClusterMetadataStore(metadataStore)

// Now clusterStore implements cluster.MetadataStore
// and can be used by BrokerRegistry and ClusterCoordinator
```

### Step 4: Initialize Broker Registry

```go
// Create broker registry with metadata store
registry := cluster.NewBrokerRegistry(clusterStore)

// Set up callbacks for cluster events
registry.SetOnBrokerAdded(func(broker *cluster.BrokerMetadata) {
    log.Printf("Broker %d added: %s", broker.ID, broker.Address())
})

registry.SetOnBrokerRemoved(func(brokerID int32) {
    log.Printf("Broker %d removed", brokerID)
})

registry.SetOnBrokerFailed(func(brokerID int32) {
    log.Printf("Broker %d failed", brokerID)
})

// Start health monitoring
if err := registry.Start(); err != nil {
    log.Fatal(err)
}
```

### Step 5: Initialize Cluster Coordinator

```go
// Choose assignment strategy
strategy := cluster.NewStickyStrategy() // Or RoundRobin, Range

// Create cluster coordinator
coordinator := cluster.NewClusterCoordinator(
    registry,
    strategy,
    clusterStore,
)

// Configure rebalancing
coordinator.SetRebalanceInterval(5 * time.Minute)
coordinator.SetRebalanceThreshold(2) // Imbalance threshold

// Start coordinator (enables automatic rebalancing)
if err := coordinator.Start(); err != nil {
    log.Fatal(err)
}
```

### Step 6: Register Broker

```go
// Create broker metadata
broker := &cluster.BrokerMetadata{
    ID:               1,
    Host:             "localhost",
    Port:             9092,
    Rack:             "rack-1",
    Capacity:         1000,
    DiskCapacityGB:   2000,
    DiskUsedGB:       100,
    NetworkBandwidth: 10000000000, // 10 Gbps
    Version:          "1.0.0",
    Tags:             map[string]string{"dc": "us-west"},
}

// Register broker (will be stored in Raft)
ctx := context.Background()
if err := registry.RegisterBroker(ctx, broker); err != nil {
    log.Fatal(err)
}

// Start heartbeat service for this broker
heartbeat := cluster.NewHeartbeatService(broker.ID, registry)
heartbeat.SetInterval(10 * time.Second)
if err := heartbeat.Start(); err != nil {
    log.Fatal(err)
}
```

### Step 7: Create Topic and Assign Partitions

```go
// Define partitions for a new topic
partitions := []cluster.PartitionInfo{
    {Topic: "events", PartitionID: 0, Replicas: 3},
    {Topic: "events", PartitionID: 1, Replicas: 3},
    {Topic: "events", PartitionID: 2, Replicas: 3},
}

// Create assignment constraints
constraints := &cluster.AssignmentConstraints{
    RackAware:       true,
    ExcludedBrokers: make(map[int32]bool),
}

// Assign partitions (will be stored in Raft)
assignment, err := coordinator.AssignPartitions(ctx, partitions, constraints)
if err != nil {
    log.Fatal(err)
}

log.Printf("Created assignment: %d partitions", assignment.TotalPartitions())
```

## Complete Flow Examples

### Example 1: Broker Registration Flow

```go
// 1. Application registers broker
broker := &cluster.BrokerMetadata{ID: 1, Host: "localhost", Port: 9092}
err := registry.RegisterBroker(ctx, broker)

// 2. Registry calls adapter
// registry.RegisterBroker → clusterStore.StoreBrokerMetadata

// 3. Adapter converts types
// cluster.BrokerMetadata → metadata.BrokerInfo

// 4. Adapter calls metadata store
// clusterStore → metadataStore.RegisterBroker

// 5. Store proposes to Raft
// metadataStore.RegisterBroker → node.Propose(operation)

// 6. Raft replicates
// Leader → Followers (log replication)

// 7. FSM applies operation
// FSM.Apply → fsm.applyRegisterBroker

// 8. State updated on all nodes
// state.Brokers[1] = brokerInfo
// state.Version++

// 9. Callback triggered
// registry.onBrokerAdded(broker)

// 10. Coordinator reacts
// coordinator.onBrokerAdded → TriggerRebalance
```

### Example 2: Broker Failure Detection Flow

```go
// 1. Heartbeat service sends heartbeat
heartbeat.sendHeartbeat()

// 2. Registry updates timestamp
registry.RecordHeartbeat(brokerID)

// 3. Health monitor detects timeout
// (runs every 5 seconds, checks if LastHeartbeat > 30s)
registry.checkBrokerHealth()

// 4. Broker marked as failed
broker.Status = cluster.BrokerStatusFailed

// 5. Callback triggered
registry.onBrokerFailed(brokerID)

// 6. Coordinator triggers rebalance
coordinator.onBrokerFailed → TriggerRebalance()

// 7. Assignment strategy rebalances
newAssignment := strategy.Rebalance(currentAssignment, activeBrokers, constraints)

// 8. New assignment stored in Raft
coordinator → clusterStore.StorePartitionAssignment(newAssignment)

// 9. Partitions reassigned
// Replicas moved from failed broker to healthy brokers
```

### Example 3: Adding New Broker with Rebalancing

```go
// Initial state: 2 brokers, 6 partitions
// Broker 1: partitions [0, 1, 2]
// Broker 2: partitions [3, 4, 5]

// 1. Register new broker
newBroker := &cluster.BrokerMetadata{ID: 3, Host: "localhost", Port: 9094}
registry.RegisterBroker(ctx, newBroker)

// 2. Stored in Raft
// All nodes now know about broker 3

// 3. Callback triggers rebalance
coordinator.onBrokerAdded(newBroker)

// 4. Sticky strategy minimizes movement
// Current: {0:[1,2], 1:[1,2], 2:[1,2], 3:[2,1], 4:[2,1], 5:[2,1]}
// New:     {0:[1,2], 1:[1,2], 2:[1,2], 3:[2,1], 4:[2,1], 5:[3,1]}
//                                                          ^^^
//                                         Only partition 5 moved

// 5. New assignment stored
clusterStore.StorePartitionAssignment(newAssignment)

// 6. Partitions transferred
// Partition 5: Broker 2 → Broker 3
// - Data replicated from broker 2 to broker 3
// - ISR updated when replica catches up
// - Leadership may transfer

// Final state: load balanced
// Broker 1: 2 partitions
// Broker 2: 2 partitions
// Broker 3: 2 partitions
```

## Testing the Integration

### Unit Test with Mock Consensus

```go
func TestBrokerRegistrationFlow(t *testing.T) {
    // Create mock consensus
    fsm := metadata.NewFSM()
    consensus := &mockConsensusNode{fsm: fsm}

    // Create stack
    metadataStore := metadata.NewStore(fsm, consensus)
    clusterStore := metadata.NewClusterMetadataStore(metadataStore)
    registry := cluster.NewBrokerRegistry(clusterStore)

    // Register broker
    broker := &cluster.BrokerMetadata{
        ID:   1,
        Host: "localhost",
        Port: 9092,
    }

    err := registry.RegisterBroker(context.Background(), broker)
    require.NoError(t, err)

    // Verify stored in FSM
    brokerInfo, exists := fsm.GetBroker(1)
    require.True(t, exists)
    assert.Equal(t, "localhost:9092", brokerInfo.Addr)
}
```

### Integration Test with Real Raft

```go
func TestEndToEndIntegration(t *testing.T) {
    // Create 3-node Raft cluster
    nodes, stores := setupRaftCluster(t, 3)
    defer cleanupCluster(nodes)

    // Wait for leader
    leader := waitForLeader(t, stores, 10*time.Second)

    // Create cluster components on leader
    clusterStore := metadata.NewClusterMetadataStore(leader)
    registry := cluster.NewBrokerRegistry(clusterStore)
    coordinator := cluster.NewClusterCoordinator(
        registry,
        cluster.NewStickyStrategy(),
        clusterStore,
    )

    // Register 3 brokers
    for i := 1; i <= 3; i++ {
        broker := &cluster.BrokerMetadata{
            ID:   int32(i),
            Host: "localhost",
            Port: 9090 + i,
        }
        err := registry.RegisterBroker(context.Background(), broker)
        require.NoError(t, err)
    }

    // Wait for replication
    time.Sleep(1 * time.Second)

    // Verify all nodes see brokers
    for i, store := range stores {
        brokers, err := clusterStore.ListBrokers(context.Background())
        require.NoError(t, err)
        assert.Equal(t, 3, len(brokers), "node %d should see 3 brokers", i+1)
    }

    // Create partition assignment
    partitions := []cluster.PartitionInfo{
        {Topic: "test", PartitionID: 0, Replicas: 2},
        {Topic: "test", PartitionID: 1, Replicas: 2},
    }

    assignment, err := coordinator.AssignPartitions(
        context.Background(),
        partitions,
        &cluster.AssignmentConstraints{},
    )
    require.NoError(t, err)
    assert.Equal(t, 2, assignment.TotalPartitions())

    // Verify assignment replicated
    time.Sleep(1 * time.Second)

    for i, store := range stores {
        parts := store.ListPartitions("test")
        assert.Equal(t, 2, len(parts), "node %d should see 2 partitions", i+1)
    }
}
```

## Production Considerations

### 1. Raft Cluster Setup

**Minimum Configuration:**
- 3 nodes for fault tolerance (tolerates 1 failure)
- 5 nodes recommended for production (tolerates 2 failures)

**Network Requirements:**
- Low latency between nodes (<10ms ideal)
- Reliable network (packet loss < 1%)
- Sufficient bandwidth for log replication

**Storage Requirements:**
- Fast disk for Raft logs (SSD recommended)
- Separate disk for data vs logs
- Regular snapshot compaction

### 2. Broker Registry Configuration

```go
// Production settings
registry := cluster.NewBrokerRegistry(clusterStore)

// Heartbeat timeout: 30 seconds
// - Too short: false positives from network blips
// - Too long: slow failure detection
registry.SetHeartbeatTimeout(30 * time.Second)

// Health check interval: 5 seconds
// - Balance between responsiveness and overhead
registry.SetCheckInterval(5 * time.Second)
```

### 3. Cluster Coordinator Configuration

```go
coordinator := cluster.NewClusterCoordinator(registry, strategy, clusterStore)

// Rebalance interval: 5 minutes
// - Periodic check for imbalance drift
coordinator.SetRebalanceInterval(5 * time.Minute)

// Rebalance threshold: 2 partitions
// - Minimum imbalance to trigger rebalance
// - Prevents excessive rebalancing
coordinator.SetRebalanceThreshold(2)
```

### 4. Monitoring and Observability

**Key Metrics to Track:**

1. **Raft Metrics:**
   - Leader election count
   - Log append latency
   - Commit index lag
   - Snapshot frequency

2. **Broker Metrics:**
   - Active broker count
   - Failed broker count
   - Heartbeat success rate
   - Registration rate

3. **Assignment Metrics:**
   - Rebalance count
   - Failed rebalances
   - Assignment imbalance
   - Partition movement

4. **Adapter Metrics:**
   - Operation latency
   - Conversion errors
   - Raft proposal failures

### 5. Error Handling

**Common Issues and Solutions:**

| Issue | Symptom | Solution |
|-------|---------|----------|
| Split brain | Multiple leaders | Check network partitioning, verify quorum |
| Slow replication | High commit lag | Check disk I/O, network bandwidth |
| Failed proposals | Timeouts on operations | Check if leader is elected, verify quorum |
| Broker flapping | Frequent fail/alive transitions | Tune heartbeat timeout, check network |
| Excessive rebalancing | Frequent partition movement | Increase rebalance threshold |

## Performance Tuning

### Raft Performance

```go
config := consensus.DefaultConfig()

// Election timeout: 1-2 seconds
// - Balance between fast failover and stability
config.ElectionTimeout = 1500 * time.Millisecond

// Heartbeat interval: 150ms
// - Keep much shorter than election timeout
config.HeartbeatInterval = 150 * time.Millisecond

// Snapshot threshold: 8192 entries
// - Balance between snapshot frequency and size
config.SnapshotThreshold = 8192

// Max append entries: 64
// - Batch size for log replication
config.MaxAppendEntries = 64
```

### Broker Registry Performance

```go
// For large clusters (100+ brokers)
registry.SetCheckInterval(10 * time.Second)  // Reduce check frequency
registry.SetHeartbeatTimeout(60 * time.Second)  // Longer timeout

// For small clusters (< 10 brokers)
registry.SetCheckInterval(2 * time.Second)  // More frequent checks
registry.SetHeartbeatTimeout(15 * time.Second)  // Faster detection
```

### Assignment Strategy Performance

```go
// For initial assignment: Use RoundRobin (O(N))
strategy := cluster.NewRoundRobinStrategy()

// For rebalancing: Use Sticky (minimal movement)
strategy := cluster.NewStickyStrategy()

// For topic affinity: Use Range (O(N))
strategy := cluster.NewRangeStrategy()
```

## Deployment Example

### Docker Compose Setup

```yaml
version: '3.8'

services:
  streambus-1:
    image: streambus:latest
    command: |
      --node-id=1
      --raft-addr=streambus-1:19001
      --broker-addr=streambus-1:9092
      --peers=streambus-1:19001,streambus-2:19002,streambus-3:19003
    ports:
      - "9092:9092"
      - "19001:19001"
    volumes:
      - streambus-1-data:/var/lib/streambus

  streambus-2:
    image: streambus:latest
    command: |
      --node-id=2
      --raft-addr=streambus-2:19002
      --broker-addr=streambus-2:9092
      --peers=streambus-1:19001,streambus-2:19002,streambus-3:19003
    ports:
      - "9093:9092"
      - "19002:19002"
    volumes:
      - streambus-2-data:/var/lib/streambus

  streambus-3:
    image: streambus:latest
    command: |
      --node-id=3
      --raft-addr=streambus-3:19003
      --broker-addr=streambus-3:9092
      --peers=streambus-1:19001,streambus-2:19002,streambus-3:19003
    ports:
      - "9094:9092"
      - "19003:19003"
    volumes:
      - streambus-3-data:/var/lib/streambus

volumes:
  streambus-1-data:
  streambus-2-data:
  streambus-3-data:
```

## Next Steps

1. **Complete Integration Testing**: Test end-to-end flow with real Raft cluster
2. **Add Monitoring**: Implement metrics collection and dashboards
3. **Chaos Testing**: Test failure scenarios (network partitions, broker crashes)
4. **Load Testing**: Measure performance under load
5. **Production Hardening**: Add retry logic, circuit breakers, graceful shutdown

## References

- [Week 11: Metadata Integration](./WEEK_11_METADATA_INTEGRATION.md)
- [Week 10: Broker Registration](./WEEK_10_BROKER_REGISTRATION.md)
- [Week 9: Partition Assignment](./WEEK_9_PARTITION_ASSIGNMENT.md)
- [Phase 2 Plan](./PHASE_2_PLAN.md)

---

**Status**: Integration guide complete
**Last Updated**: 2025-11-08
