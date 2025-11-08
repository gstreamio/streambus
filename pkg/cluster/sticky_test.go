package cluster

import (
	"testing"
)

func TestSticky_InitialAssignment(t *testing.T) {
	strategy := NewStickyStrategy()

	partitions := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 2},
		{Topic: "topic1", PartitionID: 1, Replicas: 2},
		{Topic: "topic1", PartitionID: 2, Replicas: 2},
	}

	brokers := []BrokerInfo{
		{ID: 1},
		{ID: 2},
		{ID: 3},
	}

	constraints := &AssignmentConstraints{
		ExcludedBrokers: map[int32]bool{},
	}

	assignment, err := strategy.Assign(partitions, brokers, constraints)
	if err != nil {
		t.Fatalf("Assignment failed: %v", err)
	}

	// Verify all partitions assigned
	if assignment.TotalPartitions() != 3 {
		t.Errorf("Expected 3 partitions, got %d", assignment.TotalPartitions())
	}

	// Verify balanced
	if !assignment.IsBalanced(1) {
		t.Error("Initial assignment should be balanced")
	}
}

func TestSticky_MinimalMovement(t *testing.T) {
	strategy := NewStickyStrategy()

	// Create initial assignment
	current := NewAssignment()
	current.AddReplica("topic1", 0, []int32{1, 2})
	current.AddReplica("topic1", 1, []int32{2, 3})
	current.AddReplica("topic1", 2, []int32{3, 1})

	brokers := []BrokerInfo{
		{ID: 1},
		{ID: 2},
		{ID: 3},
	}

	constraints := &AssignmentConstraints{
		ExcludedBrokers: map[int32]bool{},
	}

	// Rebalance (should keep current since it's already balanced)
	newAssignment, err := strategy.Rebalance(current, brokers, constraints)
	if err != nil {
		t.Fatalf("Rebalance failed: %v", err)
	}

	// Count changed partitions
	changes := 0
	for key := range current.Partitions {
		oldReplicas := current.Partitions[key]
		newReplicas := newAssignment.Partitions[key]

		if len(oldReplicas) != len(newReplicas) {
			changes++
			continue
		}

		// Check if replicas changed
		oldSet := make(map[int32]bool)
		for _, r := range oldReplicas {
			oldSet[r] = true
		}

		for _, r := range newReplicas {
			if !oldSet[r] {
				changes++
				break
			}
		}
	}

	// Since already balanced, should have minimal or no changes
	if changes > 1 {
		t.Errorf("Expected minimal changes (<=1), got %d", changes)
	}
}

func TestSticky_AddBroker(t *testing.T) {
	strategy := NewStickyStrategy()

	// Create assignment with 2 brokers
	current := NewAssignment()
	current.AddReplica("topic1", 0, []int32{1})
	current.AddReplica("topic1", 1, []int32{2})
	current.AddReplica("topic1", 2, []int32{1})
	current.AddReplica("topic1", 3, []int32{2})

	// Add a third broker
	brokers := []BrokerInfo{
		{ID: 1},
		{ID: 2},
		{ID: 3}, // New broker
	}

	constraints := &AssignmentConstraints{
		ExcludedBrokers: map[int32]bool{},
	}

	newAssignment, err := strategy.Rebalance(current, brokers, constraints)
	if err != nil {
		t.Fatalf("Rebalance failed: %v", err)
	}

	// Broker 3 should now have some partitions
	if newAssignment.BrokerLoad[3] == 0 {
		t.Error("New broker 3 should receive partitions")
	}

	// Should be better balanced
	stats := newAssignment.GetStats()
	if stats.Imbalance > 1 {
		t.Errorf("Expected balanced assignment, imbalance = %d", stats.Imbalance)
	}
}

func TestSticky_RemoveBroker(t *testing.T) {
	strategy := NewStickyStrategy()

	// Create assignment with 3 brokers
	current := NewAssignment()
	current.AddReplica("topic1", 0, []int32{1, 2})
	current.AddReplica("topic1", 1, []int32{2, 3})
	current.AddReplica("topic1", 2, []int32{3, 1})

	// Remove broker 3
	brokers := []BrokerInfo{
		{ID: 1},
		{ID: 2},
	}

	constraints := &AssignmentConstraints{
		ExcludedBrokers: map[int32]bool{},
	}

	newAssignment, err := strategy.Rebalance(current, brokers, constraints)
	if err != nil {
		t.Fatalf("Rebalance failed: %v", err)
	}

	// Verify broker 3 is no longer in any replica set
	for key, replicas := range newAssignment.Partitions {
		for _, replica := range replicas {
			if replica == 3 {
				t.Errorf("Partition %s still has removed broker 3", key)
			}
		}
	}

	// Verify all partitions still have correct replica count
	if newAssignment.GetReplicas("topic1", 0) == nil {
		t.Error("Partition 0 missing after rebalance")
	}
	if len(newAssignment.GetReplicas("topic1", 0)) != 2 {
		t.Error("Partition 0 should still have 2 replicas")
	}
}

