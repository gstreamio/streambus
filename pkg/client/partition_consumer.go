package client

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
)

// PartitionConsumer is a consumer that is aware of partitions and can consume from multiple partitions
type PartitionConsumer struct {
	client *Client
	config ConsumerConfig

	// Topic and partition information
	topic           string
	partitions      []uint32
	partitionStates map[uint32]*partitionState

	// State management
	mu     sync.RWMutex
	closed int32

	// Metrics
	messagesRead int64
	bytesRead    int64
	fetchCount   int64
}

// partitionState tracks the state of a single partition
type partitionState struct {
	partitionID uint32
	offset      int64
	highWater   int64
	lastFetch   time.Time
	mu          sync.Mutex
}

// NewPartitionConsumer creates a new partition-aware consumer
func NewPartitionConsumer(client *Client, topic string, partitions []uint32) *PartitionConsumer {
	return NewPartitionConsumerWithConfig(client, topic, partitions, client.config.ConsumerConfig)
}

// NewPartitionConsumerWithConfig creates a new partition consumer with custom config
func NewPartitionConsumerWithConfig(client *Client, topic string, partitions []uint32, config ConsumerConfig) *PartitionConsumer {
	pc := &PartitionConsumer{
		client:          client,
		config:          config,
		topic:           topic,
		partitions:      partitions,
		partitionStates: make(map[uint32]*partitionState),
	}

	// Initialize partition states
	for _, p := range partitions {
		pc.partitionStates[p] = &partitionState{
			partitionID: p,
			offset:      config.StartOffset,
			highWater:   -1,
			lastFetch:   time.Time{},
		}
	}

	return pc
}

// FetchFromPartition fetches messages from a specific partition
func (pc *PartitionConsumer) FetchFromPartition(partitionID uint32) ([]protocol.Message, error) {
	if atomic.LoadInt32(&pc.closed) == 1 {
		return nil, ErrConsumerClosed
	}

	pc.mu.RLock()
	state, exists := pc.partitionStates[partitionID]
	pc.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("partition %d not assigned to this consumer", partitionID)
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	// Create fetch request
	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type:    protocol.RequestTypeFetch,
			Version: protocol.ProtocolVersion,
			Flags:   protocol.FlagNone,
		},
		Payload: &protocol.FetchRequest{
			Topic:       pc.topic,
			PartitionID: partitionID,
			Offset:      state.offset,
			MaxBytes:    pc.config.MaxFetchBytes,
		},
	}

	// Send request (with partition routing in the future)
	broker := pc.client.config.Brokers[0]
	resp, err := pc.client.sendRequestWithRetry(broker, req)
	if err != nil {
		return nil, err
	}

	fetchResp, ok := resp.Payload.(*protocol.FetchResponse)
	if !ok {
		return nil, ErrInvalidResponse
	}

	// Update partition state
	if len(fetchResp.Messages) > 0 {
		lastMsg := fetchResp.Messages[len(fetchResp.Messages)-1]
		state.offset = lastMsg.Offset + 1
	}

	state.highWater = fetchResp.HighWaterMark
	state.lastFetch = time.Now()

	// Update metrics
	atomic.AddInt64(&pc.messagesRead, int64(len(fetchResp.Messages)))
	atomic.AddInt64(&pc.fetchCount, 1)

	var bytes int64
	for _, msg := range fetchResp.Messages {
		bytes += int64(len(msg.Key) + len(msg.Value))
	}
	atomic.AddInt64(&pc.bytesRead, bytes)

	return fetchResp.Messages, nil
}

// FetchAll fetches from all assigned partitions
func (pc *PartitionConsumer) FetchAll() (map[uint32][]protocol.Message, error) {
	if atomic.LoadInt32(&pc.closed) == 1 {
		return nil, ErrConsumerClosed
	}

	results := make(map[uint32][]protocol.Message)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var lastErr error

	for _, partitionID := range pc.partitions {
		wg.Add(1)
		go func(pid uint32) {
			defer wg.Done()

			messages, err := pc.FetchFromPartition(pid)
			if err != nil {
				mu.Lock()
				lastErr = err
				mu.Unlock()
				return
			}

			mu.Lock()
			results[pid] = messages
			mu.Unlock()
		}(partitionID)
	}

	wg.Wait()

	if lastErr != nil && len(results) == 0 {
		return nil, lastErr
	}

	return results, nil
}

