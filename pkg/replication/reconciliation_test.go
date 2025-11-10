package replication

import (
	"context"
	"testing"
)

func TestNewLogReconciliation(t *testing.T) {
	storage := &mockStorageAdapter{leo: 100, hw: 90}

	lr := NewLogReconciliation("test-topic", 0, 1, 5, storage)

	if lr == nil {
		t.Fatal("NewLogReconciliation returned nil")
	}

	if lr.topic != "test-topic" {
		t.Errorf("topic = %s, want test-topic", lr.topic)
	}

	if lr.partitionID != 0 {
		t.Errorf("partitionID = %d, want 0", lr.partitionID)
	}

	if lr.replicaID != 1 {
		t.Errorf("replicaID = %d, want 1", lr.replicaID)
	}

	if lr.leaderEpoch != 5 {
		t.Errorf("leaderEpoch = %d, want 5", lr.leaderEpoch)
	}
}

func TestLogReconciliation_ReconcileAsNewLeader_NoTruncation(t *testing.T) {
	// LEO == HW, no uncommitted entries
	storage := &mockStorageAdapter{leo: 100, hw: 100}
	lr := NewLogReconciliation("test-topic", 0, 1, 5, storage)

	ctx := context.Background()
	err := lr.ReconcileAsNewLeader(ctx)
	if err != nil {
		t.Fatalf("ReconcileAsNewLeader failed: %v", err)
	}

	// LEO should remain unchanged
	if storage.leo != 100 {
		t.Errorf("LEO = %d, want 100", storage.leo)
	}
}

func TestLogReconciliation_ReconcileAsNewLeader_WithTruncation(t *testing.T) {
	// LEO > HW, has uncommitted entries
	storage := &mockStorageAdapter{leo: 120, hw: 100}
	lr := NewLogReconciliation("test-topic", 0, 1, 5, storage)

	ctx := context.Background()
	err := lr.ReconcileAsNewLeader(ctx)
	if err != nil {
		t.Fatalf("ReconcileAsNewLeader failed: %v", err)
	}

	// LEO should be truncated to HW
	if storage.leo != 100 {
		t.Errorf("LEO = %d, want 100", storage.leo)
	}
}

func TestLogReconciliation_ReconcileAsFollower_NoTruncation(t *testing.T) {
	// LEO == HW, no uncommitted entries
	storage := &mockStorageAdapter{leo: 100, hw: 100}
	lr := NewLogReconciliation("test-topic", 0, 1, 5, storage)

	ctx := context.Background()
	err := lr.ReconcileAsFollower(ctx, 6)
	if err != nil {
		t.Fatalf("ReconcileAsFollower failed: %v", err)
	}

	// LEO should remain unchanged
	if storage.leo != 100 {
		t.Errorf("LEO = %d, want 100", storage.leo)
	}
}

func TestLogReconciliation_ReconcileAsFollower_WithTruncation(t *testing.T) {
	// LEO > HW, has uncommitted entries
	storage := &mockStorageAdapter{leo: 120, hw: 100}
	lr := NewLogReconciliation("test-topic", 0, 1, 5, storage)

	ctx := context.Background()
	err := lr.ReconcileAsFollower(ctx, 6)
	if err != nil {
		t.Fatalf("ReconcileAsFollower failed: %v", err)
	}

	// LEO should be truncated to HW
	if storage.leo != 100 {
		t.Errorf("LEO = %d, want 100", storage.leo)
	}
}

func TestLogReconciliation_ValidateConsistency_Valid(t *testing.T) {
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	lr := NewLogReconciliation("test-topic", 0, 1, 5, storage)

	ctx := context.Background()
	err := lr.ValidateConsistency(ctx)
	if err != nil {
		t.Errorf("ValidateConsistency failed: %v", err)
	}
}

