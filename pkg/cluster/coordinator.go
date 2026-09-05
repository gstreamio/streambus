package cluster

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// ClusterCoordinator manages cluster membership and partition assignment
type ClusterCoordinator struct {
	mu sync.RWMutex

	// Components
	registry           *BrokerRegistry
	assignmentStrategy AssignmentStrategy
	metadataClient     MetadataStore

	// Current cluster state
	currentAssignment *Assignment

	// partitionLimit is the AssignmentConstraints.MaxPartitionsPerBroker most
	// recently supplied to AssignPartitions. Rebalances triggered internally
	// (broker add/remove/failure, the periodic check) have no caller to
	// supply constraints, so the coordinator remembers this to keep the
	// cluster-wide limit enforced consistently across both paths.
	partitionLimit int

	// Configuration
	rebalanceInterval  time.Duration
	rebalanceThreshold int // Imbalance threshold to trigger rebalance

	// Rebalancing state
	rebalancing       bool
	lastRebalanceTime time.Time
	rebalanceCount    int64
	failedRebalances  int64

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewClusterCoordinator creates a new cluster coordinator
func NewClusterCoordinator(
	registry *BrokerRegistry,
	strategy AssignmentStrategy,
	metadataClient MetadataStore,
) *ClusterCoordinator {
	ctx, cancel := context.WithCancel(context.Background())

	return &ClusterCoordinator{
		registry:           registry,
		assignmentStrategy: strategy,
		metadataClient:     metadataClient,
		rebalanceInterval:  5 * time.Minute,
		rebalanceThreshold: 2,
		ctx:                ctx,
		cancel:             cancel,
	}
}

// Start starts the cluster coordinator
func (cc *ClusterCoordinator) Start() error {
	// Register callbacks for broker events
	cc.registry.SetOnBrokerAdded(cc.onBrokerAdded)
	cc.registry.SetOnBrokerRemoved(cc.onBrokerRemoved)
	cc.registry.SetOnBrokerFailed(cc.onBrokerFailed)

	// Start automatic rebalancing
	cc.wg.Add(1)
	go cc.rebalanceLoop()

	log.Printf("[ClusterCoordinator] Started with strategy: %s", cc.assignmentStrategy.Name())
	return nil
}

// Stop stops the cluster coordinator
func (cc *ClusterCoordinator) Stop() error {
	cc.cancel()
	cc.wg.Wait()
	return nil
}

// SetRebalanceInterval sets the automatic rebalance interval
func (cc *ClusterCoordinator) SetRebalanceInterval(interval time.Duration) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.rebalanceInterval = interval
}

// SetRebalanceThreshold sets the imbalance threshold for triggering rebalance
func (cc *ClusterCoordinator) SetRebalanceThreshold(threshold int) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.rebalanceThreshold = threshold
}

// AssignPartitions assigns the given partitions (typically all belonging to
// one new topic) to brokers.
//
// MaxPartitionsPerBroker is enforced across every topic the coordinator has
// ever assigned, not just the partitions passed in this call: the
// coordinator's own currentAssignment is the authoritative record of what is
// already placed (the MetadataStore only persists broker metadata, not
// partition assignments), so its BrokerLoad is folded into this call's
// ExistingLoad before invoking the strategy. Any ExistingLoad the caller
// supplies is added on top rather than replaced, since it represents load the
// coordinator has no other way of knowing about.
//
// The returned Assignment covers only the partitions passed in this call, to
// match AssignmentStrategy.Assign's contract; use GetCurrentAssignment for
// the coordinator's full, merged view of the cluster.
func (cc *ClusterCoordinator) AssignPartitions(
	ctx context.Context,
	partitions []PartitionInfo,
	constraints *AssignmentConstraints,
) (*Assignment, error) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	// Get active brokers
	brokers := cc.getActiveBrokersForAssignment()
	if len(brokers) == 0 {
		return nil, fmt.Errorf("no active brokers available")
	}

	effective := AssignmentConstraints{}
	if constraints != nil {
		effective = *constraints
	}
	effective.ExistingLoad = cc.cumulativeExistingLoad(effective.ExistingLoad)

	// Create assignment
	assignment, err := cc.assignmentStrategy.Assign(partitions, brokers, &effective)
	if err != nil {
		return nil, fmt.Errorf("assignment failed: %w", err)
	}

	// Fold into the running cluster-wide assignment rather than replacing it,
	// so a later call's ExistingLoad derivation (and rebalances) see load
	// from every topic assigned so far.
	cc.currentAssignment = mergeAssignment(cc.currentAssignment, assignment)
	cc.partitionLimit = effective.MaxPartitionsPerBroker

	log.Printf("[ClusterCoordinator] Created assignment: %d partitions across %d brokers",
		assignment.TotalPartitions(), len(brokers))

	return assignment, nil
}

