package protocol

import (
	"fmt"
)

// Protocol version
const (
	ProtocolVersion = 1
)

// Message size limits
const (
	MaxMessageSize = 1024 * 1024 * 10 // 10MB
	HeaderSize     = 20                // Length(4) + RequestID(8) + Type(1) + Version(1) + Flags(2) + CRC32(4)
)

// RequestType represents the type of request
type RequestType byte

const (
	RequestTypeProduce         RequestType = 0x01
	RequestTypeFetch           RequestType = 0x02
	RequestTypeGetOffset       RequestType = 0x03
	RequestTypeCreateTopic     RequestType = 0x04
	RequestTypeDeleteTopic     RequestType = 0x05
	RequestTypeListTopics      RequestType = 0x06
	RequestTypeHealthCheck     RequestType = 0x07
	RequestTypeJoinGroup       RequestType = 0x08
	RequestTypeSyncGroup       RequestType = 0x09
	RequestTypeHeartbeat       RequestType = 0x0A
	RequestTypeLeaveGroup      RequestType = 0x0B
	RequestTypeOffsetCommit    RequestType = 0x0C
	RequestTypeOffsetFetch     RequestType = 0x0D
	RequestTypeInitProducerID  RequestType = 0x0E
	RequestTypeAddPartitionsToTxn RequestType = 0x0F
	RequestTypeAddOffsetsToTxn RequestType = 0x10
	RequestTypeEndTxn          RequestType = 0x11
	RequestTypeTxnOffsetCommit RequestType = 0x12
)

// String returns the string representation of RequestType
func (t RequestType) String() string {
	switch t {
	case RequestTypeProduce:
		return "Produce"
	case RequestTypeFetch:
		return "Fetch"
	case RequestTypeGetOffset:
		return "GetOffset"
	case RequestTypeCreateTopic:
		return "CreateTopic"
	case RequestTypeDeleteTopic:
		return "DeleteTopic"
	case RequestTypeListTopics:
		return "ListTopics"
	case RequestTypeHealthCheck:
		return "HealthCheck"
	case RequestTypeJoinGroup:
		return "JoinGroup"
	case RequestTypeSyncGroup:
		return "SyncGroup"
	case RequestTypeHeartbeat:
		return "Heartbeat"
	case RequestTypeLeaveGroup:
		return "LeaveGroup"
	case RequestTypeOffsetCommit:
		return "OffsetCommit"
	case RequestTypeOffsetFetch:
		return "OffsetFetch"
	case RequestTypeInitProducerID:
		return "InitProducerID"
	case RequestTypeAddPartitionsToTxn:
		return "AddPartitionsToTxn"
	case RequestTypeAddOffsetsToTxn:
		return "AddOffsetsToTxn"
	case RequestTypeEndTxn:
		return "EndTxn"
	case RequestTypeTxnOffsetCommit:
		return "TxnOffsetCommit"
	default:
		return fmt.Sprintf("Unknown(%d)", t)
	}
}

// StatusCode represents the response status
type StatusCode byte

const (
	StatusOK             StatusCode = 0
	StatusError          StatusCode = 1
	StatusPartialSuccess StatusCode = 2
)

// String returns the string representation of StatusCode
func (s StatusCode) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusError:
		return "Error"
	case StatusPartialSuccess:
		return "PartialSuccess"
	default:
		return fmt.Sprintf("Unknown(%d)", s)
	}
}

// ErrorCode represents specific error codes
type ErrorCode uint16

const (
	ErrNone              ErrorCode = 0
	ErrUnknownRequest    ErrorCode = 1
	ErrInvalidRequest    ErrorCode = 2
	ErrOffsetOutOfRange  ErrorCode = 3
	ErrCorruptMessage    ErrorCode = 4
	ErrPartitionNotFound ErrorCode = 5
	ErrRequestTimeout    ErrorCode = 6
	ErrStorageError      ErrorCode = 7
	ErrTopicNotFound     ErrorCode = 8
	ErrTopicExists       ErrorCode = 9
	ErrChecksumMismatch  ErrorCode = 10
	ErrInvalidProtocol   ErrorCode = 11
	ErrMessageTooLarge   ErrorCode = 12
	// Consumer group error codes
	ErrUnknownMemberID           ErrorCode = 20
	ErrInvalidSessionTimeout     ErrorCode = 21
	ErrRebalanceInProgress       ErrorCode = 22
	ErrInvalidGenerationID       ErrorCode = 23
	ErrUnknownConsumerGroupID    ErrorCode = 24
	ErrNotCoordinator            ErrorCode = 25
	ErrInvalidCommitOffsetSize   ErrorCode = 26
	ErrGroupAuthorizationFailed  ErrorCode = 27
	ErrIllegalGeneration         ErrorCode = 28
	ErrInconsistentGroupProtocol ErrorCode = 29
	// Transaction error codes
	ErrInvalidProducerEpoch                 ErrorCode = 30
	ErrInvalidTransactionState              ErrorCode = 31
	ErrInvalidProducerIDMapping             ErrorCode = 32
	ErrTransactionCoordinatorNotAvailable   ErrorCode = 33
	ErrTransactionCoordinatorFenced         ErrorCode = 34
	ErrProducerFenced                       ErrorCode = 35
	ErrInvalidTransactionTimeout            ErrorCode = 36
	ErrConcurrentTransactions               ErrorCode = 37
	ErrTransactionAborted                   ErrorCode = 38
	ErrInvalidPartitionList                 ErrorCode = 39
	// Security error codes
	ErrAuthenticationFailed                 ErrorCode = 40
	ErrAuthorizationFailed                  ErrorCode = 41
	ErrInvalidCredentials                   ErrorCode = 42
	ErrAccountDisabled                      ErrorCode = 43
)

