package protocol

// Request represents a protocol request
type Request struct {
	Header  RequestHeader
	Payload interface{}
}

// AcksLevel represents the acknowledgment level for produce requests
type AcksLevel int16

const (
	AcksNone AcksLevel = 0  // No acknowledgment (fire and forget)
	AcksOne  AcksLevel = 1  // Acknowledgment from leader only
	AcksAll  AcksLevel = -1 // Acknowledgment from all ISR members
)

// ProduceRequest represents a produce request
type ProduceRequest struct {
	Topic       string
	PartitionID uint32
	Messages    []Message
	Acks        AcksLevel // Acknowledgment level (0, 1, or -1)
	TimeoutMs   int32     // Timeout for acks=all (default: 30000)
	// LeaderEpoch is the expected leader epoch for this partition.
	// If set (> 0) and doesn't match current leader epoch, request is rejected
	// with ErrFencedLeaderEpoch. This prevents split-brain scenarios.
	LeaderEpoch int64
	// ProducerID and ProducerEpoch identify the transactional producer this
	// batch belongs to, mirroring storage.MessageBatch. Zero means the batch
	// is not part of a transaction: that is both the default for a caller
	// who never sets these fields and the value an idempotent-only producer
	// sends, so the broker cannot mistake plain writes for transactional
	// ones. A non-zero ProducerID lets the broker track the offset a
	// transaction started at (see Partition.BeginTransaction) so
	// read-committed fetches know where to stop until its marker lands.
	ProducerID    int64
	ProducerEpoch int16
}

// FetchRequest represents a fetch request
type FetchRequest struct {
	Topic       string
	PartitionID uint32
	Offset      int64
	MaxBytes    uint32
	// IsolationLevel selects whether the fetch can see records from
	// transactions that have not yet committed or aborted. It defaults to
	// IsolationReadUncommitted (the zero value), which is also what an older
	// client that predates isolation levels effectively sends.
	IsolationLevel IsolationLevel
}

// GetOffsetRequest represents a get offset request
// Supports timestamp-based offset lookup (Kafka ListOffsets compatible)
type GetOffsetRequest struct {
	Topic       string
	PartitionID uint32
	// Timestamp for offset lookup:
	// - OffsetLatest (-1): returns log end offset
	// - OffsetEarliest (-2): returns log start offset
	// - OffsetMaxTimestamp (-3): returns offset with max timestamp
	// - Positive value: returns first offset >= timestamp (Unix nanoseconds)
	// - 0: returns earliest offset (same as OffsetEarliest)
	Timestamp int64
}

// CreateTopicRequest represents a create topic request
type CreateTopicRequest struct {
	Topic             string
	NumPartitions     uint32
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
