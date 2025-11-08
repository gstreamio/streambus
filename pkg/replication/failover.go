package replication

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// FailoverCoordinator manages automatic leader failover for partitions
type FailoverCoordinator struct {
	config *Config

	// Broker identity
	brokerID ReplicaID

	// Partition managers being monitored
	mu        sync.RWMutex
	partitions map[string]*PartitionFailoverState

	// Failure detection
	failureDetector *FailureDetector

	// Metadata client (for ISR and leader updates)
	metadataClient MetadataClient

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// PartitionFailoverState tracks failover state for a partition
type PartitionFailoverState struct {
	topic          string
	partitionID    int
	currentLeader  ReplicaID
	leaderEpoch    int64
	replicas       []ReplicaID
	isr            []ReplicaID
	lastLeaderSeen time.Time
	failoverInProgress bool
}

// MetadataClient defines operations needed for failover
type MetadataClient interface {
	// GetPartitionLeader returns current leader and epoch
	GetPartitionLeader(topic string, partitionID int) (ReplicaID, int64, error)

	// GetPartitionISR returns current ISR
	GetPartitionISR(topic string, partitionID int) ([]ReplicaID, error)

	// UpdatePartitionLeader updates leader via Raft consensus
	UpdatePartitionLeader(ctx context.Context, topic string, partitionID int, newLeader ReplicaID, newEpoch int64) error

	// UpdatePartitionISR updates ISR via Raft consensus
	UpdatePartitionISR(ctx context.Context, topic string, partitionID int, newISR []ReplicaID) error

	// GetPartitionReplicas returns all replicas for partition
	GetPartitionReplicas(topic string, partitionID int) ([]ReplicaID, error)
}

// FailureDetector detects broker failures
type FailureDetector struct {
	mu              sync.RWMutex
	lastHeartbeat   map[ReplicaID]time.Time
	failureTimeout  time.Duration
	heartbeatClient HeartbeatClient
}

// HeartbeatClient sends heartbeats to brokers
type HeartbeatClient interface {
	// SendHeartbeat sends a heartbeat to a broker
	SendHeartbeat(ctx context.Context, brokerID ReplicaID) error
}

// NewFailoverCoordinator creates a new failover coordinator
func NewFailoverCoordinator(
	config *Config,
	brokerID ReplicaID,
	metadataClient MetadataClient,
	heartbeatClient HeartbeatClient,
) *FailoverCoordinator {
	ctx, cancel := context.WithCancel(context.Background())

	failureDetector := &FailureDetector{
		lastHeartbeat:   make(map[ReplicaID]time.Time),
		failureTimeout:  30 * time.Second, // 30 second timeout
		heartbeatClient: heartbeatClient,
	}

	return &FailoverCoordinator{
		config:          config,
		brokerID:        brokerID,
		partitions:      make(map[string]*PartitionFailoverState),
		failureDetector: failureDetector,
		metadataClient:  metadataClient,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start starts the failover coordinator
func (fc *FailoverCoordinator) Start() error {
	fc.wg.Add(2)
	go fc.monitorLeadersLoop()
	go fc.failureDetector.heartbeatLoop(fc.ctx, &fc.wg)

	log.Printf("[FailoverCoordinator %d] Started", fc.brokerID)
	return nil
}

// Stop stops the failover coordinator
func (fc *FailoverCoordinator) Stop() error {
	fc.cancel()
	fc.wg.Wait()

	log.Printf("[FailoverCoordinator %d] Stopped", fc.brokerID)
	return nil
}

// RegisterPartition registers a partition for failover monitoring
func (fc *FailoverCoordinator) RegisterPartition(topic string, partitionID int) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	key := partitionKey(topic, partitionID)
	if _, exists := fc.partitions[key]; exists {
		return fmt.Errorf("partition %s already registered", key)
	}

	// Get current partition state from metadata
	leader, epoch, err := fc.metadataClient.GetPartitionLeader(topic, partitionID)
	if err != nil {
		return fmt.Errorf("failed to get partition leader: %w", err)
	}

	isr, err := fc.metadataClient.GetPartitionISR(topic, partitionID)
	if err != nil {
		return fmt.Errorf("failed to get partition ISR: %w", err)
	}

	replicas, err := fc.metadataClient.GetPartitionReplicas(topic, partitionID)
	if err != nil {
		return fmt.Errorf("failed to get partition replicas: %w", err)
	}

	state := &PartitionFailoverState{
		topic:          topic,
		partitionID:    partitionID,
		currentLeader:  leader,
		leaderEpoch:    epoch,
		replicas:       replicas,
		isr:            isr,
		lastLeaderSeen: time.Now(),
	}

	fc.partitions[key] = state

	log.Printf("[FailoverCoordinator %d] Registered partition %s, leader=%d, epoch=%d, ISR=%v",
		fc.brokerID, key, leader, epoch, isr)

	return nil
}

// monitorLeadersLoop periodically checks leader health
func (fc *FailoverCoordinator) monitorLeadersLoop() {
	defer fc.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-fc.ctx.Done():
			return
		case <-ticker.C:
			fc.checkLeaderHealth()
		}
	}
}

