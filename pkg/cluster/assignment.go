package cluster

import (
	"fmt"
	"sort"
)

// AssignmentStrategy defines how partitions are assigned to brokers
type AssignmentStrategy interface {
	// Name returns the strategy name
	Name() string

	// Assign assigns partitions to brokers
	Assign(
		partitions []PartitionInfo,
		brokers []BrokerInfo,
		constraints *AssignmentConstraints,
	) (*Assignment, error)

	// Rebalance rebalances partitions across brokers
	Rebalance(
		current *Assignment,
		brokers []BrokerInfo,
		constraints *AssignmentConstraints,
	) (*Assignment, error)
}

// PartitionInfo contains information about a partition
type PartitionInfo struct {
	Topic       string
	PartitionID int
	Replicas    int // Replication factor
}

// BrokerInfo contains information about a broker
type BrokerInfo struct {
	ID       int32
	Rack     string // Availability zone / rack ID
	Capacity int    // Max partitions this broker can handle
}

// AssignmentConstraints defines constraints for partition assignment
type AssignmentConstraints struct {
	// MinInSyncReplicas is minimum replicas that must be in-sync
	MinInSyncReplicas int

	// RackAware enables rack-aware assignment (spread replicas across racks)
	RackAware bool

	// MaxPartitionsPerBroker limits partitions per broker
	MaxPartitionsPerBroker int

	// PreferredLeaders maps partition keys to preferred leader broker IDs
	PreferredLeaders map[string]int32

	// ExcludedBrokers are brokers that should not receive new assignments
	ExcludedBrokers map[int32]bool
}

// Assignment represents the result of partition assignment
type Assignment struct {
	// Partitions maps partition key (topic:partition) to replica list
	Partitions map[string][]int32

	// Leaders maps partition key to leader broker ID
	Leaders map[string]int32

	// BrokerLoad tracks partition count per broker
	BrokerLoad map[int32]int

	// Version is incremented on each assignment
	Version int64
}

// NewAssignment creates a new empty assignment
func NewAssignment() *Assignment {
	return &Assignment{
		Partitions: make(map[string][]int32),
		Leaders:    make(map[string]int32),
		BrokerLoad: make(map[int32]int),
		Version:    0,
	}
}

// PartitionKey generates a unique key for a partition
func PartitionKey(topic string, partitionID int) string {
	return fmt.Sprintf("%s:%d", topic, partitionID)
}

// AddReplica adds a replica assignment for a partition
func (a *Assignment) AddReplica(topic string, partitionID int, replicas []int32) {
	key := PartitionKey(topic, partitionID)
	a.Partitions[key] = replicas

	// First replica is the leader
	if len(replicas) > 0 {
		a.Leaders[key] = replicas[0]
	}

	// Update broker load
	for _, broker := range replicas {
		a.BrokerLoad[broker]++
	}
}

// GetReplicas returns replicas for a partition
func (a *Assignment) GetReplicas(topic string, partitionID int) []int32 {
	key := PartitionKey(topic, partitionID)
	return a.Partitions[key]
}

// GetLeader returns the leader for a partition
func (a *Assignment) GetLeader(topic string, partitionID int) int32 {
	key := PartitionKey(topic, partitionID)
	return a.Leaders[key]
}

// TotalPartitions returns total partition count
func (a *Assignment) TotalPartitions() int {
	return len(a.Partitions)
}

// IsBalanced checks if the assignment is balanced across brokers
func (a *Assignment) IsBalanced(maxImbalance int) bool {
	if len(a.BrokerLoad) == 0 {
		return true
	}

	min, max := a.getLoadRange()
	return (max - min) <= maxImbalance
}

// getLoadRange returns min and max partition counts across brokers
func (a *Assignment) getLoadRange() (int, int) {
	if len(a.BrokerLoad) == 0 {
		return 0, 0
	}

	min, max := int(^uint(0)>>1), 0
	for _, load := range a.BrokerLoad {
		if load < min {
			min = load
		}
		if load > max {
			max = load
		}
	}
	return min, max
}

