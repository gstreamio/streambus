package transaction

import (
	"os"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *logging.Logger {
	return logging.New(&logging.Config{
		Level:  logging.LevelDebug,
		Output: os.Stdout,
	})
}

func TestTransactionCoordinator_InitProducerID(t *testing.T) {
	txnLog := NewMemoryTransactionLog()
	config := DefaultCoordinatorConfig()
	logger := testLogger()
	coordinator := NewTransactionCoordinator(txnLog, config, logger)
	defer coordinator.Stop()

	// Test first producer ID assignment
	req1 := &InitProducerIDRequest{
		TransactionID:      "txn-1",
		TransactionTimeout: 30 * time.Second,
	}

	resp1, err := coordinator.InitProducerID(req1)
	require.NoError(t, err)
	assert.Equal(t, ErrorNone, resp1.ErrorCode)
	assert.NotZero(t, resp1.ProducerID)
	assert.Equal(t, ProducerEpoch(0), resp1.ProducerEpoch)

	// Test second producer ID for same transaction (should fence)
	req2 := &InitProducerIDRequest{
		TransactionID:      "txn-1",
		TransactionTimeout: 30 * time.Second,
	}

	resp2, err := coordinator.InitProducerID(req2)
	require.NoError(t, err)
	assert.Equal(t, ErrorNone, resp2.ErrorCode)
	assert.Equal(t, resp1.ProducerID, resp2.ProducerID) // Same producer ID
	assert.Equal(t, ProducerEpoch(1), resp2.ProducerEpoch) // Epoch incremented

	// Test different transaction ID
	req3 := &InitProducerIDRequest{
		TransactionID:      "txn-2",
		TransactionTimeout: 30 * time.Second,
	}

	resp3, err := coordinator.InitProducerID(req3)
	require.NoError(t, err)
	assert.Equal(t, ErrorNone, resp3.ErrorCode)
	assert.NotEqual(t, resp1.ProducerID, resp3.ProducerID) // Different producer ID
	assert.Equal(t, ProducerEpoch(0), resp3.ProducerEpoch)
}

func TestTransactionCoordinator_InvalidTimeout(t *testing.T) {
	txnLog := NewMemoryTransactionLog()
	config := DefaultCoordinatorConfig()
	config.MaxTransactionTimeout = 5 * time.Minute
	logger := testLogger()
	coordinator := NewTransactionCoordinator(txnLog, config, logger)
	defer coordinator.Stop()

	// Test timeout too large
	req := &InitProducerIDRequest{
		TransactionID:      "txn-1",
		TransactionTimeout: 10 * time.Minute, // Exceeds max
	}

	resp, err := coordinator.InitProducerID(req)
	require.NoError(t, err)
	assert.Equal(t, ErrorInvalidTransactionTimeout, resp.ErrorCode)
}

func TestTransactionCoordinator_AddPartitionsToTxn(t *testing.T) {
	txnLog := NewMemoryTransactionLog()
	config := DefaultCoordinatorConfig()
	logger := testLogger()
	coordinator := NewTransactionCoordinator(txnLog, config, logger)
	defer coordinator.Stop()

	// Initialize producer
	initReq := &InitProducerIDRequest{
		TransactionID:      "txn-1",
		TransactionTimeout: 30 * time.Second,
	}
	initResp, err := coordinator.InitProducerID(initReq)
	require.NoError(t, err)
	require.Equal(t, ErrorNone, initResp.ErrorCode)

	// Add partitions
	addReq := &AddPartitionsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Partitions: []PartitionMetadata{
			{Topic: "topic-1", Partition: 0},
			{Topic: "topic-1", Partition: 1},
			{Topic: "topic-2", Partition: 0},
		},
	}

	addResp, err := coordinator.AddPartitionsToTxn(addReq)
	require.NoError(t, err)
	assert.Empty(t, addResp.Errors)

	// Verify transaction state
	state, err := coordinator.GetTransactionState("txn-1")
	require.NoError(t, err)
	assert.Equal(t, StateOngoing, state)

	// Add more partitions (should be idempotent)
	addReq2 := &AddPartitionsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Partitions: []PartitionMetadata{
			{Topic: "topic-1", Partition: 0}, // Duplicate
			{Topic: "topic-3", Partition: 0}, // New
		},
	}

	addResp2, err := coordinator.AddPartitionsToTxn(addReq2)
	require.NoError(t, err)
	assert.Empty(t, addResp2.Errors)
}

