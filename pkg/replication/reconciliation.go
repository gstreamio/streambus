package replication

import (
	"context"
	"fmt"
	"log"
)

// LogReconciliation handles log reconciliation after leader changes
// When a new leader is elected, it must ensure all followers have consistent logs
type LogReconciliation struct {
	topic       string
	partitionID int
	replicaID   ReplicaID
	leaderEpoch int64

	storage StorageAdapter
}

// NewLogReconciliation creates a new log reconciliation handler
func NewLogReconciliation(
	topic string,
	partitionID int,
	replicaID ReplicaID,
	leaderEpoch int64,
	storage StorageAdapter,
) *LogReconciliation {
	return &LogReconciliation{
		topic:       topic,
		partitionID: partitionID,
		replicaID:   replicaID,
		leaderEpoch: leaderEpoch,
		storage:     storage,
	}
}

// ReconcileAsNewLeader performs reconciliation when becoming leader
// This truncates any uncommitted entries from the previous epoch
func (lr *LogReconciliation) ReconcileAsNewLeader(ctx context.Context) error {
	log.Printf("[Reconciliation %d] Starting reconciliation for %s:%d as new leader, epoch=%d",
		lr.replicaID, lr.topic, lr.partitionID, lr.leaderEpoch)

	// Get current log state
	leo, err := lr.storage.GetLogEndOffset(lr.partitionID)
	if err != nil {
		return fmt.Errorf("failed to get LEO: %w", err)
	}

	hw, err := lr.storage.GetHighWaterMark(lr.partitionID)
	if err != nil {
		return fmt.Errorf("failed to get HW: %w", err)
	}

	log.Printf("[Reconciliation %d] Current state: LEO=%d, HW=%d", lr.replicaID, leo, hw)

	// If LEO > HW, we have uncommitted entries from the previous epoch
	// These must be truncated to ensure consistency
	if leo > hw {
		log.Printf("[Reconciliation %d] Truncating uncommitted entries: LEO=%d -> HW=%d",
			lr.replicaID, leo, hw)

		if err := lr.storage.Truncate(lr.partitionID, hw); err != nil {
			return fmt.Errorf("failed to truncate log: %w", err)
		}

		log.Printf("[Reconciliation %d] Truncation complete", lr.replicaID)
	}

	log.Printf("[Reconciliation %d] Reconciliation complete for new leader", lr.replicaID)
	return nil
}

// ReconcileAsFollower performs reconciliation when becoming follower
// This ensures the log is consistent with the new leader's log
func (lr *LogReconciliation) ReconcileAsFollower(ctx context.Context, leaderEpoch int64) error {
	log.Printf("[Reconciliation %d] Starting reconciliation for %s:%d as follower, leader_epoch=%d",
		lr.replicaID, lr.topic, lr.partitionID, leaderEpoch)

	// Get current log state
	leo, err := lr.storage.GetLogEndOffset(lr.partitionID)
	if err != nil {
		return fmt.Errorf("failed to get LEO: %w", err)
	}

	hw, err := lr.storage.GetHighWaterMark(lr.partitionID)
	if err != nil {
		return fmt.Errorf("failed to get HW: %w", err)
	}

	log.Printf("[Reconciliation %d] Current state: LEO=%d, HW=%d", lr.replicaID, leo, hw)

	// If we have entries beyond HW, they may diverge from the new leader
	// Truncate to HW to ensure consistency
	// The follower will fetch from the new leader starting at HW
	if leo > hw {
		log.Printf("[Reconciliation %d] Truncating to HW: LEO=%d -> HW=%d",
			lr.replicaID, leo, hw)

		if err := lr.storage.Truncate(lr.partitionID, hw); err != nil {
			return fmt.Errorf("failed to truncate log: %w", err)
		}

		log.Printf("[Reconciliation %d] Truncation complete", lr.replicaID)
	}

	log.Printf("[Reconciliation %d] Reconciliation complete for follower", lr.replicaID)
	return nil
}

// ValidateConsistency checks if the log is consistent
// This can be called periodically or after operations
func (lr *LogReconciliation) ValidateConsistency(ctx context.Context) error {
	leo, err := lr.storage.GetLogEndOffset(lr.partitionID)
	if err != nil {
		return fmt.Errorf("failed to get LEO: %w", err)
	}

	hw, err := lr.storage.GetHighWaterMark(lr.partitionID)
	if err != nil {
		return fmt.Errorf("failed to get HW: %w", err)
	}

	// HW should never exceed LEO
	if hw > leo {
		return fmt.Errorf("inconsistent state: HW=%d > LEO=%d", hw, leo)
	}

	// LEO should never be negative
	if leo < 0 {
		return fmt.Errorf("invalid LEO: %d", leo)
	}

	// HW should never be negative
	if hw < 0 {
		return fmt.Errorf("invalid HW: %d", hw)
	}

	return nil
}

// GetDivergencePoint finds the offset where two logs diverge
// Returns the offset of the first diverging entry, or -1 if logs are consistent
func (lr *LogReconciliation) GetDivergencePoint(
	ctx context.Context,
	localLEO Offset,
	leaderLEO Offset,
) (Offset, error) {
	// If leader has same or fewer entries, no divergence
	if leaderLEO <= localLEO {
		return -1, nil
	}

	// Local log is shorter, divergence starts at local LEO
	// This means we need to fetch from localLEO onwards
	return localLEO, nil
}

// ResetToOffset resets the log to a specific offset
// All entries at or after this offset are removed
func (lr *LogReconciliation) ResetToOffset(ctx context.Context, offset Offset) error {
	log.Printf("[Reconciliation %d] Resetting log to offset %d", lr.replicaID, offset)

	if err := lr.storage.Truncate(lr.partitionID, offset); err != nil {
		return fmt.Errorf("failed to truncate to offset %d: %w", offset, err)
	}

	// Update HW if needed
	hw, err := lr.storage.GetHighWaterMark(lr.partitionID)
	if err != nil {
		return fmt.Errorf("failed to get HW: %w", err)
	}

	if hw > offset {
		if err := lr.storage.SetHighWaterMark(lr.partitionID, offset); err != nil {
			return fmt.Errorf("failed to update HW: %w", err)
		}
	}

	log.Printf("[Reconciliation %d] Log reset complete", lr.replicaID)
	return nil
}
