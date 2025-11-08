package cluster

import (
	"testing"
)

func TestRoundRobin_BasicAssignment(t *testing.T) {
	strategy := NewRoundRobinStrategy()

	partitions := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 3},
		{Topic: "topic1", PartitionID: 1, Replicas: 3},
		{Topic: "topic1", PartitionID: 2, Replicas: 3},
	}

	brokers := []BrokerInfo{
		{ID: 1},
		{ID: 2},
		{ID: 3},
		{ID: 4},
	}

	constraints := &AssignmentConstraints{
		RackAware:          false,
		ExcludedBrokers:    map[int32]bool{},
	}

	assignment, err := strategy.Assign(partitions, brokers, constraints)
	if err != nil {
		t.Fatalf("Assignment failed: %v", err)
	}

	// Verify all partitions assigned
	if assignment.TotalPartitions() != 3 {
		t.Errorf("Expected 3 partitions, got %d", assignment.TotalPartitions())
	}

	// Verify each partition has 3 replicas
	for i := 0; i < 3; i++ {
		replicas := assignment.GetReplicas("topic1", i)
		if len(replicas) != 3 {
			t.Errorf("Partition %d: expected 3 replicas, got %d", i, len(replicas))
		}

		// Verify replicas are unique
		seen := make(map[int32]bool)
		for _, replica := range replicas {
			if seen[replica] {
				t.Errorf("Partition %d: duplicate replica %d", i, replica)
			}
			seen[replica] = true
		}
	}

	// Verify load is balanced
	if !assignment.IsBalanced(2) {
		t.Error("Assignment should be balanced")
	}
}

func TestRoundRobin_InsufficientBrokers(t *testing.T) {
	strategy := NewRoundRobinStrategy()

	partitions := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 5},
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

	// Should assign all available brokers (2 instead of 5)
	replicas := assignment.GetReplicas("topic1", 0)
	if len(replicas) != 2 {
		t.Errorf("Expected 2 replicas (limited by broker count), got %d", len(replicas))
	}
}

func TestRoundRobin_ExcludedBrokers(t *testing.T) {
	strategy := NewRoundRobinStrategy()

	partitions := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 2},
	}

	brokers := []BrokerInfo{
		{ID: 1},
		{ID: 2},
		{ID: 3},
	}

	constraints := &AssignmentConstraints{
		ExcludedBrokers: map[int32]bool{
			2: true,
		},
	}

	assignment, err := strategy.Assign(partitions, brokers, constraints)
	if err != nil {
		t.Fatalf("Assignment failed: %v", err)
	}

	replicas := assignment.GetReplicas("topic1", 0)

	// Verify broker 2 is not in replicas
	for _, replica := range replicas {
		if replica == 2 {
			t.Error("Excluded broker 2 should not be in replicas")
		}
	}
}

func TestRoundRobin_RackAware(t *testing.T) {
	strategy := NewRoundRobinStrategy()

	partitions := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 3},
	}

	brokers := []BrokerInfo{
		{ID: 1, Rack: "rack-1"},
		{ID: 2, Rack: "rack-1"},
		{ID: 3, Rack: "rack-2"},
		{ID: 4, Rack: "rack-2"},
		{ID: 5, Rack: "rack-3"},
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

	// Count racks
	racks := make(map[string]int)
	for _, replicaID := range replicas {
		for _, broker := range brokers {
			if broker.ID == replicaID {
				racks[broker.Rack]++
				break
			}
		}
	}

	// Should have replicas spread across 3 different racks
	if len(racks) != 3 {
		t.Errorf("Expected replicas in 3 racks, got %d: %v", len(racks), racks)
	}
}

func TestRoundRobin_Rebalance(t *testing.T) {
	strategy := NewRoundRobinStrategy()

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

	// Current is imbalanced: broker 1 has 3, broker 2 has 1, broker 3 has 0
	if current.IsBalanced(1) {
		t.Error("Current assignment should be imbalanced")
	}

	newAssignment, err := strategy.Rebalance(current, brokers, constraints)
	if err != nil {
		t.Fatalf("Rebalance failed: %v", err)
	}

	// New assignment should be better balanced
	stats := newAssignment.GetStats()
	if stats.Imbalance > 2 {
		t.Errorf("Expected better balance, imbalance = %d", stats.Imbalance)
	}

	// Verify version incremented
	if newAssignment.Version != current.Version+1 {
		t.Errorf("Expected version %d, got %d", current.Version+1, newAssignment.Version)
	}
}

func TestRoundRobin_NoPartitions(t *testing.T) {
	strategy := NewRoundRobinStrategy()

	partitions := []PartitionInfo{}
	brokers := []BrokerInfo{{ID: 1}}
	constraints := &AssignmentConstraints{ExcludedBrokers: map[int32]bool{}}

	assignment, err := strategy.Assign(partitions, brokers, constraints)
	if err != nil {
		t.Fatalf("Assignment failed: %v", err)
	}

	if assignment.TotalPartitions() != 0 {
		t.Errorf("Expected 0 partitions, got %d", assignment.TotalPartitions())
	}
}

func TestRoundRobin_NoBrokers(t *testing.T) {
	strategy := NewRoundRobinStrategy()

	partitions := []PartitionInfo{{Topic: "topic1", PartitionID: 0, Replicas: 1}}
	brokers := []BrokerInfo{}
	constraints := &AssignmentConstraints{ExcludedBrokers: map[int32]bool{}}

	_, err := strategy.Assign(partitions, brokers, constraints)
	if err == nil {
		t.Error("Expected error with no brokers")
	}
}

func TestRoundRobin_AllBrokersExcluded(t *testing.T) {
	strategy := NewRoundRobinStrategy()

	partitions := []PartitionInfo{{Topic: "topic1", PartitionID: 0, Replicas: 1}}
	brokers := []BrokerInfo{{ID: 1}, {ID: 2}}
	constraints := &AssignmentConstraints{
		ExcludedBrokers: map[int32]bool{1: true, 2: true},
	}

	_, err := strategy.Assign(partitions, brokers, constraints)
	if err == nil {
		t.Error("Expected error when all brokers excluded")
	}
}
