package cluster

import (
	"testing"
)

func TestRange_BasicAssignment(t *testing.T) {
	strategy := NewRangeStrategy()

	partitions := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 2},
		{Topic: "topic1", PartitionID: 1, Replicas: 2},
		{Topic: "topic1", PartitionID: 2, Replicas: 2},
		{Topic: "topic1", PartitionID: 3, Replicas: 2},
	}

	brokers := []BrokerInfo{
		{ID: 1},
		{ID: 2},
	}

	constraints := &AssignmentConstraints{
		RackAware:       false,
		ExcludedBrokers: map[int32]bool{},
	}

	assignment, err := strategy.Assign(partitions, brokers, constraints)
	if err != nil {
		t.Fatalf("Assignment failed: %v", err)
	}

	// Verify all partitions assigned
	if assignment.TotalPartitions() != 4 {
		t.Errorf("Expected 4 partitions, got %d", assignment.TotalPartitions())
	}

	// Verify each partition has 2 replicas
	for i := 0; i < 4; i++ {
		replicas := assignment.GetReplicas("topic1", i)
		if len(replicas) != 2 {
			t.Errorf("Partition %d: expected 2 replicas, got %d", i, len(replicas))
		}
	}

	// In range strategy, partitions should be distributed in ranges
	// Broker 1 should be leader for partitions 0-1
	// Broker 2 should be leader for partitions 2-3
	leader0 := assignment.GetLeader("topic1", 0)
	leader1 := assignment.GetLeader("topic1", 1)
	leader2 := assignment.GetLeader("topic1", 2)
	leader3 := assignment.GetLeader("topic1", 3)

	if leader0 != 1 || leader1 != 1 {
		t.Errorf("Expected broker 1 to lead partitions 0-1, got leaders: %d, %d", leader0, leader1)
	}

	if leader2 != 2 || leader3 != 2 {
		t.Errorf("Expected broker 2 to lead partitions 2-3, got leaders: %d, %d", leader2, leader3)
	}
}

func TestRange_MultipleTopics(t *testing.T) {
	strategy := NewRangeStrategy()

	partitions := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 2},
		{Topic: "topic1", PartitionID: 1, Replicas: 2},
		{Topic: "topic2", PartitionID: 0, Replicas: 2},
		{Topic: "topic2", PartitionID: 1, Replicas: 2},
	}

	brokers := []BrokerInfo{
		{ID: 1},
		{ID: 2},
	}

	constraints := &AssignmentConstraints{
		ExcludedBrokers: map[int32]bool{},
	}

	assignment, err := strategy.Assign(partitions, brokers, constraints)
	if err != nil {
		t.Fatalf("Assignment failed: %v", err)
	}

	// Verify both topics assigned
	if assignment.TotalPartitions() != 4 {
		t.Errorf("Expected 4 partitions, got %d", assignment.TotalPartitions())
	}

	// Each topic should be independently range-assigned
	// Both topics: broker 1 leads partition 0, broker 2 leads partition 1
	leader1_0 := assignment.GetLeader("topic1", 0)
	leader1_1 := assignment.GetLeader("topic1", 1)
	leader2_0 := assignment.GetLeader("topic2", 0)
	leader2_1 := assignment.GetLeader("topic2", 1)

	if leader1_0 != 1 || leader2_0 != 1 {
		t.Errorf("Expected broker 1 to lead partition 0 of both topics")
	}

	if leader1_1 != 2 || leader2_1 != 2 {
		t.Errorf("Expected broker 2 to lead partition 1 of both topics")
	}
}

func TestRange_UnevenDistribution(t *testing.T) {
	strategy := NewRangeStrategy()

	// 5 partitions, 3 brokers
	// Should distribute: broker1=2, broker2=2, broker3=1
	partitions := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 1},
		{Topic: "topic1", PartitionID: 1, Replicas: 1},
		{Topic: "topic1", PartitionID: 2, Replicas: 1},
		{Topic: "topic1", PartitionID: 3, Replicas: 1},
		{Topic: "topic1", PartitionID: 4, Replicas: 1},
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

	// Check load distribution
	stats := assignment.GetStats()

	// Max load should be 2, min load should be 1
	if stats.MaxLoad != 2 {
		t.Errorf("Expected max load 2, got %d", stats.MaxLoad)
	}
	if stats.MinLoad != 1 {
		t.Errorf("Expected min load 1, got %d", stats.MinLoad)
	}
}

func TestRange_RackAware(t *testing.T) {
	strategy := NewRangeStrategy()

	partitions := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 3},
	}

	brokers := []BrokerInfo{
		{ID: 1, Rack: "rack-1"},
		{ID: 2, Rack: "rack-1"},
		{ID: 3, Rack: "rack-2"},
		{ID: 4, Rack: "rack-3"},
	}

	constraints := &AssignmentConstraints{
		RackAware:       true,
		ExcludedBrokers: map[int32]bool{},
	}

	assignment, err := strategy.Assign(partitions, brokers, constraints)
	if err != nil {
		t.Fatalf("Assignment failed: %v", err)
	}

	replicas := assignment.GetReplicas("topic1", 0)

	// Count unique racks
	racks := make(map[string]bool)
	for _, replicaID := range replicas {
		for _, broker := range brokers {
			if broker.ID == replicaID {
				racks[broker.Rack] = true
				break
			}
		}
	}

	// Should spread across multiple racks (at least 2)
	if len(racks) < 2 {
		t.Errorf("Expected replicas spread across multiple racks, got %d racks", len(racks))
	}
}

func TestRange_Rebalance(t *testing.T) {
	strategy := NewRangeStrategy()

	// Create imbalanced assignment
	current := NewAssignment()
	current.AddReplica("topic1", 0, []int32{1})
	current.AddReplica("topic1", 1, []int32{1})
	current.AddReplica("topic1", 2, []int32{1})
	current.AddReplica("topic1", 3, []int32{2})

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

	// New assignment should be more balanced
	stats := newAssignment.GetStats()
	if stats.Imbalance > 2 {
		t.Errorf("Expected better balance, imbalance = %d", stats.Imbalance)
	}

	// Verify version incremented
	if newAssignment.Version != current.Version+1 {
		t.Errorf("Expected version %d, got %d", current.Version+1, newAssignment.Version)
	}
}

func TestRange_AlreadyBalanced(t *testing.T) {
	strategy := NewRangeStrategy()

	// Create balanced assignment
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

	// Already balanced
	if !current.IsBalanced(1) {
		t.Error("Assignment should be balanced")
	}

	newAssignment, err := strategy.Rebalance(current, brokers, constraints)
	if err != nil {
		t.Fatalf("Rebalance failed: %v", err)
	}

	// Should return current (or equally balanced) assignment
	if newAssignment.TotalPartitions() != current.TotalPartitions() {
		t.Error("Rebalance changed partition count")
	}
}
