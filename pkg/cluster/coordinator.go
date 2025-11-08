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
	registry          *BrokerRegistry
	assignmentStrategy AssignmentStrategy
	metadataClient    MetadataStore

	// Current cluster state
	currentAssignment *Assignment

	// Configuration
	rebalanceInterval  time.Duration
	rebalanceThreshold int // Imbalance threshold to trigger rebalance

	// Rebalancing state
	rebalancing        bool
	lastRebalanceTime  time.Time
	rebalanceCount     int64
	failedRebalances   int64

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

// AssignPartitions creates initial partition assignment
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

	// Create assignment
	assignment, err := cc.assignmentStrategy.Assign(partitions, brokers, constraints)
	if err != nil {
		return nil, fmt.Errorf("assignment failed: %w", err)
	}

	// Store assignment
	cc.currentAssignment = assignment

	log.Printf("[ClusterCoordinator] Created assignment: %d partitions across %d brokers",
		assignment.TotalPartitions(), len(brokers))

	return assignment, nil
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

	// Perform rebalance
	constraints := &AssignmentConstraints{
		RackAware:       true,
		ExcludedBrokers: make(map[int32]bool),
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