func TestLogReconciliation_ValidateConsistency_HWGreaterThanLEO(t *testing.T) {
	storage := &mockStorageAdapter{leo: 80, hw: 100}
	lr := NewLogReconciliation("test-topic", 0, 1, 5, storage)

	ctx := context.Background()
	err := lr.ValidateConsistency(ctx)
	if err == nil {
		t.Error("Expected error when HW > LEO")
	}
}

func TestLogReconciliation_ValidateConsistency_NegativeLEO(t *testing.T) {
	storage := &mockStorageAdapter{leo: -1, hw: 0}
	lr := NewLogReconciliation("test-topic", 0, 1, 5, storage)

	ctx := context.Background()
	err := lr.ValidateConsistency(ctx)
	if err == nil {
		t.Error("Expected error when LEO is negative")
	}
}

func TestLogReconciliation_ValidateConsistency_NegativeHW(t *testing.T) {
	storage := &mockStorageAdapter{leo: 10, hw: -1}
	lr := NewLogReconciliation("test-topic", 0, 1, 5, storage)

	ctx := context.Background()
	err := lr.ValidateConsistency(ctx)
	if err == nil {
		t.Error("Expected error when HW is negative")
	}
}

func TestLogReconciliation_GetDivergencePoint_NoDivergence(t *testing.T) {
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	lr := NewLogReconciliation("test-topic", 0, 1, 5, storage)

	ctx := context.Background()

	// Leader LEO == local LEO
	divergence, err := lr.GetDivergencePoint(ctx, 100, 100)
	if err != nil {
		t.Fatalf("GetDivergencePoint failed: %v", err)
	}

	if divergence != -1 {
		t.Errorf("divergence = %d, want -1", divergence)
	}

	// Leader LEO < local LEO
	divergence, err = lr.GetDivergencePoint(ctx, 100, 80)
	if err != nil {
		t.Fatalf("GetDivergencePoint failed: %v", err)
	}

	if divergence != -1 {
		t.Errorf("divergence = %d, want -1", divergence)
	}
}

func TestLogReconciliation_GetDivergencePoint_WithDivergence(t *testing.T) {
	storage := &mockStorageAdapter{leo: 100, hw: 90}
	lr := NewLogReconciliation("test-topic", 0, 1, 5, storage)

	ctx := context.Background()

	// Leader LEO > local LEO
	divergence, err := lr.GetDivergencePoint(ctx, 100, 150)
	if err != nil {
		t.Fatalf("GetDivergencePoint failed: %v", err)
	}

	if divergence != 100 {
		t.Errorf("divergence = %d, want 100", divergence)
	}
}

func TestLogReconciliation_ResetToOffset(t *testing.T) {
	storage := &mockStorageAdapter{leo: 120, hw: 100}
	lr := NewLogReconciliation("test-topic", 0, 1, 5, storage)

	ctx := context.Background()

	// Reset to offset 80
	err := lr.ResetToOffset(ctx, 80)
	if err != nil {
		t.Fatalf("ResetToOffset failed: %v", err)
	}

	// LEO should be 80
	if storage.leo != 80 {
		t.Errorf("LEO = %d, want 80", storage.leo)
	}

	// HW should also be 80 (since original HW=100 > 80)
	if storage.hw != 80 {
		t.Errorf("HW = %d, want 80", storage.hw)
	}
}

func TestLogReconciliation_ResetToOffset_NoHWUpdate(t *testing.T) {
	storage := &mockStorageAdapter{leo: 120, hw: 50}
	lr := NewLogReconciliation("test-topic", 0, 1, 5, storage)

	ctx := context.Background()

	// Reset to offset 80 (HW is 50, less than 80)
	err := lr.ResetToOffset(ctx, 80)
	if err != nil {
		t.Fatalf("ResetToOffset failed: %v", err)
	}

	// LEO should be 80
	if storage.leo != 80 {
		t.Errorf("LEO = %d, want 80", storage.leo)
	}

	// HW should remain 50 (not updated since 50 < 80)
	if storage.hw != 50 {
		t.Errorf("HW = %d, want 50", storage.hw)
	}
}
