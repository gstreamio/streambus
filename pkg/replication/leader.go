package replication

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// LeaderReplicator manages replication on the leader side
type LeaderReplicator struct {
	config *Config

	// Partition state
	topic       string
	partitionID int
	leaderEpoch int64
	replicaID   ReplicaID

	// Replica tracking
	mu            sync.RWMutex
	replicas      map[ReplicaID]*ReplicationState
	isr           map[ReplicaID]bool // Fast ISR lookup
	highWaterMark Offset
	logEndOffset  Offset

	// Storage interface (injected)
	storage StorageReader

	// ISR change notifications
	isrChangeCh chan *ISRChangeNotification

	// Metrics
	metrics ReplicationMetrics

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// StorageReader defines the storage operations needed by the replicator
type StorageReader interface {
	// ReadRange reads messages between start and end offsets
	ReadRange(ctx context.Context, partition int, startOffset, endOffset Offset) ([]*StorageMessage, error)

	// GetLogEndOffset returns the next offset to be written
	GetLogEndOffset(partition int) (Offset, error)

	// GetHighWaterMark returns the current high water mark
	GetHighWaterMark(partition int) (Offset, error)
}

// StorageMessage represents a message from storage
type StorageMessage struct {
	Offset    Offset
	Key       []byte
	Value     []byte
	Timestamp int64
	Headers   map[string][]byte
}

// NewLeaderReplicator creates a new leader replicator
func NewLeaderReplicator(
	config *Config,
	topic string,
	partitionID int,
	leaderEpoch int64,
	replicaID ReplicaID,
	replicas []ReplicaID,
	storage StorageReader,
) *LeaderReplicator {
	ctx, cancel := context.WithCancel(context.Background())

	lr := &LeaderReplicator{
		config:      config,
		topic:       topic,
		partitionID: partitionID,
		leaderEpoch: leaderEpoch,
		replicaID:   replicaID,
		replicas:    make(map[ReplicaID]*ReplicationState),
		isr:         make(map[ReplicaID]bool),
		storage:     storage,
		isrChangeCh: make(chan *ISRChangeNotification, 10),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Initialize replica states
	now := time.Now()
	for _, rid := range replicas {
		state := &ReplicationState{
			ReplicaID:        rid,
			LogEndOffset:     0,
			HighWaterMark:    0,
			LastFetchTime:    now,
			LastCaughtUpTime: now,
			FetchLag:         0,
			InSync:           true, // Start all in ISR
		}
		lr.replicas[rid] = state
		lr.isr[rid] = true
	}

	// Initialize offsets from storage
	if leo, err := storage.GetLogEndOffset(partitionID); err == nil {
		lr.logEndOffset = leo
	}
	if hw, err := storage.GetHighWaterMark(partitionID); err == nil {
		lr.highWaterMark = hw
	}

	log.Printf("[Leader %d] Initialized replicator for %s:%d, epoch=%d, replicas=%v, LEO=%d, HW=%d",
		replicaID, topic, partitionID, leaderEpoch, replicas, lr.logEndOffset, lr.highWaterMark)

	return lr
}

// Start starts background tasks
func (lr *LeaderReplicator) Start() error {
	lr.wg.Add(1)
	go lr.hwUpdateLoop()

	log.Printf("[Leader %d] Started replication for %s:%d",
		lr.replicaID, lr.topic, lr.partitionID)
	return nil
}

// Stop stops the replicator
func (lr *LeaderReplicator) Stop() error {
	lr.cancel()
	lr.wg.Wait()

	log.Printf("[Leader %d] Stopped replication for %s:%d",
		lr.replicaID, lr.topic, lr.partitionID)
	return nil
}

// HandleFetchRequest handles a fetch request from a follower
func (lr *LeaderReplicator) HandleFetchRequest(req *FetchRequest) (*FetchResponse, error) {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	// Validate request
	if req.Topic != lr.topic || req.PartitionID != lr.partitionID {
		return &FetchResponse{
			Topic:       req.Topic,
			PartitionID: req.PartitionID,
			ErrorCode:   ErrorPartitionNotFound,
			Error:       fmt.Errorf("partition not found"),
		}, nil
	}

	// Check if replica is in replica set
	replicaState, exists := lr.replicas[req.ReplicaID]
	if !exists {
		return &FetchResponse{
			Topic:       req.Topic,
			PartitionID: req.PartitionID,
			ErrorCode:   ErrorReplicaNotInReplicas,
			Error:       fmt.Errorf("replica %d not in replica set", req.ReplicaID),
		}, nil
	}

	// Check for stale epoch
	if req.LeaderEpoch < lr.leaderEpoch {
		return &FetchResponse{
			Topic:       req.Topic,
			PartitionID: req.PartitionID,
			LeaderEpoch: lr.leaderEpoch,
			ErrorCode:   ErrorStaleEpoch,
			Error:       fmt.Errorf("stale epoch: request=%d, leader=%d", req.LeaderEpoch, lr.leaderEpoch),
		}, nil
	}

	// Update replica state
	replicaState.LogEndOffset = req.FetchOffset
	replicaState.LastFetchTime = time.Now()
	replicaState.FetchLag = int64(lr.logEndOffset - req.FetchOffset)

	// Check offset range
	if req.FetchOffset < 0 || req.FetchOffset > lr.logEndOffset {
		return &FetchResponse{
			Topic:         req.Topic,
			PartitionID:   req.PartitionID,
			LeaderEpoch:   lr.leaderEpoch,
			HighWaterMark: lr.highWaterMark,
			LogEndOffset:  lr.logEndOffset,
			ErrorCode:     ErrorOffsetOutOfRange,
			Error:         fmt.Errorf("offset out of range: fetch=%d, leo=%d", req.FetchOffset, lr.logEndOffset),
		}, nil
	}

	// If follower is caught up, return empty response
	if req.FetchOffset >= lr.logEndOffset {
		return &FetchResponse{
			Topic:         req.Topic,
			PartitionID:   req.PartitionID,
			LeaderEpoch:   lr.leaderEpoch,
			HighWaterMark: lr.highWaterMark,
			LogEndOffset:  lr.logEndOffset,
			Messages:      nil,
			ErrorCode:     ErrorNone,
		}, nil
	}

	// Fetch messages from storage
	endOffset := req.FetchOffset + Offset(req.MaxBytes/1024) // Rough estimate
	if endOffset > lr.logEndOffset {
		endOffset = lr.logEndOffset
	}

	storageMessages, err := lr.storage.ReadRange(lr.ctx, lr.partitionID, req.FetchOffset, endOffset)
	if err != nil {
		log.Printf("[Leader %d] Failed to read messages: %v", lr.replicaID, err)
		return &FetchResponse{
			Topic:       req.Topic,
			PartitionID: req.PartitionID,
			LeaderEpoch: lr.leaderEpoch,
			ErrorCode:   ErrorUnknown,
			Error:       err,
		}, nil
	}

	// Convert storage messages to replication messages
	messages := make([]*Message, 0, len(storageMessages))
	totalBytes := 0
	for _, sm := range storageMessages {
		if totalBytes >= req.MaxBytes {
			break
		}
		msg := &Message{
			Offset:    sm.Offset,
			Key:       sm.Key,
			Value:     sm.Value,
			Timestamp: sm.Timestamp,
			Headers:   sm.Headers,
		}
		messages = append(messages, msg)
		totalBytes += len(sm.Key) + len(sm.Value)
	}

	// Update metrics
	lr.metrics.FetchRequestRate++
	lr.metrics.FetchBytesRate += float64(totalBytes)

	log.Printf("[Leader %d] Served fetch from replica %d: offset=%d, count=%d, bytes=%d",
		lr.replicaID, req.ReplicaID, req.FetchOffset, len(messages), totalBytes)

	return &FetchResponse{
		Topic:         req.Topic,
		PartitionID:   req.PartitionID,
		LeaderEpoch:   lr.leaderEpoch,
		HighWaterMark: lr.highWaterMark,
		LogEndOffset:  lr.logEndOffset,
		Messages:      messages,
		ErrorCode:     ErrorNone,
	}, nil
}

// UpdateLogEndOffset updates the leader's LEO (called after appending)
func (lr *LeaderReplicator) UpdateLogEndOffset(newLEO Offset) {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	lr.logEndOffset = newLEO
	log.Printf("[Leader %d] Updated LEO to %d", lr.replicaID, newLEO)
}

// GetHighWaterMark returns the current high water mark
func (lr *LeaderReplicator) GetHighWaterMark() Offset {
	lr.mu.RLock()
	defer lr.mu.RUnlock()
	return lr.highWaterMark
}

// GetISR returns the current ISR
func (lr *LeaderReplicator) GetISR() []ReplicaID {
	lr.mu.RLock()
	defer lr.mu.RUnlock()

	isr := make([]ReplicaID, 0, len(lr.isr))
	for rid := range lr.isr {
		isr = append(isr, rid)
	}
	return isr
}

// hwUpdateLoop periodically updates the high water mark and checks ISR
func (lr *LeaderReplicator) hwUpdateLoop() {
	defer lr.wg.Done()

	ticker := time.NewTicker(time.Duration(lr.config.HighWaterMarkCheckpointIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-lr.ctx.Done():
			return
		case <-ticker.C:
			lr.updateHighWaterMark()
			lr.checkISRMembership()
		}
	}
}

// updateHighWaterMark updates HW to the minimum LEO of all ISR members
func (lr *LeaderReplicator) updateHighWaterMark() {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	// HW is the minimum LEO of all ISR members
	minLEO := lr.logEndOffset
	for rid := range lr.isr {
		if state, exists := lr.replicas[rid]; exists {
			if state.LogEndOffset < minLEO {
				minLEO = state.LogEndOffset
			}
		}
	}

	if minLEO > lr.highWaterMark {
		oldHW := lr.highWaterMark
		lr.highWaterMark = minLEO
		log.Printf("[Leader %d] Advanced HW: %d -> %d", lr.replicaID, oldHW, minLEO)
	}
}

// checkISRMembership checks if any replicas should be added/removed from ISR
func (lr *LeaderReplicator) checkISRMembership() {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	now := time.Now()
	maxLag := lr.config.ReplicaLagMaxMessages
	maxLagTime := time.Duration(lr.config.ReplicaLagTimeMaxMs) * time.Millisecond

	for rid, state := range lr.replicas {
		isInISR := lr.isr[rid]

		// Calculate lag
		messageLag := int64(lr.logEndOffset - state.LogEndOffset)
		timeLag := now.Sub(state.LastFetchTime)

		// Check if replica should be removed from ISR
		if isInISR && (messageLag > maxLag || timeLag > maxLagTime) {
			delete(lr.isr, rid)
			state.InSync = false
			log.Printf("[Leader %d] Removed replica %d from ISR: messageLag=%d, timeLag=%v",
				lr.replicaID, rid, messageLag, timeLag)

			// Send ISR change notification
			lr.isrChangeCh <- &ISRChangeNotification{
				Topic:       lr.topic,
				PartitionID: lr.partitionID,
				NewISR:      lr.getISRList(),
				Reason:      fmt.Sprintf("Replica %d exceeded lag limits", rid),
				Timestamp:   now,
			}
			lr.metrics.ISRShrinkRate++
		}

		// Check if replica should be added back to ISR
		if !isInISR && messageLag <= maxLag/2 && timeLag <= maxLagTime/2 {
			lr.isr[rid] = true
			state.InSync = true
			state.LastCaughtUpTime = now
			log.Printf("[Leader %d] Added replica %d back to ISR: messageLag=%d, timeLag=%v",
				lr.replicaID, rid, messageLag, timeLag)

			// Send ISR change notification
			lr.isrChangeCh <- &ISRChangeNotification{
				Topic:       lr.topic,
				PartitionID: lr.partitionID,
				NewISR:      lr.getISRList(),
				Reason:      fmt.Sprintf("Replica %d caught up", rid),
				Timestamp:   now,
			}
			lr.metrics.ISRExpandRate++
		}
	}
}

// getISRList returns the current ISR as a slice (caller must hold lock)
func (lr *LeaderReplicator) getISRList() []ReplicaID {
	isr := make([]ReplicaID, 0, len(lr.isr))
	for rid := range lr.isr {
		isr = append(isr, rid)
	}
	return isr
}

// ISRChangeChannel returns the channel for ISR change notifications
func (lr *LeaderReplicator) ISRChangeChannel() <-chan *ISRChangeNotification {
	return lr.isrChangeCh
}

// GetMetrics returns replication metrics
func (lr *LeaderReplicator) GetMetrics() ReplicationMetrics {
	lr.mu.RLock()
	defer lr.mu.RUnlock()
	return lr.metrics
}

// GetPartitionState returns the full partition replication state
func (lr *LeaderReplicator) GetPartitionState() *PartitionReplicationState {
	lr.mu.RLock()
	defer lr.mu.RUnlock()

	replicas := make([]ReplicaID, 0, len(lr.replicas))
	for rid := range lr.replicas {
		replicas = append(replicas, rid)
	}

	replicaStates := make(map[ReplicaID]*ReplicationState)
	for rid, state := range lr.replicas {
		// Deep copy state
		stateCopy := *state
		replicaStates[rid] = &stateCopy
	}

	return &PartitionReplicationState{
		Topic:         lr.topic,
		PartitionID:   lr.partitionID,
		Leader:        lr.replicaID,
		LeaderEpoch:   lr.leaderEpoch,
		Replicas:      replicas,
		ISR:           lr.getISRList(),
		ReplicaStates: replicaStates,
		HighWaterMark: lr.highWaterMark,
		LogEndOffset:  lr.logEndOffset,
	}
}
