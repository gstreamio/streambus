package replication

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// ReplicationManager coordinates replication for a partition
// It manages both leader and follower roles
type ReplicationManager struct {
	config *Config

	// Partition identification
	topic       string
	partitionID int
	replicaID   ReplicaID

	// Role management
	mu          sync.RWMutex
	role        PartitionRole
	leaderRep   *LeaderReplicator
	followerRep *FollowerReplicator

	// Replication tracking (for acks=all)
	replicationWaiters map[Offset]*replicationWaiter

	// Storage interface
	storage StorageAdapter

	// Leader client (for follower mode)
	leaderClient LeaderClient

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// PartitionRole represents whether this replica is leader or follower
type PartitionRole int

const (
	RoleFollower PartitionRole = iota
	RoleLeader
)

// StorageAdapter combines reader and writer interfaces
type StorageAdapter interface {
	StorageReader
	StorageWriter
}

// replicationWaiter tracks waiters for offset replication
type replicationWaiter struct {
	offset    Offset
	doneCh    chan struct{}
	err       error
	createdAt time.Time
}

// NewReplicationManager creates a new replication manager
func NewReplicationManager(
	config *Config,
	topic string,
	partitionID int,
	replicaID ReplicaID,
	storage StorageAdapter,
	leaderClient LeaderClient,
) *ReplicationManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &ReplicationManager{
		config:             config,
		topic:              topic,
		partitionID:        partitionID,
		replicaID:          replicaID,
		role:               RoleFollower, // Start as follower
		replicationWaiters: make(map[Offset]*replicationWaiter),
		storage:            storage,
		leaderClient:       leaderClient,
		ctx:                ctx,
		cancel:             cancel,
	}
}

// Start starts the replication manager
func (rm *ReplicationManager) Start() error {
	rm.wg.Add(1)
	go rm.cleanupWaitersLoop()

	log.Printf("[ReplicationManager %d] Started for %s:%d",
		rm.replicaID, rm.topic, rm.partitionID)
	return nil
}

// Stop stops the replication manager
func (rm *ReplicationManager) Stop() error {
	rm.cancel()

	rm.mu.Lock()
	if rm.leaderRep != nil {
		rm.leaderRep.Stop()
	}
	if rm.followerRep != nil {
		rm.followerRep.Stop()
	}

	// Cancel all pending waiters
	for _, waiter := range rm.replicationWaiters {
		waiter.err = fmt.Errorf("replication manager stopped")
		close(waiter.doneCh)
	}
	rm.replicationWaiters = make(map[Offset]*replicationWaiter)
	rm.mu.Unlock()

	rm.wg.Wait()

	log.Printf("[ReplicationManager %d] Stopped for %s:%d",
		rm.replicaID, rm.topic, rm.partitionID)
	return nil
}

// BecomeLeader transitions this replica to leader role
func (rm *ReplicationManager) BecomeLeader(
	leaderEpoch int64,
	replicas []ReplicaID,
) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Stop follower replicator if running
	if rm.followerRep != nil {
		rm.followerRep.Stop()
		rm.followerRep = nil
	}

	// Create leader replicator
	rm.leaderRep = NewLeaderReplicator(
		rm.config,
		rm.topic,
		rm.partitionID,
		leaderEpoch,
		rm.replicaID,
		replicas,
		rm.storage,
	)

	if err := rm.leaderRep.Start(); err != nil {
		return fmt.Errorf("failed to start leader replicator: %w", err)
	}

	rm.role = RoleLeader

	log.Printf("[ReplicationManager %d] Became leader for %s:%d, epoch=%d, replicas=%v",
		rm.replicaID, rm.topic, rm.partitionID, leaderEpoch, replicas)

	return nil
}

// BecomeFollower transitions this replica to follower role
func (rm *ReplicationManager) BecomeFollower(
	leaderID ReplicaID,
	leaderEpoch int64,
) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Stop leader replicator if running
	if rm.leaderRep != nil {
		rm.leaderRep.Stop()
		rm.leaderRep = nil
	}

	// Cancel all pending replication waiters
	for _, waiter := range rm.replicationWaiters {
		waiter.err = fmt.Errorf("became follower, no longer leader")
		close(waiter.doneCh)
	}
	rm.replicationWaiters = make(map[Offset]*replicationWaiter)

	// Create follower replicator
	rm.followerRep = NewFollowerReplicator(
		rm.config,
		rm.topic,
		rm.partitionID,
		rm.replicaID,
		leaderID,
		leaderEpoch,
		rm.storage,
		rm.leaderClient,
	)

	if err := rm.followerRep.Start(); err != nil {
		return fmt.Errorf("failed to start follower replicator: %w", err)
	}

	rm.role = RoleFollower

	log.Printf("[ReplicationManager %d] Became follower for %s:%d, leader=%d, epoch=%d",
		rm.replicaID, rm.topic, rm.partitionID, leaderID, leaderEpoch)

	return nil
}

