package cluster

import (
	"context"
	"testing"
	"time"
)

func TestClusterCoordinator_AssignPartitions(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)
	strategy := NewRoundRobinStrategy()
	coordinator := NewClusterCoordinator(registry, strategy, store)

	ctx := context.Background()

	// Register brokers
	for i := int32(1); i <= 3; i++ {
		broker := &BrokerMetadata{
			ID:       i,
			Host:     "localhost",
			Port:     9090 + int(i),
			Status:   BrokerStatusAlive,
			Capacity: 100,
		}
		registry.RegisterBroker(ctx, broker)
	}

	// Create partition assignment
	partitions := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 2},
		{Topic: "topic1", PartitionID: 1, Replicas: 2},
		{Topic: "topic1", PartitionID: 2, Replicas: 2},
	}

	constraints := &AssignmentConstraints{
		ExcludedBrokers: make(map[int32]bool),
	}

	assignment, err := coordinator.AssignPartitions(ctx, partitions, constraints)
	if err != nil {
		t.Fatalf("AssignPartitions failed: %v", err)
	}

	if assignment.TotalPartitions() != 3 {
		t.Errorf("Total partitions = %d, want 3", assignment.TotalPartitions())
	}

	// Verify assignment stored
	current := coordinator.GetCurrentAssignment()
	if current == nil {
		t.Fatal("Current assignment should not be nil")
	}

	if current.TotalPartitions() != 3 {
		t.Errorf("Current assignment partitions = %d, want 3", current.TotalPartitions())
	}
}

func TestClusterCoordinator_TriggerRebalance(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)
	strategy := NewRoundRobinStrategy()
	coordinator := NewClusterCoordinator(registry, strategy, store)

	ctx := context.Background()

	// Register initial brokers
	for i := int32(1); i <= 2; i++ {
		broker := &BrokerMetadata{
			ID:       i,
			Host:     "localhost",
			Port:     9090 + int(i),
			Status:   BrokerStatusAlive,
			Capacity: 100,
		}
		registry.RegisterBroker(ctx, broker)
	}

	// Create initial assignment
	partitions := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 1},
		{Topic: "topic1", PartitionID: 1, Replicas: 1},
		{Topic: "topic1", PartitionID: 2, Replicas: 1},
		{Topic: "topic1", PartitionID: 3, Replicas: 1},
	}

	constraints := &AssignmentConstraints{
		ExcludedBrokers: make(map[int32]bool),
	}

	coordinator.AssignPartitions(ctx, partitions, constraints)

	// Add a new broker
	newBroker := &BrokerMetadata{
		ID:       3,
		Host:     "localhost",
		Port:     9093,
		Status:   BrokerStatusAlive,
		Capacity: 100,
	}
	registry.RegisterBroker(ctx, newBroker)

	// Trigger rebalance
	err := coordinator.TriggerRebalance(ctx)
	if err != nil {
		t.Fatalf("TriggerRebalance failed: %v", err)
	}

	// Verify rebalance stats
	stats := coordinator.GetRebalanceStats()
	if stats.RebalanceCount != 1 {
		t.Errorf("RebalanceCount = %d, want 1", stats.RebalanceCount)
	}

	if stats.LastRebalanceTime.IsZero() {
		t.Error("LastRebalanceTime should be set")
	}
}