func TestTransactionCoordinator_CommitTransaction(t *testing.T) {
	txnLog := NewMemoryTransactionLog()
	config := DefaultCoordinatorConfig()
	logger := testLogger()
	coordinator := NewTransactionCoordinator(txnLog, config, logger)
	defer coordinator.Stop()

	// Initialize producer
	initReq := &InitProducerIDRequest{
		TransactionID:      "txn-1",
		TransactionTimeout: 30 * time.Second,
	}
	initResp, err := coordinator.InitProducerID(initReq)
	require.NoError(t, err)

	// Add partitions
	addReq := &AddPartitionsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Partitions: []PartitionMetadata{
			{Topic: "topic-1", Partition: 0},
		},
	}
	_, err = coordinator.AddPartitionsToTxn(addReq)
	require.NoError(t, err)

	// Commit transaction
	endReq := &EndTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Commit:        true,
	}

	endResp, err := coordinator.EndTxn(endReq)
	require.NoError(t, err)
	assert.Equal(t, ErrorNone, endResp.ErrorCode)

	// Verify transaction state
	state, err := coordinator.GetTransactionState("txn-1")
	require.NoError(t, err)
	assert.Equal(t, StateCompleteCommit, state)
}

func TestTransactionCoordinator_AbortTransaction(t *testing.T) {
	txnLog := NewMemoryTransactionLog()
	config := DefaultCoordinatorConfig()
	logger := testLogger()
	coordinator := NewTransactionCoordinator(txnLog, config, logger)
	defer coordinator.Stop()

	// Initialize producer
	initReq := &InitProducerIDRequest{
		TransactionID:      "txn-1",
		TransactionTimeout: 30 * time.Second,
	}
	initResp, err := coordinator.InitProducerID(initReq)
	require.NoError(t, err)

	// Add partitions
	addReq := &AddPartitionsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Partitions: []PartitionMetadata{
			{Topic: "topic-1", Partition: 0},
		},
	}
	_, err = coordinator.AddPartitionsToTxn(addReq)
	require.NoError(t, err)

	// Abort transaction
	endReq := &EndTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Commit:        false,
	}

	endResp, err := coordinator.EndTxn(endReq)
	require.NoError(t, err)
	assert.Equal(t, ErrorNone, endResp.ErrorCode)

	// Verify transaction state
	state, err := coordinator.GetTransactionState("txn-1")
	require.NoError(t, err)
	assert.Equal(t, StateCompleteAbort, state)
}

func TestTransactionCoordinator_ProducerFencing(t *testing.T) {
	txnLog := NewMemoryTransactionLog()
	config := DefaultCoordinatorConfig()
	logger := testLogger()
	coordinator := NewTransactionCoordinator(txnLog, config, logger)
	defer coordinator.Stop()

	// Initialize producer (epoch 0)
	initReq1 := &InitProducerIDRequest{
		TransactionID:      "txn-1",
		TransactionTimeout: 30 * time.Second,
	}
	initResp1, err := coordinator.InitProducerID(initReq1)
	require.NoError(t, err)

	// Initialize again (epoch 1, fences previous)
	initReq2 := &InitProducerIDRequest{
		TransactionID:      "txn-1",
		TransactionTimeout: 30 * time.Second,
	}
	initResp2, err := coordinator.InitProducerID(initReq2)
	require.NoError(t, err)
	assert.Equal(t, initResp1.ProducerID, initResp2.ProducerID)
	assert.Equal(t, ProducerEpoch(1), initResp2.ProducerEpoch)

	// Try to use old epoch (should be fenced)
	addReq := &AddPartitionsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp1.ProducerID,
		ProducerEpoch: initResp1.ProducerEpoch, // Old epoch
		Partitions: []PartitionMetadata{
			{Topic: "topic-1", Partition: 0},
		},
	}

	_, err = coordinator.AddPartitionsToTxn(addReq)
	assert.Error(t, err) // Should be rejected
}

func TestTransactionCoordinator_ExpiredTransactions(t *testing.T) {
	txnLog := NewMemoryTransactionLog()
	config := DefaultCoordinatorConfig()
	config.ExpirationCheckInterval = 100 * time.Millisecond
	logger := testLogger()
	coordinator := NewTransactionCoordinator(txnLog, config, logger)
	defer coordinator.Stop()

	// Initialize producer with short timeout
	initReq := &InitProducerIDRequest{
		TransactionID:      "txn-1",
		TransactionTimeout: 200 * time.Millisecond,
	}
	initResp, err := coordinator.InitProducerID(initReq)
	require.NoError(t, err)

	// Add partitions
	addReq := &AddPartitionsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Partitions: []PartitionMetadata{
			{Topic: "topic-1", Partition: 0},
		},
	}
	_, err = coordinator.AddPartitionsToTxn(addReq)
	require.NoError(t, err)

	// Verify transaction is ongoing
	state, err := coordinator.GetTransactionState("txn-1")
	require.NoError(t, err)
	assert.Equal(t, StateOngoing, state)

	// Wait for expiration
	time.Sleep(500 * time.Millisecond)

	// Verify transaction was aborted
	state, err = coordinator.GetTransactionState("txn-1")
	require.NoError(t, err)
	assert.Equal(t, StateCompleteAbort, state)
}

