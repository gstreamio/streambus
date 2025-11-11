package replication

import (
	"context"
	"testing"
	"time"
)

// Enhanced mock storage that can return messages
type mockStorageWithMessages struct {
	mockStorageAdapter
	messages []*StorageMessage
}

func (m *mockStorageWithMessages) ReadRange(ctx context.Context, partition int, start, end Offset) ([]*StorageMessage, error) {
	result := make([]*StorageMessage, 0)
	for _, msg := range m.messages {
		if msg.Offset >= start && msg.Offset < end {
			result = append(result, msg)
		}
	}
	return result, nil
}

func TestLeaderReplicator_HandleFetchRequest_WrongPartition(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}

	leader := NewLeaderReplicator(config, "test-topic", 0, 1, 1, []ReplicaID{1, 2, 3}, storage)

	req := &FetchRequest{
		Topic:       "wrong-topic",
		PartitionID: 0,
		ReplicaID:   2,
		FetchOffset: 0,
		MaxBytes:    1024,
		LeaderEpoch: 1,
	}

	resp, err := leader.HandleFetchRequest(req)
	if err != nil {
		t.Fatalf("HandleFetchRequest failed: %v", err)
	}

	if resp.ErrorCode != ErrorPartitionNotFound {
		t.Errorf("ErrorCode = %d, want %d", resp.ErrorCode, ErrorPartitionNotFound)
	}
}

func TestLeaderReplicator_HandleFetchRequest_ReplicaNotInSet(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}

	leader := NewLeaderReplicator(config, "test-topic", 0, 1, 1, []ReplicaID{1, 2, 3}, storage)

	req := &FetchRequest{
		Topic:       "test-topic",
		PartitionID: 0,
		ReplicaID:   99, // Not in replica set
		FetchOffset: 0,
		MaxBytes:    1024,
		LeaderEpoch: 1,
	}

	resp, err := leader.HandleFetchRequest(req)
	if err != nil {
		t.Fatalf("HandleFetchRequest failed: %v", err)
	}

	if resp.ErrorCode != ErrorReplicaNotInReplicas {
		t.Errorf("ErrorCode = %d, want %d", resp.ErrorCode, ErrorReplicaNotInReplicas)
	}
}

func TestLeaderReplicator_HandleFetchRequest_StaleEpoch(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}

	leader := NewLeaderReplicator(config, "test-topic", 0, 1, 1, []ReplicaID{1, 2, 3}, storage)

	req := &FetchRequest{
		Topic:       "test-topic",
		PartitionID: 0,
		ReplicaID:   2,
		FetchOffset: 0,
		MaxBytes:    1024,
		LeaderEpoch: 0, // Stale epoch (leader is at 1)
	}

	resp, err := leader.HandleFetchRequest(req)
	if err != nil {
		t.Fatalf("HandleFetchRequest failed: %v", err)
	}

	if resp.ErrorCode != ErrorStaleEpoch {
		t.Errorf("ErrorCode = %d, want %d", resp.ErrorCode, ErrorStaleEpoch)
	}

	if resp.LeaderEpoch != 1 {
		t.Errorf("LeaderEpoch = %d, want 1", resp.LeaderEpoch)
	}
}

func TestLeaderReplicator_HandleFetchRequest_OffsetOutOfRange(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}

	leader := NewLeaderReplicator(config, "test-topic", 0, 1, 1, []ReplicaID{1, 2, 3}, storage)

	req := &FetchRequest{
		Topic:       "test-topic",
		PartitionID: 0,
		ReplicaID:   2,
		FetchOffset: 200, // Beyond LEO
		MaxBytes:    1024,
		LeaderEpoch: 1,
	}

	resp, err := leader.HandleFetchRequest(req)
	if err != nil {
		t.Fatalf("HandleFetchRequest failed: %v", err)
	}

	if resp.ErrorCode != ErrorOffsetOutOfRange {
		t.Errorf("ErrorCode = %d, want %d", resp.ErrorCode, ErrorOffsetOutOfRange)
	}

	if resp.LogEndOffset != 100 {
		t.Errorf("LogEndOffset = %d, want 100", resp.LogEndOffset)
	}
}

func TestLeaderReplicator_HandleFetchRequest_CaughtUp(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}

	leader := NewLeaderReplicator(config, "test-topic", 0, 1, 1, []ReplicaID{1, 2, 3}, storage)

	req := &FetchRequest{
		Topic:       "test-topic",
		PartitionID: 0,
		ReplicaID:   2,
		FetchOffset: 100, // At LEO (caught up)
		MaxBytes:    1024,
		LeaderEpoch: 1,
	}

	resp, err := leader.HandleFetchRequest(req)
	if err != nil {
		t.Fatalf("HandleFetchRequest failed: %v", err)
	}

	if resp.ErrorCode != ErrorNone {
		t.Errorf("ErrorCode = %d, want %d", resp.ErrorCode, ErrorNone)
	}

	if resp.Messages != nil {
		t.Errorf("Messages should be nil for caught-up replica")
	}

	if resp.HighWaterMark != 90 {
		t.Errorf("HighWaterMark = %d, want 90", resp.HighWaterMark)
	}
}

