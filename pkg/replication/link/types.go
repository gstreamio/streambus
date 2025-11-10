package link

import (
	"time"
)

// ReplicationType defines the type of cross-datacenter replication topology
type ReplicationType string

const (
	// ReplicationTypeActivePassive is unidirectional replication from source to target
	ReplicationTypeActivePassive ReplicationType = "active-passive"

	// ReplicationTypeActiveActive is bidirectional replication between clusters
	ReplicationTypeActiveActive ReplicationType = "active-active"

	// ReplicationTypeStar is hub-and-spoke replication with one central cluster
	ReplicationTypeStar ReplicationType = "star"
)

// ReplicationStatus represents the current state of a replication link
type ReplicationStatus string

const (
	// ReplicationStatusActive indicates replication is running normally
	ReplicationStatusActive ReplicationStatus = "active"

	// ReplicationStatusPaused indicates replication is temporarily paused
	ReplicationStatusPaused ReplicationStatus = "paused"

	// ReplicationStatusFailed indicates replication has encountered errors
	ReplicationStatusFailed ReplicationStatus = "failed"

	// ReplicationStatusStopped indicates replication has been stopped
	ReplicationStatusStopped ReplicationStatus = "stopped"

	// ReplicationStatusInitializing indicates initial sync is in progress
	ReplicationStatusInitializing ReplicationStatus = "initializing"
)

// ReplicationLink represents a replication connection between two clusters
type ReplicationLink struct {
	// ID is the unique identifier for this replication link
	ID string

	// Name is a human-readable name for this link
	Name string

	// Type defines the replication topology
	Type ReplicationType

	// SourceCluster is the source cluster configuration
	SourceCluster ClusterConfig

	// TargetCluster is the target cluster configuration
	TargetCluster ClusterConfig

	// Topics is the list of topics to replicate (empty = all topics)
	Topics []string

	// TopicPrefix is the prefix to add to replicated topic names (optional)
	TopicPrefix string

	// TopicConfig contains per-topic replication settings
	TopicConfig map[string]*TopicReplicationConfig

	// Filter defines message filtering rules
	Filter *FilterConfig

	// Transform defines message transformation rules
	Transform *TransformConfig

	// Config contains replication performance tuning
	Config ReplicationConfig

	// Status is the current status of the replication link
	Status ReplicationStatus

	// CreatedAt is when the link was created
	CreatedAt time.Time

	// UpdatedAt is when the link was last updated
	UpdatedAt time.Time

	// StartedAt is when replication started
	StartedAt time.Time

	// StoppedAt is when replication stopped
	StoppedAt time.Time

	// Metrics contains replication metrics
	Metrics *ReplicationMetrics

	// Health contains health status information
	Health *ReplicationHealth

	// FailoverConfig contains automatic failover settings
	FailoverConfig *FailoverConfig
}

// ClusterConfig represents connection details for a cluster
type ClusterConfig struct {
	// ClusterID is the unique identifier for the cluster
	ClusterID string

	// Brokers is the list of broker addresses (host:port)
	Brokers []string

	// BootstrapServers is a comma-separated list of broker addresses
	BootstrapServers string

	// Security contains authentication and encryption settings
	Security *SecurityConfig

	// ConnectionTimeout is the timeout for establishing connections
	ConnectionTimeout time.Duration

	// RequestTimeout is the timeout for individual requests
	RequestTimeout time.Duration

	// RetryBackoff is the backoff duration between retries
	RetryBackoff time.Duration

	// MaxRetries is the maximum number of retries
	MaxRetries int
}

// SecurityConfig contains security settings for cluster connections
type SecurityConfig struct {
	// EnableTLS enables TLS encryption
	EnableTLS bool

	// TLSCertFile is the path to the TLS certificate file
	TLSCertFile string

	// TLSKeyFile is the path to the TLS key file
	TLSKeyFile string

	// TLSCAFile is the path to the CA certificate file
	TLSCAFile string

	// TLSSkipVerify skips TLS certificate verification (insecure)
	TLSSkipVerify bool

	// SASLMechanism is the SASL authentication mechanism (PLAIN, SCRAM-SHA-256, etc.)
	SASLMechanism string

	// SASLUsername is the SASL username
	SASLUsername string

	// SASLPassword is the SASL password
	SASLPassword string
}

// TopicReplicationConfig contains per-topic replication settings
type TopicReplicationConfig struct {
	// SourceTopic is the name of the topic in the source cluster
	SourceTopic string

	// TargetTopic is the name of the topic in the target cluster
	TargetTopic string

	// Enabled indicates if replication is enabled for this topic
	Enabled bool

	// ReplicateDeletes determines if delete markers are replicated
	ReplicateDeletes bool

	// PreservePartitionCount maintains the same partition count
	PreservePartitionCount bool

	// PreserveTimestamps maintains original message timestamps
	PreserveTimestamps bool

	// CompressionType specifies compression for replicated messages
	CompressionType string

	// Priority sets the replication priority (higher = more resources)
	Priority int
}

