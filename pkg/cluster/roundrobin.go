package cluster

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// RoundRobinStrategy assigns partitions in round-robin fashion across brokers
type RoundRobinStrategy struct{}

// NewRoundRobinStrategy creates a new round-robin assignment strategy
func NewRoundRobinStrategy() *RoundRobinStrategy {
	return &RoundRobinStrategy{}
}

// Name returns the strategy name
func (rr *RoundRobinStrategy) Name() string {
	return "RoundRobin"
}

// Assign assigns partitions to brokers using round-robin
func (rr *RoundRobinStrategy) Assign(
	partitions []PartitionInfo,
	brokers []BrokerInfo,
	constraints *AssignmentConstraints,
) (*Assignment, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("no brokers available")
	}

	if len(partitions) == 0 {
		return NewAssignment(), nil
	}

	// Filter excluded brokers
	availableBrokers := filterExcludedBrokers(brokers, constraints.ExcludedBrokers)
	if len(availableBrokers) == 0 {
		return nil, fmt.Errorf("no available brokers after filtering")
	}

	// Sort brokers by ID for deterministic assignment
	sort.Slice(availableBrokers, func(i, j int) bool {
		return availableBrokers[i].ID < availableBrokers[j].ID
	})

	assignment := NewAssignment()
	brokerIndex := 0

	// Assign each partition
	for _, partition := range partitions {
		replicas, err := rr.selectReplicas(
			partition,
			availableBrokers,
			&brokerIndex,
			constraints,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to assign partition %s:%d: %w",
				partition.Topic, partition.PartitionID, err)
		}

		assignment.AddReplica(partition.Topic, partition.PartitionID, replicas)
	}

	// Validate assignment
	if err := assignment.Validate(); err != nil {
		return nil, fmt.Errorf("assignment validation failed: %w", err)
	}

	return assignment, nil
}

// selectReplicas selects replicas for a partition using round-robin
func (rr *RoundRobinStrategy) selectReplicas(
	partition PartitionInfo,
	brokers []BrokerInfo,
	brokerIndex *int,
	constraints *AssignmentConstraints,
) ([]int32, error) {
	replicaCount := partition.Replicas
	if replicaCount > len(brokers) {
		replicaCount = len(brokers)
	}

	if replicaCount == 0 {
		return nil, fmt.Errorf("replica count must be > 0")
	}

	replicas := make([]int32, 0, replicaCount)
	selectedBrokers := make(map[int32]bool)

	// If rack-aware, group brokers by rack
	if constraints.RackAware {
		return rr.selectRackAwareReplicas(partition, brokers, brokerIndex, replicaCount)
	}

	// Standard round-robin assignment
	attempts := 0
	maxAttempts := len(brokers) * 2

	for len(replicas) < replicaCount && attempts < maxAttempts {
		broker := brokers[*brokerIndex%len(brokers)]
		*brokerIndex++
		attempts++

		// Skip if already selected
		if selectedBrokers[broker.ID] {
			continue
		}

		// Check capacity constraint (not implemented yet)
		if constraints.MaxPartitionsPerBroker > 0 {
			// TODO: Track current load and enforce capacity limits
		}

		replicas = append(replicas, broker.ID)
		selectedBrokers[broker.ID] = true
	}

	if len(replicas) < replicaCount {
		return nil, fmt.Errorf("could not find %d unique brokers (found %d)",
			replicaCount, len(replicas))
	}

	return replicas, nil
}

// selectRackAwareReplicas selects replicas with rack awareness
func (rr *RoundRobinStrategy) selectRackAwareReplicas(
	partition PartitionInfo,
	brokers []BrokerInfo,
	brokerIndex *int,
	replicaCount int,
) ([]int32, error) {
	// Group brokers by rack
	rackBrokers := make(map[string][]BrokerInfo)
	for _, broker := range brokers {
		rack := broker.Rack
		if rack == "" {
			rack = "default"
		}
		rackBrokers[rack] = append(rackBrokers[rack], broker)
	}

	// Get sorted rack names for determinism
	racks := make([]string, 0, len(rackBrokers))
	for rack := range rackBrokers {
		racks = append(racks, rack)
	}
	sort.Strings(racks)

	replicas := make([]int32, 0, replicaCount)
	selectedBrokers := make(map[int32]bool)
	selectedRacks := make(map[string]bool)

	// First pass: one replica per rack
	rackIndex := *brokerIndex % len(racks)
	for len(replicas) < replicaCount && len(selectedRacks) < len(racks) {
		rack := racks[rackIndex%len(racks)]
		rackIndex++

		if selectedRacks[rack] {
			continue
		}

		// Select first available broker from this rack
		brokersInRack := rackBrokers[rack]
		for _, broker := range brokersInRack {
			if !selectedBrokers[broker.ID] {
				replicas = append(replicas, broker.ID)
				selectedBrokers[broker.ID] = true
				selectedRacks[rack] = true
				break
			}
		}
	}

	// Second pass: fill remaining replicas if needed
	if len(replicas) < replicaCount {
		for _, rack := range racks {
			brokersInRack := rackBrokers[rack]
			for _, broker := range brokersInRack {
				if len(replicas) >= replicaCount {
					break
				}
				if !selectedBrokers[broker.ID] {
					replicas = append(replicas, broker.ID)
					selectedBrokers[broker.ID] = true
				}
			}
		}
	}

	*brokerIndex++

	if len(replicas) < replicaCount {
		return nil, fmt.Errorf("could not find %d unique brokers across racks (found %d)",
			replicaCount, len(replicas))
	}

	return replicas, nil
}

// Rebalance rebalances partitions across brokers
func (rr *RoundRobinStrategy) Rebalance(
	current *Assignment,
	brokers []BrokerInfo,
	constraints *AssignmentConstraints,
) (*Assignment, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("no brokers available")
	}

	// Check if rebalancing is needed
	if current.IsBalanced(1) {
		return current, nil
	}

	// Extract partition info from current assignment
	partitions := make([]PartitionInfo, 0, len(current.Partitions))
	for key, replicas := range current.Partitions {
		topic, partitionID := parsePartitionKey(key)
		partitions = append(partitions, PartitionInfo{
			Topic:       topic,
			PartitionID: partitionID,
			Replicas:    len(replicas),
		})
	}

	// Sort partitions for deterministic rebalancing
	sort.Slice(partitions, func(i, j int) bool {
		if partitions[i].Topic != partitions[j].Topic {
			return partitions[i].Topic < partitions[j].Topic
		}
		return partitions[i].PartitionID < partitions[j].PartitionID
	})

	// Create new assignment
	newAssignment, err := rr.Assign(partitions, brokers, constraints)
	if err != nil {
		return nil, err
	}

	// Increment version
	newAssignment.Version = current.Version + 1

	return newAssignment, nil
}

// parsePartitionKey parses a partition key into topic and partition ID
func parsePartitionKey(key string) (string, int) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return "", 0
	}
	topic := parts[0]
	partitionID, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0
	}
	return topic, partitionID
}
