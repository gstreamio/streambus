package consensus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRapidProposals tests stability under rapid proposal load.
// This test reproduces the issue seen in metadata integration tests.
func TestRapidProposals(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create 3 nodes
	nodes := make([]*RaftNode, 3)
	peers := []Peer{
		{ID: 1, Addr: "localhost:19501"},
		{ID: 2, Addr: "localhost:19502"},
		{ID: 3, Addr: "localhost:19503"},
	}

	// Use simple mock state machine
	sm := &mockStateMachine{}

	// Initialize nodes
	for i := 0; i < 3; i++ {
		dir := t.TempDir()
		config := DefaultConfig()
		config.NodeID = uint64(i + 1)
		config.DataDir = dir
		config.Peers = peers

		node, err := NewNode(config, sm)
		require.NoError(t, err)

		node.SetBindAddr(peers[i].Addr)
		nodes[i] = node
	}

	// Start all nodes
	for i, node := range nodes {
		err := node.Start()
		require.NoError(t, err, "node %d failed to start", i+1)
		defer func(n *RaftNode) { _ = n.Stop() }(node)
	}

	// Wait for leader election
	leaderID, err := waitForLeader(nodes, 5*time.Second)
	require.NoError(t, err, "should elect a leader")
	t.Logf("Initial leader is Node %d", leaderID)

	// Find leader node
	var leaderNode *RaftNode
	for _, node := range nodes {
		if node.IsLeader() {
			leaderNode = node
			break
		}
	}
	require.NotNil(t, leaderNode, "should find leader node")

	// Test 1: Propose 5 entries rapidly (like creating partitions)
	t.Run("rapid proposals", func(t *testing.T) {
		proposalCount := 5
		successCount := 0

		for i := 0; i < proposalCount; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			data := []byte("test data " + string(rune('0'+i)))

			err := leaderNode.Propose(ctx, data)
			cancel()

			if err == nil {
				successCount++
				t.Logf("Proposal %d/%d succeeded", i+1, proposalCount)
			} else {
				t.Logf("Proposal %d/%d failed: %v", i+1, proposalCount, err)
			}

			// Small delay between proposals to simulate real workload
			time.Sleep(100 * time.Millisecond)
		}

		t.Logf("Successfully proposed %d/%d entries", successCount, proposalCount)
		require.Equal(t, proposalCount, successCount, "all proposals should succeed")
	})

	// Give time for replication
	time.Sleep(2 * time.Second)

	// Test 2: Verify cluster is still stable
	t.Run("cluster stability", func(t *testing.T) {
		// Count leaders
		leaderCount := 0
		var currentLeaderID uint64
		for _, node := range nodes {
			if node.IsLeader() {
				leaderCount++
				currentLeaderID = node.Leader()
			}
		}

		require.Equal(t, 1, leaderCount, "should have exactly one leader")
		t.Logf("Cluster stable with Node %d as leader", currentLeaderID)
	})

	// Test 3: Try more proposals after initial batch
	t.Run("follow-up proposals", func(t *testing.T) {
		// Re-find leader (may have changed)
		var currentLeader *RaftNode
		for _, node := range nodes {
			if node.IsLeader() {
				currentLeader = node
				break
			}
		}
		require.NotNil(t, currentLeader, "should have a leader")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := currentLeader.Propose(ctx, []byte("follow-up proposal"))
		require.NoError(t, err, "follow-up proposal should succeed")

		t.Log("Follow-up proposal succeeded")
	})

	// Final check
	time.Sleep(1 * time.Second)
	t.Log("Test completed successfully")
}
