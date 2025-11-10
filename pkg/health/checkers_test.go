package health

import (
	"context"
	"testing"
	"time"
)

// Mock implementations for testing

type mockRaftNode struct {
	isLeader bool
	leaderID uint64
}

func (m *mockRaftNode) IsLeader() bool {
	return m.isLeader
}

func (m *mockRaftNode) Leader() uint64 {
	return m.leaderID
}

type mockMetadataState struct {
	version        uint64
	brokerCount    int
	partitionCount int
}

func (m *mockMetadataState) GetVersion() uint64 {
	return m.version
}

func (m *mockMetadataState) GetBrokerCount() int {
	return m.brokerCount
}

func (m *mockMetadataState) GetPartitionCount() int {
	return m.partitionCount
}

type mockMetadataStore struct {
	isLeader bool
	state    *mockMetadataState
}

func (m *mockMetadataStore) IsLeader() bool {
	return m.isLeader
}

func (m *mockMetadataStore) GetState() MetadataState {
	return m.state
}

type mockBrokerRegistry struct {
	totalBrokers  int
	activeBrokers int
}

func (m *mockBrokerRegistry) GetBrokerCount() int {
	return m.totalBrokers
}

func (m *mockBrokerRegistry) GetActiveBrokerCount() int {
	return m.activeBrokers
}

type mockRebalanceStats struct {
	isRebalancing    bool
	lastRebalance    time.Time
	rebalanceCount   int64
	failedRebalances int64
}

func (m *mockRebalanceStats) IsRebalancing() bool {
	return m.isRebalancing
}

func (m *mockRebalanceStats) GetLastRebalanceTime() time.Time {
	return m.lastRebalance
}

func (m *mockRebalanceStats) GetRebalanceCount() int64 {
	return m.rebalanceCount
}

func (m *mockRebalanceStats) GetFailedRebalances() int64 {
	return m.failedRebalances
}

type mockCurrentAssignment struct {
	totalPartitions int
}

func (m *mockCurrentAssignment) TotalPartitions() int {
	return m.totalPartitions
}

type mockCoordinator struct {
	stats      *mockRebalanceStats
	assignment *mockCurrentAssignment
}

func (m *mockCoordinator) GetRebalanceStats() RebalanceStats {
	return m.stats
}

func (m *mockCoordinator) GetCurrentAssignment() CurrentAssignment {
	return m.assignment
}

// Tests for RaftNodeHealthChecker

func TestNewRaftNodeHealthChecker(t *testing.T) {
	node := &mockRaftNode{}
	checker := NewRaftNodeHealthChecker("raft-test", node)

	if checker == nil {
		t.Fatal("NewRaftNodeHealthChecker returned nil")
	}

	if checker.Name() != "raft-test" {
		t.Errorf("Name() = %s, want raft-test", checker.Name())
	}
}

func TestRaftNodeHealthChecker_Check_Leader(t *testing.T) {
	node := &mockRaftNode{
		isLeader: true,
		leaderID: 1,
	}
	checker := NewRaftNodeHealthChecker("raft-test", node)

	check := checker.Check(context.Background())

	if check.Status != StatusHealthy {
		t.Errorf("Status = %v, want StatusHealthy", check.Status)
	}

	if check.Message != "node is healthy (leader)" {
		t.Errorf("Message = %s, want 'node is healthy (leader)'", check.Message)
	}

	if !check.Details["is_leader"].(bool) {
		t.Error("Details[is_leader] should be true")
	}

	if check.Details["leader_id"].(uint64) != 1 {
		t.Errorf("Details[leader_id] = %d, want 1", check.Details["leader_id"])
	}
}

func TestRaftNodeHealthChecker_Check_Follower(t *testing.T) {
	node := &mockRaftNode{
		isLeader: false,
		leaderID: 2,
	}
	checker := NewRaftNodeHealthChecker("raft-test", node)

	check := checker.Check(context.Background())

	if check.Status != StatusHealthy {
		t.Errorf("Status = %v, want StatusHealthy", check.Status)
	}

	if check.Message != "node is healthy (follower)" {
		t.Errorf("Message = %s, want 'node is healthy (follower)'", check.Message)
	}

	if check.Details["is_leader"].(bool) {
		t.Error("Details[is_leader] should be false")
	}
}

