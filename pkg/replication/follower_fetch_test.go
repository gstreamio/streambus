package replication

import (
	"testing"
	"time"
)

func TestFollowerReplicator_handleFetchResponse_Success(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	follower := NewFollowerReplicator(config, "test-topic", 0, 1, 2, 1, storage, client)

	resp := &FetchResponse{
		ErrorCode:     ErrorNone,
		LeaderEpoch:   1,
		HighWaterMark: 95,
		LogEndOffset:  105,
		Messages: []*Message{
			{Offset: 100, Key: []byte("key1"), Value: []byte("value1"), Timestamp: time.Now().UnixNano()},
			{Offset: 101, Key: []byte("key2"), Value: []byte("value2"), Timestamp: time.Now().UnixNano()},
		},
	}

	err := follower.handleFetchResponse(resp, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("handleFetchResponse failed: %v", err)
	}

	// Verify LEO was updated
	if follower.logEndOffset != 102 {
		t.Errorf("logEndOffset = %d, want 102", follower.logEndOffset)
	}

	// Verify HW was updated
	if follower.highWaterMark != 95 {
		t.Errorf("highWaterMark = %d, want 95", follower.highWaterMark)
	}
}

func TestFollowerReplicator_handleFetchResponse_EpochChange(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	follower := NewFollowerReplicator(config, "test-topic", 0, 1, 2, 1, storage, client)

	// Response with higher epoch
	resp := &FetchResponse{
		ErrorCode:     ErrorNone,
		LeaderEpoch:   5,  // Higher than current epoch (1)
		HighWaterMark: 95,
		LogEndOffset:  105,
		Messages:      nil,
	}

	err := follower.handleFetchResponse(resp, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("handleFetchResponse failed: %v", err)
	}

	// Verify epoch was updated
	if follower.leaderEpoch != 5 {
		t.Errorf("leaderEpoch = %d, want 5", follower.leaderEpoch)
	}

	// Verify storage was truncated (called with current LEO)
	if storage.leo != 100 {
		t.Errorf("storage leo = %d, want 100", storage.leo)
	}
}

func TestFollowerReplicator_handleFetchResponse_HWUpdate(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	follower := NewFollowerReplicator(config, "test-topic", 0, 1, 2, 1, storage, client)

	// Response with higher HW
	resp := &FetchResponse{
		ErrorCode:     ErrorNone,
		LeaderEpoch:   1,
		HighWaterMark: 110,  // Higher than current HW (0, since not initialized from storage)
		LogEndOffset:  120,
		Messages:      nil,
	}

	err := follower.handleFetchResponse(resp, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("handleFetchResponse failed: %v", err)
	}

	// Verify HW was updated
	if follower.highWaterMark != 110 {
		t.Errorf("highWaterMark = %d, want 110", follower.highWaterMark)
	}

	// Verify storage HW was updated
	if storage.hw != 110 {
		t.Errorf("storage hw = %d, want 110", storage.hw)
	}
}

func TestFollowerReplicator_handleFetchResponse_EmptyMessages(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	follower := NewFollowerReplicator(config, "test-topic", 0, 1, 2, 1, storage, client)

	// Response with no messages (caught up)
	resp := &FetchResponse{
		ErrorCode:     ErrorNone,
		LeaderEpoch:   1,
		HighWaterMark: 95,
		LogEndOffset:  100,
		Messages:      nil,
	}

	err := follower.handleFetchResponse(resp, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("handleFetchResponse failed: %v", err)
	}

	// Verify fetch lag was calculated
	if follower.fetchLag != 0 {
		t.Errorf("fetchLag = %d, want 0", follower.fetchLag)
	}
}

func TestFollowerReplicator_handleFetchResponse_WithError(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	follower := NewFollowerReplicator(config, "test-topic", 0, 1, 2, 1, storage, client)

	// Response with error
	resp := &FetchResponse{
		ErrorCode: ErrorStaleEpoch,
		LeaderEpoch: 5,
	}

	err := follower.handleFetchResponse(resp, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("handleFetchResponse should handle error via handleFetchError: %v", err)
	}

	// Verify epoch was updated by handleFetchError
	if follower.leaderEpoch != 5 {
		t.Errorf("leaderEpoch = %d, want 5", follower.leaderEpoch)
	}
}
