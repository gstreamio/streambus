package health

import (
	"context"
	"fmt"
	"time"
)

// RaftNodeHealthChecker checks the health of a Raft consensus node
type RaftNodeHealthChecker struct {
	name string
	node RaftNode
}

// RaftNode is the interface for checking Raft node health
type RaftNode interface {
	IsLeader() bool
	Leader() uint64
	// Add more methods as needed
}

// NewRaftNodeHealthChecker creates a health checker for a Raft node
func NewRaftNodeHealthChecker(name string, node RaftNode) *RaftNodeHealthChecker {
	return &RaftNodeHealthChecker{
		name: name,
		node: node,
	}
}

// Name returns the checker name
func (r *RaftNodeHealthChecker) Name() string {
	return r.name
}

// Check performs the health check
func (r *RaftNodeHealthChecker) Check(ctx context.Context) Check {
	check := Check{
		Name:    r.name,
		Details: make(map[string]interface{}),
	}

	// Check if node exists
	if r.node == nil {
		check.Status = StatusUnhealthy
		check.Message = "raft node is nil"
		return check
	}

	// Get leader info
	leaderID := r.node.Leader()
	isLeader := r.node.IsLeader()

	check.Details["is_leader"] = isLeader
	check.Details["leader_id"] = leaderID

	// Determine health status
	if leaderID == 0 {
		// No leader elected
		check.Status = StatusDegraded
		check.Message = "no leader elected"
	} else {
		check.Status = StatusHealthy
		if isLeader {
			check.Message = "node is healthy (leader)"
		} else {
			check.Message = "node is healthy (follower)"
		}
	}

	return check
}

// MetadataStoreHealthChecker checks metadata store health
type MetadataStoreHealthChecker struct {
	name  string
	store MetadataStore
}

// MetadataStore interface for health checking
type MetadataStore interface {
	IsLeader() bool
	GetState() MetadataState
}

// MetadataState represents the metadata state
type MetadataState interface {
	GetVersion() uint64
	GetBrokerCount() int
	GetPartitionCount() int
}

// NewMetadataStoreHealthChecker creates a health checker for metadata store
func NewMetadataStoreHealthChecker(name string, store MetadataStore) *MetadataStoreHealthChecker {
	return &MetadataStoreHealthChecker{
		name:  name,
		store: store,
	}
}

// Name returns the checker name
func (m *MetadataStoreHealthChecker) Name() string {
	return m.name
}

// Check performs the health check
func (m *MetadataStoreHealthChecker) Check(ctx context.Context) Check {
	check := Check{
		Name:    m.name,
		Details: make(map[string]interface{}),
	}

	if m.store == nil {
		check.Status = StatusUnhealthy
		check.Message = "metadata store is nil"
		return check
	}

	// Get state
	state := m.store.GetState()
	isLeader := m.store.IsLeader()

	check.Details["is_leader"] = isLeader
	check.Details["version"] = state.GetVersion()
	check.Details["broker_count"] = state.GetBrokerCount()
	check.Details["partition_count"] = state.GetPartitionCount()

	check.Status = StatusHealthy
	check.Message = fmt.Sprintf("metadata store healthy (version=%d, brokers=%d, partitions=%d)",
		state.GetVersion(), state.GetBrokerCount(), state.GetPartitionCount())

	return check
}

// BrokerRegistryHealthChecker checks broker registry health
type BrokerRegistryHealthChecker struct {
	name     string
	registry BrokerRegistry
}

// BrokerRegistry interface for health checking
type BrokerRegistry interface {
	GetBrokerCount() int
	GetActiveBrokerCount() int
}

// NewBrokerRegistryHealthChecker creates a health checker for broker registry
func NewBrokerRegistryHealthChecker(name string, registry BrokerRegistry) *BrokerRegistryHealthChecker {
	return &BrokerRegistryHealthChecker{
		name:     name,
		registry: registry,
	}
}

// Name returns the checker name
func (b *BrokerRegistryHealthChecker) Name() string {
	return b.name
}

// Check performs the health check
func (b *BrokerRegistryHealthChecker) Check(ctx context.Context) Check {
	check := Check{
		Name:    b.name,
		Details: make(map[string]interface{}),
	}

	if b.registry == nil {
		check.Status = StatusUnhealthy
		check.Message = "broker registry is nil"
		return check
	}

	totalBrokers := b.registry.GetBrokerCount()
	activeBrokers := b.registry.GetActiveBrokerCount()

	check.Details["total_brokers"] = totalBrokers
	check.Details["active_brokers"] = activeBrokers

	if activeBrokers == 0 && totalBrokers > 0 {
		check.Status = StatusDegraded
		check.Message = "no active brokers available"
	} else if totalBrokers == 0 {
		check.Status = StatusDegraded
		check.Message = "no brokers registered"
	} else {
		check.Status = StatusHealthy
		check.Message = fmt.Sprintf("%d of %d brokers active", activeBrokers, totalBrokers)
	}

	return check
}

