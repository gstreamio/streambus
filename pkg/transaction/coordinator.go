package transaction

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shawntherrien/streambus/pkg/logging"
)

// TransactionCoordinator manages transactional operations
type TransactionCoordinator struct {
	mu sync.RWMutex

	// Transaction state
	transactions map[TransactionID]*TransactionMetadata
	producers    map[ProducerID]*ProducerMetadata

	// Configuration
	config CoordinatorConfig

	// Producer ID generator
	nextProducerID int64

	// Transaction log
	txnLog TransactionLog

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	closed int32

	logger *logging.Logger
}

// CoordinatorConfig holds configuration for the transaction coordinator
type CoordinatorConfig struct {
	// Default transaction timeout
	DefaultTransactionTimeout time.Duration

	// Maximum transaction timeout allowed
	MaxTransactionTimeout time.Duration

	// How often to check for expired transactions
	ExpirationCheckInterval time.Duration

	// How long to keep completed transaction metadata
	TransactionRetentionTime time.Duration
}

// DefaultCoordinatorConfig returns default configuration
func DefaultCoordinatorConfig() CoordinatorConfig {
	return CoordinatorConfig{
		DefaultTransactionTimeout: 60 * time.Second,
		MaxTransactionTimeout:     15 * time.Minute,
		ExpirationCheckInterval:   10 * time.Second,
		TransactionRetentionTime:  24 * time.Hour,
	}
}

// TransactionLog interface for persisting transaction state
type TransactionLog interface {
	Append(entry *TransactionLogEntry) error
	Read(txnID TransactionID) (*TransactionLogEntry, error)
	ReadAll() ([]*TransactionLogEntry, error)
	Delete(txnID TransactionID) error
}

// NewTransactionCoordinator creates a new transaction coordinator
func NewTransactionCoordinator(txnLog TransactionLog, config CoordinatorConfig, logger *logging.Logger) *TransactionCoordinator {
	if logger == nil {
		logger = logging.New(&logging.Config{
			Level:  logging.LevelInfo,
			Output: os.Stdout,
		})
	}

	ctx, cancel := context.WithCancel(context.Background())

	tc := &TransactionCoordinator{
		transactions:   make(map[TransactionID]*TransactionMetadata),
		producers:      make(map[ProducerID]*ProducerMetadata),
		config:         config,
		nextProducerID: 1000, // Start from 1000
		txnLog:         txnLog,
		ctx:            ctx,
		cancel:         cancel,
		logger:         logger,
	}

	// Start background tasks
	tc.wg.Add(1)
	go tc.expirationChecker()

	return tc
}

// Stop stops the coordinator
func (tc *TransactionCoordinator) Stop() {
	if !atomic.CompareAndSwapInt32(&tc.closed, 0, 1) {
		return
	}

	tc.logger.Info("Stopping transaction coordinator")
	tc.cancel()
	tc.wg.Wait()
	tc.logger.Info("Transaction coordinator stopped")
}

// InitProducerID initializes a producer ID for transactional operations
func (tc *TransactionCoordinator) InitProducerID(req *InitProducerIDRequest) (*InitProducerIDResponse, error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Validate transaction timeout
	timeout := req.TransactionTimeout
	if timeout == 0 {
		timeout = tc.config.DefaultTransactionTimeout
	}
	if timeout > tc.config.MaxTransactionTimeout {
		return &InitProducerIDResponse{
			ErrorCode: ErrorInvalidTransactionTimeout,
		}, nil
	}

	// Check if producer already exists for this transaction ID
	var producerID ProducerID
	var epoch ProducerEpoch

	existingProducer := tc.findProducerByTransactionID(req.TransactionID)
	if existingProducer != nil {
		// Increment epoch to fence previous producer
		producerID = existingProducer.ProducerID
		epoch = existingProducer.ProducerEpoch + 1
	} else {
		// Assign new producer ID
		producerID = ProducerID(atomic.AddInt64(&tc.nextProducerID, 1))
		epoch = 0
	}

	// Create producer metadata
	producer := &ProducerMetadata{
		ProducerID:         producerID,
		ProducerEpoch:      epoch,
		TransactionID:      req.TransactionID,
		TransactionTimeout: timeout,
		LastTimestamp:      time.Now(),
	}

	tc.producers[producerID] = producer

	tc.logger.Debug("Initialized producer ID", logging.Fields{
		"transaction_id": req.TransactionID,
		"producer_id":    producerID,
		"epoch":          epoch,
	})

	return &InitProducerIDResponse{
		ProducerID:    producerID,
		ProducerEpoch: epoch,
		ErrorCode:     ErrorNone,
	}, nil
}

