package replication

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Mock metadata client for testing
type mockMetadataClient struct {
	mu              sync.RWMutex
	partitionLeader map[string]ReplicaID
	partitionEpoch  map[string]int64
	partitionISR    map[string][]ReplicaID
	partitionReplicas map[string][]ReplicaID
}

func newMockMetadataClient() *mockMetadataClient {
	return &mockMetadataClient{
		partitionLeader:   make(map[string]ReplicaID),
		partitionEpoch:    make(map[string]int64),
		partitionISR:      make(map[string][]ReplicaID),
		partitionReplicas: make(map[string][]ReplicaID),
	}
}

func (m *mockMetadataClient) GetPartitionLeader(topic string, partitionID int) (ReplicaID, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := partitionKey(topic, partitionID)
	leader := m.partitionLeader[key]
	epoch := m.partitionEpoch[key]
	return leader, epoch, nil
}

func (m *mockMetadataClient) GetPartitionISR(topic string, partitionID int) ([]ReplicaID, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := partitionKey(topic, partitionID)
	isr := m.partitionISR[key]
	return isr, nil
}

func (m *mockMetadataClient) UpdatePartitionLeader(ctx context.Context, topic string, partitionID int, newLeader ReplicaID, newEpoch int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := partitionKey(topic, partitionID)
	m.partitionLeader[key] = newLeader
	m.partitionEpoch[key] = newEpoch
	return nil
}

func (m *mockMetadataClient) UpdatePartitionISR(ctx context.Context, topic string, partitionID int, newISR []ReplicaID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := partitionKey(topic, partitionID)
	m.partitionISR[key] = newISR
	return nil
}

func (m *mockMetadataClient) GetPartitionReplicas(topic string, partitionID int) ([]ReplicaID, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := partitionKey(topic, partitionID)
	replicas := m.partitionReplicas[key]
	return replicas, nil
}

// Mock heartbeat client for testing
type mockHeartbeatClient struct{}

func (m *mockHeartbeatClient) SendHeartbeat(ctx context.Context, brokerID ReplicaID) error {
	return nil
}

func TestFailureDetector_HasFailed(t *testing.T) {
	fd := &FailureDetector{
		lastHeartbeat:  make(map[ReplicaID]time.Time),
		failureTimeout: 5 * time.Second,
	}

	// Broker never seen - should not be considered failed
	if fd.HasFailed(1) {
		t.Error("Broker never seen should not be failed")
	}

	// Recent heartbeat - should not be failed
	fd.RecordHeartbeat(2)
	if fd.HasFailed(2) {
		t.Error("Broker with recent heartbeat should not be failed")
	}

	// Old heartbeat - should be failed
	fd.mu.Lock()
	fd.lastHeartbeat[3] = time.Now().Add(-10 * time.Second)
	fd.mu.Unlock()

	if !fd.HasFailed(3) {
		t.Error("Broker with old heartbeat should be failed")
	}
}

func TestFailoverCoordinator_RegisterPartition(t *testing.T) {
	metadata := newMockMetadataClient()
	heartbeat := &mockHeartbeatClient{}

	// Setup partition in metadata
	key := partitionKey("test-topic", 0)
	metadata.partitionLeader[key] = 1
	metadata.partitionEpoch[key] = 1
	metadata.partitionISR[key] = []ReplicaID{1, 2, 3}
	metadata.partitionReplicas[key] = []ReplicaID{1, 2, 3}

	fc := NewFailoverCoordinator(DefaultConfig(), 1, metadata, heartbeat)

	err := fc.RegisterPartition("test-topic", 0)
	if err != nil {
		t.Fatalf("RegisterPartition failed: %v", err)
	}

	// Verify partition registered
	fc.mu.RLock()
	state := fc.partitions[key]
	fc.mu.RUnlock()

	if state == nil {
		t.Fatal("Partition not registered")
	}

	if state.currentLeader != 1 {
		t.Errorf("Leader = %d, want 1", state.currentLeader)
	}
	if state.leaderEpoch != 1 {
		t.Errorf("Epoch = %d, want 1", state.leaderEpoch)
	}
	if len(state.isr) != 3 {
		t.Errorf("ISR length = %d, want 3", len(state.isr))
	}
}