// IsLeader returns true if this replica is the leader
func (rm *ReplicationManager) IsLeader() bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.role == RoleLeader
}

// HandleFetchRequest handles a fetch request (only valid for leader)
func (rm *ReplicationManager) HandleFetchRequest(req *FetchRequest) (*FetchResponse, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.role != RoleLeader || rm.leaderRep == nil {
		return &FetchResponse{
			Topic:       req.Topic,
			PartitionID: req.PartitionID,
			ErrorCode:   ErrorNotLeader,
			Error:       fmt.Errorf("not leader"),
		}, nil
	}

	return rm.leaderRep.HandleFetchRequest(req)
}

// WaitForISR waits for an offset to be replicated to all ISR members
// Used for acks=all produce requests
func (rm *ReplicationManager) WaitForISR(ctx context.Context, offset Offset, timeoutMs int32) error {
	rm.mu.Lock()

	// Check if we're the leader
	if rm.role != RoleLeader || rm.leaderRep == nil {
		rm.mu.Unlock()
		return fmt.Errorf("not leader")
	}

	// Check if already replicated
	hw := rm.leaderRep.GetHighWaterMark()
	if offset < hw {
		rm.mu.Unlock()
		return nil // Already replicated
	}

	// Create waiter
	waiter := &replicationWaiter{
		offset:    offset,
		doneCh:    make(chan struct{}),
		createdAt: time.Now(),
	}
	rm.replicationWaiters[offset] = waiter
	rm.mu.Unlock()

	// Wait for replication or timeout
	timeout := time.Duration(timeoutMs) * time.Millisecond
	if timeoutMs == 0 {
		timeout = 30 * time.Second // Default timeout
	}

	select {
	case <-waiter.doneCh:
		return waiter.err
	case <-time.After(timeout):
		rm.mu.Lock()
		delete(rm.replicationWaiters, offset)
		rm.mu.Unlock()
		return fmt.Errorf("timeout waiting for ISR replication")
	case <-ctx.Done():
		rm.mu.Lock()
		delete(rm.replicationWaiters, offset)
		rm.mu.Unlock()
		return ctx.Err()
	}
}

// NotifyReplication notifies waiters that replication has progressed
// Called periodically or when HW advances
func (rm *ReplicationManager) NotifyReplication() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.role != RoleLeader || rm.leaderRep == nil {
		return
	}

	hw := rm.leaderRep.GetHighWaterMark()

	// Notify all waiters whose offsets have been replicated
	for offset, waiter := range rm.replicationWaiters {
		if offset < hw {
			close(waiter.doneCh)
			delete(rm.replicationWaiters, offset)
		}
	}
}

// UpdateLogEndOffset updates the leader's LEO after appending
func (rm *ReplicationManager) UpdateLogEndOffset(newLEO Offset) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.role != RoleLeader || rm.leaderRep == nil {
		return fmt.Errorf("not leader")
	}

	rm.leaderRep.UpdateLogEndOffset(newLEO)

	// Check if any waiters can be notified
	go rm.NotifyReplication()

	return nil
}

// GetHighWaterMark returns the current high water mark
func (rm *ReplicationManager) GetHighWaterMark() (Offset, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.role == RoleLeader && rm.leaderRep != nil {
		return rm.leaderRep.GetHighWaterMark(), nil
	} else if rm.role == RoleFollower && rm.followerRep != nil {
		return rm.followerRep.GetHighWaterMark(), nil
	}

	return 0, fmt.Errorf("no replicator active")
}

// GetISR returns the current ISR (only valid for leader)
func (rm *ReplicationManager) GetISR() ([]ReplicaID, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.role != RoleLeader || rm.leaderRep == nil {
		return nil, fmt.Errorf("not leader")
	}

	return rm.leaderRep.GetISR(), nil
}

// GetMetrics returns replication metrics
func (rm *ReplicationManager) GetMetrics() ReplicationMetrics {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.role == RoleLeader && rm.leaderRep != nil {
		return rm.leaderRep.GetMetrics()
	} else if rm.role == RoleFollower && rm.followerRep != nil {
		return rm.followerRep.GetMetrics()
	}

	return ReplicationMetrics{}
}

// cleanupWaitersLoop periodically cleans up expired waiters
func (rm *ReplicationManager) cleanupWaitersLoop() {
	defer rm.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-rm.ctx.Done():
			return
		case <-ticker.C:
			rm.cleanupExpiredWaiters()
			rm.NotifyReplication()
		}
	}
}

// cleanupExpiredWaiters removes waiters that have been waiting too long
func (rm *ReplicationManager) cleanupExpiredWaiters() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	now := time.Now()
	maxAge := 60 * time.Second

	for offset, waiter := range rm.replicationWaiters {
		if now.Sub(waiter.createdAt) > maxAge {
			waiter.err = fmt.Errorf("waiter expired")
			close(waiter.doneCh)
			delete(rm.replicationWaiters, offset)
		}
	}
}
