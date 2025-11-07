package protocol

// Response represents a protocol response
type Response struct {
	Header  ResponseHeader
	Payload interface{}
}

// ProduceResponse represents a produce response
type ProduceResponse struct {
	Topic          string
	PartitionID    uint32
	BaseOffset     int64  // First offset assigned
	NumMessages    uint32 // Number of messages written
	HighWaterMark  int64  // Current high water mark
}

// FetchResponse represents a fetch response
type FetchResponse struct {
	Topic          string
	PartitionID    uint32
	HighWaterMark  int64
	Messages       []Message
}

// GetOffsetResponse represents a get offset response
type GetOffsetResponse struct {
	Topic          string
	PartitionID    uint32
	StartOffset    int64
	EndOffset      int64
	HighWaterMark  int64
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
