package cluster

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Mock metadata store for testing
type mockMetadataStore struct {
	mu      sync.RWMutex
	brokers map[int32]*BrokerMetadata
}

func newMockMetadataStore() *mockMetadataStore {
	return &mockMetadataStore{
		brokers: make(map[int32]*BrokerMetadata),
	}
}

func (m *mockMetadataStore) StoreBrokerMetadata(ctx context.Context, broker *BrokerMetadata) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Store a copy
	copy := *broker
	m.brokers[broker.ID] = &copy
	return nil
}

func (m *mockMetadataStore) GetBrokerMetadata(ctx context.Context, brokerID int32) (*BrokerMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	broker, exists := m.brokers[brokerID]
	if !exists {
		return nil, nil
	}

	copy := *broker
	return &copy, nil
}

func (m *mockMetadataStore) ListBrokers(ctx context.Context) ([]*BrokerMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	brokers := make([]*BrokerMetadata, 0, len(m.brokers))
	for _, broker := range m.brokers {
		copy := *broker
		brokers = append(brokers, &copy)
	}
	return brokers, nil
}

func (m *mockMetadataStore) DeleteBroker(ctx context.Context, brokerID int32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.brokers, brokerID)
	return nil
}

func TestBrokerMetadata_Address(t *testing.T) {
	broker := &BrokerMetadata{
		ID:   1,
		Host: "localhost",
		Port: 9092,
	}

	expected := "localhost:9092"
	if broker.Address() != expected {
		t.Errorf("Address() = %s, want %s", broker.Address(), expected)
	}
}

func TestBrokerMetadata_IsHealthy(t *testing.T) {
	tests := []struct {
		name     string
		status   BrokerStatus
		expected bool
	}{
		{"alive", BrokerStatusAlive, true},
		{"starting", BrokerStatusStarting, true},
		{"failed", BrokerStatusFailed, false},
		{"draining", BrokerStatusDraining, false},
		{"decommissioned", BrokerStatusDecommissioned, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker := &BrokerMetadata{Status: tt.status}
			if broker.IsHealthy() != tt.expected {
				t.Errorf("IsHealthy() = %v, want %v", broker.IsHealthy(), tt.expected)
			}
		})
	}
}

func TestBrokerMetadata_DiskUtilization(t *testing.T) {
	broker := &BrokerMetadata{
		DiskCapacityGB: 1000,
		DiskUsedGB:     750,
	}

	expected := 75.0
	util := broker.DiskUtilization()
	if util != expected {
		t.Errorf("DiskUtilization() = %.2f, want %.2f", util, expected)
	}

	// Test zero capacity
	broker.DiskCapacityGB = 0
	if broker.DiskUtilization() != 0 {
		t.Error("DiskUtilization() should return 0 for zero capacity")
	}
}

func TestBrokerRegistry_RegisterBroker(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)

	broker := &BrokerMetadata{
		ID:   1,
		Host: "localhost",
		Port: 9092,
		Rack: "rack-1",
	}

	ctx := context.Background()
	err := registry.RegisterBroker(ctx, broker)
	if err != nil {
		t.Fatalf("RegisterBroker failed: %v", err)
	}

	// Verify broker registered
	retrieved, err := registry.GetBroker(1)
	if err != nil {
		t.Fatalf("GetBroker failed: %v", err)
	}

	if retrieved.ID != 1 {
		t.Errorf("Broker ID = %d, want 1", retrieved.ID)
	}
	if retrieved.Status != BrokerStatusStarting {
		t.Errorf("Broker status = %s, want %s", retrieved.Status, BrokerStatusStarting)
	}
	if retrieved.RegisteredAt.IsZero() {
		t.Error("RegisteredAt should be set")
	}
}

func TestBrokerRegistry_DeregisterBroker(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)

	// Register broker
	broker := &BrokerMetadata{
		ID:   1,
		Host: "localhost",
		Port: 9092,
	}

	ctx := context.Background()
	registry.RegisterBroker(ctx, broker)

	// Deregister broker
	err := registry.DeregisterBroker(ctx, 1)
	if err != nil {
		t.Fatalf("DeregisterBroker failed: %v", err)
	}

	// Verify status changed
	retrieved, _ := registry.GetBroker(1)
	if retrieved.Status != BrokerStatusDecommissioned {
		t.Errorf("Status = %s, want %s", retrieved.Status, BrokerStatusDecommissioned)
	}
	if retrieved.DecommissionedAt.IsZero() {
		t.Error("DecommissionedAt should be set")
	}
}

func TestBrokerRegistry_RecordHeartbeat(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)

	// Register broker
	broker := &BrokerMetadata{
		ID:   1,
		Host: "localhost",
		Port: 9092,
	}

	ctx := context.Background()
	registry.RegisterBroker(ctx, broker)

	// Record heartbeat
	time.Sleep(10 * time.Millisecond)
	err := registry.RecordHeartbeat(1)
	if err != nil {
		t.Fatalf("RecordHeartbeat failed: %v", err)
	}

	// Verify heartbeat time updated
	retrieved, _ := registry.GetBroker(1)
	if time.Since(retrieved.LastHeartbeat) > 1*time.Second {
		t.Error("LastHeartbeat should be recent")
	}

	// Verify status changed to alive
	if retrieved.Status != BrokerStatusAlive {
		t.Errorf("Status = %s, want %s", retrieved.Status, BrokerStatusAlive)
	}
}