// FetchRoundRobin fetches from partitions in round-robin fashion
func (pc *PartitionConsumer) FetchRoundRobin() ([]protocol.Message, error) {
	if atomic.LoadInt32(&pc.closed) == 1 {
		return nil, ErrConsumerClosed
	}

	if len(pc.partitions) == 0 {
		return nil, fmt.Errorf("no partitions assigned")
	}

	var allMessages []protocol.Message

	// Try each partition once
	for _, partitionID := range pc.partitions {
		messages, err := pc.FetchFromPartition(partitionID)
		if err != nil {
			// Log error but continue with other partitions
			continue
		}

		allMessages = append(allMessages, messages...)

		// If we have enough messages, return
		if len(allMessages) >= int(pc.config.MaxFetchBytes/1024) {
			break
		}
	}

	return allMessages, nil
}

// SeekPartition seeks to a specific offset in a partition
func (pc *PartitionConsumer) SeekPartition(partitionID uint32, offset int64) error {
	if atomic.LoadInt32(&pc.closed) == 1 {
		return ErrConsumerClosed
	}

	pc.mu.RLock()
	state, exists := pc.partitionStates[partitionID]
	pc.mu.RUnlock()

	if !exists {
		return fmt.Errorf("partition %d not assigned to this consumer", partitionID)
	}

	if offset < 0 && offset != -1 {
		return ErrInvalidOffset
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if offset == -1 {
		// Seek to end (latest)
		// Would need to query the broker for the high water mark
		state.offset = state.highWater
	} else {
		state.offset = offset
	}

	return nil
}

// SeekAll seeks all partitions to the same offset
func (pc *PartitionConsumer) SeekAll(offset int64) error {
	if atomic.LoadInt32(&pc.closed) == 1 {
		return ErrConsumerClosed
	}

	if offset < 0 && offset != -1 {
		return ErrInvalidOffset
	}

	for _, partitionID := range pc.partitions {
		if err := pc.SeekPartition(partitionID, offset); err != nil {
			return err
		}
	}

	return nil
}

// GetOffsets returns current offsets for all partitions
func (pc *PartitionConsumer) GetOffsets() map[uint32]int64 {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	offsets := make(map[uint32]int64)
	for pid, state := range pc.partitionStates {
		state.mu.Lock()
		offsets[pid] = state.offset
		state.mu.Unlock()
	}

	return offsets
}

// GetPartitionInfo returns information about a specific partition
func (pc *PartitionConsumer) GetPartitionInfo(partitionID uint32) (PartitionInfo, error) {
	pc.mu.RLock()
	state, exists := pc.partitionStates[partitionID]
	pc.mu.RUnlock()

	if !exists {
		return PartitionInfo{}, fmt.Errorf("partition %d not assigned to this consumer", partitionID)
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	return PartitionInfo{
		PartitionID: state.partitionID,
		Offset:      state.offset,
		HighWater:   state.highWater,
		Lag:         state.highWater - state.offset,
		LastFetch:   state.lastFetch,
	}, nil
}

// PartitionInfo contains information about a partition
type PartitionInfo struct {
	PartitionID uint32
	Offset      int64
	HighWater   int64
	Lag         int64
	LastFetch   time.Time
}

// PollPartitions polls all partitions for new messages
func (pc *PartitionConsumer) PollPartitions(ctx context.Context, interval time.Duration, handler func(uint32, []protocol.Message) error) error {
	if atomic.LoadInt32(&pc.closed) == 1 {
		return ErrConsumerClosed
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			results, err := pc.FetchAll()
			if err != nil {
				// Log error but continue polling
				continue
			}

			for partitionID, messages := range results {
				if len(messages) > 0 {
					if err := handler(partitionID, messages); err != nil {
						return err
					}
				}
			}
		}
	}
}

// Close closes the partition consumer
func (pc *PartitionConsumer) Close() error {
	if !atomic.CompareAndSwapInt32(&pc.closed, 0, 1) {
		return nil // Already closed
	}

	// Clean up resources
	pc.mu.Lock()
	pc.partitionStates = nil
	pc.mu.Unlock()

	return nil
}

// Metrics returns consumer metrics
func (pc *PartitionConsumer) Metrics() ConsumerMetrics {
	return ConsumerMetrics{
		MessagesRead: atomic.LoadInt64(&pc.messagesRead),
		BytesRead:    atomic.LoadInt64(&pc.bytesRead),
		FetchCount:   atomic.LoadInt64(&pc.fetchCount),
	}
}

// ConsumerMetrics contains consumer metrics
type ConsumerMetrics struct {
	MessagesRead int64
	BytesRead    int64
	FetchCount   int64
}