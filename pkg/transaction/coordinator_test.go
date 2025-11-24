package transaction

import (
	"fmt"
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

func TestMemoryTransactionLog_Clear(t *testing.T) {
	log := NewMemoryTransactionLog()

	// Add multiple entries
	for i := 0; i < 5; i++ {
		entry := &TransactionLogEntry{
			TransactionID: TransactionID("txn-" + string(rune(i+'0'))),
			ProducerID:    ProducerID(1001 + i),
			ProducerEpoch: 0,
			State:         StateOngoing,
			Partitions: []PartitionMetadata{
				{Topic: "topic-1", Partition: int32(i)},
			},
			Timestamp: time.Now(),
		}
		err := log.Append(entry)
		require.NoError(t, err)
	}

	// Verify entries exist
	assert.Equal(t, 5, log.Count())

	// Clear the log
	log.Clear()

	// Verify log is empty
	assert.Equal(t, 0, log.Count())

	// Verify ReadAll returns empty
	entries, err := log.ReadAll()
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestMemoryTransactionLog_AppendNil(t *testing.T) {
	log := NewMemoryTransactionLog()

	// Test append nil entry
	err := log.Append(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
	assert.Equal(t, 0, log.Count())
}

func TestTransactionState_String(t *testing.T) {
	tests := []struct {
		state    TransactionState
		expected string
	}{
		{StateEmpty, "Empty"},
		{StateOngoing, "Ongoing"},
		{StatePrepareCommit, "PrepareCommit"},
		{StatePrepareAbort, "PrepareAbort"},
		{StateCompleteCommit, "CompleteCommit"},
		{StateCompleteAbort, "CompleteAbort"},
		{TransactionState(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.state.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestErrorCode_String(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		expected string
	}{
		{ErrorNone, "None"},
		{ErrorInvalidProducerEpoch, "InvalidProducerEpoch"},
		{ErrorInvalidTransactionState, "InvalidTransactionState"},
		{ErrorInvalidProducerIDMapping, "InvalidProducerIDMapping"},
		{ErrorTransactionCoordinatorNotAvailable, "TransactionCoordinatorNotAvailable"},
		{ErrorTransactionCoordinatorFenced, "TransactionCoordinatorFenced"},
		{ErrorProducerFenced, "ProducerFenced"},
		{ErrorInvalidTransactionTimeout, "InvalidTransactionTimeout"},
		{ErrorConcurrentTransactions, "ConcurrentTransactions"},
		{ErrorTransactionAborted, "TransactionAborted"},
		{ErrorInvalidPartitionList, "InvalidPartitionList"},
		{ErrorCode(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.code.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestErrorCode_Error(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		expected string
	}{
		{ErrorNone, "None"},
		{ErrorInvalidProducerEpoch, "InvalidProducerEpoch"},
		{ErrorInvalidTransactionState, "InvalidTransactionState"},
		{ErrorProducerFenced, "ProducerFenced"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.code.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTransactionCoordinator_BuildPartitionErrors(t *testing.T) {
	txnLog := NewMemoryTransactionLog()
	config := DefaultCoordinatorConfig()
	logger := testLogger()
	coordinator := NewTransactionCoordinator(txnLog, config, logger)
	defer coordinator.Stop()

	partitions := []PartitionMetadata{
		{Topic: "topic-1", Partition: 0},
		{Topic: "topic-1", Partition: 1},
		{Topic: "topic-2", Partition: 0},
	}

	errCode := ErrorInvalidProducerIDMapping
	errors := coordinator.buildPartitionErrors(partitions, errCode)

	// Verify structure
	assert.Len(t, errors, 2)
	assert.Contains(t, errors, "topic-1")
	assert.Contains(t, errors, "topic-2")

	// Verify topic-1 errors
	assert.Len(t, errors["topic-1"], 2)
	assert.Equal(t, errCode, errors["topic-1"][0])
	assert.Equal(t, errCode, errors["topic-1"][1])

	// Verify topic-2 errors
	assert.Len(t, errors["topic-2"], 1)
	assert.Equal(t, errCode, errors["topic-2"][0])
}

func TestTransactionCoordinator_LogTransactionNilLog(t *testing.T) {
	config := DefaultCoordinatorConfig()
	logger := testLogger()
	coordinator := NewTransactionCoordinator(nil, config, logger)
	defer coordinator.Stop()

	// Initialize producer
	initReq := &InitProducerIDRequest{
		TransactionID:      "txn-1",
		TransactionTimeout: 30 * time.Second,
	}
	initResp, err := coordinator.InitProducerID(initReq)
	require.NoError(t, err)

	// Add partitions (will call logTransaction with nil log)
	addReq := &AddPartitionsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Partitions: []PartitionMetadata{
			{Topic: "topic-1", Partition: 0},
		},
	}

	// Should not error even with nil log
	addResp, err := coordinator.AddPartitionsToTxn(addReq)
	require.NoError(t, err)
	assert.Empty(t, addResp.Errors)
}

func TestTransactionCoordinator_LogTransactionError(t *testing.T) {
	// Create a mock log that returns errors
	mockLog := &errorTransactionLog{}
	config := DefaultCoordinatorConfig()
	logger := testLogger()
	coordinator := NewTransactionCoordinator(mockLog, config, logger)
	defer coordinator.Stop()

	// Initialize producer
	initReq := &InitProducerIDRequest{
		TransactionID:      "txn-1",
		TransactionTimeout: 30 * time.Second,
	}
	initResp, err := coordinator.InitProducerID(initReq)
	require.NoError(t, err)

	// Add partitions (will call logTransaction which will error)
	addReq := &AddPartitionsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Partitions: []PartitionMetadata{
			{Topic: "topic-1", Partition: 0},
		},
	}

	// Should not return error but will log it internally
	addResp, err := coordinator.AddPartitionsToTxn(addReq)
	require.NoError(t, err)
	assert.Empty(t, addResp.Errors)
}

func TestTransactionCoordinator_ScheduleTransactionCleanup(t *testing.T) {
	txnLog := NewMemoryTransactionLog()
	config := DefaultCoordinatorConfig()
	config.TransactionRetentionTime = 100 * time.Millisecond
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

	// Commit transaction (triggers cleanup scheduling)
	endReq := &EndTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Commit:        true,
	}
	_, err = coordinator.EndTxn(endReq)
	require.NoError(t, err)

	// Verify transaction exists
	_, err = coordinator.GetTransactionState("txn-1")
	require.NoError(t, err)

	// Verify log entry exists
	assert.Equal(t, 1, txnLog.Count())

	// Wait for cleanup
	time.Sleep(200 * time.Millisecond)

	// Verify transaction was cleaned up
	_, err = coordinator.GetTransactionState("txn-1")
	assert.Error(t, err)

	// Verify log entry was deleted
	assert.Equal(t, 0, txnLog.Count())
}

func TestTransactionCoordinator_ScheduleTransactionCleanupWithLogError(t *testing.T) {
	mockLog := &errorTransactionLog{}
	config := DefaultCoordinatorConfig()
	config.TransactionRetentionTime = 100 * time.Millisecond
	logger := testLogger()
	coordinator := NewTransactionCoordinator(mockLog, config, logger)
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
	_, err = coordinator.EndTxn(endReq)
	require.NoError(t, err)

	// Wait for cleanup (should handle log delete error gracefully)
	time.Sleep(200 * time.Millisecond)

	// Verify transaction was cleaned up from memory despite log error
	_, err = coordinator.GetTransactionState("txn-1")
	assert.Error(t, err)
}

func TestTransactionCoordinator_AddOffsetsToTxn_ProducerFenced(t *testing.T) {
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

	// Start transaction with new producer
	addReq := &AddPartitionsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp2.ProducerID,
		ProducerEpoch: initResp2.ProducerEpoch,
		Partitions: []PartitionMetadata{
			{Topic: "topic-1", Partition: 0},
		},
	}
	_, err = coordinator.AddPartitionsToTxn(addReq)
	require.NoError(t, err)

	// Try to add offsets with old epoch (should be fenced)
	offsetReq := &AddOffsetsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp1.ProducerID,
		ProducerEpoch: initResp1.ProducerEpoch,
		GroupID:       "consumer-group-1",
	}

	offsetResp, err := coordinator.AddOffsetsToTxn(offsetReq)
	require.NoError(t, err)
	assert.Equal(t, ErrorProducerFenced, offsetResp.ErrorCode)
}

func TestTransactionCoordinator_AddOffsetsToTxn_NoTransaction(t *testing.T) {
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

	// Try to add offsets without starting transaction
	offsetReq := &AddOffsetsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		GroupID:       "consumer-group-1",
	}

	offsetResp, err := coordinator.AddOffsetsToTxn(offsetReq)
	require.NoError(t, err)
	assert.Equal(t, ErrorInvalidTransactionState, offsetResp.ErrorCode)
}

func TestTransactionCoordinator_AddOffsetsToTxn_InvalidState(t *testing.T) {
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

	// Start transaction
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
	_, err = coordinator.EndTxn(endReq)
	require.NoError(t, err)

	// Try to add offsets after transaction is committed
	offsetReq := &AddOffsetsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		GroupID:       "consumer-group-1",
	}

	offsetResp, err := coordinator.AddOffsetsToTxn(offsetReq)
	require.NoError(t, err)
	assert.Equal(t, ErrorInvalidTransactionState, offsetResp.ErrorCode)
}

// errorTransactionLog is a mock that always returns errors
type errorTransactionLog struct{}

func (l *errorTransactionLog) Append(entry *TransactionLogEntry) error {
	return fmt.Errorf("mock append error")
}

func (l *errorTransactionLog) Read(txnID TransactionID) (*TransactionLogEntry, error) {
	return nil, fmt.Errorf("mock read error")
}

func (l *errorTransactionLog) ReadAll() ([]*TransactionLogEntry, error) {
	return nil, fmt.Errorf("mock read all error")
}

func (l *errorTransactionLog) Delete(txnID TransactionID) error {
	return fmt.Errorf("mock delete error")
}
