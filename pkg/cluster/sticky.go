package cluster

import (
	"fmt"
)

// StickyStrategy minimizes partition movement during rebalancing
// It tries to keep existing assignments when possible
type StickyStrategy struct {
	// fallbackStrategy is used for initial assignments
	fallbackStrategy AssignmentStrategy
}

// NewStickyStrategy creates a new sticky assignment strategy
func NewStickyStrategy() *StickyStrategy {
	return &StickyStrategy{
		fallbackStrategy: NewRoundRobinStrategy(),
	}
}

// Name returns the strategy name
func (ss *StickyStrategy) Name() string {
	return "Sticky"
}

// Assign performs initial assignment using fallback strategy
func (ss *StickyStrategy) Assign(
	partitions []PartitionInfo,
	brokers []BrokerInfo,
	constraints *AssignmentConstraints,
) (*Assignment, error) {
	// For initial assignment, use fallback strategy
	return ss.fallbackStrategy.Assign(partitions, brokers, constraints)
}

// Rebalance rebalances while minimizing partition movement
func (ss *StickyStrategy) Rebalance(
	current *Assignment,
	brokers []BrokerInfo,
	constraints *AssignmentConstraints,
) (*Assignment, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("no brokers available")
	}

	// Filter excluded brokers
	availableBrokers := filterExcludedBrokers(brokers, constraints.ExcludedBrokers)
	if len(availableBrokers) == 0 {
		return nil, fmt.Errorf("no available brokers after filtering")
	}

	// Check if rebalancing is needed
	// Don't skip rebalancing if brokers have been removed
	if current.IsBalanced(1) && ss.allBrokersMatch(current, availableBrokers) {
		// Still increment version even if no changes needed
		result := current.Clone()
		result.Version++
		return result, nil
	}

	// Clone current assignment to start with
	newAssignment := current.Clone()
	newAssignment.Version++

	// Build current broker load map
	load := make(map[int32]int)
	for broker := range newAssignment.BrokerLoad {
		load[broker] = newAssignment.BrokerLoad[broker]
	}

	// Add new brokers with zero load
	for _, broker := range availableBrokers {
		if _, exists := load[broker.ID]; !exists {
			load[broker.ID] = 0
		}
	}

	// Remove partitions from removed brokers
	removedBrokers := ss.findRemovedBrokers(current, availableBrokers)
	partitionsToReassign := make(map[string]PartitionInfo)

	// Filter removed brokers from ALL partitions
	for key, replicas := range newAssignment.Partitions {
		topic, partitionID := parsePartitionKey(key)
		originalLen := len(replicas)

		newReplicas := ss.filterReplicas(replicas, removedBrokers)

		if len(newReplicas) != originalLen {
			// This partition had replicas on removed brokers
			partitionsToReassign[key] = PartitionInfo{
				Topic:       topic,
				PartitionID: partitionID,
				Replicas:    originalLen, // Original replica count
			}

			if len(newReplicas) > 0 {
				// Update replica list (removed some brokers)
				newAssignment.Partitions[key] = newReplicas
				newAssignment.Leaders[key] = newReplicas[0]
			} else {
				// All replicas were on removed brokers - need full reassignment
				delete(newAssignment.Partitions, key)
				delete(newAssignment.Leaders, key)
			}
		}
	}

	// Recompute broker load
	load = make(map[int32]int)
	for _, broker := range availableBrokers {
		load[broker.ID] = 0
	}
	for _, replicas := range newAssignment.Partitions {
		for _, broker := range replicas {
			load[broker]++
		}
	}
	newAssignment.BrokerLoad = load

	// Reassign partitions that need new replicas
	if err := ss.reassignPartitions(newAssignment, partitionsToReassign, availableBrokers, constraints); err != nil {
		return nil, err
	}

	// Balance load across brokers
	if err := ss.balanceLoad(newAssignment, availableBrokers, constraints); err != nil {
		return nil, err
	}

	// Final pass: ensure no removed brokers remain in any partition
	finalPartitionsToReassign := make(map[string]PartitionInfo)
	for key, replicas := range newAssignment.Partitions {
		filtered := ss.filterReplicas(replicas, removedBrokers)
		if len(filtered) != len(replicas) {
			topic, partitionID := parsePartitionKey(key)
			originalLen := len(replicas)

			newAssignment.Partitions[key] = filtered
			if len(filtered) > 0 {
				newAssignment.Leaders[key] = filtered[0]
			}

			// Track partitions that lost replicas and need reassignment
			finalPartitionsToReassign[key] = PartitionInfo{
				Topic:       topic,
				PartitionID: partitionID,
				Replicas:    originalLen,
			}
		}
	}

	// Recompute broker load to ensure consistency
	newAssignment.RecomputeBrokerLoad()

	// Reassign any partitions that lost replicas in the final pass
	if len(finalPartitionsToReassign) > 0 {
		if err := ss.reassignPartitions(newAssignment, finalPartitionsToReassign, availableBrokers, constraints); err != nil {
			return nil, fmt.Errorf("failed to reassign partitions after final cleanup: %w", err)
		}
	}

	// Final recompute after reassignment
	newAssignment.RecomputeBrokerLoad()

	// Validate final assignment
	if err := newAssignment.Validate(); err != nil {
		return nil, fmt.Errorf("assignment validation failed: %w", err)
	}

	return newAssignment, nil
}

