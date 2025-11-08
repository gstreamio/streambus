package replication

import (
	"context"
	"testing"
	"time"
)

// Mock storage adapter for testing
type mockStorageAdapter struct {
	leo Offset
	hw  Offset
}

func (m *mockStorageAdapter) ReadRange(ctx context.Context, partition int, start, end Offset) ([]*StorageMessage, error) {
	return nil, nil
}

func (m *mockStorageAdapter) GetLogEndOffset(partition int) (Offset, error) {
	return m.leo, nil
}

func (m *mockStorageAdapter) GetHighWaterMark(partition int) (Offset, error) {
	return m.hw, nil
}

func (m *mockStorageAdapter) AppendMessages(ctx context.Context, partition int, messages []*StorageMessage) error {
	return nil
}

func (m *mockStorageAdapter) SetHighWaterMark(partition int, hw Offset) error {
	m.hw = hw
	return nil
}

func (m *mockStorageAdapter) Truncate(partition int, offset Offset) error {
	m.leo = offset
	return nil
}

// Mock leader client for testing
type mockLeaderClient struct{}

func (m *mockLeaderClient) Fetch(ctx context.Context, req *FetchRequest) (*FetchResponse, error) {
	return &FetchResponse{
		Topic:       req.Topic,
		PartitionID: req.PartitionID,
		ErrorCode:   ErrorNone,
	}, nil
}

func (m *mockLeaderClient) GetLeaderEndpoint(topic string, partitionID int) (string, error) {
	return "localhost:9092", nil
}

func TestReplicationManager_Create(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 0, hw: 0}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	if rm == nil {
		t.Fatal("NewReplicationManager returned nil")
	}

	if rm.topic != "test-topic" {
		t.Errorf("topic = %s, want test-topic", rm.topic)
	}
	if rm.partitionID != 0 {
		t.Errorf("partitionID = %d, want 0", rm.partitionID)
	}
	if rm.replicaID != 1 {
		t.Errorf("replicaID = %d, want 1", rm.replicaID)
	}
	if rm.role != RoleFollower {
		t.Errorf("initial role should be RoleFollower")
	}
}

func TestReplicationManager_BecomeLeader(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer rm.Stop()

	// Become leader
	replicas := []ReplicaID{1, 2, 3}
	err = rm.BecomeLeader(1, replicas)
	if err != nil {
		t.Fatalf("BecomeLeader failed: %v", err)
	}

	if !rm.IsLeader() {
		t.Error("Should be leader after BecomeLeader")
	}

	// Verify leader replicator is active
	rm.mu.RLock()
	if rm.leaderRep == nil {
		t.Error("leaderRep should not be nil")
	}
	if rm.followerRep != nil {
		t.Error("followerRep should be nil")
	}
	rm.mu.RUnlock()
}

func TestReplicationManager_BecomeFollower(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 2, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer rm.Stop()

	// Become follower
	err = rm.BecomeFollower(1, 1)
	if err != nil {
		t.Fatalf("BecomeFollower failed: %v", err)
	}

	if rm.IsLeader() {
		t.Error("Should not be leader after BecomeFollower")
	}

	// Verify follower replicator is active
	rm.mu.RLock()
	if rm.followerRep == nil {
		t.Error("followerRep should not be nil")
	}
	if rm.leaderRep != nil {
		t.Error("leaderRep should be nil")
	}
	rm.mu.RUnlock()
}

func TestReplicationManager_LeaderFollowerTransition(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer rm.Stop()

	// Start as follower
	err = rm.BecomeFollower(2, 1)
	if err != nil {
		t.Fatalf("BecomeFollower failed: %v", err)
	}

	if rm.IsLeader() {
		t.Error("Should be follower")
	}

	// Transition to leader
	err = rm.BecomeLeader(2, []ReplicaID{1, 2, 3})
	if err != nil {
		t.Fatalf("BecomeLeader failed: %v", err)
	}

	if !rm.IsLeader() {
		t.Error("Should be leader")
	}

	// Verify old follower stopped, new leader started
	rm.mu.RLock()
	hasLeader := rm.leaderRep != nil
	hasFollower := rm.followerRep != nil
	rm.mu.RUnlock()

	if !hasLeader {
		t.Error("Should have leader replicator")
	}
	if hasFollower {
		t.Error("Should not have follower replicator")
	}
}

func TestReplicationManager_WaitForISR(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer rm.Stop()

	// Become leader
	err = rm.BecomeLeader(1, []ReplicaID{1, 2, 3})
	if err != nil {
		t.Fatalf("BecomeLeader failed: %v", err)
	}

	// Test waiting for already replicated offset
	ctx := context.Background()
	err = rm.WaitForISR(ctx, 85, 1000) // offset < HW
	if err != nil {
		t.Errorf("WaitForISR for already replicated offset should succeed: %v", err)
	}

	// Test timeout for unreplicated offset
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = rm.WaitForISR(ctx, 95, 100) // offset > HW
	if err == nil {
		t.Error("WaitForISR should timeout for unreplicated offset")
	}
}

func TestReplicationManager_NotifyReplication(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer rm.Stop()

	// Become leader
	err = rm.BecomeLeader(1, []ReplicaID{1, 2, 3})
	if err != nil {
		t.Fatalf("BecomeLeader failed: %v", err)
	}

	// Simulate waiting for replication in background
	doneCh := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		doneCh <- rm.WaitForISR(ctx, 95, 2000)
	}()

	// Give waiter time to register
	time.Sleep(50 * time.Millisecond)

	// Advance HW
	rm.mu.Lock()
	if rm.leaderRep != nil {
		rm.leaderRep.highWaterMark = 100
	}
	rm.mu.Unlock()

	// Notify replication
	rm.NotifyReplication()

	// Wait should complete successfully
	select {
	case err := <-doneCh:
		if err != nil {
			t.Errorf("WaitForISR should succeed after HW advancement: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("WaitForISR did not complete after notification")
	}
}

func TestReplicationManager_GetHighWaterMark(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer rm.Stop()

	// Become leader
	err = rm.BecomeLeader(1, []ReplicaID{1, 2, 3})
	if err != nil {
		t.Fatalf("BecomeLeader failed: %v", err)
	}

	hw, err := rm.GetHighWaterMark()
	if err != nil {
		t.Errorf("GetHighWaterMark failed: %v", err)
	}

	if hw != 90 {
		t.Errorf("HW = %d, want 90", hw)
	}
}

func TestReplicationManager_GetISR(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer rm.Stop()

	// Become leader
	replicas := []ReplicaID{1, 2, 3}
	err = rm.BecomeLeader(1, replicas)
	if err != nil {
		t.Fatalf("BecomeLeader failed: %v", err)
	}

	isr, err := rm.GetISR()
	if err != nil {
		t.Errorf("GetISR failed: %v", err)
	}

	if len(isr) != 3 {
		t.Errorf("ISR length = %d, want 3", len(isr))
	}
}