// Clone creates a deep copy of the assignment
func (a *Assignment) Clone() *Assignment {
	clone := NewAssignment()
	clone.Version = a.Version

	// Copy partitions
	for key, replicas := range a.Partitions {
		replicasCopy := make([]int32, len(replicas))
		copy(replicasCopy, replicas)
		clone.Partitions[key] = replicasCopy
	}

	// Copy leaders
	for key, leader := range a.Leaders {
		clone.Leaders[key] = leader
	}

	// Copy broker load
	for broker, load := range a.BrokerLoad {
		clone.BrokerLoad[broker] = load
	}

	return clone
}

// RecomputeBrokerLoad recomputes broker load from actual partition assignments
func (a *Assignment) RecomputeBrokerLoad() {
	a.BrokerLoad = make(map[int32]int)
	for _, replicas := range a.Partitions {
		for _, broker := range replicas {
			a.BrokerLoad[broker]++
		}
	}
}

// Validate validates the assignment
func (a *Assignment) Validate() error {
	// Check that all partitions have leaders
	for key := range a.Partitions {
		if _, exists := a.Leaders[key]; !exists {
			return fmt.Errorf("partition %s has no leader", key)
		}
	}

	// Check that leaders are in replica sets
	for key, leader := range a.Leaders {
		replicas := a.Partitions[key]
		found := false
		for _, replica := range replicas {
			if replica == leader {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("partition %s leader %d not in replica set %v", key, leader, replicas)
		}
	}

	// Verify broker load matches actual assignment
	computedLoad := make(map[int32]int)
	for _, replicas := range a.Partitions {
		for _, broker := range replicas {
			computedLoad[broker]++
		}
	}

	for broker, load := range a.BrokerLoad {
		if computedLoad[broker] != load {
			return fmt.Errorf("broker %d load mismatch: recorded=%d, actual=%d",
				broker, load, computedLoad[broker])
		}
	}

	return nil
}

// GetStats returns assignment statistics
func (a *Assignment) GetStats() AssignmentStats {
	min, max := a.getLoadRange()

	totalLoad := 0
	for _, load := range a.BrokerLoad {
		totalLoad += load
	}

	avgLoad := 0.0
	if len(a.BrokerLoad) > 0 {
		avgLoad = float64(totalLoad) / float64(len(a.BrokerLoad))
	}

	return AssignmentStats{
		TotalPartitions: len(a.Partitions),
		TotalBrokers:    len(a.BrokerLoad),
		MinLoad:         min,
		MaxLoad:         max,
		AvgLoad:         avgLoad,
		Imbalance:       max - min,
	}
}

// AssignmentStats contains statistics about an assignment
type AssignmentStats struct {
	TotalPartitions int
	TotalBrokers    int
	MinLoad         int
	MaxLoad         int
	AvgLoad         float64
	Imbalance       int
}

// sortBrokersByLoad sorts brokers by their current load (ascending)
func sortBrokersByLoad(brokers []BrokerInfo, load map[int32]int) []BrokerInfo {
	sorted := make([]BrokerInfo, len(brokers))
	copy(sorted, brokers)

	sort.Slice(sorted, func(i, j int) bool {
		loadI := load[sorted[i].ID]
		loadJ := load[sorted[j].ID]

		// Primary: sort by load
		if loadI != loadJ {
			return loadI < loadJ
		}

		// Secondary: sort by ID for determinism
		return sorted[i].ID < sorted[j].ID
	})

	return sorted
}

// filterExcludedBrokers removes excluded brokers from the list
func filterExcludedBrokers(brokers []BrokerInfo, excluded map[int32]bool) []BrokerInfo {
	if len(excluded) == 0 {
		return brokers
	}

	filtered := make([]BrokerInfo, 0, len(brokers))
	for _, broker := range brokers {
		if !excluded[broker.ID] {
			filtered = append(filtered, broker)
		}
	}
	return filtered
}
