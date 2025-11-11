package cluster

import (
	"fmt"
	"testing"
)

func TestSticky_RemoveBroker_Debug(t *testing.T) {
	strategy := NewStickyStrategy()

	// Create assignment with broker 3
	current := NewAssignment()
	current.AddReplica("topic1", 0, []int32{1, 2})
	current.AddReplica("topic1", 1, []int32{2, 3})
	current.AddReplica("topic1", 2, []int32{3, 1})

	fmt.Println("Initial assignment:")
	for key, replicas := range current.Partitions {
		fmt.Printf("  %s: %v\n", key, replicas)
	}

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

	fmt.Println("\nNew assignment:")
	for key, replicas := range newAssignment.Partitions {
		fmt.Printf("  %s: %v\n", key, replicas)
	}

	// Verify broker 3 is no longer in any replica set
	for key, replicas := range newAssignment.Partitions {
		for _, replica := range replicas {
			if replica == 3 {
				t.Errorf("Partition %s still has removed broker 3", key)
			}
		}
	}
}
