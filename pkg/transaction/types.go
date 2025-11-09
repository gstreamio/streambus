package transaction

import (
	"time"
)

// TransactionID is a unique identifier for a transaction
type TransactionID string

// ProducerID is a unique identifier assigned to a transactional producer
type ProducerID int64

// ProducerEpoch is used to fence zombie producers
type ProducerEpoch int16

// TransactionState represents the state of a transaction
type TransactionState int

const (
	// StateEmpty indicates no transaction is in progress
	StateEmpty TransactionState = iota

	// StateOngoing indicates transaction is in progress
	StateOngoing

	// StatePrepareCommit indicates transaction is preparing to commit
	StatePrepareCommit

	// StatePrepareAbort indicates transaction is preparing to abort
	StatePrepareAbort

	// StateCompleteCommit indicates transaction has been committed
	StateCompleteCommit

	// StateCompleteAbort indicates transaction has been aborted
	StateCompleteAbort
)

// String returns the string representation of TransactionState
func (s TransactionState) String() string {
	switch s {
	case StateEmpty:
		return "Empty"
	case StateOngoing:
		return "Ongoing"
	case StatePrepareCommit:
		return "PrepareCommit"
	case StatePrepareAbort:
		return "PrepareAbort"
	case StateCompleteCommit:
		return "CompleteCommit"
	case StateCompleteAbort:
		return "CompleteAbort"
	default:
		return "Unknown"
	}
}

// TransactionResult indicates the outcome of a transaction
type TransactionResult int

const (
	ResultCommit TransactionResult = iota
	ResultAbort
)

// PartitionMetadata represents a topic-partition involved in a transaction
type PartitionMetadata struct {
	Topic     string
	Partition int32
}

// TransactionMetadata contains metadata about a transaction
type TransactionMetadata struct {
	TransactionID      TransactionID
	ProducerID         ProducerID
	ProducerEpoch      ProducerEpoch
	State              TransactionState
	Partitions         []PartitionMetadata
	TransactionTimeout time.Duration
	StartTime          time.Time
	LastUpdateTime     time.Time
}

// IsExpired checks if the transaction has exceeded its timeout
func (tm *TransactionMetadata) IsExpired() bool {
	return time.Since(tm.StartTime) > tm.TransactionTimeout
}

// ProducerMetadata contains metadata about a transactional producer
type ProducerMetadata struct {
	ProducerID         ProducerID
	ProducerEpoch      ProducerEpoch
	TransactionID      TransactionID
	TransactionTimeout time.Duration
	LastProducerEpoch  ProducerEpoch
	LastTimestamp      time.Time
}

// ErrorCode represents transaction-specific error codes
type ErrorCode int16

const (
	ErrorNone ErrorCode = iota
	ErrorInvalidProducerEpoch
	ErrorInvalidTransactionState
	ErrorInvalidProducerIDMapping
	ErrorTransactionCoordinatorNotAvailable
	ErrorTransactionCoordinatorFenced
	ErrorProducerFenced
	ErrorInvalidTransactionTimeout
	ErrorConcurrentTransactions
	ErrorTransactionAborted
	ErrorInvalidPartitionList
)

// String returns the string representation of ErrorCode
func (e ErrorCode) String() string {
	switch e {
	case ErrorNone:
		return "None"
	case ErrorInvalidProducerEpoch:
		return "InvalidProducerEpoch"
	case ErrorInvalidTransactionState:
		return "InvalidTransactionState"
	case ErrorInvalidProducerIDMapping:
		return "InvalidProducerIDMapping"
	case ErrorTransactionCoordinatorNotAvailable:
		return "TransactionCoordinatorNotAvailable"
	case ErrorTransactionCoordinatorFenced:
		return "TransactionCoordinatorFenced"
	case ErrorProducerFenced:
		return "ProducerFenced"
	case ErrorInvalidTransactionTimeout:
		return "InvalidTransactionTimeout"
	case ErrorConcurrentTransactions:
		return "ConcurrentTransactions"
	case ErrorTransactionAborted:
		return "TransactionAborted"
	case ErrorInvalidPartitionList:
		return "InvalidPartitionList"
	default:
		return "Unknown"
	}
}

// Error returns the error message
func (e ErrorCode) Error() string {
	return e.String()
}

// InitProducerIDRequest is sent to get a producer ID
type InitProducerIDRequest struct {
	TransactionID      TransactionID
	TransactionTimeout time.Duration
}

// InitProducerIDResponse contains the assigned producer ID
type InitProducerIDResponse struct {
	ProducerID    ProducerID
	ProducerEpoch ProducerEpoch
	ErrorCode     ErrorCode
}

// AddPartitionsToTxnRequest adds partitions to a transaction
type AddPartitionsToTxnRequest struct {
	TransactionID TransactionID
	ProducerID    ProducerID
	ProducerEpoch ProducerEpoch
	Partitions    []PartitionMetadata
}

// AddPartitionsToTxnResponse confirms partition addition
type AddPartitionsToTxnResponse struct {
	Errors map[string]map[int32]ErrorCode // topic -> partition -> error
}

// EndTxnRequest commits or aborts a transaction
type EndTxnRequest struct {
	TransactionID TransactionID
	ProducerID    ProducerID
	ProducerEpoch ProducerEpoch
	Commit        bool // true to commit, false to abort
}

// EndTxnResponse confirms transaction completion
type EndTxnResponse struct {
	ErrorCode ErrorCode
}

// AddOffsetsToTxnRequest adds consumer group offsets to transaction
type AddOffsetsToTxnRequest struct {
	TransactionID TransactionID
	ProducerID    ProducerID
	ProducerEpoch ProducerEpoch
	GroupID       string
}

// AddOffsetsToTxnResponse confirms offset addition
type AddOffsetsToTxnResponse struct {
	ErrorCode ErrorCode
}

// TxnOffsetCommitRequest commits offsets as part of transaction
type TxnOffsetCommitRequest struct {
	TransactionID TransactionID
	GroupID       string
	ProducerID    ProducerID
	ProducerEpoch ProducerEpoch
	Offsets       map[string]map[int32]OffsetMetadata // topic -> partition -> offset
}

// OffsetMetadata contains offset and metadata
type OffsetMetadata struct {
	Offset   int64
	Metadata string
}

// TxnOffsetCommitResponse confirms offset commit
type TxnOffsetCommitResponse struct {
	Errors map[string]map[int32]ErrorCode // topic -> partition -> error
}

// TransactionMarker represents a control message for transaction boundaries
type TransactionMarker struct {
	ProducerID    ProducerID
	ProducerEpoch ProducerEpoch
	Commit        bool // true for commit, false for abort
	Timestamp     int64
}

// TransactionLogEntry represents an entry in the transaction log
type TransactionLogEntry struct {
	TransactionID TransactionID
	ProducerID    ProducerID
	ProducerEpoch ProducerEpoch
	State         TransactionState
	Partitions    []PartitionMetadata
	Timestamp     time.Time
}