// String returns the string representation of ErrorCode
func (e ErrorCode) String() string {
	switch e {
	case ErrNone:
		return "None"
	case ErrUnknownRequest:
		return "UnknownRequest"
	case ErrInvalidRequest:
		return "InvalidRequest"
	case ErrOffsetOutOfRange:
		return "OffsetOutOfRange"
	case ErrCorruptMessage:
		return "CorruptMessage"
	case ErrPartitionNotFound:
		return "PartitionNotFound"
	case ErrRequestTimeout:
		return "RequestTimeout"
	case ErrStorageError:
		return "StorageError"
	case ErrTopicNotFound:
		return "TopicNotFound"
	case ErrTopicExists:
		return "TopicExists"
	case ErrChecksumMismatch:
		return "ChecksumMismatch"
	case ErrInvalidProtocol:
		return "InvalidProtocol"
	case ErrMessageTooLarge:
		return "MessageTooLarge"
	case ErrUnknownMemberID:
		return "UnknownMemberID"
	case ErrInvalidSessionTimeout:
		return "InvalidSessionTimeout"
	case ErrRebalanceInProgress:
		return "RebalanceInProgress"
	case ErrInvalidGenerationID:
		return "InvalidGenerationID"
	case ErrUnknownConsumerGroupID:
		return "UnknownConsumerGroupID"
	case ErrNotCoordinator:
		return "NotCoordinator"
	case ErrInvalidCommitOffsetSize:
		return "InvalidCommitOffsetSize"
	case ErrGroupAuthorizationFailed:
		return "GroupAuthorizationFailed"
	case ErrIllegalGeneration:
		return "IllegalGeneration"
	case ErrInconsistentGroupProtocol:
		return "InconsistentGroupProtocol"
	case ErrInvalidProducerEpoch:
		return "InvalidProducerEpoch"
	case ErrInvalidTransactionState:
		return "InvalidTransactionState"
	case ErrInvalidProducerIDMapping:
		return "InvalidProducerIDMapping"
	case ErrTransactionCoordinatorNotAvailable:
		return "TransactionCoordinatorNotAvailable"
	case ErrTransactionCoordinatorFenced:
		return "TransactionCoordinatorFenced"
	case ErrProducerFenced:
		return "ProducerFenced"
	case ErrInvalidTransactionTimeout:
		return "InvalidTransactionTimeout"
	case ErrConcurrentTransactions:
		return "ConcurrentTransactions"
	case ErrTransactionAborted:
		return "TransactionAborted"
	case ErrInvalidPartitionList:
		return "InvalidPartitionList"
	case ErrAuthenticationFailed:
		return "AuthenticationFailed"
	case ErrAuthorizationFailed:
		return "AuthorizationFailed"
	case ErrInvalidCredentials:
		return "InvalidCredentials"
	case ErrAccountDisabled:
		return "AccountDisabled"
	default:
		return fmt.Sprintf("Unknown(%d)", e)
	}
}

// Error returns the error message
func (e ErrorCode) Error() string {
	return e.String()
}

// RequestFlags represents request flags
type RequestFlags uint16

const (
	FlagNone           RequestFlags = 0
	FlagRequireAck     RequestFlags = 1 << 0 // Require acknowledgment
	FlagCompressed     RequestFlags = 1 << 1 // Payload is compressed
	FlagBatch          RequestFlags = 1 << 2 // Batch request
	FlagAsync          RequestFlags = 1 << 3 // Async request (fire and forget)
	FlagIdempotent     RequestFlags = 1 << 4 // Idempotent request
)

// RequestHeader represents the request header
type RequestHeader struct {
	Length    uint32       // Total message length (excluding length field)
	RequestID uint64       // Unique request identifier
	Type      RequestType  // Request type
	Version   byte         // Protocol version
	Flags     RequestFlags // Request flags
}

// ResponseHeader represents the response header
type ResponseHeader struct {
	Length    uint32     // Total message length (excluding length field)
	RequestID uint64     // Matches request ID
	Status    StatusCode // Response status
	ErrorCode ErrorCode  // Error code if status != OK
}

// Message represents a single message
type Message struct {
	Offset    int64             // Message offset (set by server)
	Key       []byte            // Message key (optional)
	Value     []byte            // Message value
	Headers   map[string][]byte // Message headers
	Timestamp int64             // Unix timestamp (nanoseconds)
}

// Size returns the serialized size of the message
func (m *Message) Size() int {
	size := 8 + 4 + len(m.Key) + 4 + len(m.Value) + 8 + 4 // Offset + KeyLen + Key + ValueLen + Value + Timestamp + NumHeaders
	for k, v := range m.Headers {
		size += 4 + len(k) + 4 + len(v) // KeyLen + Key + ValueLen + Value
	}
	return size
}