func TestFailoverCoordinator_TriggerFailover(t *testing.T) {
	metadata := newMockMetadataClient()
	heartbeat := &mockHeartbeatClient{}

	// Setup partition in metadata
	key := partitionKey("test-topic", 0)
	metadata.partitionLeader[key] = 1
	metadata.partitionEpoch[key] = 1
	metadata.partitionISR[key] = []ReplicaID{1, 2, 3}
	metadata.partitionReplicas[key] = []ReplicaID{1, 2, 3}

	fc := NewFailoverCoordinator(DefaultConfig(), 2, metadata, heartbeat)

	err := fc.RegisterPartition("test-topic", 0)
	if err != nil {
		t.Fatalf("RegisterPartition failed: %v", err)
	}

	// Mark leader as failed
	fc.failureDetector.mu.Lock()
	fc.failureDetector.lastHeartbeat[1] = time.Now().Add(-1 * time.Hour)
	fc.failureDetector.mu.Unlock()

	// Get partition state
	fc.mu.RLock()
	state := fc.partitions[key]
	fc.mu.RUnlock()

	// Trigger failover
	err = fc.triggerFailover(state)
	if err != nil {
		t.Fatalf("triggerFailover failed: %v", err)
	}

	// Verify new leader elected
	if state.currentLeader == 1 {
		t.Error("Leader should have changed")
	}
	if state.currentLeader != 2 && state.currentLeader != 3 {
		t.Errorf("New leader should be 2 or 3, got %d", state.currentLeader)
	}

	// Verify epoch incremented
	if state.leaderEpoch != 2 {
		t.Errorf("Epoch = %d, want 2", state.leaderEpoch)
	}

	// Verify metadata updated
	newLeader, newEpoch, _ := metadata.GetPartitionLeader("test-topic", 0)
	if newLeader != state.currentLeader {
		t.Errorf("Metadata leader = %d, want %d", newLeader, state.currentLeader)
	}
	if newEpoch != 2 {
		t.Errorf("Metadata epoch = %d, want 2", newEpoch)
	}
}

func TestFailoverCoordinator_ElectNewLeader(t *testing.T) {
	metadata := newMockMetadataClient()
	heartbeat := &mockHeartbeatClient{}

	fc := NewFailoverCoordinator(DefaultConfig(), 1, metadata, heartbeat)

	// Mark broker 1 as failed
	fc.failureDetector.mu.Lock()
	fc.failureDetector.lastHeartbeat[1] = time.Now().Add(-1 * time.Hour)
	fc.failureDetector.mu.Unlock()

	state := &PartitionFailoverState{
		topic:         "test-topic",
		partitionID:   0,
		currentLeader: 1,
		leaderEpoch:   1,
		isr:           []ReplicaID{1, 2, 3},
		replicas:      []ReplicaID{1, 2, 3},
	}

	newLeader, err := fc.electNewLeader(state)
	if err != nil {
		t.Fatalf("electNewLeader failed: %v", err)
	}

	// Should elect 2 (first alive ISR member after failed 1)
	if newLeader != 2 {
		t.Errorf("New leader = %d, want 2", newLeader)
	}
}

func TestFailoverCoordinator_NoAliveISR(t *testing.T) {
	metadata := newMockMetadataClient()
	heartbeat := &mockHeartbeatClient{}

	fc := NewFailoverCoordinator(DefaultConfig(), 1, metadata, heartbeat)

	// Mark all brokers as failed
	for _, broker := range []ReplicaID{1, 2, 3} {
		fc.failureDetector.mu.Lock()
		fc.failureDetector.lastHeartbeat[broker] = time.Now().Add(-1 * time.Hour)
		fc.failureDetector.mu.Unlock()
	}

	state := &PartitionFailoverState{
		topic:         "test-topic",
		partitionID:   0,
		currentLeader: 1,
		leaderEpoch:   1,
		isr:           []ReplicaID{1, 2, 3},
		replicas:      []ReplicaID{1, 2, 3},
	}

	_, err := fc.electNewLeader(state)
	if err == nil {
		t.Error("Should fail when no alive ISR members")
	}
}

func TestFailoverCoordinator_DuplicateRegistration(t *testing.T) {
	metadata := newMockMetadataClient()
	heartbeat := &mockHeartbeatClient{}

	key := partitionKey("test-topic", 0)
	metadata.partitionLeader[key] = 1
	metadata.partitionEpoch[key] = 1
	metadata.partitionISR[key] = []ReplicaID{1, 2, 3}
	metadata.partitionReplicas[key] = []ReplicaID{1, 2, 3}

	fc := NewFailoverCoordinator(DefaultConfig(), 1, metadata, heartbeat)

	// First registration should succeed
	err := fc.RegisterPartition("test-topic", 0)
	if err != nil {
		t.Fatalf("First registration failed: %v", err)
	}

	// Second registration should fail
	err = fc.RegisterPartition("test-topic", 0)
	if err == nil {
		t.Error("Duplicate registration should fail")
	}
}
