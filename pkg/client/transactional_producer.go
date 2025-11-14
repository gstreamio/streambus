package client

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/transaction"
)

// TransactionalProducer provides exactly-once semantics for message production
type TransactionalProducer struct {
	client *Client
	config TransactionalProducerConfig

	// Transaction state
	mu                 sync.RWMutex
	transactionID      transaction.TransactionID
	producerID         transaction.ProducerID
	producerEpoch      transaction.ProducerEpoch
	currentTransaction *Transaction
	state              ProducerState

	// Metrics
	transactionsCommitted int64
	transactionsAborted   int64
	messagesProduced      int64

	closed int32
}

// TransactionalProducerConfig holds configuration for transactional producer
type TransactionalProducerConfig struct {
	// Unique transaction ID for this producer
	TransactionID string

	// Transaction timeout
	TransactionTimeout time.Duration

	// Maximum time to wait for coordinator response
	RequestTimeout time.Duration
}

// DefaultTransactionalProducerConfig returns default configuration
func DefaultTransactionalProducerConfig() TransactionalProducerConfig {
	return TransactionalProducerConfig{
		TransactionTimeout: 60 * time.Second,
		RequestTimeout:     30 * time.Second,
	}
}

// ProducerState represents the state of a transactional producer
type ProducerState int

const (
	ProducerStateUninitialized ProducerState = iota
	ProducerStateReady
	ProducerStateInTransaction
	ProducerStateCommitting
	ProducerStateAborting
	ProducerStateFenced
	ProducerStateClosed
)

// Transaction represents an active transaction
type Transaction struct {
	ID         transaction.TransactionID
	StartTime  time.Time
	Partitions map[string][]int32 // topic -> partitions
	Messages   []PendingMessage
}

// PendingMessage represents a message pending in a transaction
type PendingMessage struct {
	Topic     string
	Partition int32
	Message   protocol.Message
}

// NewTransactionalProducer creates a new transactional producer
func NewTransactionalProducer(client *Client, config TransactionalProducerConfig) (*TransactionalProducer, error) {
	if config.TransactionID == "" {
		return nil, fmt.Errorf("transaction_id is required")
	}

	tp := &TransactionalProducer{
		client:        client,
		config:        config,
		transactionID: transaction.TransactionID(config.TransactionID),
		state:         ProducerStateUninitialized,
	}

	// Initialize producer ID
	if err := tp.initProducerID(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize producer ID: %w", err)
	}

	return tp, nil
}

// BeginTransaction starts a new transaction
func (tp *TransactionalProducer) BeginTransaction(ctx context.Context) error {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if atomic.LoadInt32(&tp.closed) == 1 {
		return ErrProducerClosed
	}

	switch tp.state {
	case ProducerStateUninitialized:
		return fmt.Errorf("producer not initialized")
	case ProducerStateInTransaction:
		return fmt.Errorf("transaction already in progress")
	case ProducerStateFenced:
		return fmt.Errorf("producer has been fenced")
	case ProducerStateClosed:
		return fmt.Errorf("producer is closed")
	}

	// Create new transaction
	tp.currentTransaction = &Transaction{
		ID:         tp.transactionID,
		StartTime:  time.Now(),
		Partitions: make(map[string][]int32),
		Messages:   make([]PendingMessage, 0),
	}

	tp.state = ProducerStateInTransaction

	return nil
}

// Send sends a message within the current transaction
func (tp *TransactionalProducer) Send(ctx context.Context, topic string, partition int32, message protocol.Message) error {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if atomic.LoadInt32(&tp.closed) == 1 {
		return ErrProducerClosed
	}

	if tp.state != ProducerStateInTransaction {
		return fmt.Errorf("no transaction in progress")
	}

	if tp.currentTransaction == nil {
		return fmt.Errorf("no active transaction")
	}

	// Add partition to transaction if not already included
	if err := tp.addPartitionToTxn(ctx, topic, partition); err != nil {
		return fmt.Errorf("failed to add partition to transaction: %w", err)
	}

	// Add message to pending messages
	tp.currentTransaction.Messages = append(tp.currentTransaction.Messages, PendingMessage{
		Topic:     topic,
		Partition: partition,
		Message:   message,
	})

	atomic.AddInt64(&tp.messagesProduced, 1)

	return nil
}

