package cluster

import (
	"strings"
	"testing"
)

// brokersWithIDs builds a slice of unconstrained brokers with the given IDs.
func brokersWithIDs(ids ...int32) []BrokerInfo {
	brokers := make([]BrokerInfo, 0, len(ids))
	for _, id := range ids {
		brokers = append(brokers, BrokerInfo{ID: id})
	}
	return brokers
}

// replicaCounts counts how many partition replicas each broker holds.
func replicaCounts(a *Assignment) map[int32]int {
	counts := make(map[int32]int)
	for _, replicas := range a.Partitions {
		for _, broker := range replicas {
			counts[broker]++
		}
	}
	return counts
}

func TestTighterLimit(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{"both unlimited", 0, 0, 0},
		{"only a", 5, 0, 5},
		{"only b", 0, 7, 7},
		{"a tighter", 3, 9, 3},
		{"b tighter", 9, 3, 3},
		{"equal", 4, 4, 4},
		{"negative treated as unlimited", -1, 6, 6},
		{"both negative", -1, -2, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tighterLimit(tt.a, tt.b); got != tt.want {
				t.Errorf("tighterLimit(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestBrokerLoad_UnlimitedByDefault(t *testing.T) {
	load := newBrokerLoad(brokersWithIDs(1, 2), &AssignmentConstraints{})

	if load.limited() {
		t.Fatal("expected no broker to be limited")
	}
	for i := 0; i < 100; i++ {
		if !load.hasCapacity(1) {
			t.Fatalf("broker 1 unexpectedly out of capacity after %d adds", i)
		}
		load.add(1)
	}
}

func TestBrokerLoad_MaxPartitionsPerBroker(t *testing.T) {
	load := newBrokerLoad(brokersWithIDs(1, 2), &AssignmentConstraints{MaxPartitionsPerBroker: 2})

	if !load.limited() {
		t.Fatal("expected brokers to be limited")
	}

	load.add(1)
	if !load.hasCapacity(1) {
		t.Error("broker 1 should still have capacity at 1/2")
	}
	load.add(1)
	if load.hasCapacity(1) {
		t.Error("broker 1 should be full at 2/2")
	}
	if !load.hasCapacity(2) {
		t.Error("broker 2 should be untouched")
	}

	load.remove(1)
	if !load.hasCapacity(1) {
		t.Error("broker 1 should have capacity again after remove")
	}
}

func TestBrokerLoad_PerBrokerCapacityWins(t *testing.T) {
	brokers := []BrokerInfo{
		{ID: 1, Capacity: 1},
		{ID: 2, Capacity: 10},
	}
	load := newBrokerLoad(brokers, &AssignmentConstraints{MaxPartitionsPerBroker: 5})

	load.add(1)
	if load.hasCapacity(1) {
		t.Error("broker 1 should be capped at its own Capacity of 1")
	}

	for i := 0; i < 5; i++ {
		if !load.hasCapacity(2) {
			t.Fatalf("broker 2 should have capacity at %d/5", i)
		}
		load.add(2)
	}
	if load.hasCapacity(2) {
		t.Error("broker 2 should be capped at MaxPartitionsPerBroker of 5")
	}
}

func TestBrokerLoad_SeededFromExistingLoad(t *testing.T) {
	load := newBrokerLoad(brokersWithIDs(1, 2), &AssignmentConstraints{
		MaxPartitionsPerBroker: 3,
		ExistingLoad:           map[int32]int{1: 3},
	})

	if load.hasCapacity(1) {
		t.Error("broker 1 was seeded at its limit and should have no capacity")
	}
	if load.count(1) != 3 {
		t.Errorf("expected seeded count 3, got %d", load.count(1))
	}
	if !load.hasCapacity(2) {
		t.Error("broker 2 was not seeded and should have capacity")
	}
}

func TestRoundRobin_EnforcesMaxPartitionsPerBroker(t *testing.T) {
	strategy := NewRoundRobinStrategy()

	// 4 partitions x 1 replica across 2 brokers capped at 2 each == exactly full.
	partitions := []PartitionInfo{
		{Topic: "t", PartitionID: 0, Replicas: 1},
		{Topic: "t", PartitionID: 1, Replicas: 1},
		{Topic: "t", PartitionID: 2, Replicas: 1},
		{Topic: "t", PartitionID: 3, Replicas: 1},
	}

	constraints := &AssignmentConstraints{MaxPartitionsPerBroker: 2}

	assignment, err := strategy.Assign(partitions, brokersWithIDs(1, 2), constraints)
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}

	for brokerID, count := range replicaCounts(assignment) {
		if count > 2 {
			t.Errorf("broker %d holds %d partitions, limit is 2", brokerID, count)
		}
	}
}

func TestRoundRobin_FailsWhenCapacityExhausted(t *testing.T) {
	strategy := NewRoundRobinStrategy()

	// 3 partitions but only 2 slots total.
	partitions := []PartitionInfo{
		{Topic: "t", PartitionID: 0, Replicas: 1},
		{Topic: "t", PartitionID: 1, Replicas: 1},
		{Topic: "t", PartitionID: 2, Replicas: 1},
	}

	constraints := &AssignmentConstraints{MaxPartitionsPerBroker: 1}

	_, err := strategy.Assign(partitions, brokersWithIDs(1, 2), constraints)
	if err == nil {
		t.Fatal("expected assignment to fail once every broker is at its limit")
	}
	if !strings.Contains(err.Error(), "partition limit") {
		t.Errorf("expected a capacity error, got: %v", err)
	}
}

func TestRoundRobin_RespectsExistingLoad(t *testing.T) {
	strategy := NewRoundRobinStrategy()

	partitions := []PartitionInfo{
		{Topic: "t", PartitionID: 0, Replicas: 1},
		{Topic: "t", PartitionID: 1, Replicas: 1},
	}

	// Broker 1 already holds 2 partitions elsewhere, so both new partitions
	// must land on broker 2.
	constraints := &AssignmentConstraints{
		MaxPartitionsPerBroker: 2,
		ExistingLoad:           map[int32]int{1: 2},
	}

	assignment, err := strategy.Assign(partitions, brokersWithIDs(1, 2), constraints)
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}

	counts := replicaCounts(assignment)
	if counts[1] != 0 {
		t.Errorf("broker 1 was already full but received %d partitions", counts[1])
	}
	if counts[2] != 2 {
		t.Errorf("expected broker 2 to receive both partitions, got %d", counts[2])
	}
}

func TestRoundRobin_RackAwareEnforcesCapacity(t *testing.T) {
	strategy := NewRoundRobinStrategy()

	brokers := []BrokerInfo{
		{ID: 1, Rack: "a"},
		{ID: 2, Rack: "b"},
		{ID: 3, Rack: "c"},
		{ID: 4, Rack: "d"},
	}

	partitions := []PartitionInfo{
		{Topic: "t", PartitionID: 0, Replicas: 2},
		{Topic: "t", PartitionID: 1, Replicas: 2},
		{Topic: "t", PartitionID: 2, Replicas: 2},
		{Topic: "t", PartitionID: 3, Replicas: 2},
	}

	constraints := &AssignmentConstraints{RackAware: true, MaxPartitionsPerBroker: 2}

	assignment, err := strategy.Assign(partitions, brokers, constraints)
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}

	for brokerID, count := range replicaCounts(assignment) {
		if count > 2 {
			t.Errorf("rack-aware assignment put %d partitions on broker %d, limit is 2", count, brokerID)
		}
	}
}

