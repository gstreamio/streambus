package replication

import (
	"testing"
)

func TestReplicationManager_BecomeLeader_FromFollower(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = rm.Stop() }()

	// First become follower
	err = rm.BecomeFollower(2, 1)
	if err != nil {
		t.Fatalf("BecomeFollower failed: %v", err)
	}

	// Verify we're follower
	if rm.IsLeader() {
		t.Fatal("Should be follower")
	}

	// Now become leader (should stop follower replicator)
	replicas := []ReplicaID{1, 2, 3}
	err = rm.BecomeLeader(2, replicas)
	if err != nil {
		t.Fatalf("BecomeLeader failed: %v", err)
	}

	// Verify we're now leader
	if !rm.IsLeader() {
		t.Error("Should be leader after BecomeLeader")
	}

	// Verify follower replicator was stopped
	rm.mu.RLock()
	if rm.followerRep != nil {
		t.Error("followerRep should be nil")
	}
	if rm.leaderRep == nil {
		t.Error("leaderRep should not be nil")
	}
	rm.mu.RUnlock()
}

func TestReplicationManager_UpdateLogEndOffset_NotLeader(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = rm.Stop() }()

	// Try to update LEO when not leader
	err = rm.UpdateLogEndOffset(150)
	if err == nil {
		t.Error("Expected error when not leader")
	}
}

func TestReplicationManager_NotifyReplication_NotLeader(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = rm.Stop() }()

	// Call NotifyReplication when not leader (should just return)
	rm.NotifyReplication()
	// If we get here without panic, test passes
}

func TestReplicationManager_GetISR_NotLeader(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = rm.Stop() }()

	// Try to get ISR when not leader
	_, err = rm.GetISR()
	if err == nil {
		t.Error("Expected error when not leader")
	}
}

func TestReplicationManager_GetHighWaterMark_NotLeader(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = rm.Stop() }()

	// Try to get HW when not leader or follower
	_, err = rm.GetHighWaterMark()
	if err == nil {
		t.Error("Expected error when not leader or follower")
	}
}

func TestReplicationManager_HandleFetchRequest_NotLeader(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = rm.Stop() }()

	// Create fetch request
	req := &FetchRequest{
		Topic:       "test-topic",
		PartitionID: 0,
		ReplicaID:   2,
		FetchOffset: 0,
		MaxBytes:    1024,
		LeaderEpoch: 1,
	}

	// Try to handle fetch when not leader
	resp, err := rm.HandleFetchRequest(req)
	if err != nil {
		t.Fatalf("HandleFetchRequest failed: %v", err)
	}

	// Should return NotLeader error
	if resp.ErrorCode != ErrorNotLeader {
		t.Errorf("ErrorCode = %d, want ErrorNotLeader (%d)", resp.ErrorCode, ErrorNotLeader)
	}
}