func TestClusterCoordinator_RebalanceOnBrokerAdd(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)
	strategy := NewStickyStrategy()
	coordinator := NewClusterCoordinator(registry, strategy, store)

	ctx := context.Background()

	// Start coordinator
	coordinator.Start()
	defer coordinator.Stop()

	// Register initial brokers
	for i := int32(1); i <= 2; i++ {
		broker := &BrokerMetadata{
			ID:       i,
			Host:     "localhost",
			Port:     9090 + int(i),
			Status:   BrokerStatusAlive,
			Capacity: 100,
		}
		registry.RegisterBroker(ctx, broker)
	}

	// Create initial assignment
	partitions := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 1},
		{Topic: "topic1", PartitionID: 1, Replicas: 1},
	}

	constraints := &AssignmentConstraints{
		ExcludedBrokers: make(map[int32]bool),
	}

	coordinator.AssignPartitions(ctx, partitions, constraints)

	// Track rebalance count before
	statsBefore := coordinator.GetRebalanceStats()

	// Add new broker (should trigger rebalance via callback)
	newBroker := &BrokerMetadata{
		ID:       3,
		Host:     "localhost",
		Port:     9093,
		Status:   BrokerStatusAlive,
		Capacity: 100,
	}
	registry.RegisterBroker(ctx, newBroker)

	// Wait for async rebalance
	time.Sleep(100 * time.Millisecond)

	// Verify rebalance occurred
	statsAfter := coordinator.GetRebalanceStats()
	if statsAfter.RebalanceCount <= statsBefore.RebalanceCount {
		t.Error("Rebalance should have been triggered after broker add")
	}
}

func TestClusterCoordinator_SetRebalanceInterval(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)
	strategy := NewRoundRobinStrategy()
	coordinator := NewClusterCoordinator(registry, strategy, store)

	interval := 10 * time.Minute
	coordinator.SetRebalanceInterval(interval)

	// Access via reflection or just verify no panic
	// For now, just ensure it doesn't panic
}

func TestClusterCoordinator_SetRebalanceThreshold(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)
	strategy := NewRoundRobinStrategy()
	coordinator := NewClusterCoordinator(registry, strategy, store)

	threshold := 3
	coordinator.SetRebalanceThreshold(threshold)

	// Access via reflection or just verify no panic
	// For now, just ensure it doesn't panic
}

func TestClusterCoordinator_NoActiveBrokers(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)
	strategy := NewRoundRobinStrategy()
	coordinator := NewClusterCoordinator(registry, strategy, store)

	ctx := context.Background()

	// Try to assign partitions with no active brokers
	partitions := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 1},
	}

	constraints := &AssignmentConstraints{
		ExcludedBrokers: make(map[int32]bool),
	}

	_, err := coordinator.AssignPartitions(ctx, partitions, constraints)
	if err == nil {
		t.Error("AssignPartitions should fail with no active brokers")
	}
}

func TestClusterCoordinator_ConcurrentRebalance(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)
	strategy := NewRoundRobinStrategy()
	coordinator := NewClusterCoordinator(registry, strategy, store)

	ctx := context.Background()

	// Register brokers
	for i := int32(1); i <= 3; i++ {
		broker := &BrokerMetadata{
			ID:       i,
			Host:     "localhost",
			Port:     9090 + int(i),
			Status:   BrokerStatusAlive,
			Capacity: 100,
		}
		registry.RegisterBroker(ctx, broker)
	}

	// Create assignment
	partitions := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 1},
		{Topic: "topic1", PartitionID: 1, Replicas: 1},
	}

	constraints := &AssignmentConstraints{
		ExcludedBrokers: make(map[int32]bool),
	}

	coordinator.AssignPartitions(ctx, partitions, constraints)

	// Try to trigger multiple concurrent rebalances
	err1 := coordinator.TriggerRebalance(ctx)

	// Second call should fail because rebalance in progress
	err2 := coordinator.TriggerRebalance(ctx)

	if err1 != nil {
		t.Errorf("First rebalance failed: %v", err1)
	}

	if err2 == nil {
		t.Error("Second concurrent rebalance should fail")
	}
}

func TestClusterCoordinator_GetRebalanceStats(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)
	strategy := NewRoundRobinStrategy()
	coordinator := NewClusterCoordinator(registry, strategy, store)

	stats := coordinator.GetRebalanceStats()

	if stats.Rebalancing {
		t.Error("Should not be rebalancing initially")
	}

	if stats.RebalanceCount != 0 {
		t.Error("Initial rebalance count should be 0")
	}

	if stats.FailedRebalances != 0 {
		t.Error("Initial failed rebalances should be 0")
	}
}
