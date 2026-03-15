package group

import (
	"sort"
)

// TopicPartition represents a topic and partition pair
type TopicPartition struct {
	Topic     string
	Partition int32
}

// MemberSubscription represents a member and the topics it subscribes to
type MemberSubscription struct {
	MemberID string
	Topics   []string
}

// PartitionAssignor assigns partitions to consumer group members
type PartitionAssignor interface {
	Name() string
	Assign(members []MemberSubscription, partitions []TopicPartition) map[string][]TopicPartition
}

// --- Range Assignor ---

// RangeAssignor assigns partitions using a range-based strategy.
// For each topic, partitions are divided evenly among subscribed members.
// If not evenly divisible, the first N members get one extra partition.
type RangeAssignor struct{}

// Name returns the assignor strategy name.
func (r *RangeAssignor) Name() string {
	return "range"
}

// Assign distributes partitions to members using range assignment.
func (r *RangeAssignor) Assign(members []MemberSubscription, partitions []TopicPartition) map[string][]TopicPartition {
	result := make(map[string][]TopicPartition)
	if len(members) == 0 || len(partitions) == 0 {
		return result
	}

	// Initialize result for each member
	for _, m := range members {
		result[m.MemberID] = []TopicPartition{}
	}

	// Group partitions by topic
	topicPartitions := groupPartitionsByTopic(partitions)

	// For each topic, assign partitions to subscribed members
	for topic, tpList := range topicPartitions {
		subscribers := subscribersForTopic(members, topic)
		if len(subscribers) == 0 {
			continue
		}

		sort.Strings(subscribers)
		sortPartitions(tpList)

		assignRangeForTopic(result, subscribers, tpList)
	}

	return result
}

// assignRangeForTopic assigns a single topic's partitions to subscribers using range strategy.
func assignRangeForTopic(result map[string][]TopicPartition, subscribers []string, partitions []TopicPartition) {
	numPartitions := len(partitions)
	numSubscribers := len(subscribers)
	partitionsPerMember := numPartitions / numSubscribers
	extra := numPartitions % numSubscribers

	idx := 0
	for i, memberID := range subscribers {
		count := partitionsPerMember
		if i < extra {
			count++
		}
		for j := 0; j < count && idx < numPartitions; j++ {
			result[memberID] = append(result[memberID], partitions[idx])
			idx++
		}
	}
}

// --- Round-Robin Assignor ---

// RoundRobinAssignor assigns partitions in round-robin order across members.
// All partitions across all subscribed topics are sorted, then distributed
// one at a time to each consumer in order.
type RoundRobinAssignor struct{}

// Name returns the assignor strategy name.
func (rr *RoundRobinAssignor) Name() string {
	return "roundrobin"
}

// Assign distributes partitions to members using round-robin assignment.
func (rr *RoundRobinAssignor) Assign(members []MemberSubscription, partitions []TopicPartition) map[string][]TopicPartition {
	result := make(map[string][]TopicPartition)
	if len(members) == 0 || len(partitions) == 0 {
		return result
	}

	// Initialize result for each member
	for _, m := range members {
		result[m.MemberID] = []TopicPartition{}
	}

	// Build subscription lookup: memberID -> set of topics
	subscriptions := buildSubscriptionLookup(members)

	// Sort all partitions by topic then partition
	sorted := make([]TopicPartition, len(partitions))
	copy(sorted, partitions)
	sortTopicPartitions(sorted)

	// Sort member IDs for deterministic order
	memberIDs := sortedMemberIDs(members)

	// Round-robin assign, skipping members not subscribed to the partition's topic
	idx := 0
	for _, tp := range sorted {
		assigned := false
		for attempts := 0; attempts < len(memberIDs); attempts++ {
			candidate := memberIDs[idx%len(memberIDs)]
			idx++
			if subscriptions[candidate][tp.Topic] {
				result[candidate] = append(result[candidate], tp)
				assigned = true
				break
			}
		}
		// If no member subscribes to this topic, partition is unassigned
		_ = assigned
	}

	return result
}

// --- Sticky Assignor ---

// StickyAssignor tries to preserve existing assignments when possible.
// On initial assignment it uses round-robin. On subsequent rebalances,
// it keeps existing assignments and only reassigns partitions that must move.
type StickyAssignor struct{}

// Name returns the assignor strategy name.
func (s *StickyAssignor) Name() string {
	return "sticky"
}