// FilterConfig defines message filtering rules
type FilterConfig struct {
	// Enabled indicates if filtering is enabled
	Enabled bool

	// IncludePatterns are regex patterns for messages to include
	IncludePatterns []string

	// ExcludePatterns are regex patterns for messages to exclude
	ExcludePatterns []string

	// FilterByHeader filters messages based on header values
	FilterByHeader map[string]string

	// MinTimestamp filters messages older than this timestamp
	MinTimestamp time.Time

	// MaxTimestamp filters messages newer than this timestamp
	MaxTimestamp time.Time
}

// TransformConfig defines message transformation rules
type TransformConfig struct {
	// Enabled indicates if transformation is enabled
	Enabled bool

	// HeaderTransforms are header transformation rules
	HeaderTransforms map[string]string

	// KeyTransform is a transformation expression for message keys
	KeyTransform string

	// ValueTransform is a transformation expression for message values
	ValueTransform string

	// AddHeaders adds static headers to all messages
	AddHeaders map[string]string

	// RemoveHeaders removes headers from messages
	RemoveHeaders []string
}

// ReplicationConfig contains performance tuning parameters
type ReplicationConfig struct {
	// MaxBytes is the maximum bytes to fetch per request
	MaxBytes int64

	// MaxMessages is the maximum messages to fetch per request
	MaxMessages int

	// FetchWaitMaxMs is the max time to wait for fetch
	FetchWaitMaxMs int

	// MinBytes is the minimum bytes to accumulate before fetch
	MinBytes int

	// BatchSize is the batch size for replication
	BatchSize int

	// BufferSize is the internal buffer size
	BufferSize int

	// ConcurrentPartitions is the number of partitions to replicate concurrently
	ConcurrentPartitions int

	// EnableCompression enables compression for replication traffic
	EnableCompression bool

	// CompressionType is the compression algorithm (gzip, snappy, lz4, zstd)
	CompressionType string

	// ThrottleRateBytesPerSec limits replication throughput (0 = unlimited)
	ThrottleRateBytesPerSec int64

	// CheckpointIntervalMs is how often to checkpoint offsets
	CheckpointIntervalMs int64

	// SyncIntervalMs is how often to sync metadata
	SyncIntervalMs int64

	// HeartbeatIntervalMs is how often to send heartbeats
	HeartbeatIntervalMs int64

	// EnableExactlyOnce enables exactly-once semantics
	EnableExactlyOnce bool

	// EnableIdempotence enables idempotent producer
	EnableIdempotence bool
}

// ReplicationMetrics contains replication metrics and statistics
type ReplicationMetrics struct {
	// TotalMessagesReplicated is the total number of messages replicated
	TotalMessagesReplicated int64

	// TotalBytesReplicated is the total bytes replicated
	TotalBytesReplicated int64

	// MessagesPerSecond is the current replication rate
	MessagesPerSecond float64

	// BytesPerSecond is the current byte rate
	BytesPerSecond float64

	// ReplicationLag is the lag in milliseconds
	ReplicationLag int64

	// AverageReplicationLag is the average lag over time
	AverageReplicationLag int64

	// MaxReplicationLag is the maximum observed lag
	MaxReplicationLag int64

	// TotalErrors is the total number of errors
	TotalErrors int64

	// ErrorsPerSecond is the current error rate
	ErrorsPerSecond float64

	// LastCheckpoint is the timestamp of the last checkpoint
	LastCheckpoint time.Time

	// LastSuccessfulReplication is the timestamp of last successful replication
	LastSuccessfulReplication time.Time

	// PartitionMetrics contains per-partition metrics
	PartitionMetrics map[string]*PartitionReplicationMetrics

	// ConsecutiveFailures is the number of consecutive failures
	ConsecutiveFailures int

	// UptimeSeconds is the total uptime in seconds
	UptimeSeconds int64
}

// PartitionReplicationMetrics contains per-partition replication metrics
type PartitionReplicationMetrics struct {
	// Topic is the topic name
	Topic string

	// Partition is the partition ID
	Partition int32

	// SourceOffset is the current offset in source cluster
	SourceOffset int64

	// TargetOffset is the current offset in target cluster
	TargetOffset int64

	// Lag is the replication lag for this partition
	Lag int64

	// MessagesReplicated is the total messages replicated
	MessagesReplicated int64

	// BytesReplicated is the total bytes replicated
	BytesReplicated int64

	// LastReplicatedAt is when the last message was replicated
	LastReplicatedAt time.Time

	// Errors is the number of errors for this partition
	Errors int64
}