// cumulativeExistingLoad merges the coordinator's own record of already-
// assigned partitions with any ExistingLoad the caller supplied. Must be
// called with cc.mu held.
func (cc *ClusterCoordinator) cumulativeExistingLoad(callerLoad map[int32]int) map[int32]int {
	merged := make(map[int32]int, len(callerLoad))
	if cc.currentAssignment != nil {
		for brokerID, count := range cc.currentAssignment.BrokerLoad {
			merged[brokerID] = count
		}
	}
	for brokerID, count := range callerLoad {
		merged[brokerID] += count
	}
	return merged
}

// mergeAssignment folds a newly assigned batch of partitions into the
// coordinator's running assignment. existing may be nil (first call).
func mergeAssignment(existing, added *Assignment) *Assignment {
	// Replica slices are copied rather than aliased: the freshly assigned
	// Assignment is also handed back to the caller, so sharing its slices
	// would let a caller mutating its own result silently corrupt the
	// coordinator's record of the cluster.
	merged := NewAssignment()
	if existing != nil {
		merged.Version = existing.Version
		for key, replicas := range existing.Partitions {
			merged.Partitions[key] = append([]int32(nil), replicas...)
		}
		for key, leader := range existing.Leaders {
			merged.Leaders[key] = leader
		}
	}
	for key, replicas := range added.Partitions {
		merged.Partitions[key] = append([]int32(nil), replicas...)
	}
	for key, leader := range added.Leaders {
		merged.Leaders[key] = leader
	}
	merged.Version++
	merged.RecomputeBrokerLoad()
	return merged
}

// TriggerRebalance manually triggers a rebalance
func (cc *ClusterCoordinator) TriggerRebalance(ctx context.Context) error {
	cc.mu.Lock()
	if cc.rebalancing {
		cc.mu.Unlock()
		return fmt.Errorf("rebalance already in progress")
	}
	cc.rebalancing = true
	cc.mu.Unlock()

	defer func() {
		cc.mu.Lock()
		cc.rebalancing = false
		cc.mu.Unlock()
	}()

	return cc.performRebalance(ctx)
}

// GetCurrentAssignment returns the current partition assignment
func (cc *ClusterCoordinator) GetCurrentAssignment() *Assignment {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.currentAssignment == nil {
		return nil
	}

	return cc.currentAssignment.Clone()
}

// GetRebalanceStats returns rebalancing statistics
func (cc *ClusterCoordinator) GetRebalanceStats() RebalanceStats {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	return RebalanceStats{
		Rebalancing:       cc.rebalancing,
		LastRebalanceTime: cc.lastRebalanceTime,
		RebalanceCount:    cc.rebalanceCount,
		FailedRebalances:  cc.failedRebalances,
	}
}

// onBrokerAdded handles broker addition
func (cc *ClusterCoordinator) onBrokerAdded(broker *BrokerMetadata) {
	log.Printf("[ClusterCoordinator] Broker %d added, triggering rebalance", broker.ID)

	// Trigger async rebalance
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := cc.TriggerRebalance(ctx); err != nil {
			log.Printf("[ClusterCoordinator] Rebalance failed after broker add: %v", err)
		}
	}()
}

// onBrokerRemoved handles broker removal
func (cc *ClusterCoordinator) onBrokerRemoved(brokerID int32) {
	log.Printf("[ClusterCoordinator] Broker %d removed, triggering rebalance", brokerID)

	// Trigger async rebalance
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := cc.TriggerRebalance(ctx); err != nil {
			log.Printf("[ClusterCoordinator] Rebalance failed after broker removal: %v", err)
		}
	}()
}

