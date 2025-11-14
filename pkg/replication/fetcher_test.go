package replication

import (
	"testing"
)

func TestNewFetcher(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 0, hw: 0}
	client := &mockLeaderClient{}

	fetcher := NewFetcher(config, 1, storage, client)
	if fetcher == nil {
		t.Fatal("NewFetcher returned nil")
	}

	if fetcher.replicaID != 1 {
		t.Errorf("replicaID = %d, want 1", fetcher.replicaID)
	}

	if fetcher.config != config {
		t.Error("config not set correctly")
	}

	if fetcher.replicators == nil {
		t.Error("replicators map not initialized")
	}

	if len(fetcher.replicators) != 0 {
		t.Errorf("replicators count = %d, want 0", len(fetcher.replicators))
	}
}

func TestFetcher_StartStop(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 0, hw: 0}
	client := &mockLeaderClient{}

	fetcher := NewFetcher(config, 1, storage, client)

	// Start
	err := fetcher.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Stop
	err = fetcher.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestFetcher_AddPartition(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 0, hw: 0}
	client := &mockLeaderClient{}

	fetcher := NewFetcher(config, 1, storage, client)

	// Add partition
	err := fetcher.AddPartition("test-topic", 0, 2, 1)
	if err != nil {
		t.Fatalf("AddPartition failed: %v", err)
	}

	// Verify partition was added
	if fetcher.GetPartitionCount() != 1 {
		t.Errorf("partition count = %d, want 1", fetcher.GetPartitionCount())
	}

	// Try to add same partition again
	err = fetcher.AddPartition("test-topic", 0, 2, 1)
	if err == nil {
		t.Error("Expected error when adding duplicate partition")
	}
}

func TestFetcher_RemovePartition(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 0, hw: 0}
	client := &mockLeaderClient{}

	fetcher := NewFetcher(config, 1, storage, client)

	// Add partition
	err := fetcher.AddPartition("test-topic", 0, 2, 1)
	if err != nil {
		t.Fatalf("AddPartition failed: %v", err)
	}

	// Remove partition
	err = fetcher.RemovePartition("test-topic", 0)
	if err != nil {
		t.Fatalf("RemovePartition failed: %v", err)
	}

	// Verify partition was removed
	if fetcher.GetPartitionCount() != 0 {
		t.Errorf("partition count = %d, want 0", fetcher.GetPartitionCount())
	}

	// Try to remove non-existent partition
	err = fetcher.RemovePartition("test-topic", 0)
	if err == nil {
		t.Error("Expected error when removing non-existent partition")
	}
}

func TestFetcher_UpdateLeader(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 0, hw: 0}
	client := &mockLeaderClient{}

	fetcher := NewFetcher(config, 1, storage, client)

	// Add partition
	err := fetcher.AddPartition("test-topic", 0, 2, 1)
	if err != nil {
		t.Fatalf("AddPartition failed: %v", err)
	}

	// Update leader
	err = fetcher.UpdateLeader("test-topic", 0, 3, 2)
	if err != nil {
		t.Fatalf("UpdateLeader failed: %v", err)
	}

	// Try to update non-existent partition
	err = fetcher.UpdateLeader("test-topic", 1, 3, 2)
	if err == nil {
		t.Error("Expected error when updating non-existent partition")
	}
}

func TestFetcher_GetReplicationLag(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 0, hw: 0}
	client := &mockLeaderClient{}

	fetcher := NewFetcher(config, 1, storage, client)

	// Add partition
	err := fetcher.AddPartition("test-topic", 0, 2, 1)
	if err != nil {
		t.Fatalf("AddPartition failed: %v", err)
	}

	// Get replication lag
	lag, err := fetcher.GetReplicationLag("test-topic", 0)
	if err != nil {
		t.Fatalf("GetReplicationLag failed: %v", err)
	}

	// Lag should be 0 initially
	if lag != 0 {
		t.Errorf("lag = %d, want 0", lag)
	}

	// Try to get lag for non-existent partition
	_, err = fetcher.GetReplicationLag("test-topic", 1)
	if err == nil {
		t.Error("Expected error when getting lag for non-existent partition")
	}
}

func TestFetcher_GetAllMetrics(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 0, hw: 0}
	client := &mockLeaderClient{}

	fetcher := NewFetcher(config, 1, storage, client)

	// Add multiple partitions
	err := fetcher.AddPartition("test-topic", 0, 2, 1)
	if err != nil {
		t.Fatalf("AddPartition failed: %v", err)
	}

	err = fetcher.AddPartition("test-topic", 1, 2, 1)
	if err != nil {
		t.Fatalf("AddPartition failed: %v", err)
	}

	// Get all metrics
	metrics := fetcher.GetAllMetrics()
	if len(metrics) != 2 {
		t.Errorf("metrics count = %d, want 2", len(metrics))
	}

	// Verify keys
	if _, exists := metrics["test-topic:0"]; !exists {
		t.Error("Expected metrics for test-topic:0")
	}

	if _, exists := metrics["test-topic:1"]; !exists {
		t.Error("Expected metrics for test-topic:1")
	}
}

func TestFetcher_GetPartitionCount(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 0, hw: 0}
	client := &mockLeaderClient{}

	fetcher := NewFetcher(config, 1, storage, client)

	// Initially 0
	if count := fetcher.GetPartitionCount(); count != 0 {
		t.Errorf("initial count = %d, want 0", count)
	}

	// Add partitions
	_ = fetcher.AddPartition("test-topic", 0, 2, 1)
	if count := fetcher.GetPartitionCount(); count != 1 {
		t.Errorf("count after 1 add = %d, want 1", count)
	}

	_ = fetcher.AddPartition("test-topic", 1, 2, 1)
	if count := fetcher.GetPartitionCount(); count != 2 {
		t.Errorf("count after 2 adds = %d, want 2", count)
	}

	// Remove partition
	_ = fetcher.RemovePartition("test-topic", 0)
	if count := fetcher.GetPartitionCount(); count != 1 {
		t.Errorf("count after remove = %d, want 1", count)
	}
}

func TestPartitionKey(t *testing.T) {
	tests := []struct {
		topic       string
		partitionID int
		want        string
	}{
		{"test-topic", 0, "test-topic:0"},
		{"my-topic", 5, "my-topic:5"},
		{"topic-123", 100, "topic-123:100"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := partitionKey(tt.topic, tt.partitionID)
			if got != tt.want {
				t.Errorf("partitionKey(%q, %d) = %q, want %q",
					tt.topic, tt.partitionID, got, tt.want)
			}
		})
	}
}
