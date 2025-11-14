package replication

import (
	"context"
	"testing"
	"time"
)

func TestReplicationManager_WaitForISR_NotLeader(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = rm.Stop() }()

	// Try to wait when not leader
	ctx := context.Background()
	err = rm.WaitForISR(ctx, 100, 1000)
	if err == nil {
		t.Error("Expected error when not leader")
	}
}

func TestReplicationManager_WaitForISR_AlreadyReplicated(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = rm.Stop() }()

	// Become leader
	replicas := []ReplicaID{1, 2, 3}
	err = rm.BecomeLeader(1, replicas)
	if err != nil {
		t.Fatalf("BecomeLeader failed: %v", err)
	}

	// Wait for offset that's already replicated (< HW)
	ctx := context.Background()
	err = rm.WaitForISR(ctx, 50, 1000) // 50 < HW (90)
	if err != nil {
		t.Errorf("WaitForISR failed for already replicated offset: %v", err)
	}
}

func TestReplicationManager_WaitForISR_Timeout(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = rm.Stop() }()

	// Become leader
	replicas := []ReplicaID{1, 2, 3}
	err = rm.BecomeLeader(1, replicas)
	if err != nil {
		t.Fatalf("BecomeLeader failed: %v", err)
	}

	// Wait for high offset with short timeout (will timeout)
	ctx := context.Background()
	start := time.Now()
	err = rm.WaitForISR(ctx, 200, 100) // 100ms timeout
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected timeout error")
	}

	if elapsed < 100*time.Millisecond {
		t.Errorf("Timeout occurred too quickly: %v", elapsed)
	}
}

func TestReplicationManager_WaitForISR_ContextCancelled(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = rm.Stop() }()

	// Become leader
	replicas := []ReplicaID{1, 2, 3}
	err = rm.BecomeLeader(1, replicas)
	if err != nil {
		t.Fatalf("BecomeLeader failed: %v", err)
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after 50ms
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Wait for high offset (will be cancelled)
	start := time.Now()
	err = rm.WaitForISR(ctx, 200, 5000)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected context cancellation error")
	}

	if elapsed > 200*time.Millisecond {
		t.Errorf("Context cancellation took too long: %v", elapsed)
	}
}

func TestReplicationManager_WaitForISR_DefaultTimeout(t *testing.T) {
	config := DefaultConfig()
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	client := &mockLeaderClient{}

	rm := NewReplicationManager(config, "test-topic", 0, 1, storage, client)
	err := rm.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = rm.Stop() }()

	// Become leader
	replicas := []ReplicaID{1, 2, 3}
	err = rm.BecomeLeader(1, replicas)
	if err != nil {
		t.Fatalf("BecomeLeader failed: %v", err)
	}

	// Create cancellable context to avoid long wait
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Wait with timeout=0 (should use default 30s, but we'll cancel sooner)
	err = rm.WaitForISR(ctx, 200, 0)
	if err == nil {
		t.Error("Expected error (context cancelled)")
	}
}
