package replication

import (
	"testing"
	"time"
)

func TestReplicationManager_BecomeFollower_FromLeader(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer rm.Stop()

	// First become leader
	replicas := []ReplicaID{1, 2, 3}
	err = rm.BecomeLeader(1, replicas)
	if err != nil {
		t.Fatalf("BecomeLeader failed: %v", err)
	}

	// Verify we're leader
	if !rm.IsLeader() {
		t.Fatal("Should be leader")
	}

	// Add a pending replication waiter
	go func() {
		// This will block and should be cancelled when we become follower
		ctx := rm.ctx
		_ = rm.WaitForISR(ctx, 150, 5000)
	}()

	// Give the goroutine time to register the waiter
	time.Sleep(50 * time.Millisecond)

	// Now become follower (should stop leader replicator and cancel waiters)
	err = rm.BecomeFollower(2, 2)
	if err != nil {
		t.Fatalf("BecomeFollower failed: %v", err)
	}

	// Verify we're now follower
	if rm.IsLeader() {
		t.Error("Should not be leader after BecomeFollower")
	}

	// Verify leader replicator was stopped
	rm.mu.RLock()
	if rm.leaderRep != nil {
		t.Error("leaderRep should be nil")
	}
	if rm.followerRep == nil {
		t.Error("followerRep should not be nil")
	}
	// Verify waiters were cleared
	if len(rm.replicationWaiters) != 0 {
		t.Errorf("replicationWaiters count = %d, want 0", len(rm.replicationWaiters))
	}
	rm.mu.RUnlock()
}

func TestReplicationManager_GetHighWaterMark_AsFollower(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer rm.Stop()

	// Become follower
	err = rm.BecomeFollower(2, 1)
	if err != nil {
		t.Fatalf("BecomeFollower failed: %v", err)
	}

	// Get HW as follower
	hw, err := rm.GetHighWaterMark()
	if err != nil {
		t.Fatalf("GetHighWaterMark failed: %v", err)
	}

	// Should return follower's HW (initially 0 since not initialized from storage)
	if hw < 0 {
		t.Errorf("GetHighWaterMark = %d, should be non-negative", hw)
	}
}

func TestReplicationManager_GetMetrics_AsFollower(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer rm.Stop()

	// Become follower
	err = rm.BecomeFollower(2, 1)
	if err != nil {
		t.Fatalf("BecomeFollower failed: %v", err)
	}

	// Get metrics as follower
	metrics := rm.GetMetrics()

	// Should return valid metrics
	if metrics.FetchRequestRate < 0 {
		t.Errorf("FetchRequestRate = %f, should be non-negative", metrics.FetchRequestRate)
	}
}

func TestReplicationManager_GetMetrics_NoRole(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer rm.Stop()

	// Get metrics without being leader or follower
	metrics := rm.GetMetrics()

	// Should return valid (empty) metrics
	if metrics.FetchRequestRate < 0 {
		t.Errorf("FetchRequestRate = %f, should be non-negative", metrics.FetchRequestRate)
	}
}
