package client

import (
	"context"
	"errors"
	"testing"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionalProducer_Create(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultTransactionalProducerConfig()
	config.TransactionID = "test-txn-1"

	producer, err := NewTransactionalProducer(client, config)
	require.NoError(t, err)
	assert.NotNil(t, producer)
	assert.Equal(t, ProducerStateReady, producer.state)
	assert.NotZero(t, producer.producerID)
	assert.Equal(t, int16(0), int16(producer.producerEpoch))
}

func TestTransactionalProducer_CreateValidation(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	// Test missing transaction ID
	config := DefaultTransactionalProducerConfig()
	_, err := NewTransactionalProducer(client, config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction_id")
}

func TestTransactionalProducer_BeginTransaction(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultTransactionalProducerConfig()
	config.TransactionID = "test-txn-1"

	producer, err := NewTransactionalProducer(client, config)
	require.NoError(t, err)

	ctx := context.Background()

	// Begin transaction
	err = producer.BeginTransaction(ctx)
	require.NoError(t, err)
	assert.Equal(t, ProducerStateInTransaction, producer.state)
	assert.NotNil(t, producer.currentTransaction)

	// Try to begin another transaction (should fail)
	err = producer.BeginTransaction(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already in progress")
}

func TestTransactionalProducer_SendMessages(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultTransactionalProducerConfig()
	config.TransactionID = "test-txn-1"

	producer, err := NewTransactionalProducer(client, config)
	require.NoError(t, err)

	ctx := context.Background()

	// Begin transaction
	err = producer.BeginTransaction(ctx)
	require.NoError(t, err)

	// Send messages
	msg1 := protocol.Message{
		Key:   []byte("key-1"),
		Value: []byte("value-1"),
	}

	err = producer.Send(ctx, "test-topic", 0, msg1)
	require.NoError(t, err)

	msg2 := protocol.Message{
		Key:   []byte("key-2"),
		Value: []byte("value-2"),
	}

	err = producer.Send(ctx, "test-topic", 1, msg2)
	require.NoError(t, err)

	// Verify messages are tracked
	assert.Len(t, producer.currentTransaction.Messages, 2)
	assert.Contains(t, producer.currentTransaction.Partitions, "test-topic")
	assert.Len(t, producer.currentTransaction.Partitions["test-topic"], 2)
}

func TestTransactionalProducer_CommitTransaction(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultTransactionalProducerConfig()
	config.TransactionID = "test-txn-1"

	producer, err := NewTransactionalProducer(client, config)
	require.NoError(t, err)

	ctx := context.Background()

	// Begin transaction
	err = producer.BeginTransaction(ctx)
	require.NoError(t, err)

	// Send message
	msg := protocol.Message{
		Key:   []byte("key-1"),
		Value: []byte("value-1"),
	}

	err = producer.Send(ctx, "test-topic", 0, msg)
	require.NoError(t, err)

	// Commit transaction: must fail rather than silently discarding the
	// message, since there's no coordinator to durably flush it to.
	err = producer.CommitTransaction(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTransactionCoordinationNotImplemented))

	// A failed commit aborts the transaction rather than leaving it stuck.
	assert.Equal(t, ProducerStateReady, producer.state)
	assert.Nil(t, producer.currentTransaction)

	// Verify stats: commit did not happen, but the abort triggered by the
	// failed flush is tracked.
	stats := producer.Stats()
	assert.Equal(t, int64(0), stats.TransactionsCommitted)
	assert.Equal(t, int64(1), stats.TransactionsAborted)
	assert.Equal(t, int64(1), stats.MessagesProduced)
}

func TestTransactionalProducer_AbortTransaction(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultTransactionalProducerConfig()
	config.TransactionID = "test-txn-1"

	producer, err := NewTransactionalProducer(client, config)
	require.NoError(t, err)

	ctx := context.Background()

	// Begin transaction
	err = producer.BeginTransaction(ctx)
	require.NoError(t, err)

	// Send message
	msg := protocol.Message{
		Key:   []byte("key-1"),
		Value: []byte("value-1"),
	}

	err = producer.Send(ctx, "test-topic", 0, msg)
	require.NoError(t, err)

	// Abort transaction
	err = producer.AbortTransaction(ctx)
	require.NoError(t, err)

	// Verify state
	assert.Equal(t, ProducerStateReady, producer.state)
	assert.Nil(t, producer.currentTransaction)

	// Verify stats
	stats := producer.Stats()
	assert.Equal(t, int64(0), stats.TransactionsCommitted)
	assert.Equal(t, int64(1), stats.TransactionsAborted)
	assert.Equal(t, int64(1), stats.MessagesProduced)
}

func TestTransactionalProducer_MultipleTransactions(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultTransactionalProducerConfig()
	config.TransactionID = "test-txn-1"

	producer, err := NewTransactionalProducer(client, config)
	require.NoError(t, err)

	ctx := context.Background()

	// Transaction 1 - attempted commit (fails: not implemented, auto-aborts)
	err = producer.BeginTransaction(ctx)
	require.NoError(t, err)

	msg1 := protocol.Message{
		Key:   []byte("key-1"),
		Value: []byte("value-1"),
	}

	err = producer.Send(ctx, "test-topic", 0, msg1)
	require.NoError(t, err)

	err = producer.CommitTransaction(ctx)
	require.Error(t, err)

	// Transaction 2 - Abort
	err = producer.BeginTransaction(ctx)
	require.NoError(t, err)

	msg2 := protocol.Message{
		Key:   []byte("key-2"),
		Value: []byte("value-2"),
	}

	err = producer.Send(ctx, "test-topic", 0, msg2)
	require.NoError(t, err)

	err = producer.AbortTransaction(ctx)
	require.NoError(t, err)

	// Transaction 3 - attempted commit (fails: not implemented, auto-aborts)
	err = producer.BeginTransaction(ctx)
	require.NoError(t, err)

	msg3 := protocol.Message{
		Key:   []byte("key-3"),
		Value: []byte("value-3"),
	}

	err = producer.Send(ctx, "test-topic", 0, msg3)
	require.NoError(t, err)

	err = producer.CommitTransaction(ctx)
	require.Error(t, err)

	// Verify stats: no transaction can actually commit yet, so all three
	// end up counted as aborts (the failed-commit auto-abort included).
	stats := producer.Stats()
	assert.Equal(t, int64(0), stats.TransactionsCommitted)
	assert.Equal(t, int64(3), stats.TransactionsAborted)
	assert.Equal(t, int64(3), stats.MessagesProduced)
}

func TestTransactionalProducer_SendWithoutTransaction(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultTransactionalProducerConfig()
	config.TransactionID = "test-txn-1"

	producer, err := NewTransactionalProducer(client, config)
	require.NoError(t, err)

	ctx := context.Background()

	// Try to send without beginning transaction
	msg := protocol.Message{
		Key:   []byte("key-1"),
		Value: []byte("value-1"),
	}

	err = producer.Send(ctx, "test-topic", 0, msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no transaction in progress")
}

func TestTransactionalProducer_SendOffsetsToTransaction(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultTransactionalProducerConfig()
	config.TransactionID = "test-txn-1"

	producer, err := NewTransactionalProducer(client, config)
	require.NoError(t, err)

	ctx := context.Background()

	// Begin transaction
	err = producer.BeginTransaction(ctx)
	require.NoError(t, err)

	// Send some messages
	msg := protocol.Message{
		Key:   []byte("key-1"),
		Value: []byte("value-1"),
	}

	err = producer.Send(ctx, "test-topic", 0, msg)
	require.NoError(t, err)

	// Send offsets
	offsets := map[string]map[int32]int64{
		"input-topic": {
			0: 100,
			1: 200,
		},
	}

	err = producer.SendOffsetsToTransaction(ctx, "consumer-group-1", offsets)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTransactionCoordinationNotImplemented))

	// Commit also fails: not implemented yet.
	err = producer.CommitTransaction(ctx)
	require.Error(t, err)
}

func TestTransactionalProducer_Close(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultTransactionalProducerConfig()
	config.TransactionID = "test-txn-1"

	producer, err := NewTransactionalProducer(client, config)
	require.NoError(t, err)

	ctx := context.Background()

	// Begin transaction
	err = producer.BeginTransaction(ctx)
	require.NoError(t, err)

	// Send message
	msg := protocol.Message{
		Key:   []byte("key-1"),
		Value: []byte("value-1"),
	}

	err = producer.Send(ctx, "test-topic", 0, msg)
	require.NoError(t, err)

	// Close (should abort in-progress transaction)
	err = producer.Close()
	require.NoError(t, err)

	// Verify state
	assert.Equal(t, ProducerStateClosed, producer.state)

	// Try operations after close
	err = producer.BeginTransaction(ctx)
	assert.Error(t, err)

	err = producer.Send(ctx, "test-topic", 0, msg)
	assert.Error(t, err)

	// Second close should return error
	err = producer.Close()
	assert.Equal(t, ErrProducerClosed, err)
}

func TestTransactionalProducer_Stats(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultTransactionalProducerConfig()
	config.TransactionID = "test-txn-1"

	producer, err := NewTransactionalProducer(client, config)
	require.NoError(t, err)

	// Initial stats
	stats := producer.Stats()
	assert.NotZero(t, stats.ProducerID)
	assert.Equal(t, int16(0), int16(stats.ProducerEpoch))
	assert.Equal(t, ProducerStateReady, stats.State)
	assert.Equal(t, int64(0), stats.TransactionsCommitted)
	assert.Equal(t, int64(0), stats.TransactionsAborted)
	assert.Equal(t, int64(0), stats.MessagesProduced)

	ctx := context.Background()

	// Do some transactions. Commit isn't implemented yet, so a "commit"
	// attempt fails and auto-aborts; only the explicit abort path succeeds
	// directly.
	for i := range 3 {
		err = producer.BeginTransaction(ctx)
		require.NoError(t, err)

		msg := protocol.Message{
			Key:   []byte("key"),
			Value: []byte("value"),
		}

		err = producer.Send(ctx, "test-topic", 0, msg)
		require.NoError(t, err)

		if i%2 == 0 {
			err = producer.CommitTransaction(ctx)
			require.Error(t, err)
		} else {
			err = producer.AbortTransaction(ctx)
			require.NoError(t, err)
		}
	}

	// Final stats: every transaction ends up aborted (two failed commits
	// that auto-abort, plus one explicit abort).
	stats = producer.Stats()
	assert.Equal(t, int64(0), stats.TransactionsCommitted)
	assert.Equal(t, int64(3), stats.TransactionsAborted)
	assert.Equal(t, int64(3), stats.MessagesProduced)
}