// onBrokerFailed handles broker failure
func (cc *ClusterCoordinator) onBrokerFailed(brokerID int32) {
	log.Printf("[ClusterCoordinator] Broker %d failed, triggering rebalance", brokerID)

	// Trigger async rebalance
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := cc.TriggerRebalance(ctx); err != nil {
			log.Printf("[ClusterCoordinator] Rebalance failed after broker failure: %v", err)
		}
	}()
}

// rebalanceLoop periodically checks if rebalancing is needed
func (cc *ClusterCoordinator) rebalanceLoop() {
	defer cc.wg.Done()

	ticker := time.NewTicker(cc.rebalanceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cc.ctx.Done():
			return
		case <-ticker.C:
			cc.checkAndRebalance()
		}
	}
}

// checkAndRebalance checks if rebalancing is needed and triggers it
func (cc *ClusterCoordinator) checkAndRebalance() {
	cc.mu.Lock()
	if cc.rebalancing || cc.currentAssignment == nil {
		cc.mu.Unlock()
		return
	}

	// Check if rebalancing is needed
	if cc.currentAssignment.IsBalanced(cc.rebalanceThreshold) {
		cc.mu.Unlock()
		return
	}

	cc.rebalancing = true
	cc.mu.Unlock()

	defer func() {
		cc.mu.Lock()
		cc.rebalancing = false
		cc.mu.Unlock()
	}()

	// Perform rebalance
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := cc.performRebalance(ctx); err != nil {
		log.Printf("[ClusterCoordinator] Periodic rebalance failed: %v", err)
	}
}

// performRebalance performs the actual rebalancing
func (cc *ClusterCoordinator) performRebalance(ctx context.Context) error {
	cc.mu.RLock()
	current := cc.currentAssignment
	partitionLimit := cc.partitionLimit
	cc.mu.RUnlock()

	if current == nil {
		return fmt.Errorf("no current assignment to rebalance")
	}

	startTime := time.Now()
	log.Printf("[ClusterCoordinator] Starting rebalance...")

	// Get active brokers
	brokers := cc.getActiveBrokersForAssignment()
	if len(brokers) == 0 {
		cc.mu.Lock()
		cc.failedRebalances++
		cc.mu.Unlock()
		return fmt.Errorf("no active brokers available")
	}

	// Perform rebalance. MaxPartitionsPerBroker is carried over from the last
	// AssignPartitions call rather than left unset, so a rebalance triggered
	// internally (this method has no caller-supplied constraints) still
	// honours the cluster-wide limit. Rebalance derives broker load from
	// current.BrokerLoad itself (a move decrements the old broker and
	// increments the new one in the same map), so there is no ExistingLoad to
	// set here and no risk of double-counting a moved partition.
	constraints := &AssignmentConstraints{
		RackAware:              true,
		ExcludedBrokers:        make(map[int32]bool),
		MaxPartitionsPerBroker: partitionLimit,
	}

	newAssignment, err := cc.assignmentStrategy.Rebalance(current, brokers, constraints)
	if err != nil {
		cc.mu.Lock()
		cc.failedRebalances++
		cc.mu.Unlock()
		return fmt.Errorf("rebalance failed: %w", err)
	}

	// Update current assignment
	cc.mu.Lock()
	cc.currentAssignment = newAssignment
	cc.lastRebalanceTime = time.Now()
	cc.rebalanceCount++
	cc.mu.Unlock()

	duration := time.Since(startTime)
	stats := newAssignment.GetStats()

	log.Printf("[ClusterCoordinator] Rebalance complete in %v: "+
		"%d partitions, %d brokers, imbalance=%d, min=%d, max=%d",
		duration, stats.TotalPartitions, stats.TotalBrokers,
		stats.Imbalance, stats.MinLoad, stats.MaxLoad)

	return nil
}

// getActiveBrokersForAssignment converts broker metadata to BrokerInfo
func (cc *ClusterCoordinator) getActiveBrokersForAssignment() []BrokerInfo {
	brokers := cc.registry.ListActiveBrokers()

	brokerInfo := make([]BrokerInfo, len(brokers))
	for i, broker := range brokers {
		brokerInfo[i] = BrokerInfo{
			ID:       broker.ID,
			Rack:     broker.Rack,
			Capacity: broker.Capacity,
		}
	}

	return brokerInfo
}

// RebalanceStats contains rebalancing statistics
type RebalanceStats struct {
	Rebalancing       bool
	LastRebalanceTime time.Time
	RebalanceCount    int64
	FailedRebalances  int64
}
