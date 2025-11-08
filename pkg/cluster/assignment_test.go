package cluster

import (
	"testing"
)

func TestAssignment_AddReplica(t *testing.T) {
	assignment := NewAssignment()

	assignment.AddReplica("test-topic", 0, []int32{1, 2, 3})

	// Verify replicas
	replicas := assignment.GetReplicas("test-topic", 0)
	if len(replicas) != 3 {
		t.Errorf("Expected 3 replicas, got %d", len(replicas))
	}

	// Verify leader
	leader := assignment.GetLeader("test-topic", 0)
	if leader != 1 {
		t.Errorf("Expected leader 1, got %d", leader)
	}

	// Verify broker load
	if assignment.BrokerLoad[1] != 1 {
		t.Errorf("Expected broker 1 load = 1, got %d", assignment.BrokerLoad[1])
	}
	if assignment.BrokerLoad[2] != 1 {
		t.Errorf("Expected broker 2 load = 1, got %d", assignment.BrokerLoad[2])
	}
	if assignment.BrokerLoad[3] != 1 {
		t.Errorf("Expected broker 3 load = 1, got %d", assignment.BrokerLoad[3])
	}
}

func TestAssignment_IsBalanced(t *testing.T) {
	assignment := NewAssignment()

	// Balanced assignment (all brokers have 2 partitions)
	assignment.AddReplica("topic1", 0, []int32{1})
	assignment.AddReplica("topic1", 1, []int32{2})
	assignment.AddReplica("topic1", 2, []int32{3})
	assignment.AddReplica("topic2", 0, []int32{1})
	assignment.AddReplica("topic2", 1, []int32{2})
	assignment.AddReplica("topic2", 2, []int32{3})

	if !assignment.IsBalanced(0) {
		t.Error("Assignment should be balanced")
	}

	// Unbalanced assignment
	assignment.AddReplica("topic3", 0, []int32{1})
	assignment.AddReplica("topic3", 1, []int32{1})

	if assignment.IsBalanced(0) {
		t.Error("Assignment should be unbalanced")
	}

	if !assignment.IsBalanced(2) {
		t.Error("Assignment should be balanced with maxImbalance=2")
	}
}

func TestAssignment_Validate(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Assignment)
		wantErr bool
	}{
		{
			name: "valid assignment",
			setup: func(a *Assignment) {
				a.AddReplica("topic1", 0, []int32{1, 2, 3})
			},
			wantErr: false,
		},
		{
			name: "missing leader",
			setup: func(a *Assignment) {
				a.Partitions[PartitionKey("topic1", 0)] = []int32{1, 2, 3}
				// Don't set leader
			},
			wantErr: true,
		},
		{
			name: "leader not in replicas",
			setup: func(a *Assignment) {
				a.Partitions[PartitionKey("topic1", 0)] = []int32{1, 2, 3}
				a.Leaders[PartitionKey("topic1", 0)] = 4 // Not in replicas
			},
			wantErr: true,
		},
		{
			name: "broker load mismatch",
			setup: func(a *Assignment) {
				a.Partitions[PartitionKey("topic1", 0)] = []int32{1, 2}
				a.Leaders[PartitionKey("topic1", 0)] = 1
				a.BrokerLoad[1] = 5 // Wrong count
				a.BrokerLoad[2] = 1
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assignment := NewAssignment()
			tt.setup(assignment)

			err := assignment.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAssignment_GetStats(t *testing.T) {
	assignment := NewAssignment()

	assignment.AddReplica("topic1", 0, []int32{1, 2})
	assignment.AddReplica("topic1", 1, []int32{2, 3})
	assignment.AddReplica("topic2", 0, []int32{3, 1})
	assignment.AddReplica("topic2", 1, []int32{1, 2})

	stats := assignment.GetStats()

	if stats.TotalPartitions != 4 {
		t.Errorf("Expected 4 partitions, got %d", stats.TotalPartitions)
	}

	if stats.TotalBrokers != 3 {
		t.Errorf("Expected 3 brokers, got %d", stats.TotalBrokers)
	}

	// Broker 1: 3 partitions, Broker 2: 3 partitions, Broker 3: 2 partitions
	if stats.MinLoad != 2 {
		t.Errorf("Expected min load 2, got %d", stats.MinLoad)
	}

	if stats.MaxLoad != 3 {
		t.Errorf("Expected max load 3, got %d", stats.MaxLoad)
	}

	if stats.Imbalance != 1 {
		t.Errorf("Expected imbalance 1, got %d", stats.Imbalance)
	}

	expectedAvg := 8.0 / 3.0
	if stats.AvgLoad < expectedAvg-0.01 || stats.AvgLoad > expectedAvg+0.01 {
		t.Errorf("Expected avg load ~%.2f, got %.2f", expectedAvg, stats.AvgLoad)
	}
}

func TestAssignment_Clone(t *testing.T) {
	original := NewAssignment()
	original.AddReplica("topic1", 0, []int32{1, 2, 3})
	original.Version = 5

	clone := original.Clone()

	// Verify clone has same data
	if clone.Version != original.Version {
		t.Errorf("Version mismatch: %d != %d", clone.Version, original.Version)
	}

	if len(clone.Partitions) != len(original.Partitions) {
		t.Error("Partition count mismatch")
	}

	// Verify it's a deep copy
	clone.AddReplica("topic2", 0, []int32{4})

	if len(original.Partitions) == len(clone.Partitions) {
		t.Error("Clone should be independent of original")
	}
}

func TestPartitionKey(t *testing.T) {
	key := PartitionKey("my-topic", 5)
	expected := "my-topic:5"

	if key != expected {
		t.Errorf("Expected %s, got %s", expected, key)
	}
}

func TestFilterExcludedBrokers(t *testing.T) {
	brokers := []BrokerInfo{
		{ID: 1},
		{ID: 2},
		{ID: 3},
		{ID: 4},
	}

	excluded := map[int32]bool{
		2: true,
		4: true,
	}

	filtered := filterExcludedBrokers(brokers, excluded)

	if len(filtered) != 2 {
		t.Errorf("Expected 2 brokers, got %d", len(filtered))
	}

	// Verify only 1 and 3 remain
	for _, broker := range filtered {
		if broker.ID == 2 || broker.ID == 4 {
			t.Errorf("Broker %d should be excluded", broker.ID)
		}
	}
}

func TestSortBrokersByLoad(t *testing.T) {
	brokers := []BrokerInfo{
		{ID: 1},
		{ID: 2},
		{ID: 3},
	}

	load := map[int32]int{
		1: 5,
		2: 2,
		3: 8,
	}

	sorted := sortBrokersByLoad(brokers, load)

	// Should be sorted: 2 (load 2), 1 (load 5), 3 (load 8)
	if sorted[0].ID != 2 {
		t.Errorf("Expected broker 2 first, got %d", sorted[0].ID)
	}
	if sorted[1].ID != 1 {
		t.Errorf("Expected broker 1 second, got %d", sorted[1].ID)
	}
	if sorted[2].ID != 3 {
		t.Errorf("Expected broker 3 third, got %d", sorted[2].ID)
	}
}
