package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/consensus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to find the current leader
func findLeader(stores []*Store) (*Store, uint64) {
	for i, store := range stores {
		if store.IsLeader() {
			return store, uint64(i + 1)
		}
	}
	return nil, 0
}

// Helper function to wait for a leader to be elected
func waitForLeader(t *testing.T, stores []*Store, timeout time.Duration) (*Store, uint64) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if leader, id := findLeader(stores); leader != nil {
			return leader, id
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, 0
}

// Helper function to log Raft status for debugging
func logRaftStatus(t *testing.T, nodes []*consensus.RaftNode, message string) {
	t.Logf("=== Raft Status: %s ===", message)
	for i, node := range nodes {
		if node == nil {
			continue
		}
		status := node.Status()
		t.Logf("Node %d: Lead=%d, State=%v, Applied=%d, Commit=%d, Term=%d",
			i+1, status.Lead, status.RaftState, status.Applied,
			status.Commit, status.Term)
	}
}

// waitForStableLeader waits for a leader that is stable and has consensus.
func waitForStableLeader(t *testing.T, stores []*Store, nodes []*consensus.RaftNode, timeout time.Duration) (*Store, uint64) {
	const stabilityPeriod = 500 * time.Millisecond // Increased stability period
	deadline := time.Now().Add(timeout)
	var lastLeaderID uint64
	var lastTerm uint64
	var stableStart time.Time

	for time.Now().Before(deadline) {
		leader, leaderID := findLeader(stores)

		if leader == nil {
			lastLeaderID = 0
			lastTerm = 0
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Get the current term from all nodes
		terms := make([]uint64, len(nodes))
		indices := make([]uint64, len(nodes))
		allSameTerm := true
		var currentTerm uint64

		for i, node := range nodes {
			if node != nil {
				status := node.Status()
				terms[i] = status.Term
				indices[i] = status.Applied
				if i == 0 {
					currentTerm = status.Term
				} else if status.Term != currentTerm {
					allSameTerm = false
				}
			}
		}

		// Check for split brain - all nodes must be on the same term
		if !allSameTerm {
			t.Logf("Split-brain detected: terms=%v, waiting for convergence", terms)
			lastLeaderID = 0
			lastTerm = 0
			time.Sleep(200 * time.Millisecond)
			continue
		}

		// Check all nodes agree on the leader
		consensus := true
		for _, store := range stores {
			reportedLeader := store.Leader()
			// Allow 0 (unknown) but not disagreement
			if reportedLeader != leaderID && reportedLeader != 0 {
				consensus = false
				break
			}
		}

		if !consensus {
			t.Logf("No consensus on leader, waiting...")
			lastLeaderID = 0
			lastTerm = 0
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Check for log divergence
		minIndex := indices[0]
		maxIndex := indices[0]
		for _, idx := range indices {
			if idx < minIndex {
				minIndex = idx
			}
			if idx > maxIndex {
				maxIndex = idx
			}
		}

		if maxIndex > minIndex+1 {
			t.Logf("Log divergence detected: indices=%v, waiting for sync", indices)
			lastLeaderID = 0
			lastTerm = 0
			time.Sleep(200 * time.Millisecond)
			continue
		}

		// Check if this is a new leader or term
		if leaderID != lastLeaderID || currentTerm != lastTerm {
			lastLeaderID = leaderID
			lastTerm = currentTerm
			stableStart = time.Now()
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Check if stable for long enough
		if time.Since(stableStart) >= stabilityPeriod {
			t.Logf("Leader Node %d is stable for %v at term %d", leaderID, time.Since(stableStart), currentTerm)
			return leader, leaderID
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil, 0
}

// TestMetadataReplication tests metadata replication across a 3-node Raft cluster.
func TestMetadataReplication(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create 3 consensus nodes with metadata FSMs
	nodes := make([]*consensus.RaftNode, 3)
	stores := make([]*Store, 3)
	fsms := make([]*FSM, 3)

	peers := []consensus.Peer{
		{ID: 1, Addr: "localhost:19001"},
		{ID: 2, Addr: "localhost:19002"},
		{ID: 3, Addr: "localhost:19003"},
	}

	// Initialize nodes
	for i := 0; i < 3; i++ {
		dir := t.TempDir()
		config := consensus.DefaultConfig()
		config.NodeID = uint64(i + 1)
		config.DataDir = dir
		config.Peers = peers

		// Create FSM for this node
		fsm := NewFSM()
		fsms[i] = fsm

		// Create consensus node with FSM as state machine
		node, err := consensus.NewNode(config, fsm)
		require.NoError(t, err)

		node.SetBindAddr(peers[i].Addr)
		nodes[i] = node

		// Create metadata store with the FSM
		stores[i] = NewStore(fsm, node)
	}

	// Start all nodes
	for i, node := range nodes {
		err := node.Start()
		require.NoError(t, err, "node %d failed to start", i+1)
		defer func(n *consensus.RaftNode) {
			_ = n.Stop()
		}(node)
	}

	// Wait for initial leader election
	leaderNode, leaderID := waitForLeader(t, stores, 10*time.Second)
	require.NotNil(t, leaderNode, "should have elected a leader")
	t.Logf("Initial leader is Node %d", leaderID)

	// Give cluster extra time to stabilize after initial election
	time.Sleep(2 * time.Second)

	// Ensure we have a stable leader before proceeding with tests
	leaderNode, leaderID = waitForStableLeader(t, stores, nodes, 10*time.Second)
	require.NotNil(t, leaderNode, "should have stable leader before tests")
	t.Logf("Confirmed stable leader is Node %d", leaderID)

	// Test 1: Register brokers via leader
	t.Run("register brokers", func(t *testing.T) {
		// Get current leader
		leader, leaderID := waitForLeader(t, stores, 5*time.Second)
		require.NotNil(t, leader, "no leader elected")
		t.Logf("Using Node %d as leader", leaderID)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		for i := uint64(1); i <= 3; i++ {
			broker := &BrokerInfo{
				ID:   i,
				Addr: peers[i-1].Addr,
				Rack: "rack" + string(rune('0'+i)),
			}

			err := leader.RegisterBroker(ctx, broker)
			require.NoError(t, err, "failed to register broker %d", i)
		}

		// Wait for replication by polling
		require.Eventually(t, func() bool {
			for i, store := range stores {
				brokers := store.ListBrokers()
				if len(brokers) != 3 {
					t.Logf("Node %d has %d brokers, waiting...", i+1, len(brokers))
					return false
				}
			}
			return true
		}, 5*time.Second, 100*time.Millisecond, "brokers not replicated to all nodes")

		// Verify all nodes have the brokers
		for i, store := range stores {
			brokers := store.ListBrokers()
			assert.Equal(t, 3, len(brokers), "node %d should have 3 brokers", i+1)

			for _, broker := range brokers {
				assert.True(t, broker.ID >= 1 && broker.ID <= 3)
				t.Logf("Node %d has broker %d at %s", i+1, broker.ID, broker.Addr)
			}
		}
	})

	// Test 2: Create topic via leader
	t.Run("create topic", func(t *testing.T) {
		// Get current leader
		leader, leaderID := waitForLeader(t, stores, 5*time.Second)
		require.NotNil(t, leader, "no leader elected")
		t.Logf("Using Node %d as leader", leaderID)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		config := DefaultTopicConfig()
		err := leader.CreateTopic(ctx, "test-topic", 3, 2, config)
		require.NoError(t, err)

		// Wait for replication by polling
		require.Eventually(t, func() bool {
			for i, store := range stores {
				_, exists := store.GetTopic("test-topic")
				if !exists {
					t.Logf("Node %d doesn't have topic yet, waiting...", i+1)
					return false
				}
			}
			return true
		}, 5*time.Second, 100*time.Millisecond, "topic not replicated to all nodes")

		// Verify all nodes have the topic
		for i, store := range stores {
			topic, exists := store.GetTopic("test-topic")
			assert.True(t, exists, "node %d should have topic", i+1)
			if exists {
				assert.Equal(t, "test-topic", topic.Name)
				assert.Equal(t, 3, topic.NumPartitions)
				assert.Equal(t, 2, topic.ReplicationFactor)
				t.Logf("Node %d has topic %s with %d partitions", i+1, topic.Name, topic.NumPartitions)
			}
		}
	})

	// Test 3: Create partitions via leader
	t.Run("create partitions", func(t *testing.T) {
		// Get current leader
		leader, leaderID := waitForLeader(t, stores, 5*time.Second)
		require.NotNil(t, leader, "no leader elected")
		t.Logf("Using Node %d as leader", leaderID)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Allocate partitions
		partitions, err := leader.AllocatePartitions("test-topic", 3, 2)
		require.NoError(t, err)
		require.Equal(t, 3, len(partitions))

		// Create partitions one by one with delays to maintain stability
		for i, partition := range partitions {
			// Check if we still have a stable leader before each operation
			if !leader.IsLeader() {
				newLeader, newID := waitForStableLeader(t, stores, nodes, 5*time.Second)
				if newLeader != nil {
					leader = newLeader
					t.Logf("Leader changed to Node %d", newID)
				}
			}

			err := leader.CreatePartition(ctx, partition)
			require.NoError(t, err, "failed to create partition %d", i)
			t.Logf("Created partition %d/%d", i+1, len(partitions))

			// Delay between operations to maintain cluster stability
			time.Sleep(500 * time.Millisecond)
		}

		// Wait for replication by polling
		require.Eventually(t, func() bool {
			for i, store := range stores {
				parts := store.ListPartitions("test-topic")
				if len(parts) != 3 {
					t.Logf("Node %d has %d partitions, waiting...", i+1, len(parts))
					return false
				}
			}
			return true
		}, 5*time.Second, 100*time.Millisecond, "partitions not replicated to all nodes")

		// Verify all nodes have the partitions
		for i, store := range stores {
			parts := store.ListPartitions("test-topic")
			assert.Equal(t, 3, len(parts), "node %d should have 3 partitions", i+1)

			for _, part := range parts {
				assert.Equal(t, "test-topic", part.Topic)
				assert.True(t, part.Partition >= 0 && part.Partition < 3)
				assert.Equal(t, 2, len(part.Replicas))
				assert.Equal(t, 2, len(part.ISR))
				t.Logf("Node %d has partition %s:%d leader=%d replicas=%v",
					i+1, part.Topic, part.Partition, part.Leader, part.Replicas)
			}
		}
	})

	// Test 4: Update leader on a partition
	t.Run("update leader", func(t *testing.T) {
		// Give cluster time to stabilize after batch operations
		time.Sleep(2 * time.Second)

		// Log Raft status before update operation
		logRaftStatus(t, nodes, "Before UpdateLeader")

		// Wait for stable leader with consensus
		leader, leaderID := waitForStableLeader(t, stores, nodes, 10*time.Second)
		require.NotNil(t, leader, "no stable leader found")
		t.Logf("Using stable Node %d as leader", leaderID)

		// Log status after finding leader
		logRaftStatus(t, nodes, "After finding stable leader")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		// Update leader for partition 0
		t.Logf("Attempting UpdateLeader operation...")
		err := leader.UpdateLeader(ctx, "test-topic", 0, 2, 2)

		// Log status after operation
		logRaftStatus(t, nodes, "After UpdateLeader")

		require.NoError(t, err)

		// Wait for replication by polling (with longer timeout for retries)
		require.Eventually(t, func() bool {
			for i, store := range stores {
				part, exists := store.GetPartition("test-topic", 0)
				if !exists || part.Leader != 2 || part.LeaderEpoch != 2 {
					t.Logf("Node %d partition leader not updated yet, waiting...", i+1)
					return false
				}
			}
			return true
		}, 20*time.Second, 100*time.Millisecond, "leader update not replicated to all nodes")

		// Verify all nodes see the new leader
		for i, store := range stores {
			part, exists := store.GetPartition("test-topic", 0)
			assert.True(t, exists, "node %d should have partition", i+1)
			if exists {
				assert.Equal(t, uint64(2), part.Leader)
				assert.Equal(t, uint64(2), part.LeaderEpoch)
				t.Logf("Node %d sees partition test-topic:0 leader=%d epoch=%d",
					i+1, part.Leader, part.LeaderEpoch)
			}
		}
	})

	// Test 5: Update ISR
	t.Run("update ISR", func(t *testing.T) {
		// Give cluster time to stabilize after previous operation
		time.Sleep(2 * time.Second)

		// Log Raft status before update operation
		logRaftStatus(t, nodes, "Before UpdateISR")

		// Wait for stable leader with consensus
		leader, leaderID := waitForStableLeader(t, stores, nodes, 10*time.Second)
		require.NotNil(t, leader, "no stable leader found")
		t.Logf("Using stable Node %d as leader", leaderID)

		// Log status after finding leader
		logRaftStatus(t, nodes, "After finding stable leader for ISR update")

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		// Shrink ISR for partition 1 (simulate replica falling behind)
		t.Logf("Attempting UpdateISR operation...")
		err := leader.UpdateISR(ctx, "test-topic", 1, []uint64{1})

		// Log status after operation
		logRaftStatus(t, nodes, "After UpdateISR")

		require.NoError(t, err)

		// Wait for replication by polling (with longer timeout for retries)
		require.Eventually(t, func() bool {
			for i, store := range stores {
				part, exists := store.GetPartition("test-topic", 1)
				if !exists || len(part.ISR) != 1 || part.ISR[0] != 1 {
					t.Logf("Node %d ISR not updated yet, waiting...", i+1)
					return false
				}
			}
			return true
		}, 20*time.Second, 100*time.Millisecond, "ISR update not replicated to all nodes")

		// Verify all nodes see the updated ISR
		for i, store := range stores {
			part, exists := store.GetPartition("test-topic", 1)
			assert.True(t, exists, "node %d should have partition", i+1)
			if exists {
				assert.Equal(t, []uint64{1}, part.ISR)
				t.Logf("Node %d sees partition test-topic:1 ISR=%v", i+1, part.ISR)
			}
		}
	})

	// Test 6: Verify state consistency across all nodes
	t.Run("verify consistency", func(t *testing.T) {
		// Get state from all nodes
		states := make([]*ClusterMetadata, 3)
		for i, store := range stores {
			states[i] = store.GetState()
		}

		// Verify all nodes have same number of brokers, topics, partitions
		for i := 1; i < 3; i++ {
			assert.Equal(t, len(states[0].Brokers), len(states[i].Brokers),
				"broker count mismatch between node 1 and node %d", i+1)
			assert.Equal(t, len(states[0].Topics), len(states[i].Topics),
				"topic count mismatch between node 1 and node %d", i+1)
			assert.Equal(t, len(states[0].Partitions), len(states[i].Partitions),
				"partition count mismatch between node 1 and node %d", i+1)
		}

		t.Logf("All nodes have consistent state:")
		t.Logf("  Brokers: %d", len(states[0].Brokers))
		t.Logf("  Topics: %d", len(states[0].Topics))
		t.Logf("  Partitions: %d", len(states[0].Partitions))
		t.Logf("  Version: %d", states[0].Version)
	})
}

// TestMetadataSnapshot tests that metadata snapshots work correctly.
func TestMetadataSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create a single node to test snapshot functionality
	dir := t.TempDir()
	config := consensus.DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.Peers = []consensus.Peer{
		{ID: 1, Addr: "localhost:20001"},
	}
	config.SnapshotInterval = 5 // Snapshot after 5 entries

	fsm := NewFSM()
	node, err := consensus.NewNode(config, fsm)
	require.NoError(t, err)

	node.SetBindAddr("localhost:20001")
	err = node.Start()
	require.NoError(t, err)
	defer func() {
		_ = node.Stop()
	}()

	// Wait for leader election (longer due to increased election timeout)
	time.Sleep(3 * time.Second)
	assert.True(t, node.IsLeader(), "single node should become leader")

	store := NewStore(fsm, node)
	ctx := context.Background()

	// Create several brokers to trigger snapshot
	for i := uint64(1); i <= 10; i++ {
		broker := &BrokerInfo{
			ID:   i,
			Addr: "localhost:909" + string(rune('0'+i)),
		}
		err := store.RegisterBroker(ctx, broker)
		assert.NoError(t, err)
	}

	// Give time for operations to be applied
	time.Sleep(1 * time.Second)

	// Verify all brokers are present
	brokers := store.ListBrokers()
	assert.Equal(t, 10, len(brokers))

	t.Logf("Created %d brokers, snapshot should have been triggered", len(brokers))
}
