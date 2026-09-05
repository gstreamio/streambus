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

// TransactionalProducer provides exactly-once semantics for message production.
//
// Messages sent inside a transaction are buffered locally and written to the
// broker only when CommitTransaction runs, so an aborted transaction never
// puts anything on a partition. Commit then asks the coordinator to end the
// transaction, which writes a marker to every participating partition before
// reporting success. SendOffsetsToTransaction folds a consumer group's
// positions into the same transaction, making a read-process-write loop
// atomic across the records produced and the positions consumed.
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

	// GroupID and Offsets hold consumer positions folded into this
	// transaction by SendOffsetsToTransaction.
	GroupID string
	Offsets map[string]map[int32]int64
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

// SendOffsetsToTransaction adds consumer group offsets to the transaction.
//
// The offsets become visible to the group only if the transaction commits,
// which is what makes a read-process-write loop atomic: either the produced
// records and the advanced consumer positions both take effect, or neither
// does.
func (tp *TransactionalProducer) SendOffsetsToTransaction(ctx context.Context, groupID string, offsets map[string]map[int32]int64) error {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if atomic.LoadInt32(&tp.closed) == 1 {
		return ErrProducerClosed
	}

	if tp.state != ProducerStateInTransaction || tp.currentTransaction == nil {
		return fmt.Errorf("no transaction in progress")
	}

	if groupID == "" {
		return fmt.Errorf("group_id is required")
	}

	// A transaction commits offsets for exactly one group; switching groups
	// mid-transaction would make the atomicity guarantee ambiguous.
	if tp.currentTransaction.GroupID != "" && tp.currentTransaction.GroupID != groupID {
		return fmt.Errorf("transaction already carries offsets for group %s",
			tp.currentTransaction.GroupID)
	}

	// Register the group with the coordinator before sending offsets, so it
	// knows the group's offset partition takes part in this transaction.
	if tp.currentTransaction.GroupID == "" {
		resp, err := tp.client.AddOffsetsToTxn(ctx, &protocol.AddOffsetsToTxnRequest{
			TransactionID: string(tp.transactionID),
			ProducerID:    int64(tp.producerID),
			ProducerEpoch: int16(tp.producerEpoch),
			GroupID:       groupID,
		})
		if err != nil {
			return fmt.Errorf("adding offsets to transaction: %w", err)
		}
		if resp.ErrorCode != protocol.ErrNone {
			return fmt.Errorf("adding offsets to transaction: %s", coordinationErrorText(resp.ErrorCode))
		}
		tp.currentTransaction.GroupID = groupID
	}

	commitResp, err := tp.client.TxnOffsetCommit(ctx, &protocol.TxnOffsetCommitRequest{
		TransactionID: string(tp.transactionID),
		GroupID:       groupID,
		ProducerID:    int64(tp.producerID),
		ProducerEpoch: int16(tp.producerEpoch),
		Topics:        offsetCommitTopics(offsets),
	})
	if err != nil {
		return fmt.Errorf("committing transactional offsets: %w", err)
	}
	if code := commitResp.FirstError(); code != protocol.ErrNone {
		return fmt.Errorf("committing transactional offsets: %s", coordinationErrorText(code))
	}

	if tp.currentTransaction.Offsets == nil {
		tp.currentTransaction.Offsets = make(map[string]map[int32]int64)
	}
	for topic, byPartition := range offsets {
		if tp.currentTransaction.Offsets[topic] == nil {
			tp.currentTransaction.Offsets[topic] = make(map[int32]int64)
		}
		for partition, offset := range byPartition {
			tp.currentTransaction.Offsets[topic][partition] = offset
		}
	}

	return nil
}

// CommitTransaction commits the current transaction.
//
// The buffered messages are written to their partitions first, then the
// coordinator ends the transaction, which writes a commit marker to every
// participating partition. The call returns success only once the coordinator
// confirms every marker is durable.
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
		// The records are not all on their partitions, so the transaction
		// cannot commit; abort so the coordinator writes abort markers for
		// whatever did land.
		_ = tp.AbortTransaction(ctx)
		return fmt.Errorf("failed to flush messages: %w", err)
	}

	resp, err := tp.client.EndTxn(ctx, &protocol.EndTxnRequest{
		TransactionID: string(tp.transactionID),
		ProducerID:    int64(tp.producerID),
		ProducerEpoch: int16(tp.producerEpoch),
		Commit:        true,
	})
	if err != nil {
		// The outcome is unknown: leave the producer in the committing state
		// rather than claiming either result, so a retry is possible and no
		// caller is told a transaction committed when it may not have.
		return fmt.Errorf("committing transaction %s: %w", tp.transactionID, err)
	}
	if resp.ErrorCode != protocol.ErrNone {
		return fmt.Errorf("committing transaction %s: %s",
			tp.transactionID, coordinationErrorText(resp.ErrorCode))
	}

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
	hasPartitions := tp.currentTransaction != nil && len(tp.currentTransaction.Partitions) > 0
	tp.mu.Unlock()

	// Buffered messages are simply dropped: nothing was written for a
	// transaction that never reached commit. The coordinator still needs to
	// hear about the abort if any partition was registered with it.
	var abortErr error
	if hasPartitions {
		resp, err := tp.client.EndTxn(ctx, &protocol.EndTxnRequest{
			TransactionID: string(tp.transactionID),
			ProducerID:    int64(tp.producerID),
			ProducerEpoch: int16(tp.producerEpoch),
			Commit:        false,
		})
		switch {
		case err != nil:
			abortErr = fmt.Errorf("aborting transaction %s: %w", tp.transactionID, err)
		case resp.ErrorCode != protocol.ErrNone:
			abortErr = fmt.Errorf("aborting transaction %s: %s",
				tp.transactionID, coordinationErrorText(resp.ErrorCode))
		}
	}

	// The local transaction is discarded either way: its messages were never
	// written, so leaving the producer stuck would strand it over a
	// bookkeeping call. The error is still reported.
	tp.mu.Lock()
	tp.currentTransaction = nil
	tp.state = ProducerStateReady
	atomic.AddInt64(&tp.transactionsAborted, 1)
	tp.mu.Unlock()

	return abortErr
}

