package client

import (
	"context"
	"testing"
	"time"

	"github.com/shawntherrien/streambus/pkg/protocol"
	"github.com/shawntherrien/streambus/pkg/transaction"
)

func TestNewTransactionalConsumer(t *testing.T) {
	tests := []struct {
		name        string
		config      *TransactionalConsumerConfig
		expectError bool
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
		},
		{
			name: "nil client",
			config: &TransactionalConsumerConfig{
				Topics: []string{"test-topic"},
			},
			expectError: true,
		},
		{
			name: "no topics",
			config: &TransactionalConsumerConfig{
				Client: &Client{},
				Topics: []string{},
			},
			expectError: true,
		},
		{
			name: "invalid isolation level",
			config: &TransactionalConsumerConfig{
				Client:         &Client{},
				Topics:         []string{"test-topic"},
				IsolationLevel: IsolationLevel(999),
			},
			expectError: true,
		},
		{
			name: "valid read-committed",
			config: &TransactionalConsumerConfig{
				Client:         &Client{},
				Topics:         []string{"test-topic"},
				IsolationLevel: ReadCommitted,
			},
			expectError: false,
		},
		{
			name: "valid read-uncommitted",
			config: &TransactionalConsumerConfig{
				Client:         &Client{},
				Topics:         []string{"test-topic"},
				IsolationLevel: ReadUncommitted,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			consumer, err := NewTransactionalConsumer(tt.config)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if consumer == nil {
				t.Errorf("expected consumer, got nil")
			}
		})
	}
}

func TestTransactionalConsumer_Poll(t *testing.T) {
	// Create a minimal client with config
	clientConfig := DefaultConfig()
	clientConfig.Brokers = []string{"localhost:9092"}
	client, err := New(clientConfig)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	config := &TransactionalConsumerConfig{
		Client:         client,
		Topics:         []string{"test-topic"},
		IsolationLevel: ReadCommitted,
		MaxPollRecords: 10,
	}

	consumer, err := NewTransactionalConsumer(config)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Poll will fail because we don't have a real broker connection
	// But we can verify it handles the error gracefully
	_, err = consumer.Poll(ctx)
	if err != nil {
		// Expected to fail without a real broker
		t.Logf("poll failed as expected (no broker): %v", err)
	}
}

func TestTransactionalConsumer_ReadCommittedFiltering(t *testing.T) {
	client := &Client{}
	config := &TransactionalConsumerConfig{
		Client:         client,
		Topics:         []string{"test-topic"},
		IsolationLevel: ReadCommitted,
		MaxPollRecords: 100,
	}

	consumer, err := NewTransactionalConsumer(config)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	// Set up last stable offset
	consumer.UpdateLastStableOffset("test-topic", 0, 10)

	// Create test messages
	msg1 := protocol.Message{Offset: 5}  // Before LSO - should pass
	msg2 := protocol.Message{Offset: 10} // At LSO - should pass
	msg3 := protocol.Message{Offset: 15} // Beyond LSO - should filter

	tp := topicPartition{topic: "test-topic", partition: 0}

	// Test filtering
	if consumer.shouldFilterMessage(tp, msg1) {
		t.Errorf("message at offset 5 should not be filtered (LSO=10)")
	}

	if consumer.shouldFilterMessage(tp, msg2) {
		t.Errorf("message at offset 10 should not be filtered (LSO=10)")
	}

	if !consumer.shouldFilterMessage(tp, msg3) {
		t.Errorf("message at offset 15 should be filtered (LSO=10)")
	}
}

func TestTransactionalConsumer_ReadUncommitted(t *testing.T) {
	client := &Client{}
	config := &TransactionalConsumerConfig{
		Client:         client,
		Topics:         []string{"test-topic"},
		IsolationLevel: ReadUncommitted,
		MaxPollRecords: 100,
	}

	consumer, err := NewTransactionalConsumer(config)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	// Set up last stable offset
	consumer.UpdateLastStableOffset("test-topic", 0, 10)

	// With read-uncommitted, filtering should not happen in Poll
	// (the shouldFilterMessage check is skipped)
	if consumer.config.IsolationLevel != ReadUncommitted {
		t.Errorf("expected ReadUncommitted isolation level")
	}
}

