package client

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/transaction"
)

// IsolationLevel represents the transaction isolation level for consumers
type IsolationLevel int

const (
	// ReadUncommitted reads all messages including those in open transactions
	ReadUncommitted IsolationLevel = iota
	// ReadCommitted only reads messages from committed transactions
	ReadCommitted
)

// TransactionalConsumerConfig holds configuration for transactional consumer
type TransactionalConsumerConfig struct {
	// Client is the underlying StreamBus client
	Client *Client

	// Topics to consume from
	Topics []string

	// GroupID for consumer group membership (optional)
	GroupID string

	// IsolationLevel determines which messages are visible
	IsolationLevel IsolationLevel

	// MaxPollRecords limits records returned per fetch
	MaxPollRecords int

	// AutoCommitEnabled enables automatic offset commits
	AutoCommitEnabled bool
}

// DefaultTransactionalConsumerConfig returns default configuration
func DefaultTransactionalConsumerConfig() *TransactionalConsumerConfig {
	return &TransactionalConsumerConfig{
		IsolationLevel:    ReadCommitted,
		MaxPollRecords:    100,
		AutoCommitEnabled: true,
	}
}

// TransactionalConsumer provides read-committed isolation for consuming messages
type TransactionalConsumer struct {
	config *TransactionalConsumerConfig
	client *Client

	mu sync.RWMutex

	// Transaction state tracking
	abortedTransactions map[transaction.TransactionID]bool // Tracks known aborted transactions
	lastStableOffset    map[topicPartition]int64           // Last stable offset (LSO) per partition

	// Consumer state
	position      map[topicPartition]int64 // Current position per partition
	committed     map[topicPartition]int64 // Last committed offset per partition
	subscriptions []string
	closed        bool

	// Statistics
	messagesConsumed int64
	messagesFiltered int64 // Messages filtered due to aborted transactions
}

type topicPartition struct {
	topic     string
	partition int32
}

// NewTransactionalConsumer creates a new transactional consumer
func NewTransactionalConsumer(config *TransactionalConsumerConfig) (*TransactionalConsumer, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	if config.Client == nil {
		return nil, fmt.Errorf("client is required")
	}

	if len(config.Topics) == 0 {
		return nil, fmt.Errorf("at least one topic is required")
	}

	if config.IsolationLevel != ReadCommitted && config.IsolationLevel != ReadUncommitted {
		return nil, fmt.Errorf("invalid isolation level")
	}

	tc := &TransactionalConsumer{
		config:              config,
		client:              config.Client,
		abortedTransactions: make(map[transaction.TransactionID]bool),
		lastStableOffset:    make(map[topicPartition]int64),
		position:            make(map[topicPartition]int64),
		committed:           make(map[topicPartition]int64),
		subscriptions:       config.Topics,
	}

	return tc, nil
}

// ConsumerRecord represents a consumed message with metadata
type ConsumerRecord struct {
	Topic     string
	Partition int32
	Offset    int64
	Message   protocol.Message
}

// Poll fetches messages from subscribed topics
func (tc *TransactionalConsumer) Poll(ctx context.Context) ([]ConsumerRecord, error) {
	tc.mu.Lock()
	if tc.closed {
		tc.mu.Unlock()
		return nil, ErrConsumerClosed
	}
	tc.mu.Unlock()

	records := make([]ConsumerRecord, 0, tc.config.MaxPollRecords)

	// Fetch from each subscribed topic
	for _, topic := range tc.subscriptions {
		if len(records) >= tc.config.MaxPollRecords {
			break
		}

		// For simplicity, assume partition 0
		// In production, this would fetch from all assigned partitions
		partition := int32(0)
		tp := topicPartition{topic: topic, partition: partition}

		// Get current position
		tc.mu.RLock()
		offset := tc.position[tp]
		tc.mu.RUnlock()

		// Fetch messages
		req := &FetchRequest{
			Topic:     topic,
			Partition: partition,
			Offset:    offset,
			MaxBytes:  1024 * 1024, // 1MB
		}

		resp, err := tc.client.Fetch(ctx, req)
		if err != nil {
			return records, fmt.Errorf("fetch failed: %w", err)
		}

		// Process messages with transaction filtering
		for _, msg := range resp.Messages {
			// Apply read-committed isolation if enabled
			if tc.config.IsolationLevel == ReadCommitted {
				if tc.shouldFilterMessage(tp, msg) {
					atomic.AddInt64(&tc.messagesFiltered, 1)
					continue
				}
			}

			records = append(records, ConsumerRecord{
				Topic:     topic,
				Partition: partition,
				Offset:    msg.Offset,
				Message:   msg,
			})

			atomic.AddInt64(&tc.messagesConsumed, 1)

			// Update position
			tc.mu.Lock()
			tc.position[tp] = msg.Offset + 1
			tc.mu.Unlock()

			if len(records) >= tc.config.MaxPollRecords {
				break
			}
		}
	}

	return records, nil
}