// Assign distributes partitions to members using sticky assignment.
// The previousAssignments parameter is embedded in the method via StickyAssignWithPrevious.
// When called through the PartitionAssignor interface, it falls back to round-robin.
func (s *StickyAssignor) Assign(members []MemberSubscription, partitions []TopicPartition) map[string][]TopicPartition {
	return s.AssignWithPrevious(members, partitions, nil)
}

// AssignWithPrevious distributes partitions preserving previous assignments where possible.
func (s *StickyAssignor) AssignWithPrevious(
	members []MemberSubscription,
	partitions []TopicPartition,
	previousAssignments map[string][]TopicPartition,
) map[string][]TopicPartition {
	result := make(map[string][]TopicPartition)
	if len(members) == 0 || len(partitions) == 0 {
		return result
	}

	// Initialize result for each member
	for _, m := range members {
		result[m.MemberID] = []TopicPartition{}
	}

	// If no previous assignments, fall back to round-robin
	if len(previousAssignments) == 0 {
		rr := &RoundRobinAssignor{}
		return rr.Assign(members, partitions)
	}

	subscriptions := buildSubscriptionLookup(members)
	memberSet := buildMemberSet(members)

	// Retain valid previous assignments
	assigned := retainPreviousAssignments(result, previousAssignments, memberSet, subscriptions)

	// Collect unassigned partitions
	unassigned := collectUnassigned(partitions, assigned)

	// Sort unassigned for deterministic assignment
	sortTopicPartitions(unassigned)

	// Assign unassigned partitions to least-loaded members
	assignToLeastLoaded(result, unassigned, members, subscriptions)

	// Rebalance: move partitions from overloaded members to underloaded ones
	rebalanceSticky(result, members, subscriptions)

	return result
}

// retainPreviousAssignments keeps partitions that are still valid from prior assignments.
// Returns a set of already-assigned partitions.
func retainPreviousAssignments(
	result map[string][]TopicPartition,
	previous map[string][]TopicPartition,
	memberSet map[string]bool,
	subscriptions map[string]map[string]bool,
) map[TopicPartition]bool {
	assigned := make(map[TopicPartition]bool)
	for memberID, tps := range previous {
		if !memberSet[memberID] {
			continue // Member no longer in group
		}
		for _, tp := range tps {
			if subscriptions[memberID][tp.Topic] {
				result[memberID] = append(result[memberID], tp)
				assigned[tp] = true
			}
		}
	}
	return assigned
}

// assignToLeastLoaded assigns each unassigned partition to the subscribed member
// with the fewest current assignments.
func assignToLeastLoaded(
	result map[string][]TopicPartition,
	unassigned []TopicPartition,
	members []MemberSubscription,
	subscriptions map[string]map[string]bool,
) {
	memberIDs := sortedMemberIDs(members)

	for _, tp := range unassigned {
		bestMember := findLeastLoadedSubscriber(result, memberIDs, subscriptions, tp.Topic)
		if bestMember == "" {
			continue
		}
		result[bestMember] = append(result[bestMember], tp)
	}
}

// findLeastLoadedSubscriber finds the member subscribed to the topic with fewest assignments.
func findLeastLoadedSubscriber(
	result map[string][]TopicPartition,
	memberIDs []string,
	subscriptions map[string]map[string]bool,
	topic string,
) string {
	bestMember := ""
	bestCount := -1
	for _, mid := range memberIDs {
		if !subscriptions[mid][topic] {
			continue
		}
		count := len(result[mid])
		if bestMember == "" || count < bestCount {
			bestMember = mid
			bestCount = count
		}
	}
	return bestMember
}

// rebalanceSticky moves partitions from overloaded members to underloaded members
// to achieve a balanced distribution while minimizing total moves.
func rebalanceSticky(
	result map[string][]TopicPartition,
	members []MemberSubscription,
	subscriptions map[string]map[string]bool,
) {
	memberIDs := sortedMemberIDs(members)
	totalPartitions := countTotalPartitions(result)
	numMembers := len(memberIDs)
	if numMembers == 0 {
		return
	}

	maxPerMember := (totalPartitions + numMembers - 1) / numMembers // ceiling division
	minPerMember := totalPartitions / numMembers

	// Move partitions from overloaded to underloaded members.
	// Cap iterations at total partitions to guarantee termination.
	for i := 0; i < totalPartitions; i++ {
		moved := moveOnePartition(result, memberIDs, subscriptions, maxPerMember, minPerMember)
		if !moved {
			break
		}
	}
}