// CoordinatorHealthChecker checks cluster coordinator health
type CoordinatorHealthChecker struct {
	name        string
	coordinator ClusterCoordinator
}

// ClusterCoordinator interface for health checking
type ClusterCoordinator interface {
	GetRebalanceStats() RebalanceStats
	GetCurrentAssignment() CurrentAssignment
}

// RebalanceStats represents rebalancing statistics
type RebalanceStats interface {
	IsRebalancing() bool
	GetLastRebalanceTime() time.Time
	GetRebalanceCount() int64
	GetFailedRebalances() int64
}

// CurrentAssignment represents the current partition assignment
type CurrentAssignment interface {
	TotalPartitions() int
}

// NewCoordinatorHealthChecker creates a health checker for the coordinator
func NewCoordinatorHealthChecker(name string, coordinator ClusterCoordinator) *CoordinatorHealthChecker {
	return &CoordinatorHealthChecker{
		name:        name,
		coordinator: coordinator,
	}
}

// Name returns the checker name
func (c *CoordinatorHealthChecker) Name() string {
	return c.name
}

// Check performs the health check
func (c *CoordinatorHealthChecker) Check(ctx context.Context) Check {
	check := Check{
		Name:    c.name,
		Details: make(map[string]interface{}),
	}

	if c.coordinator == nil {
		check.Status = StatusUnhealthy
		check.Message = "coordinator is nil"
		return check
	}

	stats := c.coordinator.GetRebalanceStats()
	assignment := c.coordinator.GetCurrentAssignment()

	isRebalancing := stats.IsRebalancing()
	lastRebalance := stats.GetLastRebalanceTime()
	rebalanceCount := stats.GetRebalanceCount()
	failedRebalances := stats.GetFailedRebalances()

	check.Details["is_rebalancing"] = isRebalancing
	check.Details["rebalance_count"] = rebalanceCount
	check.Details["failed_rebalances"] = failedRebalances

	if !lastRebalance.IsZero() {
		timeSinceRebalance := time.Since(lastRebalance)
		check.Details["last_rebalance"] = lastRebalance.Format(time.RFC3339)
		check.Details["time_since_last_rebalance_seconds"] = int(timeSinceRebalance.Seconds())
	}

	if assignment != nil {
		check.Details["total_partitions"] = assignment.TotalPartitions()
	}

	// Determine health
	failureRate := float64(0)
	if rebalanceCount > 0 {
		failureRate = float64(failedRebalances) / float64(rebalanceCount)
	}

	if failureRate > 0.5 {
		check.Status = StatusDegraded
		check.Message = fmt.Sprintf("high rebalance failure rate: %.1f%%", failureRate*100)
	} else if isRebalancing {
		check.Status = StatusHealthy
		check.Message = "rebalancing in progress"
	} else {
		check.Status = StatusHealthy
		check.Message = "coordinator healthy"
	}

	return check
}

// ComponentHealthChecker aggregates multiple health checks
type ComponentHealthChecker struct {
	name     string
	checkers []Checker
}

// NewComponentHealthChecker creates a checker that aggregates multiple checks
func NewComponentHealthChecker(name string, checkers ...Checker) *ComponentHealthChecker {
	return &ComponentHealthChecker{
		name:     name,
		checkers: checkers,
	}
}

// Name returns the checker name
func (c *ComponentHealthChecker) Name() string {
	return c.name
}

// Check runs all component checks and aggregates the result
func (c *ComponentHealthChecker) Check(ctx context.Context) Check {
	check := Check{
		Name:    c.name,
		Details: make(map[string]interface{}),
	}

	if len(c.checkers) == 0 {
		check.Status = StatusUnknown
		check.Message = "no components to check"
		return check
	}

	// Run all checks
	results := make(map[string]Check)
	for _, checker := range c.checkers {
		results[checker.Name()] = checker.Check(ctx)
	}

	// Aggregate status
	hasUnhealthy := false
	hasDegraded := false
	for _, result := range results {
		if result.Status == StatusUnhealthy {
			hasUnhealthy = true
		} else if result.Status == StatusDegraded || result.Status == StatusUnknown {
			hasDegraded = true
		}
	}

	check.Details["component_checks"] = results

	if hasUnhealthy {
		check.Status = StatusUnhealthy
		check.Message = "one or more components unhealthy"
	} else if hasDegraded {
		check.Status = StatusDegraded
		check.Message = "one or more components degraded"
	} else {
		check.Status = StatusHealthy
		check.Message = "all components healthy"
	}

	return check
}