// Checkpoint represents a replication checkpoint
type Checkpoint struct {
	// LinkID is the replication link ID
	LinkID string

	// Topic is the topic name
	Topic string

	// Partition is the partition ID
	Partition int32

	// SourceOffset is the offset in the source cluster
	SourceOffset int64

	// TargetOffset is the offset in the target cluster
	TargetOffset int64

	// Timestamp is when the checkpoint was created
	Timestamp time.Time

	// Metadata contains additional checkpoint metadata
	Metadata map[string]string
}

// OffsetMapping maps offsets between source and target clusters
type OffsetMapping struct {
	// LinkID is the replication link ID
	LinkID string

	// Topic is the topic name
	Topic string

	// Partition is the partition ID
	Partition int32

	// Mappings maps source offsets to target offsets
	Mappings map[int64]int64

	// LastUpdated is when the mapping was last updated
	LastUpdated time.Time
}

// FailoverConfig contains automatic failover settings
type FailoverConfig struct {
	// Enabled indicates if automatic failover is enabled
	Enabled bool

	// FailoverThreshold is the lag threshold that triggers failover
	FailoverThreshold int64

	// FailoverTimeoutMs is the timeout before declaring failure
	FailoverTimeoutMs int64

	// MaxConsecutiveFailures is the max failures before failover
	MaxConsecutiveFailures int

	// AutoFailback enables automatic failback when primary recovers
	AutoFailback bool

	// FailbackDelayMs is the delay before attempting failback
	FailbackDelayMs int64

	// NotificationWebhook is a webhook URL for failover notifications
	NotificationWebhook string

	// NotificationEmail is an email address for failover notifications
	NotificationEmail string
}

// FailoverEvent represents a failover event
type FailoverEvent struct {
	// ID is the unique identifier for this event
	ID string

	// LinkID is the replication link ID
	LinkID string

	// Type is the event type (failover, failback, manual-switch)
	Type string

	// Reason is why the failover occurred
	Reason string

	// SourceClusterID is the cluster being failed over from
	SourceClusterID string

	// TargetClusterID is the cluster being failed over to
	TargetClusterID string

	// Timestamp is when the event occurred
	Timestamp time.Time

	// Success indicates if the failover was successful
	Success bool

	// ErrorMessage contains error details if failed
	ErrorMessage string

	// OffsetMappings contains offset mappings at time of failover
	OffsetMappings map[string]*OffsetMapping

	// Duration is how long the failover took
	Duration time.Duration
}

// ReplicationHealth represents health status of a replication link
type ReplicationHealth struct {
	// Status is the overall health status
	Status string // "healthy", "degraded", "unhealthy"

	// LastHealthCheck is when health was last checked
	LastHealthCheck time.Time

	// SourceClusterReachable indicates if source cluster is reachable
	SourceClusterReachable bool

	// TargetClusterReachable indicates if target cluster is reachable
	TargetClusterReachable bool

	// ReplicationLagHealthy indicates if lag is within acceptable range
	ReplicationLagHealthy bool

	// ErrorRateHealthy indicates if error rate is acceptable
	ErrorRateHealthy bool

	// CheckpointHealthy indicates if checkpointing is working
	CheckpointHealthy bool

	// Issues contains descriptions of any health issues
	Issues []string

	// Warnings contains non-critical warnings
	Warnings []string
}

// Manager manages all cross-datacenter replication links
type Manager interface {
	// CreateLink creates a new replication link
	CreateLink(config *ReplicationLink) error

	// DeleteLink deletes a replication link
	DeleteLink(linkID string) error

	// UpdateLink updates a replication link configuration
	UpdateLink(linkID string, config *ReplicationLink) error

	// GetLink retrieves a replication link
	GetLink(linkID string) (*ReplicationLink, error)

	// ListLinks lists all replication links
	ListLinks() ([]*ReplicationLink, error)

	// StartLink starts a replication link
	StartLink(linkID string) error

	// StopLink stops a replication link
	StopLink(linkID string) error

	// PauseLink pauses a replication link
	PauseLink(linkID string) error

	// ResumeLink resumes a paused replication link
	ResumeLink(linkID string) error

	// GetMetrics retrieves metrics for a replication link
	GetMetrics(linkID string) (*ReplicationMetrics, error)

	// GetHealth retrieves health status for a replication link
	GetHealth(linkID string) (*ReplicationHealth, error)

	// Failover triggers a manual failover
	Failover(linkID string) (*FailoverEvent, error)

	// Failback triggers a manual failback
	Failback(linkID string) (*FailoverEvent, error)

	// GetCheckpoint retrieves the checkpoint for a topic-partition
	GetCheckpoint(linkID, topic string, partition int32) (*Checkpoint, error)

	// SetCheckpoint sets the checkpoint for a topic-partition
	SetCheckpoint(checkpoint *Checkpoint) error

	// Close closes the replication manager
	Close() error
}
