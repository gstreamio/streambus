package replication

import (
	"testing"
)

func TestFollowerReplicator_handleFetchError_StaleEpoch(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	follower := NewFollowerReplicator(config, "test-topic", 0, 1, 2, 1, storage, client)

	resp := &FetchResponse{
		ErrorCode:   ErrorStaleEpoch,
		LeaderEpoch: 5,
	}

	err := follower.handleFetchError(resp)
	if err != nil {
		t.Fatalf("handleFetchError failed: %v", err)
	}

	// Verify epoch was updated
	if follower.leaderEpoch != 5 {
		t.Errorf("leaderEpoch = %d, want 5", follower.leaderEpoch)
	}
}

func TestFollowerReplicator_handleFetchError_OffsetOutOfRange(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	follower := NewFollowerReplicator(config, "test-topic", 0, 1, 2, 1, storage, client)

	resp := &FetchResponse{
		ErrorCode: ErrorOffsetOutOfRange,
	}

	err := follower.handleFetchError(resp)
	if err != nil {
		t.Fatalf("handleFetchError failed: %v", err)
	}

	// Verify offset was reset
	if follower.logEndOffset != 0 {
		t.Errorf("logEndOffset = %d, want 0", follower.logEndOffset)
	}

	// Verify storage was truncated
	if storage.leo != 0 {
		t.Errorf("storage leo = %d, want 0 (after truncate)", storage.leo)
	}
}

func TestFollowerReplicator_handleFetchError_NotLeader(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	follower := NewFollowerReplicator(config, "test-topic", 0, 1, 2, 1, storage, client)

	resp := &FetchResponse{
		ErrorCode: ErrorNotLeader,
	}

	err := follower.handleFetchError(resp)
	if err == nil {
		t.Error("Expected error for NotLeader")
	}
}

func TestFollowerReplicator_handleFetchError_Unknown(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	follower := NewFollowerReplicator(config, "test-topic", 0, 1, 2, 1, storage, client)

	resp := &FetchResponse{
		ErrorCode: ErrorUnknown,
	}

	err := follower.handleFetchError(resp)
	if err == nil {
		t.Error("Expected error for unknown error code")
	}
}

func TestFollowerReplicator_UpdateLeader_Success(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	follower := NewFollowerReplicator(config, "test-topic", 0, 1, 2, 1, storage, client)

	// Update to higher epoch
	err := follower.UpdateLeader(3, 5)
	if err != nil {
		t.Fatalf("UpdateLeader failed: %v", err)
	}

	if follower.leaderID != 3 {
		t.Errorf("leaderID = %d, want 3", follower.leaderID)
	}

	if follower.leaderEpoch != 5 {
		t.Errorf("leaderEpoch = %d, want 5", follower.leaderEpoch)
	}
}

func TestFollowerReplicator_UpdateLeader_StaleEpoch(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	follower := NewFollowerReplicator(config, "test-topic", 0, 1, 2, 5, storage, client)

	// Try to update to older epoch
	err := follower.UpdateLeader(3, 2)
	if err == nil {
		t.Error("Expected error when updating to stale epoch")
	}

	// Leader should not change
	if follower.leaderID != 2 {
		t.Errorf("leaderID = %d, want 2", follower.leaderID)
	}

	if follower.leaderEpoch != 5 {
		t.Errorf("leaderEpoch = %d, want 5", follower.leaderEpoch)
	}
}