// checkLeaderHealth checks if leaders are healthy and triggers failover if needed
func (fc *FailoverCoordinator) checkLeaderHealth() {
	fc.mu.RLock()
	partitions := make([]*PartitionFailoverState, 0, len(fc.partitions))
	for _, state := range fc.partitions {
		partitions = append(partitions, state)
	}
	fc.mu.RUnlock()

	for _, state := range partitions {
		// Skip if failover already in progress
		if state.failoverInProgress {
			continue
		}

		// Check if leader has failed
		if fc.failureDetector.HasFailed(state.currentLeader) {
			log.Printf("[FailoverCoordinator %d] Detected leader failure for %s:%d, leader=%d",
				fc.brokerID, state.topic, state.partitionID, state.currentLeader)

			// Trigger failover
			if err := fc.triggerFailover(state); err != nil {
				log.Printf("[FailoverCoordinator %d] Failover failed for %s:%d: %v",
					fc.brokerID, state.topic, state.partitionID, err)
			}
		}
	}
}

// triggerFailover initiates failover for a partition
func (fc *FailoverCoordinator) triggerFailover(state *PartitionFailoverState) error {
	fc.mu.Lock()
	state.failoverInProgress = true
	fc.mu.Unlock()

	defer func() {
		fc.mu.Lock()
		state.failoverInProgress = false
		fc.mu.Unlock()
	}()

	log.Printf("[FailoverCoordinator %d] Starting failover for %s:%d",
		fc.brokerID, state.topic, state.partitionID)

	// Step 1: Elect new leader from ISR
	newLeader, err := fc.electNewLeader(state)
	if err != nil {
		return fmt.Errorf("leader election failed: %w", err)
	}

	// Step 2: Increment epoch
	newEpoch := state.leaderEpoch + 1

	// Step 3: Update metadata via Raft
	ctx, cancel := context.WithTimeout(fc.ctx, 10*time.Second)
	defer cancel()

	if err := fc.metadataClient.UpdatePartitionLeader(ctx, state.topic, state.partitionID, newLeader, newEpoch); err != nil {
		return fmt.Errorf("failed to update partition leader: %w", err)
	}

	// Step 4: Update local state
	fc.mu.Lock()
	state.currentLeader = newLeader
	state.leaderEpoch = newEpoch
	state.lastLeaderSeen = time.Now()
	fc.mu.Unlock()

	log.Printf("[FailoverCoordinator %d] Failover complete for %s:%d: new leader=%d, epoch=%d",
		fc.brokerID, state.topic, state.partitionID, newLeader, newEpoch)

	return nil
}

// electNewLeader elects a new leader from ISR using preferred replica strategy
func (fc *FailoverCoordinator) electNewLeader(state *PartitionFailoverState) (ReplicaID, error) {
	// Filter out failed replicas from ISR
	aliveISR := make([]ReplicaID, 0, len(state.isr))
	for _, replica := range state.isr {
		if !fc.failureDetector.HasFailed(replica) {
			aliveISR = append(aliveISR, replica)
		}
	}

	if len(aliveISR) == 0 {
		return 0, fmt.Errorf("no alive replicas in ISR")
	}

	// Preferred replica strategy: choose first alive ISR member
	// In production, we could use:
	// - Least loaded replica
	// - Replica with highest LEO
	// - Rack-aware selection
	newLeader := aliveISR[0]

	log.Printf("[FailoverCoordinator %d] Elected new leader: %d from ISR: %v",
		fc.brokerID, newLeader, aliveISR)

	return newLeader, nil
}

// HasFailed checks if a broker has failed
func (fd *FailureDetector) HasFailed(brokerID ReplicaID) bool {
	fd.mu.RLock()
	defer fd.mu.RUnlock()

	lastSeen, exists := fd.lastHeartbeat[brokerID]
	if !exists {
		// Never seen = not failed (assume healthy until proven otherwise)
		return false
	}

	return time.Since(lastSeen) > fd.failureTimeout
}

// RecordHeartbeat records a successful heartbeat from a broker
func (fd *FailureDetector) RecordHeartbeat(brokerID ReplicaID) {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	fd.lastHeartbeat[brokerID] = time.Now()
}

// heartbeatLoop periodically sends heartbeats to brokers
func (fd *FailureDetector) heartbeatLoop(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// In production, this would send heartbeats to all known brokers
			// For now, we just track heartbeats from other components
		}
	}
}

// FailoverMetrics tracks failover statistics
type FailoverMetrics struct {
	TotalFailovers       int64
	SuccessfulFailovers  int64
	FailedFailovers      int64
	AverageFailoverTime  time.Duration
	LastFailoverTime     time.Time
}

// GetMetrics returns failover metrics
func (fc *FailoverCoordinator) GetMetrics() FailoverMetrics {
	// TODO: Implement metrics tracking
	return FailoverMetrics{}
}
