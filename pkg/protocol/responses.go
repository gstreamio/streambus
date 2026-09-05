package protocol

// Response represents a protocol response
type Response struct {
	Header  ResponseHeader
	Payload interface{}
}

// ProduceResponse represents a produce response
type ProduceResponse struct {
	Topic         string
	PartitionID   uint32
	BaseOffset    int64  // First offset assigned
	NumMessages   uint32 // Number of messages written
	HighWaterMark int64  // Current high water mark
	// LeaderEpoch is the current leader epoch for this partition.
	// Clients should cache this and include it in subsequent requests.
	LeaderEpoch int64
}

// FetchResponse represents a fetch response
type FetchResponse struct {
	Topic         string
	PartitionID   uint32
	HighWaterMark int64
	Messages      []Message
	// LastStableOffset is the offset up to which a read-committed fetch is
	// allowed to read: the start of the earliest transaction on this
	// partition that has not yet had its marker written, or HighWaterMark if
	// none is open. A server predating this field is decoded as reporting
	// HighWaterMark here, i.e. no additional constraint.
	LastStableOffset int64
	// NextOffset is the offset the client should fetch from next. It is not
	// always LastMessage.Offset+1: control records are filtered out of
	// Messages before the client ever sees them, so a fetch window that
	// contained only a marker returns no messages but must still advance
	// the client past it. A server predating this field is decoded as -1,
	// a sentinel telling the client to fall back to the legacy
	// last-message-plus-one rule, which was correct before filtering existed.
	NextOffset int64
}

// GetOffsetResponse represents a get offset response
type GetOffsetResponse struct {
	Topic         string
	PartitionID   uint32
	StartOffset   int64 // First available offset (log start)
	EndOffset     int64 // Next offset to be assigned (log end)
	HighWaterMark int64 // Last committed offset + 1
	// Offset is the result of a timestamp-based query.
	// For OffsetLatest: returns EndOffset
	// For OffsetEarliest: returns StartOffset
	// For timestamp query: returns first offset >= requested timestamp
	Offset int64
	// Timestamp is the timestamp of the message at Offset (Unix nanoseconds).
	// Only populated for timestamp-based queries.
	Timestamp int64
	// LeaderEpoch is the current leader epoch for this partition.
	LeaderEpoch int64
}

// CreateTopicResponse represents a create topic response
type CreateTopicResponse struct {
	Topic     string
	Created   bool
	ErrorCode ErrorCode
}

// DeleteTopicResponse represents a delete topic response
type DeleteTopicResponse struct {
	Topic     string
	Deleted   bool
	ErrorCode ErrorCode
}

// ListTopicsResponse represents a list topics response
type ListTopicsResponse struct {
	Topics []TopicInfo
}

// TopicInfo represents information about a topic
type TopicInfo struct {
	Name          string
	NumPartitions uint32
}

// HealthCheckResponse represents a health check response
type HealthCheckResponse struct {
	Status string // "healthy" or "unhealthy"
	Uptime int64  // Uptime in seconds
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	ErrorCode ErrorCode
	Message   string
}
