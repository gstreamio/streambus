package integration

import (
	"context"
	"testing"
	"time"

	"github.com/shawntherrien/streambus/pkg/cluster"
	"github.com/shawntherrien/streambus/pkg/consensus"
	"github.com/shawntherrien/streambus/pkg/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEndToEndIntegration tests the complete flow from broker registration
// through Raft consensus to partition assignment and rebalancing
func TestEndToEndIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end integration test in short mode")
	}

	// Create 3-node Raft cluster
	t.Log("Setting up 3-node Raft cluster...")
	peers := []consensus.Peer{
		{ID: 1, Addr: "localhost:20001"},
		{ID: 2, Addr: "localhost:20002"},
		{ID: 3, Addr: "localhost:20003"},
	}

	nodes := make([]*consensus.RaftNode, 3)
	fsms := make([]*metadata.FSM, 3)
	stores := make([]*metadata.Store, 3)
	adapters := make([]*metadata.ClusterMetadataStore, 3)

	// Initialize Raft nodes
	for i := 0; i < 3; i++ {
		dir := t.TempDir()
		config := consensus.DefaultConfig()
		config.NodeID = uint64(i + 1)
		config.DataDir = dir
		config.Peers = peers

		// Create FSM
		fsm := metadata.NewFSM()
		fsms[i] = fsm

		// Create consensus node
		node, err := consensus.NewNode(config, fsm)
		require.NoError(t, err)
		node.SetBindAddr(peers[i].Addr)
		nodes[i] = node

		// Create metadata store
		store := metadata.NewStore(fsm, node)
		stores[i] = store

		// Create adapter
		adapter := metadata.NewClusterMetadataStore(store)
		adapters[i] = adapter
	}

	// Start all nodes
	for i, node := range nodes {
		err := node.Start()
		require.NoError(t, err, "node %d failed to start", i+1)
		defer node.Stop()
	}

	// Wait for leader election
	t.Log("Waiting for leader election...")
	var leaderStore *metadata.Store
	var leaderAdapter *metadata.ClusterMetadataStore
	var leaderID int

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for i, store := range stores {
			if store.IsLeader() {
				leaderStore = store
				leaderAdapter = adapters[i]
				leaderID = i + 1
				break
			}
		}
		if leaderStore != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	require.NotNil(t, leaderStore, "no leader elected")
	t.Logf("Leader elected: Node %d", leaderID)

	// Give cluster extra time to stabilize
	time.Sleep(2 * time.Second)

	ctx := context.Background()

	// Test 1: Register brokers via BrokerRegistry
	t.Run("broker_registration", func(t *testing.T) {
		t.Log("Creating BrokerRegistry on leader...")
		registry := cluster.NewBrokerRegistry(leaderAdapter)

		// Track events
		var addedBrokers []int32
		registry.SetOnBrokerAdded(func(broker *cluster.BrokerMetadata) {
			addedBrokers = append(addedBrokers, broker.ID)
			t.Logf("Broker added: %d", broker.ID)
		})

		// Register 3 brokers
		t.Log("Registering 3 brokers...")
		for i := int32(1); i <= 3; i++ {
			broker := &cluster.BrokerMetadata{
				ID:               i,
				Host:             "localhost",
				Port:             9090 + int(i),
				Rack:             "rack-1",
				Status:           cluster.BrokerStatusStarting,
				DiskCapacityGB:   1000,
				DiskUsedGB:       100,
				NetworkBandwidth: 10000000000,
			}

			err := registry.RegisterBroker(ctx, broker)
			require.NoError(t, err, "failed to register broker %d", i)
			t.Logf("Registered broker %d", i)
		}

		// Wait for replication
		time.Sleep(1 * time.Second)

		// Verify all nodes have the brokers
		t.Log("Verifying broker replication across all nodes...")
		for i, adapter := range adapters {
			brokers, err := adapter.ListBrokers(ctx)
			require.NoError(t, err, "node %d failed to list brokers", i+1)
			assert.Equal(t, 3, len(brokers), "node %d should have 3 brokers", i+1)

			// Verify broker details
			for _, broker := range brokers {
				assert.True(t, broker.ID >= 1 && broker.ID <= 3, "unexpected broker ID")
				assert.Equal(t, "localhost", broker.Host)
				assert.Equal(t, cluster.BrokerStatusAlive, broker.Status, "broker should be Alive")
			}
		}

		t.Log("✅ Broker registration test passed")
	})

	// Test 2: Partition assignment with ClusterCoordinator
	t.Run("partition_assignment", func(t *testing.T) {
		t.Log("Creating ClusterCoordinator on leader...")
		registry := cluster.NewBrokerRegistry(leaderAdapter)

		// Re-register brokers for coordinator
		for i := int32(1); i <= 3; i++ {
			broker := &cluster.BrokerMetadata{
				ID:       i,
				Host:     "localhost",
				Port:     9090 + int(i),
				Rack:     "rack-1",
				Status:   cluster.BrokerStatusAlive,
				Capacity: 100,
			}
			err := registry.RegisterBroker(ctx, broker)
			require.NoError(t, err, "failed to register broker %d", i)

			// Record heartbeat to transition to Alive status
			err = registry.RecordHeartbeat(i)
			require.NoError(t, err, "failed to record heartbeat for broker %d", i)
		}

		// Wait for broker replication
		t.Log("Waiting for broker replication...")
		time.Sleep(2 * time.Second)

		// Verify brokers are available
		activeBrokers := registry.ListActiveBrokers()
		require.Greater(t, len(activeBrokers), 0, "no active brokers found")
		t.Logf("Found %d active brokers", len(activeBrokers))

		// Create coordinator with Round-Robin strategy
		strategy := cluster.NewRoundRobinStrategy()
		coordinator := cluster.NewClusterCoordinator(registry, strategy, leaderAdapter)

		// Define partitions
		t.Log("Creating partition assignment...")
		partitions := []cluster.PartitionInfo{
			{Topic: "test-topic", PartitionID: 0, Replicas: 2},
			{Topic: "test-topic", PartitionID: 1, Replicas: 2},
			{Topic: "test-topic", PartitionID: 2, Replicas: 2},
		}

		constraints := &cluster.AssignmentConstraints{
			RackAware:       false,
			ExcludedBrokers: make(map[int32]bool),
		}

		// Assign partitions
		assignment, err := coordinator.AssignPartitions(ctx, partitions, constraints)
		require.NoError(t, err, "failed to assign partitions")
		assert.Equal(t, 3, assignment.TotalPartitions())
		t.Logf("Created assignment with %d partitions", assignment.TotalPartitions())

		// Store assignment to metadata (persist via Raft)
		err = leaderAdapter.StorePartitionAssignment(ctx, assignment)
		require.NoError(t, err, "failed to store partition assignment")

		// Wait for replication
		time.Sleep(2 * time.Second)

		// Verify all nodes have the partition assignments
		t.Log("Verifying partition assignment replication...")
		for i, store := range stores {
			parts := store.ListPartitions("test-topic")
			assert.Equal(t, 3, len(parts), "node %d should have 3 partitions", i+1)

			// Verify partition details
			for _, part := range parts {
				assert.Equal(t, "test-topic", part.Topic)
				assert.True(t, part.Partition >= 0 && part.Partition <= 2)
				assert.Equal(t, 2, len(part.Replicas), "partition should have 2 replicas")
				assert.NotEqual(t, uint64(0), part.Leader, "partition should have a leader")
			}
		}

		t.Log("✅ Partition assignment test passed")
	})

	// Test 3: Broker heartbeats and health monitoring
	t.Run("heartbeat_monitoring", func(t *testing.T) {
		t.Log("Testing heartbeat and health monitoring...")
		registry := cluster.NewBrokerRegistry(leaderAdapter)

		// Register broker
		broker := &cluster.BrokerMetadata{
			ID:     10,
			Host:   "localhost",
			Port:   9100,
			Status: cluster.BrokerStatusStarting,
		}
		err := registry.RegisterBroker(ctx, broker)
		require.NoError(t, err)

		// Start heartbeat service
		heartbeat := cluster.NewHeartbeatService(10, registry)
		heartbeat.SetInterval(1 * time.Second)
		err = heartbeat.Start()
		require.NoError(t, err)
		defer heartbeat.Stop()

		// Wait for heartbeats
		time.Sleep(2 * time.Second)

		// Verify broker is alive
		retrieved, err := registry.GetBroker(10)
		require.NoError(t, err)
		assert.Equal(t, cluster.BrokerStatusAlive, retrieved.Status)
		assert.True(t, time.Since(retrieved.LastHeartbeat) < 3*time.Second)

		t.Log("✅ Heartbeat monitoring test passed")
		t.Log("Note: Health check failure detection test skipped (requires longer timeout)")
	})

	// Test 4: Rebalancing on broker addition
	t.Run("rebalancing_on_broker_add", func(t *testing.T) {
		t.Log("Testing automatic rebalancing on broker addition...")
		registry := cluster.NewBrokerRegistry(leaderAdapter)
		strategy := cluster.NewStickyStrategy()
		coordinator := cluster.NewClusterCoordinator(registry, strategy, leaderAdapter)

		// Register initial brokers
		for i := int32(11); i <= 12; i++ {
			broker := &cluster.BrokerMetadata{
				ID:       i,
				Host:     "localhost",
				Port:     9090 + int(i),
				Status:   cluster.BrokerStatusAlive,
				Capacity: 100,
			}
			err := registry.RegisterBroker(ctx, broker)
			require.NoError(t, err, "failed to register broker %d", i)

			// Record heartbeat to transition to Alive status
			err = registry.RecordHeartbeat(i)
			require.NoError(t, err, "failed to record heartbeat for broker %d", i)
		}

		// Wait for broker replication
		t.Log("Waiting for broker replication...")
		time.Sleep(2 * time.Second)

		// Verify brokers are available
		activeBrokers := registry.ListActiveBrokers()
		require.Greater(t, len(activeBrokers), 0, "no active brokers found")
		t.Logf("Found %d active brokers", len(activeBrokers))

		// Create initial assignment
		partitions := []cluster.PartitionInfo{
			{Topic: "events", PartitionID: 0, Replicas: 2},
			{Topic: "events", PartitionID: 1, Replicas: 2},
			{Topic: "events", PartitionID: 2, Replicas: 2},
			{Topic: "events", PartitionID: 3, Replicas: 2},
		}

		constraints := &cluster.AssignmentConstraints{
			RackAware:       false,
			ExcludedBrokers: make(map[int32]bool),
		}

		initialAssignment, err := coordinator.AssignPartitions(ctx, partitions, constraints)
		require.NoError(t, err)
		t.Logf("Initial assignment: %d partitions", initialAssignment.TotalPartitions())

		// Start coordinator (enables automatic rebalancing)
		err = coordinator.Start()
		require.NoError(t, err)
		defer coordinator.Stop()

		// Get initial rebalance count
		statsBefore := coordinator.GetRebalanceStats()

		// Add new broker (should trigger rebalance via callback)
		newBroker := &cluster.BrokerMetadata{
			ID:       13,
			Host:     "localhost",
			Port:     9103,
			Status:   cluster.BrokerStatusAlive,
			Capacity: 100,
		}
		err = registry.RegisterBroker(ctx, newBroker)
		require.NoError(t, err)

		// Mark broker as active via heartbeat
		err = registry.RecordHeartbeat(13)
		require.NoError(t, err)
		t.Log("Added new broker 13")

		// Wait for async rebalance
		time.Sleep(3 * time.Second)

		// Verify rebalance occurred
		statsAfter := coordinator.GetRebalanceStats()
		assert.Greater(t, statsAfter.RebalanceCount, statsBefore.RebalanceCount,
			"rebalance should have been triggered")

		// Verify new assignment
		currentAssignment := coordinator.GetCurrentAssignment()
		require.NotNil(t, currentAssignment)
		assert.Equal(t, 4, currentAssignment.TotalPartitions())

		// Verify broker 13 has partitions assigned
		load := currentAssignment.BrokerLoad[13]
		assert.Greater(t, load, 0, "new broker should have partitions assigned")

		t.Log("✅ Rebalancing test passed")
	})

	// Test 5: Consistency across all nodes
	t.Run("verify_consistency", func(t *testing.T) {
		t.Log("Verifying state consistency across all nodes...")

		// Check that all nodes have the same state
		time.Sleep(1 * time.Second)

		var brokerCounts, topicCounts, partitionCounts []int
		var versions []uint64

		for i, store := range stores {
			state := store.GetState()

			brokerCount := len(state.Brokers)
			topicCount := len(state.Topics)
			partitionCount := len(state.Partitions)

			brokerCounts = append(brokerCounts, brokerCount)
			topicCounts = append(topicCounts, topicCount)
			partitionCounts = append(partitionCounts, partitionCount)
			versions = append(versions, state.Version)

			t.Logf("Node %d: Brokers=%d, Topics=%d, Partitions=%d, Version=%d",
				i+1, brokerCount, topicCount, partitionCount, state.Version)
		}

		// Verify all nodes have the same counts
		for i := 1; i < 3; i++ {
			assert.Equal(t, brokerCounts[0], brokerCounts[i],
				"broker count mismatch between nodes")
			assert.Equal(t, topicCounts[0], topicCounts[i],
				"topic count mismatch between nodes")
			assert.Equal(t, partitionCounts[0], partitionCounts[i],
				"partition count mismatch between nodes")
			assert.Equal(t, versions[0], versions[i],
				"version mismatch between nodes")
		}

		t.Log("✅ Consistency verification passed")
		t.Logf("Final state: %d brokers, %d topics, %d partitions, version %d",
			brokerCounts[0], topicCounts[0], partitionCounts[0], versions[0])
	})

	t.Log("========================================")
	t.Log("✅ All end-to-end integration tests passed!")
	t.Log("========================================")
}