func TestLeaderReplicator_HandleFetchRequest_Success(t *testing.T) {
	config := DefaultConfig()

	// Create storage with test messages
	storage := &mockStorageWithMessages{
		mockStorageAdapter: mockStorageAdapter{leo: 100, hw: 90},
		messages: []*StorageMessage{
			{
				Offset:    10,
				Key:       []byte("key1"),
				Value:     []byte("value1"),
				Timestamp: time.Now().UnixNano(),
			},
			{
				Offset:    11,
				Key:       []byte("key2"),
				Value:     []byte("value2"),
				Timestamp: time.Now().UnixNano(),
			},
		},
	}

	leader := NewLeaderReplicator(config, "test-topic", 0, 1, 1, []ReplicaID{1, 2, 3}, storage)

	req := &FetchRequest{
		Topic:       "test-topic",
		PartitionID: 0,
		ReplicaID:   2,
		FetchOffset: 10,
		MaxBytes:    1024,
		LeaderEpoch: 1,
	}

	resp, err := leader.HandleFetchRequest(req)
	if err != nil {
		t.Fatalf("HandleFetchRequest failed: %v", err)
	}

	if resp.ErrorCode != ErrorNone {
		t.Errorf("ErrorCode = %d, want %d", resp.ErrorCode, ErrorNone)
	}

	// Due to offset calculation (FetchOffset + MaxBytes/1024), only messages in [10, 11) are fetched
	// which is just message at offset 10
	if len(resp.Messages) != 1 {
		t.Errorf("Message count = %d, want 1", len(resp.Messages))
	}

	if resp.HighWaterMark != 90 {
		t.Errorf("HighWaterMark = %d, want 90", resp.HighWaterMark)
	}
}

func TestLeaderReplicator_GetMetrics(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}

	leader := NewLeaderReplicator(config, "test-topic", 0, 1, 1, []ReplicaID{1, 2, 3}, storage)

	metrics := leader.GetMetrics()

	// Should return valid metrics (initially zeros)
	if metrics.FetchRequestRate < 0 {
		t.Errorf("FetchRequestRate = %f, should be non-negative", metrics.FetchRequestRate)
	}

	if metrics.FetchBytesRate < 0 {
		t.Errorf("FetchBytesRate = %f, should be non-negative", metrics.FetchBytesRate)
	}
}

func TestLeaderReplicator_GetPartitionState(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}

	leader := NewLeaderReplicator(config, "test-topic", 0, 1, 1, []ReplicaID{1, 2, 3}, storage)

	state := leader.GetPartitionState()

	if state.Topic != "test-topic" {
		t.Errorf("Topic = %s, want test-topic", state.Topic)
	}

	if state.PartitionID != 0 {
		t.Errorf("PartitionID = %d, want 0", state.PartitionID)
	}

	if state.LeaderEpoch != 1 {
		t.Errorf("LeaderEpoch = %d, want 1", state.LeaderEpoch)
	}

	if state.LogEndOffset != 100 {
		t.Errorf("LogEndOffset = %d, want 100", state.LogEndOffset)
	}

	if state.HighWaterMark != 90 {
		t.Errorf("HighWaterMark = %d, want 90", state.HighWaterMark)
	}
}

func TestLeaderReplicator_ISRChangeChannel(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}

	leader := NewLeaderReplicator(config, "test-topic", 0, 1, 1, []ReplicaID{1, 2, 3}, storage)

	ch := leader.ISRChangeChannel()
	if ch == nil {
		t.Error("ISRChangeChannel should not return nil")
	}
}

func TestManagerHandleFetchRequest(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer rm.Stop()

	// Become leader first
	replicas := []ReplicaID{1, 2, 3}
	err = rm.BecomeLeader(1, replicas)
	if err != nil {
		t.Fatalf("BecomeLeader failed: %v", err)
	}

	// Create fetch request
	req := &FetchRequest{
		Topic:       "test-topic",
		PartitionID: 0,
		ReplicaID:   2,
		FetchOffset: 0,
		MaxBytes:    1024,
		LeaderEpoch: 1,
	}

	resp, err := rm.HandleFetchRequest(req)
	if err != nil {
		t.Fatalf("HandleFetchRequest failed: %v", err)
	}

	if resp.ErrorCode != ErrorNone {
		t.Errorf("ErrorCode = %d, want %d", resp.ErrorCode, ErrorNone)
	}
}