func TestRange_EnforcesMaxPartitionsPerBroker(t *testing.T) {
	strategy := NewRangeStrategy()

	partitions := []PartitionInfo{
		{Topic: "t", PartitionID: 0, Replicas: 1},
		{Topic: "t", PartitionID: 1, Replicas: 1},
		{Topic: "t", PartitionID: 2, Replicas: 1},
		{Topic: "t", PartitionID: 3, Replicas: 1},
	}

	constraints := &AssignmentConstraints{MaxPartitionsPerBroker: 2}

	assignment, err := strategy.Assign(partitions, brokersWithIDs(1, 2), constraints)
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}

	for brokerID, count := range replicaCounts(assignment) {
		if count > 2 {
			t.Errorf("broker %d holds %d partitions, limit is 2", brokerID, count)
		}
	}
}

func TestRange_FailsWhenCapacityExhausted(t *testing.T) {
	strategy := NewRangeStrategy()

	partitions := []PartitionInfo{
		{Topic: "t", PartitionID: 0, Replicas: 1},
		{Topic: "t", PartitionID: 1, Replicas: 1},
		{Topic: "t", PartitionID: 2, Replicas: 1},
	}

	constraints := &AssignmentConstraints{MaxPartitionsPerBroker: 1}

	_, err := strategy.Assign(partitions, brokersWithIDs(1, 2), constraints)
	if err == nil {
		t.Fatal("expected assignment to fail once every broker is at its limit")
	}
	if !strings.Contains(err.Error(), "partition limit") {
		t.Errorf("expected a capacity error, got: %v", err)
	}
}