func TestTransactionCoordinator_AddOffsetsToTxn(t *testing.T) {
	txnLog := NewMemoryTransactionLog()
	config := DefaultCoordinatorConfig()
	logger := testLogger()
	coordinator := NewTransactionCoordinator(txnLog, config, logger)
	defer coordinator.Stop()

	// Initialize producer
	initReq := &InitProducerIDRequest{
		TransactionID:      "txn-1",
		TransactionTimeout: 30 * time.Second,
	}
	initResp, err := coordinator.InitProducerID(initReq)
	require.NoError(t, err)

	// Add partitions to start transaction
	addReq := &AddPartitionsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Partitions: []PartitionMetadata{
			{Topic: "topic-1", Partition: 0},
		},
	}
	_, err = coordinator.AddPartitionsToTxn(addReq)
	require.NoError(t, err)

	// Add offsets to transaction
	offsetReq := &AddOffsetsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		GroupID:       "consumer-group-1",
	}

	offsetResp, err := coordinator.AddOffsetsToTxn(offsetReq)
	require.NoError(t, err)
	assert.Equal(t, ErrorNone, offsetResp.ErrorCode)
}

func TestTransactionCoordinator_Stats(t *testing.T) {
	txnLog := NewMemoryTransactionLog()
	config := DefaultCoordinatorConfig()
	logger := testLogger()
	coordinator := NewTransactionCoordinator(txnLog, config, logger)
	defer coordinator.Stop()

	// Initial stats
	stats := coordinator.Stats()
	assert.Equal(t, 0, stats.ActiveTransactions)
	assert.Equal(t, 0, stats.CompletedTransactions)
	assert.Equal(t, 0, stats.TotalProducers)

	// Create a transaction
	initReq := &InitProducerIDRequest{
		TransactionID:      "txn-1",
		TransactionTimeout: 30 * time.Second,
	}
	initResp, err := coordinator.InitProducerID(initReq)
	require.NoError(t, err)

	addReq := &AddPartitionsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Partitions: []PartitionMetadata{
			{Topic: "topic-1", Partition: 0},
		},
	}
	_, err = coordinator.AddPartitionsToTxn(addReq)
	require.NoError(t, err)

	// Check stats
	stats = coordinator.Stats()
	assert.Equal(t, 1, stats.ActiveTransactions)
	assert.Equal(t, 1, stats.TotalProducers)

	// Commit transaction
	endReq := &EndTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Commit:        true,
	}
	_, err = coordinator.EndTxn(endReq)
	require.NoError(t, err)

	// Check stats after commit
	stats = coordinator.Stats()
	assert.Equal(t, 0, stats.ActiveTransactions)
	assert.Equal(t, 1, stats.CompletedTransactions)
}

func TestMemoryTransactionLog(t *testing.T) {
	log := NewMemoryTransactionLog()

	// Test empty log
	assert.Equal(t, 0, log.Count())

	// Test append
	entry1 := &TransactionLogEntry{
		TransactionID: "txn-1",
		ProducerID:    1001,
		ProducerEpoch: 0,
		State:         StateOngoing,
		Partitions: []PartitionMetadata{
			{Topic: "topic-1", Partition: 0},
		},
		Timestamp: time.Now(),
	}

	err := log.Append(entry1)
	require.NoError(t, err)
	assert.Equal(t, 1, log.Count())

	// Test read
	readEntry, err := log.Read("txn-1")
	require.NoError(t, err)
	assert.Equal(t, entry1.TransactionID, readEntry.TransactionID)
	assert.Equal(t, entry1.ProducerID, readEntry.ProducerID)
	assert.Equal(t, entry1.State, readEntry.State)

	// Test read all
	allEntries, err := log.ReadAll()
	require.NoError(t, err)
	assert.Len(t, allEntries, 1)

	// Test delete
	err = log.Delete("txn-1")
	require.NoError(t, err)
	assert.Equal(t, 0, log.Count())

	// Test read non-existent
	_, err = log.Read("txn-1")
	assert.Error(t, err)
}