// AddPartitionsToTxn adds partitions to an ongoing transaction
func (tc *TransactionCoordinator) AddPartitionsToTxn(req *AddPartitionsToTxnRequest) (*AddPartitionsToTxnResponse, error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Validate producer
	if err := tc.validateProducer(req.ProducerID, req.ProducerEpoch); err != nil {
		return &AddPartitionsToTxnResponse{
			Errors: map[string]map[int32]ErrorCode{},
		}, err
	}

	// Get or create transaction
	txn, exists := tc.transactions[req.TransactionID]
	if !exists {
		// Get producer to retrieve transaction timeout
		producer, producerExists := tc.producers[req.ProducerID]
		if !producerExists {
			return &AddPartitionsToTxnResponse{
				Errors: tc.buildPartitionErrors(req.Partitions, ErrorInvalidProducerIDMapping),
			}, nil
		}

		// Start new transaction
		txn = &TransactionMetadata{
			TransactionID:      req.TransactionID,
			ProducerID:         req.ProducerID,
			ProducerEpoch:      req.ProducerEpoch,
			State:              StateOngoing,
			Partitions:         make([]PartitionMetadata, 0),
			TransactionTimeout: producer.TransactionTimeout,
			StartTime:          time.Now(),
			LastUpdateTime:     time.Now(),
		}
		tc.transactions[req.TransactionID] = txn

		// Log transaction start
		tc.logTransaction(txn)
	} else {
		// Verify transaction is ongoing
		if txn.State != StateOngoing {
			return &AddPartitionsToTxnResponse{
				Errors: tc.buildPartitionErrors(req.Partitions, ErrorInvalidTransactionState),
			}, nil
		}

		// Verify producer matches
		if txn.ProducerID != req.ProducerID || txn.ProducerEpoch != req.ProducerEpoch {
			return &AddPartitionsToTxnResponse{
				Errors: tc.buildPartitionErrors(req.Partitions, ErrorInvalidProducerIDMapping),
			}, nil
		}
	}

	// Add partitions to transaction
	for _, partition := range req.Partitions {
		if !tc.partitionExists(txn, partition) {
			txn.Partitions = append(txn.Partitions, partition)
		}
	}

	txn.LastUpdateTime = time.Now()

	// Log partition addition
	tc.logTransaction(txn)

	tc.logger.Debug("Added partitions to transaction", logging.Fields{
		"transaction_id":    req.TransactionID,
		"partition_count":   len(req.Partitions),
		"total_partitions": len(txn.Partitions),
	})

	return &AddPartitionsToTxnResponse{
		Errors: map[string]map[int32]ErrorCode{},
	}, nil
}

// EndTxn commits or aborts a transaction
func (tc *TransactionCoordinator) EndTxn(req *EndTxnRequest) (*EndTxnResponse, error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Validate producer
	if err := tc.validateProducer(req.ProducerID, req.ProducerEpoch); err != nil {
		return &EndTxnResponse{
			ErrorCode: ErrorProducerFenced,
		}, nil
	}

	// Get transaction
	txn, exists := tc.transactions[req.TransactionID]
	if !exists {
		return &EndTxnResponse{
			ErrorCode: ErrorInvalidTransactionState,
		}, nil
	}

	// Verify transaction is ongoing
	if txn.State != StateOngoing {
		return &EndTxnResponse{
			ErrorCode: ErrorInvalidTransactionState,
		}, nil
	}

	// Verify producer matches
	if txn.ProducerID != req.ProducerID || txn.ProducerEpoch != req.ProducerEpoch {
		return &EndTxnResponse{
			ErrorCode: ErrorInvalidProducerIDMapping,
		}, nil
	}

	// Phase 1: Prepare
	if req.Commit {
		txn.State = StatePrepareCommit
	} else {
		txn.State = StatePrepareAbort
	}
	txn.LastUpdateTime = time.Now()

	// Log prepare phase
	tc.logTransaction(txn)

	// Phase 2: Complete
	// In a real implementation, this would write transaction markers to all partitions
	// and wait for acknowledgments before completing
	if req.Commit {
		txn.State = StateCompleteCommit
	} else {
		txn.State = StateCompleteAbort
	}
	txn.LastUpdateTime = time.Now()

	// Log complete phase
	tc.logTransaction(txn)

	action := "committed"
	if !req.Commit {
		action = "aborted"
	}

	tc.logger.Info(fmt.Sprintf("Transaction %s", action), logging.Fields{
		"transaction_id": req.TransactionID,
		"producer_id":    req.ProducerID,
		"partitions":     len(txn.Partitions),
	})

	// Schedule transaction cleanup
	go tc.scheduleTransactionCleanup(req.TransactionID)

	return &EndTxnResponse{
		ErrorCode: ErrorNone,
	}, nil
}

// AddOffsetsToTxn adds consumer group offsets to a transaction
func (tc *TransactionCoordinator) AddOffsetsToTxn(req *AddOffsetsToTxnRequest) (*AddOffsetsToTxnResponse, error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Validate producer
	if err := tc.validateProducer(req.ProducerID, req.ProducerEpoch); err != nil {
		return &AddOffsetsToTxnResponse{
			ErrorCode: ErrorProducerFenced,
		}, nil
	}

	// Get transaction
	txn, exists := tc.transactions[req.TransactionID]
	if !exists {
		return &AddOffsetsToTxnResponse{
			ErrorCode: ErrorInvalidTransactionState,
		}, nil
	}

	// Verify transaction is ongoing
	if txn.State != StateOngoing {
		return &AddOffsetsToTxnResponse{
			ErrorCode: ErrorInvalidTransactionState,
		}, nil
	}

	// In a real implementation, this would add the consumer group offsets
	// to the transaction's metadata for coordinated commit

	tc.logger.Debug("Added offsets to transaction", logging.Fields{
		"transaction_id": req.TransactionID,
		"group_id":       req.GroupID,
	})

	return &AddOffsetsToTxnResponse{
		ErrorCode: ErrorNone,
	}, nil
}

