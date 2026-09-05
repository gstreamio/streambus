package cluster

import (
	"context"
	"strings"
	"testing"
	"time"
)

// registerAliveBrokers registers and heartbeats a broker for each given ID,
// matching the pattern used throughout this file's existing tests.
func registerAliveBrokers(t *testing.T, ctx context.Context, registry *BrokerRegistry, ids ...int32) {
	t.Helper()
	for _, id := range ids {
		broker := &BrokerMetadata{
			ID:       id,
			Host:     "localhost",
			Port:     9090 + int(id),
			Status:   BrokerStatusAlive,
			Capacity: 100,
		}
		if err := registry.RegisterBroker(ctx, broker); err != nil {
			t.Fatalf("RegisterBroker(%d) failed: %v", id, err)
		}
		if err := registry.RecordHeartbeat(id); err != nil {
			t.Fatalf("RecordHeartbeat(%d) failed: %v", id, err)
		}
	}
}

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
		_ = registry.RegisterBroker(ctx, broker)
		// Mark broker as alive by sending heartbeat
		_ = registry.RecordHeartbeat(i)
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
		_ = registry.RegisterBroker(ctx, broker)
		// Mark broker as alive by sending heartbeat
		_ = registry.RecordHeartbeat(i)
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

	_, _ = coordinator.AssignPartitions(ctx, partitions, constraints)

	// Add a new broker
	newBroker := &BrokerMetadata{
		ID:       3,
		Host:     "localhost",
		Port:     9093,
		Status:   BrokerStatusAlive,
		Capacity: 100,
	}
	_ = registry.RegisterBroker(ctx, newBroker)

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
	_ = coordinator.Start()
	defer func() { _ = coordinator.Stop() }()

	// Register initial brokers
	for i := int32(1); i <= 2; i++ {
		broker := &BrokerMetadata{
			ID:       i,
			Host:     "localhost",
			Port:     9090 + int(i),
			Status:   BrokerStatusAlive,
			Capacity: 100,
		}
		_ = registry.RegisterBroker(ctx, broker)
		// Mark broker as alive by sending heartbeat
		_ = registry.RecordHeartbeat(i)
	}

	// Create initial assignment
	partitions := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 1},
		{Topic: "topic1", PartitionID: 1, Replicas: 1},
	}

	constraints := &AssignmentConstraints{
		ExcludedBrokers: make(map[int32]bool),
	}

	_, _ = coordinator.AssignPartitions(ctx, partitions, constraints)

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
	_ = registry.RegisterBroker(ctx, newBroker)

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
		_ = registry.RegisterBroker(ctx, broker)
		// Mark broker as alive by sending heartbeat
		_ = registry.RecordHeartbeat(i)
	}

	// Create assignment
	partitions := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 1},
		{Topic: "topic1", PartitionID: 1, Replicas: 1},
	}

	constraints := &AssignmentConstraints{
		ExcludedBrokers: make(map[int32]bool),
	}

	_, _ = coordinator.AssignPartitions(ctx, partitions, constraints)

	// Try to trigger multiple concurrent rebalances using goroutines
	errChan1 := make(chan error, 1)
	errChan2 := make(chan error, 1)

	go func() {
		errChan1 <- coordinator.TriggerRebalance(ctx)
	}()

	go func() {
		errChan2 <- coordinator.TriggerRebalance(ctx)
	}()

	err1 := <-errChan1
	err2 := <-errChan2

	// Both failing is bad
	if err1 != nil && err2 != nil {
		t.Error("Both rebalances failed, expected at least one to succeed")
	}

	// Note: Due to timing, both may succeed if they don't overlap, or one may fail
	// if they do overlap. Both outcomes are acceptable as long as at least one succeeds.
	// The important thing is that the system handles concurrent rebalances safely.
	if err1 == nil && err2 == nil {
		t.Log("Both rebalances succeeded (no overlap occurred)")
	} else if err1 != nil || err2 != nil {
		t.Log("One rebalance failed due to concurrent access (expected behavior)")
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

// TestClusterCoordinator_MaxPartitionsPerBrokerCumulative verifies that
// MaxPartitionsPerBroker is enforced across successive AssignPartitions
// calls (i.e. across topics), not just within a single call.
func TestClusterCoordinator_MaxPartitionsPerBrokerCumulative(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)
	strategy := NewRoundRobinStrategy()
	coordinator := NewClusterCoordinator(registry, strategy, store)
	ctx := context.Background()

	registerAliveBrokers(t, ctx, registry, 1, 2)

	constraints := &AssignmentConstraints{
		ExcludedBrokers:        make(map[int32]bool),
		MaxPartitionsPerBroker: 3,
	}

	// Three successive single-topic assignments, two partitions each. Round
	// robin sorts brokers by ID and restarts its index every call, so each
	// call places exactly one partition on broker 1 and one on broker 2.
	for i, topic := range []string{"topic1", "topic2", "topic3"} {
		partitions := []PartitionInfo{
			{Topic: topic, PartitionID: 0, Replicas: 1},
			{Topic: topic, PartitionID: 1, Replicas: 1},
		}
		if _, err := coordinator.AssignPartitions(ctx, partitions, constraints); err != nil {
			t.Fatalf("assigning %s (call %d) failed: %v", topic, i+1, err)
		}
	}

	current := coordinator.GetCurrentAssignment()
	if current.TotalPartitions() != 6 {
		t.Fatalf("expected 6 partitions tracked across 3 topics, got %d", current.TotalPartitions())
	}
	for brokerID, load := range current.BrokerLoad {
		if load != 3 {
			t.Errorf("broker %d has cumulative load %d, want exactly 3 (at its limit)", brokerID, load)
		}
	}

	// A fourth topic has nowhere to go: both brokers are already at the
	// cluster-wide limit purely from the previous three calls, something a
	// single Assign call has no way of knowing about on its own.
	topic4 := []PartitionInfo{
		{Topic: "topic4", PartitionID: 0, Replicas: 1},
	}
	_, err := coordinator.AssignPartitions(ctx, topic4, constraints)
	if err == nil {
		t.Fatal("expected topic4 assignment to fail: both brokers are already at their cumulative limit")
	}
	if !strings.Contains(err.Error(), "partition limit") {
		t.Errorf("expected a capacity error, got: %v", err)
	}

	// The failed call must not have corrupted previously tracked state.
	current = coordinator.GetCurrentAssignment()
	if current.TotalPartitions() != 6 {
		t.Errorf("current assignment has %d partitions after failed call, want unchanged 6", current.TotalPartitions())
	}
}