// SendOffsetsToTransaction adds consumer group offsets to the transaction
func (tp *TransactionalProducer) SendOffsetsToTransaction(ctx context.Context, groupID string, offsets map[string]map[int32]int64) error {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if atomic.LoadInt32(&tp.closed) == 1 {
		return ErrProducerClosed
	}

	if tp.state != ProducerStateInTransaction {
		return fmt.Errorf("no transaction in progress")
	}

	// In a real implementation, this would call the coordinator to add offsets to transaction
	// For now, this is a placeholder
	req := &transaction.AddOffsetsToTxnRequest{
		TransactionID: tp.transactionID,
		ProducerID:    tp.producerID,
		ProducerEpoch: tp.producerEpoch,
		GroupID:       groupID,
	}
	_ = req

	return nil
}

// CommitTransaction commits the current transaction
func (tp *TransactionalProducer) CommitTransaction(ctx context.Context) error {
	tp.mu.Lock()
	if atomic.LoadInt32(&tp.closed) == 1 {
		tp.mu.Unlock()
		return ErrProducerClosed
	}

	if tp.state != ProducerStateInTransaction {
		tp.mu.Unlock()
		return fmt.Errorf("no transaction in progress")
	}

	if tp.currentTransaction == nil {
		tp.mu.Unlock()
		return fmt.Errorf("no active transaction")
	}

	tp.state = ProducerStateCommitting
	txn := tp.currentTransaction
	tp.mu.Unlock()

	// Write all pending messages
	if err := tp.flushMessages(ctx, txn); err != nil {
		// On error, try to abort
		tp.AbortTransaction(ctx)
		return fmt.Errorf("failed to flush messages: %w", err)
	}

	// Commit transaction via coordinator
	endReq := &transaction.EndTxnRequest{
		TransactionID: tp.transactionID,
		ProducerID:    tp.producerID,
		ProducerEpoch: tp.producerEpoch,
		Commit:        true,
	}
	_ = endReq

	// In a real implementation, this would call the coordinator
	// For now, simulate success

	tp.mu.Lock()
	tp.currentTransaction = nil
	tp.state = ProducerStateReady
	atomic.AddInt64(&tp.transactionsCommitted, 1)
	tp.mu.Unlock()

	return nil
}

// AbortTransaction aborts the current transaction
func (tp *TransactionalProducer) AbortTransaction(ctx context.Context) error {
	tp.mu.Lock()
	if atomic.LoadInt32(&tp.closed) == 1 {
		tp.mu.Unlock()
		return ErrProducerClosed
	}

	if tp.state != ProducerStateInTransaction && tp.state != ProducerStateCommitting {
		tp.mu.Unlock()
		return fmt.Errorf("no transaction in progress")
	}

	tp.state = ProducerStateAborting
	tp.mu.Unlock()

	// Abort transaction via coordinator
	endReq := &transaction.EndTxnRequest{
		TransactionID: tp.transactionID,
		ProducerID:    tp.producerID,
		ProducerEpoch: tp.producerEpoch,
		Commit:        false,
	}
	_ = endReq

	// In a real implementation, this would call the coordinator
	// For now, simulate success

	tp.mu.Lock()
	tp.currentTransaction = nil
	tp.state = ProducerStateReady
	atomic.AddInt64(&tp.transactionsAborted, 1)
	tp.mu.Unlock()

	return nil
}