// allBrokersInAssignment checks if all available brokers are in the assignment
func (ss *StickyStrategy) allBrokersInAssignment(assignment *Assignment, brokers []BrokerInfo) bool {
	for _, broker := range brokers {
		if _, exists := assignment.BrokerLoad[broker.ID]; !exists {
			return false
		}
	}
	return true
}

// allBrokersMatch checks if the brokers in the assignment exactly match the available brokers
func (ss *StickyStrategy) allBrokersMatch(assignment *Assignment, brokers []BrokerInfo) bool {
	// Check if all available brokers are in the assignment
	availableSet := make(map[int32]bool)
	for _, broker := range brokers {
		availableSet[broker.ID] = true
	}

	// Check if assignment has any brokers not in available set
	for brokerID := range assignment.BrokerLoad {
		if !availableSet[brokerID] {
			return false // Assignment has broker not in available list
		}
	}

	// Check if all available brokers are in the assignment
	for _, broker := range brokers {
		if _, exists := assignment.BrokerLoad[broker.ID]; !exists {
			return false // Available broker not in assignment
		}
	}

	return true
}

// findRemovedBrokers identifies brokers that are no longer available
func (ss *StickyStrategy) findRemovedBrokers(assignment *Assignment, availableBrokers []BrokerInfo) map[int32]bool {
	available := make(map[int32]bool)
	for _, broker := range availableBrokers {
		available[broker.ID] = true
	}

	removed := make(map[int32]bool)
	for broker := range assignment.BrokerLoad {
		if !available[broker] {
			removed[broker] = true
		}
	}

	return removed
}

// collectPartitionsFromRemovedBrokers collects partitions that need reassignment
func (ss *StickyStrategy) collectPartitionsFromRemovedBrokers(
	assignment *Assignment,
	removedBrokers map[int32]bool,
) map[string]PartitionInfo {
	partitions := make(map[string]PartitionInfo)

	for key, replicas := range assignment.Partitions {
		topic, partitionID := parsePartitionKey(key)

		// Check if any replica is on a removed broker
		needsReassignment := false
		for _, replica := range replicas {
			if removedBrokers[replica] {
				needsReassignment = true
				break
			}
		}

		if needsReassignment {
			partitions[key] = PartitionInfo{
				Topic:       topic,
				PartitionID: partitionID,
				Replicas:    len(replicas),
			}
		}
	}

	return partitions
}

// filterReplicas removes brokers from replica list
func (ss *StickyStrategy) filterReplicas(replicas []int32, removed map[int32]bool) []int32 {
	filtered := make([]int32, 0, len(replicas))
	for _, replica := range replicas {
		if !removed[replica] {
			filtered = append(filtered, replica)
		}
	}
	return filtered
}

