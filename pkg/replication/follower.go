package replication

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// FollowerReplicator manages replication on the follower side
type FollowerReplicator struct {
	config *Config

	// Partition identification
	topic       string
	partitionID int
	replicaID   ReplicaID

	// Leader information
	mu          sync.RWMutex
	leaderID    ReplicaID
	leaderEpoch int64

	// Offset tracking
	logEndOffset  Offset // Follower's LEO (next offset to write)
	highWaterMark Offset // Leader's HW from last fetch

	// Fetch state
	lastFetchTime time.Time
	fetchLag      int64

	// Storage interface (injected)
	storage StorageWriter

	// Leader communication (injected)
	leaderClient LeaderClient

	// Metrics
	metrics ReplicationMetrics

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// StorageWriter defines the storage operations needed by the follower
type StorageWriter interface {
	// AppendMessages appends messages to the log
	AppendMessages(ctx context.Context, partition int, messages []*StorageMessage) error

	// GetLogEndOffset returns the next offset to be written
	GetLogEndOffset(partition int) (Offset, error)

	// SetHighWaterMark updates the high water mark
	SetHighWaterMark(partition int, hw Offset) error

	// Truncate truncates the log to the given offset (for leader changes)
	Truncate(partition int, offset Offset) error
}

// LeaderClient defines the interface for communicating with the leader
type LeaderClient interface {
	// Fetch sends a fetch request to the leader
	Fetch(ctx context.Context, req *FetchRequest) (*FetchResponse, error)

	// GetLeaderEndpoint returns the leader's network endpoint
	GetLeaderEndpoint(topic string, partitionID int) (string, error)
}

// NewFollowerReplicator creates a new follower replicator
func NewFollowerReplicator(
	config *Config,
	topic string,
	partitionID int,
	replicaID ReplicaID,
	leaderID ReplicaID,
	leaderEpoch int64,
	storage StorageWriter,
	leaderClient LeaderClient,
) *FollowerReplicator {
	ctx, cancel := context.WithCancel(context.Background())

	fr := &FollowerReplicator{
		config:       config,
		topic:        topic,
		partitionID:  partitionID,
		replicaID:    replicaID,
		leaderID:     leaderID,
		leaderEpoch:  leaderEpoch,
		storage:      storage,
		leaderClient: leaderClient,
		ctx:          ctx,
		cancel:       cancel,
	}

	// Initialize offsets from storage
	if leo, err := storage.GetLogEndOffset(partitionID); err == nil {
		fr.logEndOffset = leo
	}

	log.Printf("[Follower %d] Initialized replicator for %s:%d, leader=%d, epoch=%d, LEO=%d",
		replicaID, topic, partitionID, leaderID, leaderEpoch, fr.logEndOffset)

	return fr
}

// Start starts the fetch loop
func (fr *FollowerReplicator) Start() error {
	fr.wg.Add(1)
	go fr.fetchLoop()

	log.Printf("[Follower %d] Started replication for %s:%d",
		fr.replicaID, fr.topic, fr.partitionID)
	return nil
}

// Stop stops the replicator
func (fr *FollowerReplicator) Stop() error {
	fr.cancel()
	fr.wg.Wait()

	log.Printf("[Follower %d] Stopped replication for %s:%d",
		fr.replicaID, fr.topic, fr.partitionID)
	return nil
}

// fetchLoop continuously fetches data from the leader
func (fr *FollowerReplicator) fetchLoop() {
	defer fr.wg.Done()

	ticker := time.NewTicker(fr.config.FetcherInterval)
	defer ticker.Stop()

	consecutiveErrors := 0
	maxErrors := 5

	for {
		select {
		case <-fr.ctx.Done():
			return
		case <-ticker.C:
			if err := fr.performFetch(); err != nil {
				consecutiveErrors++
				log.Printf("[Follower %d] Fetch error (%d/%d): %v",
					fr.replicaID, consecutiveErrors, maxErrors, err)

				// Exponential backoff on errors
				if consecutiveErrors >= maxErrors {
					log.Printf("[Follower %d] Too many consecutive errors, backing off",
						fr.replicaID)
					time.Sleep(time.Duration(fr.config.FetchBackoffMs) * time.Millisecond)
					consecutiveErrors = 0
				}
			} else {
				consecutiveErrors = 0
			}
		}
	}
}

// performFetch performs a single fetch operation
func (fr *FollowerReplicator) performFetch() error {
	fr.mu.RLock()
	fetchOffset := fr.logEndOffset
	leaderEpoch := fr.leaderEpoch
	fr.mu.RUnlock()

	// Create fetch request
	req := &FetchRequest{
		ReplicaID:   fr.replicaID,
		Topic:       fr.topic,
		PartitionID: fr.partitionID,
		FetchOffset: fetchOffset,
		MaxBytes:    fr.config.FetchMaxBytes,
		LeaderEpoch: leaderEpoch,
		MinBytes:    fr.config.FetchMinBytes,
		MaxWaitMs:   fr.config.FetchMaxWaitMs,
	}

	startTime := time.Now()

	// Send fetch request to leader
	ctx, cancel := context.WithTimeout(fr.ctx, 30*time.Second)
	defer cancel()

	resp, err := fr.leaderClient.Fetch(ctx, req)
	if err != nil {
		return fmt.Errorf("fetch failed: %w", err)
	}

	fetchLatency := time.Since(startTime)

	// Handle response
	if err := fr.handleFetchResponse(resp, fetchLatency); err != nil {
		return fmt.Errorf("handle fetch response failed: %w", err)
	}

	return nil
}

// handleFetchResponse processes the fetch response
func (fr *FollowerReplicator) handleFetchResponse(resp *FetchResponse, latency time.Duration) error {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	// Capture starting offset for logging
	startingOffset := fr.logEndOffset

	// Check for errors
	if resp.ErrorCode != ErrorNone {
		return fr.handleFetchError(resp)
	}

	// Update leader epoch if it changed
	if resp.LeaderEpoch > fr.leaderEpoch {
		log.Printf("[Follower %d] Leader epoch changed: %d -> %d",
			fr.replicaID, fr.leaderEpoch, resp.LeaderEpoch)
		fr.leaderEpoch = resp.LeaderEpoch

		// Truncate log if needed (leader changed)
		if err := fr.storage.Truncate(fr.partitionID, fr.logEndOffset); err != nil {
			return fmt.Errorf("truncate failed: %w", err)
		}
	}

	// Update high water mark from leader
	if resp.HighWaterMark > fr.highWaterMark {
		fr.highWaterMark = resp.HighWaterMark
		if err := fr.storage.SetHighWaterMark(fr.partitionID, resp.HighWaterMark); err != nil {
			log.Printf("[Follower %d] Failed to update HW: %v", fr.replicaID, err)
		}
	}

	// If no messages, we're caught up
	if len(resp.Messages) == 0 {
		fr.lastFetchTime = time.Now()
		fr.fetchLag = int64(resp.LogEndOffset - fr.logEndOffset)
		return nil
	}

	// Convert replication messages to storage messages
	storageMessages := make([]*StorageMessage, len(resp.Messages))
	for i, msg := range resp.Messages {
		storageMessages[i] = &StorageMessage{
			Offset:    msg.Offset,
			Key:       msg.Key,
			Value:     msg.Value,
			Timestamp: msg.Timestamp,
			Headers:   msg.Headers,
		}
	}

	// Append messages to local log
	ctx, cancel := context.WithTimeout(fr.ctx, 10*time.Second)
	defer cancel()

	if err := fr.storage.AppendMessages(ctx, fr.partitionID, storageMessages); err != nil {
		return fmt.Errorf("append failed: %w", err)
	}

	// Update local LEO
	if len(resp.Messages) > 0 {
		lastOffset := resp.Messages[len(resp.Messages)-1].Offset
		fr.logEndOffset = lastOffset + 1
	}

	// Update metrics
	fr.lastFetchTime = time.Now()
	fr.fetchLag = int64(resp.LogEndOffset - fr.logEndOffset)
	fr.metrics.FetchRequestRate++
	fr.metrics.FetchBytesRate += float64(calculateMessageBytes(resp.Messages))
	fr.metrics.ReplicationLagMessages = fr.fetchLag
	fr.metrics.LastFetchLatencyMs = latency.Milliseconds()

	log.Printf("[Follower %d] Fetched %d messages, LEO: %d -> %d, HW: %d, lag: %d",
		fr.replicaID, len(resp.Messages), startingOffset, fr.logEndOffset,
		fr.highWaterMark, fr.fetchLag)

	return nil
}

// handleFetchError handles errors in fetch response
func (fr *FollowerReplicator) handleFetchError(resp *FetchResponse) error {
	switch resp.ErrorCode {
	case ErrorStaleEpoch:
		// Our epoch is stale, update it
		log.Printf("[Follower %d] Stale epoch detected, updating to %d",
			fr.replicaID, resp.LeaderEpoch)
		fr.leaderEpoch = resp.LeaderEpoch
		return nil

	case ErrorOffsetOutOfRange:
		// Our offset is out of range, reset to beginning
		log.Printf("[Follower %d] Offset out of range, resetting LEO to 0",
			fr.replicaID)
		fr.logEndOffset = 0
		if err := fr.storage.Truncate(fr.partitionID, 0); err != nil {
			return fmt.Errorf("truncate failed: %w", err)
		}
		return nil

	case ErrorNotLeader:
		// Leader changed, need to discover new leader
		log.Printf("[Follower %d] Current node is not leader",
			fr.replicaID)
		// In a real system, we'd query metadata to find new leader
		return fmt.Errorf("leader changed")

	default:
		return fmt.Errorf("fetch error: %s", resp.ErrorCode)
	}
}

// UpdateLeader updates the leader information
func (fr *FollowerReplicator) UpdateLeader(leaderID ReplicaID, leaderEpoch int64) error {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	if leaderEpoch < fr.leaderEpoch {
		return fmt.Errorf("cannot update to older epoch: current=%d, new=%d",
			fr.leaderEpoch, leaderEpoch)
	}

	fr.leaderID = leaderID
	fr.leaderEpoch = leaderEpoch

	log.Printf("[Follower %d] Updated leader to %d, epoch %d",
		fr.replicaID, leaderID, leaderEpoch)
	return nil
}

// GetLogEndOffset returns the current LEO
func (fr *FollowerReplicator) GetLogEndOffset() Offset {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	return fr.logEndOffset
}

// GetHighWaterMark returns the current HW
func (fr *FollowerReplicator) GetHighWaterMark() Offset {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	return fr.highWaterMark
}

// GetFetchLag returns the current lag behind leader
func (fr *FollowerReplicator) GetFetchLag() int64 {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	return fr.fetchLag
}

// GetMetrics returns replication metrics
func (fr *FollowerReplicator) GetMetrics() ReplicationMetrics {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	return fr.metrics
}

// calculateMessageBytes calculates total bytes in messages
func calculateMessageBytes(messages []*Message) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Key) + len(msg.Value)
	}
	return total
}