func TestRaftNodeHealthChecker_Check_NoLeader(t *testing.T) {
	node := &mockRaftNode{
		isLeader: false,
		leaderID: 0,
	}
	checker := NewRaftNodeHealthChecker("raft-test", node)

	check := checker.Check(context.Background())

	if check.Status != StatusDegraded {
		t.Errorf("Status = %v, want StatusDegraded", check.Status)
	}

	if check.Message != "no leader elected" {
		t.Errorf("Message = %s, want 'no leader elected'", check.Message)
	}
}

func TestRaftNodeHealthChecker_Check_NilNode(t *testing.T) {
	checker := NewRaftNodeHealthChecker("raft-test", nil)

	check := checker.Check(context.Background())

	if check.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want StatusUnhealthy", check.Status)
	}

	if check.Message != "raft node is nil" {
		t.Errorf("Message = %s, want 'raft node is nil'", check.Message)
	}
}

// Tests for MetadataStoreHealthChecker

func TestNewMetadataStoreHealthChecker(t *testing.T) {
	store := &mockMetadataStore{}
	checker := NewMetadataStoreHealthChecker("metadata-test", store)

	if checker == nil {
		t.Fatal("NewMetadataStoreHealthChecker returned nil")
	}

	if checker.Name() != "metadata-test" {
		t.Errorf("Name() = %s, want metadata-test", checker.Name())
	}
}

func TestMetadataStoreHealthChecker_Check(t *testing.T) {
	store := &mockMetadataStore{
		isLeader: true,
		state: &mockMetadataState{
			version:        100,
			brokerCount:    3,
			partitionCount: 10,
		},
	}
	checker := NewMetadataStoreHealthChecker("metadata-test", store)

	check := checker.Check(context.Background())

	if check.Status != StatusHealthy {
		t.Errorf("Status = %v, want StatusHealthy", check.Status)
	}

	if !check.Details["is_leader"].(bool) {
		t.Error("Details[is_leader] should be true")
	}

	if check.Details["version"].(uint64) != 100 {
		t.Errorf("Details[version] = %d, want 100", check.Details["version"])
	}

	if check.Details["broker_count"].(int) != 3 {
		t.Errorf("Details[broker_count] = %d, want 3", check.Details["broker_count"])
	}

	if check.Details["partition_count"].(int) != 10 {
		t.Errorf("Details[partition_count] = %d, want 10", check.Details["partition_count"])
	}
}

func TestMetadataStoreHealthChecker_Check_NilStore(t *testing.T) {
	checker := NewMetadataStoreHealthChecker("metadata-test", nil)

	check := checker.Check(context.Background())

	if check.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want StatusUnhealthy", check.Status)
	}

	if check.Message != "metadata store is nil" {
		t.Errorf("Message = %s, want 'metadata store is nil'", check.Message)
	}
}

// Tests for BrokerRegistryHealthChecker

func TestNewBrokerRegistryHealthChecker(t *testing.T) {
	registry := &mockBrokerRegistry{}
	checker := NewBrokerRegistryHealthChecker("registry-test", registry)

	if checker == nil {
		t.Fatal("NewBrokerRegistryHealthChecker returned nil")
	}

	if checker.Name() != "registry-test" {
		t.Errorf("Name() = %s, want registry-test", checker.Name())
	}
}

func TestBrokerRegistryHealthChecker_Check_AllActive(t *testing.T) {
	registry := &mockBrokerRegistry{
		totalBrokers:  3,
		activeBrokers: 3,
	}
	checker := NewBrokerRegistryHealthChecker("registry-test", registry)

	check := checker.Check(context.Background())

	if check.Status != StatusHealthy {
		t.Errorf("Status = %v, want StatusHealthy", check.Status)
	}

	if check.Message != "3 of 3 brokers active" {
		t.Errorf("Message = %s, want '3 of 3 brokers active'", check.Message)
	}
}