// Close closes the transactional producer
func (tp *TransactionalProducer) Close() error {
	if !atomic.CompareAndSwapInt32(&tp.closed, 0, 1) {
		return ErrProducerClosed
	}

	tp.mu.Lock()
	inTransaction := tp.state == ProducerStateInTransaction
	hasPartitions := tp.currentTransaction != nil && len(tp.currentTransaction.Partitions) > 0
	transactionID := tp.transactionID
	producerID := tp.producerID
	producerEpoch := tp.producerEpoch
	tp.currentTransaction = nil
	tp.state = ProducerStateClosed
	if inTransaction {
		atomic.AddInt64(&tp.transactionsAborted, 1)
	}
	tp.mu.Unlock()

	// Tell the coordinator to abort anything still open, so a closed
	// producer does not leave a transaction hanging until it times out.
	// The lock is released first: EndTxn is a network call.
	if inTransaction && hasPartitions {
		ctx, cancel := context.WithTimeout(context.Background(), tp.config.RequestTimeout)
		defer cancel()

		if _, err := tp.client.EndTxn(ctx, &protocol.EndTxnRequest{
			TransactionID: string(transactionID),
			ProducerID:    int64(producerID),
			ProducerEpoch: int16(producerEpoch),
			Commit:        false,
		}); err != nil {
			return fmt.Errorf("aborting transaction %s during close: %w", transactionID, err)
		}
	}

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

// initProducerID claims a producer identity from the coordinator. Reclaiming
// an existing transactional ID bumps the epoch, which fences any older
// producer instance still running under the same ID.
func (tp *TransactionalProducer) initProducerID(ctx context.Context) error {
	resp, err := tp.client.InitProducerID(ctx, &protocol.InitProducerIDRequest{
		TransactionID:        string(tp.transactionID),
		TransactionTimeoutMs: int32(tp.config.TransactionTimeout.Milliseconds()),
	})
	if err != nil {
		return err
	}
	if resp.ErrorCode != protocol.ErrNone {
		return fmt.Errorf("initializing producer for %s: %s",
			tp.transactionID, coordinationErrorText(resp.ErrorCode))
	}

	tp.mu.Lock()
	tp.producerID = transaction.ProducerID(resp.ProducerID)
	tp.producerEpoch = transaction.ProducerEpoch(resp.ProducerEpoch)
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

	// Register with the coordinator before recording it locally: the
	// coordinator must know about a partition before anything is written to
	// it, otherwise EndTxn would not write that partition a marker.
	resp, err := tp.client.AddPartitionsToTxn(ctx, &protocol.AddPartitionsToTxnRequest{
		TransactionID: string(tp.transactionID),
		ProducerID:    int64(tp.producerID),
		ProducerEpoch: int16(tp.producerEpoch),
		Partitions:    []protocol.TxnPartition{{Topic: topic, Partition: partition}},
	})
	if err != nil {
		return err
	}
	if code := resp.FirstError(); code != protocol.ErrNone {
		return fmt.Errorf("registering %s-%d: %s", topic, partition, coordinationErrorText(code))
	}

	if tp.currentTransaction.Partitions[topic] == nil {
		tp.currentTransaction.Partitions[topic] = make([]int32, 0)
	}
	tp.currentTransaction.Partitions[topic] = append(tp.currentTransaction.Partitions[topic], partition)

	return nil
}

// flushMessages writes a transaction's buffered messages to their partitions,
// grouped so each topic-partition takes a single produce request.
//
// Messages are held until commit rather than streamed as they are sent: an
// aborted transaction then leaves nothing behind on any partition, so a reader
// never has to filter out records from a transaction that did not happen.
func (tp *TransactionalProducer) flushMessages(ctx context.Context, txn *Transaction) error {
	if len(txn.Messages) == 0 {
		return nil
	}

	type partitionKey struct {
		topic     string
		partition int32
	}

	// Preserve send order within a partition, and produce partitions in a
	// stable order so a failure is reproducible.
	order := make([]partitionKey, 0)
	batches := make(map[partitionKey][]protocol.Message)
	for _, pending := range txn.Messages {
		key := partitionKey{topic: pending.Topic, partition: pending.Partition}
		if _, seen := batches[key]; !seen {
			order = append(order, key)
		}
		batches[key] = append(batches[key], pending.Message)
	}

	producer := NewProducer(tp.client)
	defer func() { _ = producer.Close() }()

	for _, key := range order {
		if key.partition < 0 {
			return fmt.Errorf("invalid partition %d for topic %s", key.partition, key.topic)
		}
		//nolint:gosec // partition is checked non-negative above
		if err := producer.SendMessagesToPartition(ctx, key.topic, uint32(key.partition), batches[key]); err != nil {
			return fmt.Errorf("writing %s-%d: %w", key.topic, key.partition, err)
		}
	}

	if err := producer.FlushAll(ctx); err != nil {
		return fmt.Errorf("flushing transactional messages: %w", err)
	}

	return nil
}