// TestClusterCoordinator_ExistingLoadMergesWithCumulative verifies that an
// explicit AssignmentConstraints.ExistingLoad supplied by the caller is
// still honoured, and that it adds on top of the load the coordinator has
// derived from its own previously-assigned topics rather than overriding it.
func TestClusterCoordinator_ExistingLoadMergesWithCumulative(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)
	strategy := NewRoundRobinStrategy()
	coordinator := NewClusterCoordinator(registry, strategy, store)
	ctx := context.Background()

	registerAliveBrokers(t, ctx, registry, 1, 2)

	// topic1 puts one partition on each broker (round robin, sorted by ID).
	topic1 := []PartitionInfo{
		{Topic: "topic1", PartitionID: 0, Replicas: 1},
		{Topic: "topic1", PartitionID: 1, Replicas: 1},
	}
	if _, err := coordinator.AssignPartitions(ctx, topic1, &AssignmentConstraints{
		ExcludedBrokers: make(map[int32]bool),
	}); err != nil {
		t.Fatalf("assigning topic1 failed: %v", err)
	}

	before := coordinator.GetCurrentAssignment()
	if before.BrokerLoad[1] != 1 || before.BrokerLoad[2] != 1 {
		t.Fatalf("expected 1 partition on each broker after topic1, got %v", before.BrokerLoad)
	}

	// Broker 1 supposedly also holds 2 partitions the coordinator has no
	// other way of knowing about (e.g. tracked externally). Combined with its
	// derived cumulative count of 1, broker 1 is at the limit of 3 and must
	// receive nothing new; broker 2 has capacity to spare.
	topic2 := []PartitionInfo{
		{Topic: "topic2", PartitionID: 0, Replicas: 1},
	}
	assignment, err := coordinator.AssignPartitions(ctx, topic2, &AssignmentConstraints{
		ExcludedBrokers:        make(map[int32]bool),
		MaxPartitionsPerBroker: 3,
		ExistingLoad:           map[int32]int{1: 2},
	})
	if err != nil {
		t.Fatalf("assigning topic2 failed: %v", err)
	}

	replicas := assignment.GetReplicas("topic2", 0)
	if len(replicas) != 1 || replicas[0] != 2 {
		t.Errorf("expected topic2's partition on broker 2 (broker 1 full via merged ExistingLoad), got replicas=%v", replicas)
	}
}

// TestClusterCoordinator_RebalanceDoesNotDoubleCount verifies that a
// rebalance triggered after several topics have been assigned neither loses
// nor double-counts a partition moved off a removed broker, and continues to
// enforce the cluster-wide MaxPartitionsPerBroker carried over from the
// original AssignPartitions call.
func TestClusterCoordinator_RebalanceDoesNotDoubleCount(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)
	strategy := NewStickyStrategy()
	coordinator := NewClusterCoordinator(registry, strategy, store)
	ctx := context.Background()

	registerAliveBrokers(t, ctx, registry, 1, 2, 3)

	constraints := &AssignmentConstraints{
		ExcludedBrokers:        make(map[int32]bool),
		MaxPartitionsPerBroker: 2,
	}

	partitions := []PartitionInfo{
		{Topic: "t", PartitionID: 0, Replicas: 1},
		{Topic: "t", PartitionID: 1, Replicas: 1},
		{Topic: "t", PartitionID: 2, Replicas: 1},
	}
	if _, err := coordinator.AssignPartitions(ctx, partitions, constraints); err != nil {
		t.Fatalf("AssignPartitions failed: %v", err)
	}

	totalBefore := 0
	for _, load := range coordinator.GetCurrentAssignment().BrokerLoad {
		totalBefore += load
	}

	// Remove a broker and rebalance; its partition must be re-homed onto one
	// of the survivors without being counted twice.
	if err := registry.DeregisterBroker(ctx, 3); err != nil {
		t.Fatalf("DeregisterBroker failed: %v", err)
	}
	if err := coordinator.TriggerRebalance(ctx); err != nil {
		t.Fatalf("TriggerRebalance failed: %v", err)
	}

	final := coordinator.GetCurrentAssignment()
	if final.TotalPartitions() != 3 {
		t.Errorf("expected 3 partitions to survive rebalance, got %d", final.TotalPartitions())
	}

	totalAfter := 0
	for brokerID, load := range final.BrokerLoad {
		totalAfter += load
		if brokerID == 3 {
			t.Error("removed broker 3 still holds replicas after rebalance")
		}
		if load > 2 {
			t.Errorf("broker %d holds %d partitions after rebalance, cluster-wide limit is 2", brokerID, load)
		}
	}
	if totalAfter != totalBefore {
		t.Errorf("total replica count changed across rebalance: before=%d, after=%d (a partition was lost or double-counted)",
			totalBefore, totalAfter)
	}
}
