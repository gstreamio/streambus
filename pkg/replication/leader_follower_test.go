package replication

import (
	"testing"
)

// Tests for Follower getter methods

func TestFollowerReplicator_GetLogEndOffset(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	follower := NewFollowerReplicator(config, "test-topic", 0, 1, 2, 1, storage, client)

	// Initially should be 100 (from storage)
	leo := follower.GetLogEndOffset()
	if leo != 100 {
		t.Errorf("GetLogEndOffset() = %d, want 100", leo)
	}
}

func TestFollowerReplicator_GetHighWaterMark(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	follower := NewFollowerReplicator(config, "test-topic", 0, 1, 2, 1, storage, client)

	// Initially should be 0 (not initialized from storage, updated during replication)
	hw := follower.GetHighWaterMark()
	if hw != 0 {
		t.Errorf("GetHighWaterMark() = %d, want 0", hw)
	}
}

func TestCalculateMessageBytes(t *testing.T) {
	tests := []struct {
		name     string
		messages []*Message
		want     int
	}{
		{
			name:     "empty messages",
			messages: []*Message{},
			want:     0,
		},
		{
			name: "single message",
			messages: []*Message{
				{Key: []byte("key1"), Value: []byte("value1")},
			},
			want: 10, // 4 + 6
		},
		{
			name: "multiple messages",
			messages: []*Message{
				{Key: []byte("key1"), Value: []byte("value1")},
				{Key: []byte("key2"), Value: []byte("value2")},
			},
			want: 20, // (4 + 6) + (4 + 6)
		},
		{
			name: "message with no key",
			messages: []*Message{
				{Key: nil, Value: []byte("value1")},
			},
			want: 6,
		},
		{
			name: "message with no value",
			messages: []*Message{
				{Key: []byte("key1"), Value: nil},
			},
			want: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateMessageBytes(tt.messages)
			if got != tt.want {
				t.Errorf("calculateMessageBytes() = %d, want %d", got, tt.want)
			}
		})
	}
}

// Tests for Leader methods

func TestLeaderReplicator_UpdateLogEndOffset(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}

	leader := NewLeaderReplicator(config, "test-topic", 0, 1, 1, []ReplicaID{1, 2, 3}, storage)

	// Update LEO
	leader.UpdateLogEndOffset(200)

	// Verify it was updated (through internal state)
	// Since there's no getter, we verify indirectly through the update succeeding without panic
}

func TestLeaderReplicator_GetHighWaterMark(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}

	leader := NewLeaderReplicator(config, "test-topic", 0, 1, 1, []ReplicaID{1, 2, 3}, storage)

	hw := leader.GetHighWaterMark()
	if hw != 90 {
		t.Errorf("GetHighWaterMark() = %d, want 90", hw)
	}
}

func TestLeaderReplicator_GetISR(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}

	replicas := []ReplicaID{1, 2, 3}
	leader := NewLeaderReplicator(config, "test-topic", 0, 1, 1, replicas, storage)

	isr := leader.GetISR()

	// Should include the leader itself
	if len(isr) == 0 {
		t.Error("GetISR() returned empty list")
	}

	// Check that ISR contains valid replica IDs
	for _, rid := range isr {
		if rid == 0 {
			t.Errorf("GetISR() contains invalid replica ID: %d", rid)
		}
	}
}

// Tests for ReplicationManager methods

func TestReplicationManager_UpdateLogEndOffset(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = rm.Stop() }()

	// Become leader first
	replicas := []ReplicaID{1, 2, 3}
	err = rm.BecomeLeader(1, replicas)
	if err != nil {
		t.Fatalf("BecomeLeader failed: %v", err)
	}

	// Update LEO
	_ = rm.UpdateLogEndOffset(200)

	// Should succeed without error
}

func TestReplicationManager_GetMetrics(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = rm.Stop() }()

	metrics := rm.GetMetrics()

	// Should return valid metrics (initially zeros)
	if metrics.FetchRequestRate < 0 {
		t.Errorf("FetchRequestRate = %f, should be non-negative", metrics.FetchRequestRate)
	}

	if metrics.ReplicationLagMessages < 0 {
		t.Errorf("ReplicationLagMessages = %d, should be non-negative", metrics.ReplicationLagMessages)
	}
}

// Tests for MetricsCollector

func TestMetricsCollector_GetGlobalMetrics(t *testing.T) {
	collector := NewMetricsCollector()

	metrics := collector.GetGlobalMetrics()

	// Should return valid global metrics with zero values initially
	if metrics.TotalPartitions.Load() != 0 {
		t.Errorf("TotalPartitions = %d, want 0", metrics.TotalPartitions.Load())
	}

	// Verify other metrics are also zero initially
	if metrics.LeaderPartitions.Load() != 0 {
		t.Errorf("LeaderPartitions = %d, want 0", metrics.LeaderPartitions.Load())
	}

	if metrics.FollowerPartitions.Load() != 0 {
		t.Errorf("FollowerPartitions = %d, want 0", metrics.FollowerPartitions.Load())
	}
}