// moveOnePartition finds an overloaded member and moves one partition to an underloaded member.
func moveOnePartition(
	result map[string][]TopicPartition,
	memberIDs []string,
	subscriptions map[string]map[string]bool,
	maxPerMember int,
	minPerMember int,
) bool {
	for _, fromID := range memberIDs {
		if len(result[fromID]) <= maxPerMember {
			continue
		}
		for i := len(result[fromID]) - 1; i >= 0; i-- {
			tp := result[fromID][i]
			toID := findUnderloadedSubscriber(result, memberIDs, subscriptions, tp.Topic, fromID, minPerMember)
			if toID == "" {
				continue
			}
			result[fromID] = append(result[fromID][:i], result[fromID][i+1:]...)
			result[toID] = append(result[toID], tp)
			return true
		}
	}
	return false
}

// findUnderloadedSubscriber finds a member subscribed to topic with fewer than threshold partitions.
func findUnderloadedSubscriber(
	result map[string][]TopicPartition,
	memberIDs []string,
	subscriptions map[string]map[string]bool,
	topic string,
	excludeID string,
	minPerMember int,
) string {
	for _, mid := range memberIDs {
		if mid == excludeID {
			continue
		}
		if len(result[mid]) >= minPerMember+1 {
			continue
		}
		if subscriptions[mid][topic] {
			return mid
		}
	}
	return ""
}

// countTotalPartitions counts total assigned partitions across all members.
func countTotalPartitions(result map[string][]TopicPartition) int {
	total := 0
	for _, tps := range result {
		total += len(tps)
	}
	return total
}

// --- Shared helpers ---

// groupPartitionsByTopic groups partitions by their topic name.
func groupPartitionsByTopic(partitions []TopicPartition) map[string][]TopicPartition {
	result := make(map[string][]TopicPartition)
	for _, tp := range partitions {
		result[tp.Topic] = append(result[tp.Topic], tp)
	}
	return result
}

// subscribersForTopic returns sorted member IDs subscribed to the given topic.
func subscribersForTopic(members []MemberSubscription, topic string) []string {
	var subscribers []string
	for _, m := range members {
		for _, t := range m.Topics {
			if t == topic {
				subscribers = append(subscribers, m.MemberID)
				break
			}
		}
	}
	return subscribers
}

// sortPartitions sorts topic partitions by partition number.
func sortPartitions(partitions []TopicPartition) {
	sort.Slice(partitions, func(i, j int) bool {
		return partitions[i].Partition < partitions[j].Partition
	})
}

// sortTopicPartitions sorts partitions by topic name, then by partition number.
func sortTopicPartitions(partitions []TopicPartition) {
	sort.Slice(partitions, func(i, j int) bool {
		if partitions[i].Topic != partitions[j].Topic {
			return partitions[i].Topic < partitions[j].Topic
		}
		return partitions[i].Partition < partitions[j].Partition
	})
}

// buildSubscriptionLookup builds a memberID -> topic -> bool lookup from member subscriptions.
func buildSubscriptionLookup(members []MemberSubscription) map[string]map[string]bool {
	subs := make(map[string]map[string]bool)
	for _, m := range members {
		subs[m.MemberID] = make(map[string]bool)
		for _, t := range m.Topics {
			subs[m.MemberID][t] = true
		}
	}
	return subs
}

// buildMemberSet returns a set of current member IDs.
func buildMemberSet(members []MemberSubscription) map[string]bool {
	set := make(map[string]bool)
	for _, m := range members {
		set[m.MemberID] = true
	}
	return set
}

// sortedMemberIDs returns sorted member IDs for deterministic ordering.
func sortedMemberIDs(members []MemberSubscription) []string {
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.MemberID
	}
	sort.Strings(ids)
	return ids
}

// collectUnassigned returns partitions not present in the assigned set.
func collectUnassigned(partitions []TopicPartition, assigned map[TopicPartition]bool) []TopicPartition {
	var unassigned []TopicPartition
	for _, tp := range partitions {
		if !assigned[tp] {
			unassigned = append(unassigned, tp)
		}
	}
	return unassigned
}

// GetAssignor returns a PartitionAssignor by strategy name.
// Supported strategies: "range", "roundrobin", "sticky".
func GetAssignor(name string) PartitionAssignor {
	switch name {
	case "range":
		return &RangeAssignor{}
	case "roundrobin":
		return &RoundRobinAssignor{}
	case "sticky":
		return &StickyAssignor{}
	default:
		return &RangeAssignor{} // Default to range
	}
}