// GetTransactionState returns the current state of a transaction
func (tc *TransactionCoordinator) GetTransactionState(txnID TransactionID) (TransactionState, error) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	txn, exists := tc.transactions[txnID]
	if !exists {
		return StateEmpty, fmt.Errorf("transaction not found: %s", txnID)
	}

	return txn.State, nil
}

// Internal methods

func (tc *TransactionCoordinator) validateProducer(producerID ProducerID, epoch ProducerEpoch) error {
	producer, exists := tc.producers[producerID]
	if !exists {
		return fmt.Errorf("unknown producer ID: %d", producerID)
	}

	if producer.ProducerEpoch != epoch {
		return fmt.Errorf("producer epoch mismatch: expected %d, got %d", producer.ProducerEpoch, epoch)
	}

	return nil
}

func (tc *TransactionCoordinator) findProducerByTransactionID(txnID TransactionID) *ProducerMetadata {
	for _, producer := range tc.producers {
		if producer.TransactionID == txnID {
			return producer
		}
	}
	return nil
}

func (tc *TransactionCoordinator) partitionExists(txn *TransactionMetadata, partition PartitionMetadata) bool {
	for _, p := range txn.Partitions {
		if p.Topic == partition.Topic && p.Partition == partition.Partition {
			return true
		}
	}
	return false
}

func (tc *TransactionCoordinator) buildPartitionErrors(partitions []PartitionMetadata, errCode ErrorCode) map[string]map[int32]ErrorCode {
	errors := make(map[string]map[int32]ErrorCode)
	for _, partition := range partitions {
		if errors[partition.Topic] == nil {
			errors[partition.Topic] = make(map[int32]ErrorCode)
		}
		errors[partition.Topic][partition.Partition] = errCode
	}
	return errors
}

func (tc *TransactionCoordinator) logTransaction(txn *TransactionMetadata) {
	if tc.txnLog == nil {
		return
	}

	entry := &TransactionLogEntry{
		TransactionID: txn.TransactionID,
		ProducerID:    txn.ProducerID,
		ProducerEpoch: txn.ProducerEpoch,
		State:         txn.State,
		Partitions:    txn.Partitions,
		Timestamp:     time.Now(),
	}

	if err := tc.txnLog.Append(entry); err != nil {
		tc.logger.Error("Failed to log transaction", err)
	}
}

func (tc *TransactionCoordinator) expirationChecker() {
	defer tc.wg.Done()

	ticker := time.NewTicker(tc.config.ExpirationCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-tc.ctx.Done():
			return
		case <-ticker.C:
			tc.checkExpiredTransactions()
		}
	}
}

func (tc *TransactionCoordinator) checkExpiredTransactions() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	now := time.Now()
	for txnID, txn := range tc.transactions {
		if txn.State == StateOngoing && txn.IsExpired() {
			tc.logger.Warn("Transaction expired, aborting", logging.Fields{
				"transaction_id": txnID,
				"age":            now.Sub(txn.StartTime),
			})

			// Abort expired transaction
			txn.State = StatePrepareAbort
			txn.LastUpdateTime = now
			tc.logTransaction(txn)

			txn.State = StateCompleteAbort
			txn.LastUpdateTime = now
			tc.logTransaction(txn)

			// Schedule cleanup
			go tc.scheduleTransactionCleanup(txnID)
		}
	}
}

func (tc *TransactionCoordinator) scheduleTransactionCleanup(txnID TransactionID) {
	time.Sleep(tc.config.TransactionRetentionTime)

	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Delete transaction metadata
	delete(tc.transactions, txnID)

	// Delete from log
	if tc.txnLog != nil {
		if err := tc.txnLog.Delete(txnID); err != nil {
			tc.logger.Error("Failed to delete transaction from log", err)
		}
	}

	tc.logger.Debug("Transaction metadata cleaned up", logging.Fields{
		"transaction_id": txnID,
	})
}

// Stats returns coordinator statistics
func (tc *TransactionCoordinator) Stats() CoordinatorStats {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	stats := CoordinatorStats{
		ActiveTransactions:   0,
		CompletedTransactions: 0,
		AbortedTransactions:  0,
		TotalProducers:       len(tc.producers),
	}

	for _, txn := range tc.transactions {
		switch txn.State {
		case StateOngoing, StatePrepareCommit, StatePrepareAbort:
			stats.ActiveTransactions++
		case StateCompleteCommit:
			stats.CompletedTransactions++
		case StateCompleteAbort:
			stats.AbortedTransactions++
		}
	}

	return stats
}

// CoordinatorStats holds coordinator statistics
type CoordinatorStats struct {
	ActiveTransactions     int
	CompletedTransactions  int
	AbortedTransactions    int
	TotalProducers         int
}