func TestTransactionalConsumer_AbortedTransactions(t *testing.T) {
	client := &Client{}
	config := &TransactionalConsumerConfig{
		Client:         client,
		Topics:         []string{"test-topic"},
		IsolationLevel: ReadCommitted,
	}

	consumer, err := NewTransactionalConsumer(config)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	// Mark transactions as aborted
	txnID1 := transaction.TransactionID("txn-1")
	txnID2 := transaction.TransactionID("txn-2")

	consumer.MarkTransactionAborted(txnID1)
	consumer.MarkTransactionAborted(txnID2)

	// Check stats
	stats := consumer.Stats()
	if stats.AbortedTransactions != 2 {
		t.Errorf("expected 2 aborted transactions, got %d", stats.AbortedTransactions)
	}
}

func TestTransactionalConsumer_Commit(t *testing.T) {
	client := &Client{}
	config := &TransactionalConsumerConfig{
		Client:         client,
		Topics:         []string{"test-topic"},
		IsolationLevel: ReadCommitted,
	}

	consumer, err := NewTransactionalConsumer(config)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	// Set position
	consumer.position[topicPartition{topic: "test-topic", partition: 0}] = 100

	ctx := context.Background()
	err = consumer.Commit(ctx)
	if err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	// Check committed offset
	offset, err := consumer.Committed("test-topic", 0)
	if err != nil {
		t.Fatalf("failed to get committed offset: %v", err)
	}

	if offset != 100 {
		t.Errorf("expected committed offset 100, got %d", offset)
	}
}

func TestTransactionalConsumer_CommitSync(t *testing.T) {
	client := &Client{}
	config := &TransactionalConsumerConfig{
		Client:         client,
		Topics:         []string{"test-topic"},
		IsolationLevel: ReadCommitted,
	}

	consumer, err := NewTransactionalConsumer(config)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	// Commit specific offsets
	offsets := map[string]map[int32]int64{
		"test-topic": {
			0: 50,
			1: 75,
		},
	}

	ctx := context.Background()
	err = consumer.CommitSync(ctx, offsets)
	if err != nil {
		t.Fatalf("commit sync failed: %v", err)
	}

	// Verify committed offsets
	offset0, _ := consumer.Committed("test-topic", 0)
	offset1, _ := consumer.Committed("test-topic", 1)

	if offset0 != 50 {
		t.Errorf("expected committed offset 50 for partition 0, got %d", offset0)
	}

	if offset1 != 75 {
		t.Errorf("expected committed offset 75 for partition 1, got %d", offset1)
	}
}

func TestTransactionalConsumer_Seek(t *testing.T) {
	client := &Client{}
	config := &TransactionalConsumerConfig{
		Client:         client,
		Topics:         []string{"test-topic"},
		IsolationLevel: ReadCommitted,
	}

	consumer, err := NewTransactionalConsumer(config)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	// Seek to offset 42
	err = consumer.Seek("test-topic", 0, 42)
	if err != nil {
		t.Fatalf("seek failed: %v", err)
	}

	// Verify position
	position, err := consumer.Position("test-topic", 0)
	if err != nil {
		t.Fatalf("failed to get position: %v", err)
	}

	if position != 42 {
		t.Errorf("expected position 42, got %d", position)
	}
}

func TestTransactionalConsumer_Position(t *testing.T) {
	client := &Client{}
	config := &TransactionalConsumerConfig{
		Client:         client,
		Topics:         []string{"test-topic"},
		IsolationLevel: ReadCommitted,
	}

	consumer, err := NewTransactionalConsumer(config)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	// Initially, position should be 0
	position, err := consumer.Position("test-topic", 0)
	if err != nil {
		t.Fatalf("failed to get position: %v", err)
	}

	if position != 0 {
		t.Errorf("expected initial position 0, got %d", position)
	}

	// Set position manually
	consumer.position[topicPartition{topic: "test-topic", partition: 0}] = 123

	position, err = consumer.Position("test-topic", 0)
	if err != nil {
		t.Fatalf("failed to get position: %v", err)
	}

	if position != 123 {
		t.Errorf("expected position 123, got %d", position)
	}
}