// reassignPartitions assigns partitions that need new replicas
func (ss *StickyStrategy) reassignPartitions(
	assignment *Assignment,
	partitions map[string]PartitionInfo,
	brokers []BrokerInfo,
	constraints *AssignmentConstraints,
) error {
	for key, partition := range partitions {
		currentReplicas := assignment.Partitions[key]
		neededReplicas := partition.Replicas - len(currentReplicas)

		if neededReplicas <= 0 {
			continue
		}

		// Find brokers with lowest load
		sortedBrokers := sortBrokersByLoad(brokers, assignment.BrokerLoad)

		// Select new replicas
		selectedCount := 0
		for _, broker := range sortedBrokers {
			if selectedCount >= neededReplicas {
				break
			}

			// Skip if already a replica
			alreadyReplica := false
			for _, existing := range currentReplicas {
				if existing == broker.ID {
					alreadyReplica = true
					break
				}
			}
			if alreadyReplica {
				continue
			}

			// Add replica
			currentReplicas = append(currentReplicas, broker.ID)
			assignment.BrokerLoad[broker.ID]++
			selectedCount++
		}

		if selectedCount < neededReplicas {
			return fmt.Errorf("could not find enough brokers for partition %s", key)
		}

		// Update assignment
		assignment.Partitions[key] = currentReplicas
		if len(currentReplicas) > 0 {
			assignment.Leaders[key] = currentReplicas[0]
		}
	}

	return nil
}

// balanceLoad balances partition load across brokers
func (ss *StickyStrategy) balanceLoad(
	assignment *Assignment,
	brokers []BrokerInfo,
	constraints *AssignmentConstraints,
) error {
	// Check current balance
	if assignment.IsBalanced(1) {
		return nil
	}

	// Calculate target load per broker
	totalPartitions := len(assignment.Partitions)
	numBrokers := len(brokers)
	if numBrokers == 0 {
		return fmt.Errorf("no brokers available")
	}

	targetLoad := totalPartitions / numBrokers
	maxImbalance := 1

	// Iteratively move partitions from overloaded to underloaded brokers
	maxIterations := totalPartitions * 2
	iteration := 0

	for !assignment.IsBalanced(maxImbalance) && iteration < maxIterations {
		iteration++

		// Find most overloaded and underloaded brokers
		overloaded, underloaded := ss.findImbalancedBrokers(assignment, targetLoad, brokers)
		if overloaded == nil || underloaded == nil {
			break
		}

		// Find a partition to move
		moved := ss.movePartition(assignment, overloaded.ID, underloaded.ID, constraints)
		if !moved {
			break
		}
	}

	return nil
}

// findImbalancedBrokers finds the most overloaded and underloaded brokers
func (ss *StickyStrategy) findImbalancedBrokers(
	assignment *Assignment,
	targetLoad int,
	brokers []BrokerInfo,
) (*BrokerInfo, *BrokerInfo) {
	var overloaded *BrokerInfo
	var underloaded *BrokerInfo
	maxLoad := targetLoad
	minLoad := targetLoad

	brokerMap := make(map[int32]BrokerInfo)
	for _, broker := range brokers {
		brokerMap[broker.ID] = broker
	}

	for brokerID, load := range assignment.BrokerLoad {
		broker := brokerMap[brokerID]

		if load > maxLoad {
			maxLoad = load
			b := broker
			overloaded = &b
		}

		if load < minLoad {
			minLoad = load
			b := broker
			underloaded = &b
		}
	}

	return overloaded, underloaded
}

// movePartition attempts to move a partition from one broker to another
func (ss *StickyStrategy) movePartition(
	assignment *Assignment,
	fromBroker, toBroker int32,
	constraints *AssignmentConstraints,
) bool {
	// Find a partition on fromBroker that we can move
	var partitionToMove string
	var replicaIndex int

	for key, replicas := range assignment.Partitions {
		for i, replica := range replicas {
			if replica == fromBroker {
				// Check if toBroker is not already a replica
				alreadyReplica := false
				for _, r := range replicas {
					if r == toBroker {
						alreadyReplica = true
						break
					}
				}

				if !alreadyReplica {
					partitionToMove = key
					replicaIndex = i
					break
				}
			}
		}
		if partitionToMove != "" {
			break
		}
	}

	if partitionToMove == "" {
		return false
	}

	// Move the partition
	replicas := assignment.Partitions[partitionToMove]
	replicas[replicaIndex] = toBroker

	// Update broker load
	assignment.BrokerLoad[fromBroker]--
	assignment.BrokerLoad[toBroker]++

	// Update leader if necessary
	if assignment.Leaders[partitionToMove] == fromBroker {
		assignment.Leaders[partitionToMove] = replicas[0]
	}

	return true
}
