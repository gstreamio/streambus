package protocol

// Request represents a protocol request
type Request struct {
	Header  RequestHeader
	Payload interface{}
}

// ProduceRequest represents a produce request
type ProduceRequest struct {
	Topic       string
	PartitionID uint32
	Messages    []Message
}

// FetchRequest represents a fetch request
type FetchRequest struct {
	Topic       string
	PartitionID uint32
	Offset      int64
	MaxBytes    uint32
}

// GetOffsetRequest represents a get offset request
type GetOffsetRequest struct {
	Topic       string
	PartitionID uint32
}

// CreateTopicRequest represents a create topic request
type CreateTopicRequest struct {
	Topic          string
	NumPartitions  uint32
	ReplicationFactor uint16
}

// DeleteTopicRequest represents a delete topic request
type DeleteTopicRequest struct {
	Topic string
}

// ListTopicsRequest represents a list topics request
type ListTopicsRequest struct {
	// Empty for now
}

// HealthCheckRequest represents a health check request
type HealthCheckRequest struct {
	// Empty for now
}