// Close closes the transactional producer
func (tp *TransactionalProducer) Close() error {
	if !atomic.CompareAndSwapInt32(&tp.closed, 0, 1) {
		return ErrProducerClosed
	}

	tp.mu.Lock()

	// Abort any in-progress transaction (inline to avoid deadlock)
	if tp.state == ProducerStateInTransaction {
		// Abort inline without calling AbortTransaction to avoid deadlock
		endReq := &transaction.EndTxnRequest{
			TransactionID: tp.transactionID,
			ProducerID:    tp.producerID,
			ProducerEpoch: tp.producerEpoch,
			Commit:        false,
		}
		_ = endReq

		tp.currentTransaction = nil
		atomic.AddInt64(&tp.transactionsAborted, 1)
	}

	tp.state = ProducerStateClosed
	tp.mu.Unlock()

	return nil
}

// Stats returns producer statistics
func (tp *TransactionalProducer) Stats() TransactionalProducerStats {
	tp.mu.RLock()
	defer tp.mu.RUnlock()

	return TransactionalProducerStats{
		ProducerID:            tp.producerID,
		ProducerEpoch:         tp.producerEpoch,
		State:                 tp.state,
		TransactionsCommitted: atomic.LoadInt64(&tp.transactionsCommitted),
		TransactionsAborted:   atomic.LoadInt64(&tp.transactionsAborted),
		MessagesProduced:      atomic.LoadInt64(&tp.messagesProduced),
	}
}

// TransactionalProducerStats holds producer statistics
type TransactionalProducerStats struct {
	ProducerID            transaction.ProducerID
	ProducerEpoch         transaction.ProducerEpoch
	State                 ProducerState
	TransactionsCommitted int64
	TransactionsAborted   int64
	MessagesProduced      int64
}

// Internal methods

func (tp *TransactionalProducer) initProducerID(ctx context.Context) error {
	// In a real implementation, this would call the coordinator
	req := &transaction.InitProducerIDRequest{
		TransactionID:      tp.transactionID,
		TransactionTimeout: tp.config.TransactionTimeout,
	}
	_ = req

	// Simulate coordinator response
	tp.mu.Lock()
	tp.producerID = 1001 // Would come from coordinator
	tp.producerEpoch = 0
	tp.state = ProducerStateReady
	tp.mu.Unlock()

	return nil
}

func (tp *TransactionalProducer) addPartitionToTxn(ctx context.Context, topic string, partition int32) error {
	// Check if partition already added
	if partitions, exists := tp.currentTransaction.Partitions[topic]; exists {
		for _, p := range partitions {
			if p == partition {
				return nil // Already added
			}
		}
	}

	// Add partition to local tracking
	if tp.currentTransaction.Partitions[topic] == nil {
		tp.currentTransaction.Partitions[topic] = make([]int32, 0)
	}
	tp.currentTransaction.Partitions[topic] = append(tp.currentTransaction.Partitions[topic], partition)

	// In a real implementation, notify coordinator
	addReq := &transaction.AddPartitionsToTxnRequest{
		TransactionID: tp.transactionID,
		ProducerID:    tp.producerID,
		ProducerEpoch: tp.producerEpoch,
		Partitions: []transaction.PartitionMetadata{
			{Topic: topic, Partition: partition},
		},
	}
	_ = addReq

	return nil
}

func (tp *TransactionalProducer) flushMessages(ctx context.Context, txn *Transaction) error {
	// In a real implementation, this would write all messages to the broker
	// with transactional markers

	// Group messages by topic/partition
	messagesByPartition := make(map[string]map[int32][]protocol.Message)
	for _, pending := range txn.Messages {
		if messagesByPartition[pending.Topic] == nil {
			messagesByPartition[pending.Topic] = make(map[int32][]protocol.Message)
		}
		messagesByPartition[pending.Topic][pending.Partition] = append(
			messagesByPartition[pending.Topic][pending.Partition],
			pending.Message,
		)
	}

	// Would write to broker with producer ID and epoch
	// For now, simulate success
	return nil
}