func TestSticky_MultiplePartitionsReassign(t *testing.T) {
	strategy := NewStickyStrategy()

	// Create assignment where broker 3 has many partitions
	current := NewAssignment()
	current.AddReplica("topic1", 0, []int32{3})
	current.AddReplica("topic1", 1, []int32{3})
	current.AddReplica("topic1", 2, []int32{3})
	current.AddReplica("topic1", 3, []int32{1})
	current.AddReplica("topic1", 4, []int32{2})

	// Remove broker 3 (has 3 partitions)
	brokers := []BrokerInfo{
		{ID: 1},
		{ID: 2},
	}

	constraints := &AssignmentConstraints{
		ExcludedBrokers: map[int32]bool{},
	}

	newAssignment, err := strategy.Rebalance(current, brokers, constraints)
	if err != nil {
		t.Fatalf("Rebalance failed: %v", err)
	}

	// All 3 partitions from broker 3 should be reassigned
	if newAssignment.BrokerLoad[3] != 0 {
		t.Error("Broker 3 should have zero load")
	}

	// Verify all 5 partitions still exist
	if newAssignment.TotalPartitions() != 5 {
		t.Errorf("Expected 5 partitions, got %d", newAssignment.TotalPartitions())
	}

	// Load should be distributed between brokers 1 and 2
	total := newAssignment.BrokerLoad[1] + newAssignment.BrokerLoad[2]
	if total != 5 {
		t.Errorf("Expected total load 5, got %d", total)
	}
}

func TestSticky_PreserveExistingReplicas(t *testing.T) {
	strategy := NewStickyStrategy()

	// Create assignment with multi-replica partitions
	current := NewAssignment()
	current.AddReplica("topic1", 0, []int32{1, 2, 3})
	current.AddReplica("topic1", 1, []int32{2, 3, 1})

	// Remove broker 3
	brokers := []BrokerInfo{
		{ID: 1},
		{ID: 2},
		{ID: 4}, // New broker
	}

	constraints := &AssignmentConstraints{
		ExcludedBrokers: map[int32]bool{},
	}

	newAssignment, err := strategy.Rebalance(current, brokers, constraints)
	if err != nil {
		t.Fatalf("Rebalance failed: %v", err)
	}

	// Verify brokers 1 and 2 are still in the replica sets
	for i := 0; i < 2; i++ {
		replicas := newAssignment.GetReplicas("topic1", i)
		has1 := false
		has2 := false

		for _, r := range replicas {
			if r == 1 {
				has1 = true
			}
			if r == 2 {
				has2 = true
			}
		}

		if !has1 || !has2 {
			t.Errorf("Partition %d should preserve brokers 1 and 2", i)
		}
	}
}

func TestSticky_BalancedNoOp(t *testing.T) {
	strategy := NewStickyStrategy()

	// Create perfectly balanced assignment
	current := NewAssignment()
	current.AddReplica("topic1", 0, []int32{1})
	current.AddReplica("topic1", 1, []int32{2})
	current.AddReplica("topic1", 2, []int32{3})

	brokers := []BrokerInfo{
		{ID: 1},
		{ID: 2},
		{ID: 3},
	}

	constraints := &AssignmentConstraints{
		ExcludedBrokers: map[int32]bool{},
	}

	newAssignment, err := strategy.Rebalance(current, brokers, constraints)
	if err != nil {
		t.Fatalf("Rebalance failed: %v", err)
	}

	// Should be identical (or very similar) to current
	if newAssignment.TotalPartitions() != current.TotalPartitions() {
		t.Error("Partition count changed")
	}

	// Version should be incremented even if no changes
	if newAssignment.Version != current.Version+1 {
		t.Errorf("Expected version %d, got %d", current.Version+1, newAssignment.Version)
	}
}
