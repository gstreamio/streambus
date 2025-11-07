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
	RequestTypeProduce     RequestType = 0x01
	RequestTypeFetch       RequestType = 0x02
	RequestTypeGetOffset   RequestType = 0x03
	RequestTypeCreateTopic RequestType = 0x04
	RequestTypeDeleteTopic RequestType = 0x05
	RequestTypeListTopics  RequestType = 0x06
	RequestTypeHealthCheck RequestType = 0x07
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