func TestBrokerRegistryHealthChecker_Check_NoActiveBrokers(t *testing.T) {
	registry := &mockBrokerRegistry{
		totalBrokers:  3,
		activeBrokers: 0,
	}
	checker := NewBrokerRegistryHealthChecker("registry-test", registry)

	check := checker.Check(context.Background())

	if check.Status != StatusDegraded {
		t.Errorf("Status = %v, want StatusDegraded", check.Status)
	}

	if check.Message != "no active brokers available" {
		t.Errorf("Message = %s, want 'no active brokers available'", check.Message)
	}
}

func TestBrokerRegistryHealthChecker_Check_NoBrokers(t *testing.T) {
	registry := &mockBrokerRegistry{
		totalBrokers:  0,
		activeBrokers: 0,
	}
	checker := NewBrokerRegistryHealthChecker("registry-test", registry)

	check := checker.Check(context.Background())

	if check.Status != StatusDegraded {
		t.Errorf("Status = %v, want StatusDegraded", check.Status)
	}

	if check.Message != "no brokers registered" {
		t.Errorf("Message = %s, want 'no brokers registered'", check.Message)
	}
}

func TestBrokerRegistryHealthChecker_Check_NilRegistry(t *testing.T) {
	checker := NewBrokerRegistryHealthChecker("registry-test", nil)

	check := checker.Check(context.Background())

	if check.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want StatusUnhealthy", check.Status)
	}

	if check.Message != "broker registry is nil" {
		t.Errorf("Message = %s, want 'broker registry is nil'", check.Message)
	}
}

// Tests for CoordinatorHealthChecker

func TestNewCoordinatorHealthChecker(t *testing.T) {
	coord := &mockCoordinator{}
	checker := NewCoordinatorHealthChecker("coord-test", coord)

	if checker == nil {
		t.Fatal("NewCoordinatorHealthChecker returned nil")
	}

	if checker.Name() != "coord-test" {
		t.Errorf("Name() = %s, want coord-test", checker.Name())
	}
}

func TestCoordinatorHealthChecker_Check_Healthy(t *testing.T) {
	coord := &mockCoordinator{
		stats: &mockRebalanceStats{
			isRebalancing:    false,
			lastRebalance:    time.Now().Add(-1 * time.Hour),
			rebalanceCount:   10,
			failedRebalances: 1,
		},
		assignment: &mockCurrentAssignment{
			totalPartitions: 100,
		},
	}
	checker := NewCoordinatorHealthChecker("coord-test", coord)

	check := checker.Check(context.Background())

	if check.Status != StatusHealthy {
		t.Errorf("Status = %v, want StatusHealthy", check.Status)
	}

	if check.Details["rebalance_count"].(int64) != 10 {
		t.Errorf("Details[rebalance_count] = %d, want 10", check.Details["rebalance_count"])
	}

	if check.Details["total_partitions"].(int) != 100 {
		t.Errorf("Details[total_partitions] = %d, want 100", check.Details["total_partitions"])
	}
}

func TestCoordinatorHealthChecker_Check_Rebalancing(t *testing.T) {
	coord := &mockCoordinator{
		stats: &mockRebalanceStats{
			isRebalancing:    true,
			rebalanceCount:   5,
			failedRebalances: 0,
		},
		assignment: &mockCurrentAssignment{
			totalPartitions: 50,
		},
	}
	checker := NewCoordinatorHealthChecker("coord-test", coord)

	check := checker.Check(context.Background())

	if check.Status != StatusHealthy {
		t.Errorf("Status = %v, want StatusHealthy", check.Status)
	}

	if check.Message != "rebalancing in progress" {
		t.Errorf("Message = %s, want 'rebalancing in progress'", check.Message)
	}
}

func TestCoordinatorHealthChecker_Check_HighFailureRate(t *testing.T) {
	coord := &mockCoordinator{
		stats: &mockRebalanceStats{
			isRebalancing:    false,
			rebalanceCount:   10,
			failedRebalances: 6,
		},
		assignment: &mockCurrentAssignment{
			totalPartitions: 100,
		},
	}
	checker := NewCoordinatorHealthChecker("coord-test", coord)

	check := checker.Check(context.Background())

	if check.Status != StatusDegraded {
		t.Errorf("Status = %v, want StatusDegraded", check.Status)
	}

	if check.Details["failed_rebalances"].(int64) != 6 {
		t.Errorf("Details[failed_rebalances] = %d, want 6", check.Details["failed_rebalances"])
	}
}