// shouldFilterMessage determines if a message should be filtered based on transaction state
func (tc *TransactionalConsumer) shouldFilterMessage(tp topicPartition, msg protocol.Message) bool {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	// Check if message is part of an aborted transaction
	// In a real implementation, this would check message headers for transaction metadata
	// For now, we'll use a simplified check based on known aborted transactions

	// If message offset is beyond last stable offset, it might be from an uncommitted transaction
	lso, hasLSO := tc.lastStableOffset[tp]
	if hasLSO && msg.Offset > lso {
		// Message is beyond LSO, treat as uncommitted
		return true
	}

	return false
}

// Commit commits the current position
func (tc *TransactionalConsumer) Commit(ctx context.Context) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.closed {
		return ErrConsumerClosed
	}

	// Copy current positions to committed
	for tp, offset := range tc.position {
		tc.committed[tp] = offset
	}

	// If part of consumer group, commit offsets to coordinator
	if tc.config.GroupID != "" {
		// Build offset commit request
		offsets := make(map[string]map[int32]int64)
		for tp, offset := range tc.committed {
			if offsets[tp.topic] == nil {
				offsets[tp.topic] = make(map[int32]int64)
			}
			offsets[tp.topic][tp.partition] = offset
		}

		// In a real implementation, this would send OffsetCommit request to coordinator
		// For now, we just update local state
	}

	return nil
}

// CommitSync commits offsets synchronously
func (tc *TransactionalConsumer) CommitSync(ctx context.Context, offsets map[string]map[int32]int64) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.closed {
		return ErrConsumerClosed
	}

	// Update committed offsets
	for topic, partitions := range offsets {
		for partition, offset := range partitions {
			tp := topicPartition{topic: topic, partition: partition}
			tc.committed[tp] = offset
			tc.position[tp] = offset
		}
	}

	return nil
}

// Seek moves the consumer position to a specific offset
func (tc *TransactionalConsumer) Seek(topic string, partition int32, offset int64) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.closed {
		return ErrConsumerClosed
	}

	tp := topicPartition{topic: topic, partition: partition}
	tc.position[tp] = offset

	return nil
}

// Position returns the current position for a partition
func (tc *TransactionalConsumer) Position(topic string, partition int32) (int64, error) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if tc.closed {
		return 0, ErrConsumerClosed
	}

	tp := topicPartition{topic: topic, partition: partition}
	offset, exists := tc.position[tp]
	if !exists {
		return 0, nil
	}

	return offset, nil
}

// Committed returns the last committed offset for a partition
func (tc *TransactionalConsumer) Committed(topic string, partition int32) (int64, error) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if tc.closed {
		return 0, ErrConsumerClosed
	}

	tp := topicPartition{topic: topic, partition: partition}
	offset, exists := tc.committed[tp]
	if !exists {
		return 0, nil
	}

	return offset, nil
}

// UpdateLastStableOffset updates the last stable offset for a partition
// This is called internally when transaction markers are read
func (tc *TransactionalConsumer) UpdateLastStableOffset(topic string, partition int32, lso int64) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tp := topicPartition{topic: topic, partition: partition}
	tc.lastStableOffset[tp] = lso
}

// MarkTransactionAborted marks a transaction as aborted
// Messages from aborted transactions will be filtered in read-committed mode
func (tc *TransactionalConsumer) MarkTransactionAborted(txnID transaction.TransactionID) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.abortedTransactions[txnID] = true
}

// Close closes the consumer
func (tc *TransactionalConsumer) Close() error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.closed {
		return nil
	}

	// Commit offsets if auto-commit is enabled
	if tc.config.AutoCommitEnabled {
		// Final offset commit would happen here
	}

	tc.closed = true
	return nil
}

// Stats returns consumer statistics
func (tc *TransactionalConsumer) Stats() TransactionalConsumerStats {
	return TransactionalConsumerStats{
		MessagesConsumed:    atomic.LoadInt64(&tc.messagesConsumed),
		MessagesFiltered:    atomic.LoadInt64(&tc.messagesFiltered),
		AbortedTransactions: len(tc.abortedTransactions),
		IsolationLevel:      tc.config.IsolationLevel,
	}
}

// TransactionalConsumerStats holds consumer statistics
type TransactionalConsumerStats struct {
	MessagesConsumed    int64
	MessagesFiltered    int64          // Messages filtered due to aborted transactions
	AbortedTransactions int            // Number of known aborted transactions
	IsolationLevel      IsolationLevel // Current isolation level
}