func TestRange_DeterministicAcrossTopics(t *testing.T) {
	strategy := NewRangeStrategy()

	partitions := []PartitionInfo{
		{Topic: "alpha", PartitionID: 0, Replicas: 1},
		{Topic: "beta", PartitionID: 0, Replicas: 1},
		{Topic: "gamma", PartitionID: 0, Replicas: 1},
	}
	brokers := brokersWithIDs(1, 2, 3)

	first, err := strategy.Assign(partitions, brokers, &AssignmentConstraints{})
	if err != nil {
		t.Fatalf("Assign failed: %v", err)
	}

	// Multi-topic assignment must not depend on map iteration order.
	for i := 0; i < 20; i++ {
		next, err := strategy.Assign(partitions, brokers, &AssignmentConstraints{})
		if err != nil {
			t.Fatalf("Assign failed on iteration %d: %v", i, err)
		}
		for key, replicas := range first.Partitions {
			other := next.Partitions[key]
			if len(other) != len(replicas) {
				t.Fatalf("partition %s: replica count changed between runs", key)
			}
			for j := range replicas {
				if replicas[j] != other[j] {
					t.Fatalf("partition %s: replicas differ between runs (%v vs %v)", key, replicas, other)
				}
			}
		}
	}
}

func TestSticky_RebalanceRespectsCapacity(t *testing.T) {
	strategy := NewStickyStrategy()

	// Three brokers, one partition each; broker 3 then goes away and its
	// partition must be re-homed without exceeding the per-broker limit.
	current := NewAssignment()
	current.AddReplica("t", 0, []int32{1})
	current.AddReplica("t", 1, []int32{2})
	current.AddReplica("t", 2, []int32{3})
	current.RecomputeBrokerLoad()

	constraints := &AssignmentConstraints{MaxPartitionsPerBroker: 2}

	result, err := strategy.Rebalance(current, brokersWithIDs(1, 2), constraints)
	if err != nil {
		t.Fatalf("Rebalance failed: %v", err)
	}

	for brokerID, count := range replicaCounts(result) {
		if count > 2 {
			t.Errorf("broker %d holds %d partitions after rebalance, limit is 2", brokerID, count)
		}
		if brokerID == 3 {
			t.Error("removed broker 3 still holds replicas")
		}
	}
}

func TestSticky_RebalanceFailsWhenCapacityExhausted(t *testing.T) {
	strategy := NewStickyStrategy()

	current := NewAssignment()
	current.AddReplica("t", 0, []int32{1})
	current.AddReplica("t", 1, []int32{2})
	current.AddReplica("t", 2, []int32{3})
	current.RecomputeBrokerLoad()

	// Only two brokers survive, each capped at one partition: the third
	// partition cannot be placed anywhere.
	constraints := &AssignmentConstraints{MaxPartitionsPerBroker: 1}

	_, err := strategy.Rebalance(current, brokersWithIDs(1, 2), constraints)
	if err == nil {
		t.Fatal("expected rebalance to fail once every broker is at its limit")
	}
	if !strings.Contains(err.Error(), "partition limit") {
		t.Errorf("expected a capacity error, got: %v", err)
	}
}