func TestCoordinatorHealthChecker_Check_NilCoordinator(t *testing.T) {
	checker := NewCoordinatorHealthChecker("coord-test", nil)

	check := checker.Check(context.Background())

	if check.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want StatusUnhealthy", check.Status)
	}

	if check.Message != "coordinator is nil" {
		t.Errorf("Message = %s, want 'coordinator is nil'", check.Message)
	}
}

// Tests for ComponentHealthChecker

func TestNewComponentHealthChecker(t *testing.T) {
	checker1 := NewSimpleChecker("test1", func(ctx context.Context) Check {
		return Check{Name: "test1", Status: StatusHealthy, Message: "test1 ok"}
	})
	checker2 := NewSimpleChecker("test2", func(ctx context.Context) Check {
		return Check{Name: "test2", Status: StatusHealthy, Message: "test2 ok"}
	})

	component := NewComponentHealthChecker("component-test", checker1, checker2)

	if component == nil {
		t.Fatal("NewComponentHealthChecker returned nil")
	}

	if component.Name() != "component-test" {
		t.Errorf("Name() = %s, want component-test", component.Name())
	}
}

func TestComponentHealthChecker_Check_AllHealthy(t *testing.T) {
	checker1 := NewSimpleChecker("test1", func(ctx context.Context) Check {
		return Check{Name: "test1", Status: StatusHealthy, Message: "test1 ok"}
	})
	checker2 := NewSimpleChecker("test2", func(ctx context.Context) Check {
		return Check{Name: "test2", Status: StatusHealthy, Message: "test2 ok"}
	})

	component := NewComponentHealthChecker("component-test", checker1, checker2)
	check := component.Check(context.Background())

	if check.Status != StatusHealthy {
		t.Errorf("Status = %v, want StatusHealthy", check.Status)
	}

	if check.Message != "all components healthy" {
		t.Errorf("Message = %s, want 'all components healthy'", check.Message)
	}
}

func TestComponentHealthChecker_Check_OneDegraded(t *testing.T) {
	checker1 := NewSimpleChecker("test1", func(ctx context.Context) Check {
		return Check{Name: "test1", Status: StatusHealthy, Message: "test1 ok"}
	})
	checker2 := NewSimpleChecker("test2", func(ctx context.Context) Check {
		return Check{Name: "test2", Status: StatusDegraded, Message: "test2 degraded"}
	})

	component := NewComponentHealthChecker("component-test", checker1, checker2)
	check := component.Check(context.Background())

	if check.Status != StatusDegraded {
		t.Errorf("Status = %v, want StatusDegraded", check.Status)
	}

	if check.Message != "one or more components degraded" {
		t.Errorf("Message = %s, want 'one or more components degraded'", check.Message)
	}
}

func TestComponentHealthChecker_Check_OneUnhealthy(t *testing.T) {
	checker1 := NewSimpleChecker("test1", func(ctx context.Context) Check {
		return Check{Name: "test1", Status: StatusHealthy, Message: "test1 ok"}
	})
	checker2 := NewSimpleChecker("test2", func(ctx context.Context) Check {
		return Check{Name: "test2", Status: StatusUnhealthy, Message: "test2 unhealthy"}
	})

	component := NewComponentHealthChecker("component-test", checker1, checker2)
	check := component.Check(context.Background())

	if check.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want StatusUnhealthy", check.Status)
	}

	if check.Message != "one or more components unhealthy" {
		t.Errorf("Message = %s, want 'one or more components unhealthy'", check.Message)
	}
}

func TestComponentHealthChecker_Check_NoCheckers(t *testing.T) {
	component := NewComponentHealthChecker("component-test")
	check := component.Check(context.Background())

	if check.Status != StatusUnknown {
		t.Errorf("Status = %v, want StatusUnknown", check.Status)
	}

	if check.Message != "no components to check" {
		t.Errorf("Message = %s, want 'no components to check'", check.Message)
	}
}