func TestBrokerRegistry_ListBrokers(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)

	ctx := context.Background()

	// Register multiple brokers
	for i := int32(1); i <= 3; i++ {
		broker := &BrokerMetadata{
			ID:   i,
			Host: "localhost",
			Port: 9090 + int(i),
		}
		registry.RegisterBroker(ctx, broker)
	}

	// List all brokers
	brokers := registry.ListBrokers()
	if len(brokers) != 3 {
		t.Errorf("ListBrokers() returned %d brokers, want 3", len(brokers))
	}
}

func TestBrokerRegistry_ListActiveBrokers(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)

	ctx := context.Background()

	// Register brokers with different statuses
	registry.RegisterBroker(ctx, &BrokerMetadata{ID: 1, Host: "host1", Port: 9092})
	registry.RegisterBroker(ctx, &BrokerMetadata{ID: 2, Host: "host2", Port: 9092})
	registry.RegisterBroker(ctx, &BrokerMetadata{ID: 3, Host: "host3", Port: 9092})

	// Mark broker 1 as alive
	registry.RecordHeartbeat(1)

	// Decommission broker 3
	registry.DeregisterBroker(ctx, 3)

	// List active brokers (should only include broker 1 and 2)
	active := registry.ListActiveBrokers()

	// Broker 1 is alive, broker 2 is starting, broker 3 is decommissioned
	// IsActive() returns true for ALIVE status only
	count := 0
	for _, b := range active {
		if b.Status == BrokerStatusAlive {
			count++
		}
	}

	if count != 1 {
		t.Errorf("Expected 1 alive broker, got %d", count)
	}
}

func TestBrokerRegistry_GetBrokerCount(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)

	ctx := context.Background()

	if registry.GetBrokerCount() != 0 {
		t.Error("Initial broker count should be 0")
	}

	// Register brokers
	for i := int32(1); i <= 5; i++ {
		broker := &BrokerMetadata{
			ID:   i,
			Host: "localhost",
			Port: 9090 + int(i),
		}
		registry.RegisterBroker(ctx, broker)
	}

	if registry.GetBrokerCount() != 5 {
		t.Errorf("Broker count = %d, want 5", registry.GetBrokerCount())
	}
}

func TestBrokerRegistry_HealthMonitoring(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)
	registry.heartbeatTimeout = 100 * time.Millisecond
	registry.checkInterval = 50 * time.Millisecond

	ctx := context.Background()

	// Register broker
	broker := &BrokerMetadata{
		ID:   1,
		Host: "localhost",
		Port: 9092,
	}
	registry.RegisterBroker(ctx, broker)

	// Mark as alive
	registry.RecordHeartbeat(1)

	// Start health monitoring
	registry.Start()
	defer registry.Stop()

	// Wait for heartbeat timeout
	time.Sleep(200 * time.Millisecond)

	// Check broker status should be failed
	retrieved, _ := registry.GetBroker(1)
	if retrieved.Status != BrokerStatusFailed {
		t.Errorf("Status = %s, want %s after timeout", retrieved.Status, BrokerStatusFailed)
	}
}

func TestBrokerRegistry_Callbacks(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)
	registry.heartbeatTimeout = 100 * time.Millisecond

	ctx := context.Background()

	// Track callbacks
	var addedCalled, removedCalled bool
	var mu sync.Mutex

	registry.SetOnBrokerAdded(func(broker *BrokerMetadata) {
		mu.Lock()
		addedCalled = true
		mu.Unlock()
	})

	registry.SetOnBrokerRemoved(func(brokerID int32) {
		mu.Lock()
		removedCalled = true
		mu.Unlock()
	})

	registry.SetOnBrokerFailed(func(brokerID int32) {
		// Failure callback
	})

	// Register broker (should trigger onAdded)
	broker := &BrokerMetadata{
		ID:   1,
		Host: "localhost",
		Port: 9092,
	}
	registry.RegisterBroker(ctx, broker)

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if !addedCalled {
		t.Error("onBrokerAdded callback not called")
	}
	mu.Unlock()

	// Deregister broker (should trigger onRemoved)
	registry.DeregisterBroker(ctx, 1)

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if !removedCalled {
		t.Error("onBrokerRemoved callback not called")
	}
	mu.Unlock()
}

func TestBrokerRegistry_GetBrokerStats(t *testing.T) {
	store := newMockMetadataStore()
	registry := NewBrokerRegistry(store)

	ctx := context.Background()

	// Register brokers with different statuses and disk usage
	registry.RegisterBroker(ctx, &BrokerMetadata{
		ID:             1,
		Host:           "host1",
		Port:           9092,
		DiskCapacityGB: 1000,
		DiskUsedGB:     500,
	})

	registry.RegisterBroker(ctx, &BrokerMetadata{
		ID:             2,
		Host:           "host2",
		Port:           9092,
		DiskCapacityGB: 1000,
		DiskUsedGB:     700,
	})

	// Mark broker 1 as alive
	registry.RecordHeartbeat(1)

	stats := registry.GetBrokerStats()

	if stats.TotalBrokers != 2 {
		t.Errorf("TotalBrokers = %d, want 2", stats.TotalBrokers)
	}

	if stats.TotalDiskCapacityGB != 2000 {
		t.Errorf("TotalDiskCapacityGB = %d, want 2000", stats.TotalDiskCapacityGB)
	}

	if stats.TotalDiskUsedGB != 1200 {
		t.Errorf("TotalDiskUsedGB = %d, want 1200", stats.TotalDiskUsedGB)
	}

	expectedUtil := 60.0 // (1200 / 2000) * 100
	if stats.AvgDiskUtilization < expectedUtil-0.1 || stats.AvgDiskUtilization > expectedUtil+0.1 {
		t.Errorf("AvgDiskUtilization = %.2f, want %.2f", stats.AvgDiskUtilization, expectedUtil)
	}
}
