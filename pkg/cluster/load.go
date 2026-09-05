package cluster

import "fmt"

// brokerLoad tracks how many partition replicas each broker holds during a
// single assignment pass and enforces the per-broker capacity limits.
//
// Two independent limits are honoured, and the tighter of the two wins:
//   - AssignmentConstraints.MaxPartitionsPerBroker, a cluster-wide cap
//   - BrokerInfo.Capacity, a per-broker cap
//
// A zero value for either means "no limit from that source". A broker with no
// limit from either source is always considered to have capacity.
type brokerLoad struct {
	counts map[int32]int
	limits map[int32]int // 0 == unlimited
}

// newBrokerLoad builds a load tracker for the given brokers, seeded with any
// pre-existing per-broker counts supplied by the caller via
// AssignmentConstraints.ExistingLoad.
func newBrokerLoad(brokers []BrokerInfo, constraints *AssignmentConstraints) *brokerLoad {
	l := &brokerLoad{
		counts: make(map[int32]int, len(brokers)),
		limits: make(map[int32]int, len(brokers)),
	}

	maxPerBroker := 0
	var existing map[int32]int
	if constraints != nil {
		maxPerBroker = constraints.MaxPartitionsPerBroker
		existing = constraints.ExistingLoad
	}

	for _, broker := range brokers {
		l.limits[broker.ID] = tighterLimit(maxPerBroker, broker.Capacity)
		l.counts[broker.ID] = existing[broker.ID]
	}

	return l
}

// brokerCapacityLimits returns the effective per-broker partition limit for
// each broker, where 0 means unlimited. Use this when the caller already
// tracks per-broker counts itself (as StickyStrategy does via
// Assignment.BrokerLoad) and only needs the limits.
func brokerCapacityLimits(brokers []BrokerInfo, constraints *AssignmentConstraints) map[int32]int {
	maxPerBroker := 0
	if constraints != nil {
		maxPerBroker = constraints.MaxPartitionsPerBroker
	}

	limits := make(map[int32]int, len(brokers))
	for _, broker := range brokers {
		limits[broker.ID] = tighterLimit(maxPerBroker, broker.Capacity)
	}
	return limits
}

// withinLimit reports whether a broker currently holding current replicas can
// take one more.
func withinLimit(limits map[int32]int, brokerID int32, current int) bool {
	limit, ok := limits[brokerID]
	if !ok || limit <= 0 {
		return true
	}
	return current < limit
}

// anyLimited reports whether any broker in limits has a finite limit.
func anyLimited(limits map[int32]int) bool {
	for _, limit := range limits {
		if limit > 0 {
			return true
		}
	}
	return false
}

// tighterLimit returns the smaller of two limits, treating 0 as "unlimited"
// and ignoring negative values.
func tighterLimit(a, b int) int {
	if a < 0 {
		a = 0
	}
	if b < 0 {
		b = 0
	}
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	case a < b:
		return a
	default:
		return b
	}
}

// limited reports whether any broker has a finite capacity limit. When no
// broker is limited, callers can skip capacity bookkeeping entirely.
func (l *brokerLoad) limited() bool {
	for _, limit := range l.limits {
		if limit > 0 {
			return true
		}
	}
	return false
}

// hasCapacity reports whether the broker can take one more partition replica.
func (l *brokerLoad) hasCapacity(brokerID int32) bool {
	limit, ok := l.limits[brokerID]
	if !ok || limit <= 0 {
		return true
	}
	return l.counts[brokerID] < limit
}

// add records one more partition replica on the broker.
func (l *brokerLoad) add(brokerID int32) {
	l.counts[brokerID]++
}

// addAll records one partition replica on each of the given brokers.
func (l *brokerLoad) addAll(brokerIDs []int32) {
	for _, id := range brokerIDs {
		l.add(id)
	}
}

// remove drops one partition replica from the broker, never going below zero.
func (l *brokerLoad) remove(brokerID int32) {
	if l.counts[brokerID] > 0 {
		l.counts[brokerID]--
	}
}

// count returns the tracked replica count for the broker.
func (l *brokerLoad) count(brokerID int32) int {
	return l.counts[brokerID]
}

// capacityError builds the error returned when a partition cannot be placed
// because every candidate broker is at its limit.
func (l *brokerLoad) capacityError(partition PartitionInfo, want, got int) error {
	return fmt.Errorf(
		"could not place %d replicas for %s:%d (placed %d): all remaining brokers are at their partition limit",
		want, partition.Topic, partition.PartitionID, got)
}
