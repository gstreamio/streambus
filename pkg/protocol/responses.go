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