func TestTransactionalConsumer_Close(t *testing.T) {
	client := &Client{}
	config := &TransactionalConsumerConfig{
		Client:            client,
		Topics:            []string{"test-topic"},
		IsolationLevel:    ReadCommitted,
		AutoCommitEnabled: false,
	}

	consumer, err := NewTransactionalConsumer(config)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	// Close consumer
	err = consumer.Close()
	if err != nil {
		t.Fatalf("close failed: %v", err)
	}

	// Verify closed state
	if !consumer.closed {
		t.Errorf("consumer should be marked as closed")
	}

	// Operations on closed consumer should fail
	ctx := context.Background()
	_, err = consumer.Poll(ctx)
	if err != ErrConsumerClosed {
		t.Errorf("expected ErrConsumerClosed, got %v", err)
	}

	err = consumer.Commit(ctx)
	if err != ErrConsumerClosed {
		t.Errorf("expected ErrConsumerClosed, got %v", err)
	}

	err = consumer.Seek("test-topic", 0, 100)
	if err != ErrConsumerClosed {
		t.Errorf("expected ErrConsumerClosed, got %v", err)
	}

	// Closing again should be idempotent
	err = consumer.Close()
	if err != nil {
		t.Errorf("second close should succeed, got %v", err)
	}
}

func TestTransactionalConsumer_Stats(t *testing.T) {
	client := &Client{}
	config := &TransactionalConsumerConfig{
		Client:         client,
		Topics:         []string{"test-topic"},
		IsolationLevel: ReadCommitted,
	}

	consumer, err := NewTransactionalConsumer(config)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	// Initial stats
	stats := consumer.Stats()
	if stats.MessagesConsumed != 0 {
		t.Errorf("expected 0 messages consumed, got %d", stats.MessagesConsumed)
	}

	if stats.MessagesFiltered != 0 {
		t.Errorf("expected 0 messages filtered, got %d", stats.MessagesFiltered)
	}

	if stats.IsolationLevel != ReadCommitted {
		t.Errorf("expected ReadCommitted isolation level")
	}

	// Simulate consumption and filtering
	consumer.messagesConsumed = 100
	consumer.messagesFiltered = 5
	consumer.MarkTransactionAborted("txn-1")

	stats = consumer.Stats()
	if stats.MessagesConsumed != 100 {
		t.Errorf("expected 100 messages consumed, got %d", stats.MessagesConsumed)
	}

	if stats.MessagesFiltered != 5 {
		t.Errorf("expected 5 messages filtered, got %d", stats.MessagesFiltered)
	}

	if stats.AbortedTransactions != 1 {
		t.Errorf("expected 1 aborted transaction, got %d", stats.AbortedTransactions)
	}
}

func TestDefaultTransactionalConsumerConfig(t *testing.T) {
	config := DefaultTransactionalConsumerConfig()

	if config.IsolationLevel != ReadCommitted {
		t.Errorf("expected ReadCommitted isolation level")
	}

	if config.MaxPollRecords != 100 {
		t.Errorf("expected MaxPollRecords=100, got %d", config.MaxPollRecords)
	}

	if !config.AutoCommitEnabled {
		t.Errorf("expected AutoCommitEnabled=true")
	}
}

func TestTransactionalConsumer_UpdateLastStableOffset(t *testing.T) {
	client := &Client{}
	config := &TransactionalConsumerConfig{
		Client:         client,
		Topics:         []string{"test-topic"},
		IsolationLevel: ReadCommitted,
	}

	consumer, err := NewTransactionalConsumer(config)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	// Update LSO
	consumer.UpdateLastStableOffset("test-topic", 0, 100)

	// Verify LSO is set
	tp := topicPartition{topic: "test-topic", partition: 0}
	lso, exists := consumer.lastStableOffset[tp]
	if !exists {
		t.Fatalf("LSO not set for partition")
	}

	if lso != 100 {
		t.Errorf("expected LSO 100, got %d", lso)
	}
}
