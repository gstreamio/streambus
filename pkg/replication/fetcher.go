package replication

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// Fetcher coordinates replication for multiple partitions on a follower broker
type Fetcher struct {
	config    *Config
	replicaID ReplicaID

	mu          sync.RWMutex
	replicators map[string]*FollowerReplicator // key: "topic:partition"

	storage      StorageWriter
	leaderClient LeaderClient

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewFetcher creates a new fetcher coordinator
func NewFetcher(config *Config, replicaID ReplicaID, storage StorageWriter, leaderClient LeaderClient) *Fetcher {
	ctx, cancel := context.WithCancel(context.Background())

	return &Fetcher{
		config:       config,
		replicaID:    replicaID,
		replicators:  make(map[string]*FollowerReplicator),
		storage:      storage,
		leaderClient: leaderClient,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start starts the fetcher
func (f *Fetcher) Start() error {
	log.Printf("[Fetcher %d] Started", f.replicaID)
	return nil
}

// Stop stops all replicators
func (f *Fetcher) Stop() error {
	f.cancel()

	f.mu.Lock()
	defer f.mu.Unlock()

	// Stop all replicators
	for key, replicator := range f.replicators {
		if err := replicator.Stop(); err != nil {
			log.Printf("[Fetcher %d] Error stopping replicator %s: %v",
				f.replicaID, key, err)
		}
	}

	f.wg.Wait()

	log.Printf("[Fetcher %d] Stopped", f.replicaID)
	return nil
}

// AddPartition adds a partition to replicate
func (f *Fetcher) AddPartition(
	topic string,
	partitionID int,
	leaderID ReplicaID,
	leaderEpoch int64,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := partitionKey(topic, partitionID)

	// Check if already replicating
	if _, exists := f.replicators[key]; exists {
		return fmt.Errorf("already replicating %s", key)
	}

	// Create follower replicator
	replicator := NewFollowerReplicator(
		f.config,
		topic,
		partitionID,
		f.replicaID,
		leaderID,
		leaderEpoch,
		f.storage,
		f.leaderClient,
	)

	// Start replication
	if err := replicator.Start(); err != nil {
		return fmt.Errorf("failed to start replicator: %w", err)
	}

	f.replicators[key] = replicator

	log.Printf("[Fetcher %d] Added partition %s (leader=%d, epoch=%d)",
		f.replicaID, key, leaderID, leaderEpoch)

	return nil
}

// RemovePartition stops replicating a partition
func (f *Fetcher) RemovePartition(topic string, partitionID int) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := partitionKey(topic, partitionID)

	replicator, exists := f.replicators[key]
	if !exists {
		return fmt.Errorf("not replicating %s", key)
	}

	// Stop replicator
	if err := replicator.Stop(); err != nil {
		log.Printf("[Fetcher %d] Error stopping replicator %s: %v",
			f.replicaID, key, err)
	}

	delete(f.replicators, key)

	log.Printf("[Fetcher %d] Removed partition %s", f.replicaID, key)

	return nil
}

// UpdateLeader updates the leader for a partition
func (f *Fetcher) UpdateLeader(
	topic string,
	partitionID int,
	leaderID ReplicaID,
	leaderEpoch int64,
) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	key := partitionKey(topic, partitionID)

	replicator, exists := f.replicators[key]
	if !exists {
		return fmt.Errorf("not replicating %s", key)
	}

	return replicator.UpdateLeader(leaderID, leaderEpoch)
}

// GetReplicationLag returns the replication lag for a partition
func (f *Fetcher) GetReplicationLag(topic string, partitionID int) (int64, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	key := partitionKey(topic, partitionID)

	replicator, exists := f.replicators[key]
	if !exists {
		return 0, fmt.Errorf("not replicating %s", key)
	}

	return replicator.GetFetchLag(), nil
}

// GetAllMetrics returns metrics for all partitions
func (f *Fetcher) GetAllMetrics() map[string]ReplicationMetrics {
	f.mu.RLock()
	defer f.mu.RUnlock()

	metrics := make(map[string]ReplicationMetrics)
	for key, replicator := range f.replicators {
		metrics[key] = replicator.GetMetrics()
	}

	return metrics
}

// GetPartitionCount returns the number of partitions being replicated
func (f *Fetcher) GetPartitionCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.replicators)
}

// partitionKey generates a unique key for a topic:partition
func partitionKey(topic string, partitionID int) string {
	return fmt.Sprintf("%s:%d", topic, partitionID)
}
