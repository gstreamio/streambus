package replication

import (
	"time"
)

// ReplicaID represents a unique identifier for a broker replica
type ReplicaID uint64

// Offset represents a message offset in a partition log
type Offset int64

// ReplicationState tracks the replication state of a follower replica
type ReplicationState struct {
	// ReplicaID is the broker ID of this replica
	ReplicaID ReplicaID

	// LogEndOffset (LEO) is the offset of the last message in the replica's log
	// This represents the next offset to be written
	LogEndOffset Offset

	// HighWaterMark (HW) is the offset up to which messages are guaranteed
	// to be replicated to all ISR members. Only messages below HW are visible to consumers.
	HighWaterMark Offset

	// LastFetchTime is the timestamp of the last successful fetch from leader
	LastFetchTime time.Time

	// LastCaughtUpTime is the timestamp when this replica was last fully caught up
	LastCaughtUpTime time.Time

	// FetchLag is the number of messages this replica is behind the leader
	FetchLag int64

	// InSync indicates if this replica is in the ISR
	InSync bool
}

// PartitionReplicationState tracks replication state for an entire partition
type PartitionReplicationState struct {
	// Topic name
	Topic string

	// PartitionID within the topic
	PartitionID int

	// Leader broker ID
	Leader ReplicaID

	// LeaderEpoch increments on each leader change to prevent stale reads
	LeaderEpoch int64

	// Replicas is the full list of replicas (leader + followers)
	Replicas []ReplicaID

	// ISR (In-Sync Replicas) are replicas within acceptable lag limits
	ISR []ReplicaID

	// ReplicaStates maps each replica to its replication state
	ReplicaStates map[ReplicaID]*ReplicationState

	// HighWaterMark is the partition-wide HW (minimum LEO of all ISR members)
	HighWaterMark Offset

	// LogEndOffset is the leader's LEO
	LogEndOffset Offset
}

// FetchRequest is sent by a follower to fetch messages from the leader
type FetchRequest struct {
	// ReplicaID of the follower making the request
	ReplicaID ReplicaID

	// Topic to fetch from
	Topic string

	// PartitionID to fetch from
	PartitionID int

	// FetchOffset is the offset to start fetching from
	FetchOffset Offset

	// MaxBytes is the maximum bytes to return
	MaxBytes int

	// LeaderEpoch is the epoch the follower believes the leader to be in
	// Used to detect stale leaders
	LeaderEpoch int64

	// MinBytes is the minimum bytes to return (for long-polling)
	MinBytes int

	// MaxWaitMs is the maximum time to wait for MinBytes
	MaxWaitMs int
}

// FetchResponse is the leader's response to a FetchRequest
type FetchResponse struct {
	// Topic being fetched
	Topic string

	// PartitionID being fetched
	PartitionID int

	// LeaderEpoch of the current leader
	LeaderEpoch int64

	// HighWaterMark of the partition
	HighWaterMark Offset

	// LogEndOffset (LEO) of the leader
	LogEndOffset Offset

	// Messages fetched
	Messages []*Message

	// Error indicates any error during fetch
	Error error

	// ErrorCode for specific error types
	ErrorCode ErrorCode
}

// Message represents a single message in the replication protocol
type Message struct {
	// Offset of the message in the log
	Offset Offset

	// Key (optional)
	Key []byte

	// Value is the message payload
	Value []byte

	// Timestamp in Unix nanoseconds
	Timestamp int64

	// Headers (optional metadata)
	Headers map[string][]byte
}

// ErrorCode represents replication-specific errors
type ErrorCode int

const (
	ErrorNone ErrorCode = iota
	ErrorNotLeader
	ErrorOffsetOutOfRange
	ErrorStaleEpoch
	ErrorPartitionNotFound
	ErrorReplicaNotInReplicas
	ErrorUnknown
)

// String returns the string representation of ErrorCode
func (e ErrorCode) String() string {
	switch e {
	case ErrorNone:
		return "None"
	case ErrorNotLeader:
		return "NotLeader"
	case ErrorOffsetOutOfRange:
		return "OffsetOutOfRange"
	case ErrorStaleEpoch:
		return "StaleEpoch"
	case ErrorPartitionNotFound:
		return "PartitionNotFound"
	case ErrorReplicaNotInReplicas:
		return "ReplicaNotInReplicas"
	case ErrorUnknown:
		return "Unknown"
	default:
		return "Unknown"
	}
}

// ISRChangeNotification represents a change in ISR membership
type ISRChangeNotification struct {
	Topic       string
	PartitionID int
	OldISR      []ReplicaID
	NewISR      []ReplicaID
	Reason      string
	Timestamp   time.Time
}

// ReplicationMetrics tracks replication performance metrics
type ReplicationMetrics struct {
	// FetchRequestRate is fetches per second
	FetchRequestRate float64

	// FetchBytesRate is bytes fetched per second
	FetchBytesRate float64

	// ReplicationLagMessages is number of messages behind leader
	ReplicationLagMessages int64

	// ReplicationLagMs is time lag in milliseconds
	ReplicationLagMs int64

	// LastFetchLatencyMs is latency of last fetch in milliseconds
	LastFetchLatencyMs int64

	// ISRShrinkRate is ISR shrinks per minute
	ISRShrinkRate float64

	// ISRExpandRate is ISR expands per minute
	ISRExpandRate float64
}
