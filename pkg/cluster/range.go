package cluster

import (
	"fmt"
	"sort"
)

// RangeStrategy assigns contiguous ranges of partitions to brokers
// This strategy groups partitions by topic and assigns ranges to brokers
type RangeStrategy struct{}

// NewRangeStrategy creates a new range assignment strategy
func NewRangeStrategy() *RangeStrategy {
	return &RangeStrategy{}
}

// Name returns the strategy name
func (rs *RangeStrategy) Name() string {
	return "Range"
}

// Assign assigns partitions to brokers using range-based assignment
func (rs *RangeStrategy) Assign(
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

	// Group partitions by topic
	topicPartitions := rs.groupByTopic(partitions)

	assignment := NewAssignment()

	// Assign each topic's partitions
	for topic, topicParts := range topicPartitions {
		if err := rs.assignTopicPartitions(
			topic,
			topicParts,
			availableBrokers,
			assignment,
			constraints,
		); err != nil {
			return nil, fmt.Errorf("failed to assign topic %s: %w", topic, err)
		}
	}

	// Validate assignment
	if err := assignment.Validate(); err != nil {
		return nil, fmt.Errorf("assignment validation failed: %w", err)
	}

	return assignment, nil
}

// groupByTopic groups partitions by topic
func (rs *RangeStrategy) groupByTopic(partitions []PartitionInfo) map[string][]PartitionInfo {
	grouped := make(map[string][]PartitionInfo)

	for _, partition := range partitions {
		grouped[partition.Topic] = append(grouped[partition.Topic], partition)
	}

	// Sort partitions within each topic by partition ID
	for topic := range grouped {
		sort.Slice(grouped[topic], func(i, j int) bool {
			return grouped[topic][i].PartitionID < grouped[topic][j].PartitionID
		})
	}

	return grouped
}

// assignTopicPartitions assigns partitions for a single topic
func (rs *RangeStrategy) assignTopicPartitions(
	topic string,
	partitions []PartitionInfo,
	brokers []BrokerInfo,
	assignment *Assignment,
	constraints *AssignmentConstraints,
) error {
	if len(partitions) == 0 {
		return nil
	}

	// Note: replicationFactor is determined per partition in selectReplicas
	// based on partition.Replicas and available brokers

	// Calculate partitions per broker
	numPartitions := len(partitions)
	numBrokers := len(brokers)
	partitionsPerBroker := numPartitions / numBrokers
	extraPartitions := numPartitions % numBrokers

	// Assign ranges to brokers
	partitionIndex := 0
	for brokerIdx := 0; brokerIdx < numBrokers && partitionIndex < numPartitions; brokerIdx++ {
		// Calculate range for this broker
		rangeSize := partitionsPerBroker
		if brokerIdx < extraPartitions {
			rangeSize++
		}

		// Assign partitions in this range
		for i := 0; i < rangeSize && partitionIndex < numPartitions; i++ {
			partition := partitions[partitionIndex]

			replicas, err := rs.selectReplicas(
				partition,
				brokers,
				brokerIdx,
				constraints,
			)
			if err != nil {
				return fmt.Errorf("failed to select replicas for partition %d: %w",
					partition.PartitionID, err)
			}

			assignment.AddReplica(topic, partition.PartitionID, replicas)
			partitionIndex++
		}
	}

	return nil
}

// selectReplicas selects replicas for a partition with the primary broker
func (rs *RangeStrategy) selectReplicas(
	partition PartitionInfo,
	brokers []BrokerInfo,
	primaryBrokerIdx int,
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

	// If rack-aware, use rack-aware selection
	if constraints.RackAware {
		return rs.selectRackAwareReplicas(partition, brokers, primaryBrokerIdx, replicaCount)
	}

	// First replica is the primary broker
	primaryBroker := brokers[primaryBrokerIdx]
	replicas = append(replicas, primaryBroker.ID)
	selectedBrokers[primaryBroker.ID] = true

	// Select remaining replicas round-robin from next brokers
	brokerIdx := primaryBrokerIdx + 1
	attempts := 0
	maxAttempts := len(brokers) * 2

	for len(replicas) < replicaCount && attempts < maxAttempts {
		broker := brokers[brokerIdx%len(brokers)]
		brokerIdx++
		attempts++

		if selectedBrokers[broker.ID] {
			continue
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
func (rs *RangeStrategy) selectRackAwareReplicas(
	partition PartitionInfo,
	brokers []BrokerInfo,
	primaryBrokerIdx int,
	replicaCount int,
) ([]int32, error) {
	// Group brokers by rack
	rackBrokers := make(map[string][]BrokerInfo)
	brokerRack := make(map[int32]string)

	for _, broker := range brokers {
		rack := broker.Rack
		if rack == "" {
			rack = "default"
		}
		rackBrokers[rack] = append(rackBrokers[rack], broker)
		brokerRack[broker.ID] = rack
	}

	replicas := make([]int32, 0, replicaCount)
	selectedBrokers := make(map[int32]bool)
	selectedRacks := make(map[string]bool)

	// First replica is the primary broker
	primaryBroker := brokers[primaryBrokerIdx]
	replicas = append(replicas, primaryBroker.ID)
	selectedBrokers[primaryBroker.ID] = true
	selectedRacks[brokerRack[primaryBroker.ID]] = true

	// Select remaining replicas from different racks
	for _, broker := range brokers {
		if len(replicas) >= replicaCount {
			break
		}

		// Skip if already selected
		if selectedBrokers[broker.ID] {
			continue
		}

		rack := brokerRack[broker.ID]

		// Prefer brokers from unselected racks
		if !selectedRacks[rack] || len(selectedRacks) == len(rackBrokers) {
			replicas = append(replicas, broker.ID)
			selectedBrokers[broker.ID] = true
			selectedRacks[rack] = true
		}
	}

	// If still need more replicas, select from any rack
	if len(replicas) < replicaCount {
		for _, broker := range brokers {
			if len(replicas) >= replicaCount {
				break
			}
			if !selectedBrokers[broker.ID] {
				replicas = append(replicas, broker.ID)
				selectedBrokers[broker.ID] = true
			}
		}
	}

	if len(replicas) < replicaCount {
		return nil, fmt.Errorf("could not find %d unique brokers (found %d)",
			replicaCount, len(replicas))
	}

	return replicas, nil
}

// Rebalance rebalances partitions across brokers
func (rs *RangeStrategy) Rebalance(
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

	// Create new assignment
	newAssignment, err := rs.Assign(partitions, brokers, constraints)
	if err != nil {
		return nil, err
	}

	// Increment version
	newAssignment.Version = current.Version + 1

	return newAssignment, nil
}
