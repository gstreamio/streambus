package group

import (
	"time"
)

// GroupState represents the state of a consumer group
type GroupState string

const (
	// GroupStateEmpty means the group has no members
	GroupStateEmpty GroupState = "Empty"
	// GroupStatePreparingRebalance means the group is preparing for rebalancing
	GroupStatePreparingRebalance GroupState = "PreparingRebalance"
	// GroupStateCompletingRebalance means rebalance assignments are being synced
	GroupStateCompletingRebalance GroupState = "CompletingRebalance"
	// GroupStateStable means the group is stable with active members
	GroupStateStable GroupState = "Stable"
	// GroupStateDead means the group has no members and is being deleted
	GroupStateDead GroupState = "Dead"
)

// MemberState represents the state of a group member
type MemberState string

const (
	// MemberStateJoining means member is joining the group
	MemberStateJoining MemberState = "Joining"
	// MemberStateAwaitingSync means member is waiting for assignment
	MemberStateAwaitingSync MemberState = "AwaitingSync"
	// MemberStateStable means member is active with assignment
	MemberStateStable MemberState = "Stable"
	// MemberStateLeaving means member is leaving the group
	MemberStateLeaving MemberState = "Leaving"
)

// GroupMetadata represents metadata about a consumer group
type GroupMetadata struct {
	// Group identifier
	GroupID string

	// Current group state
	State GroupState

	// Protocol type (e.g., "consumer")
	ProtocolType string

	// Protocol name (e.g., "range", "roundrobin")
	ProtocolName string

	// Group leader member ID
	LeaderID string

	// Generation ID (incremented on each rebalance)
	GenerationID int32

	// Members in the group
	Members map[string]*MemberMetadata

	// Timestamp of state change
	StateTimestamp time.Time

	// Rebalance timeout
	RebalanceTimeoutMs int32

	// Session timeout
	SessionTimeoutMs int32
}

// MemberMetadata represents metadata about a group member
type MemberMetadata struct {
	// Unique member identifier
	MemberID string

	// Client ID for logging
	ClientID string

	// Client host
	ClientHost string

	// Member state
	State MemberState

	// Topics the member is subscribed to
	Subscription []string

	// Current partition assignment
	Assignment *MemberAssignment

	// Protocol metadata provided by member
	ProtocolMetadata []byte

	// Timestamp of last heartbeat
	LastHeartbeat time.Time

	// Session timeout in milliseconds
	SessionTimeoutMs int32

	// Rebalance timeout in milliseconds
	RebalanceTimeoutMs int32

	// Join timestamp
	JoinTime time.Time
}

// MemberAssignment represents partition assignments for a member
type MemberAssignment struct {
	// Version of assignment
	Version int16

	// Partition assignments by topic
	Partitions map[string][]int32

	// User data (for custom assignment strategies)
	UserData []byte
}

// OffsetAndMetadata represents a committed offset with metadata
type OffsetAndMetadata struct {
	// Committed offset
	Offset int64

	// Metadata string (optional)
	Metadata string

	// Commit timestamp
	CommitTime time.Time

	// Expiration timestamp (for cleanup)
	ExpireTime time.Time
}

// GroupOffsets represents all committed offsets for a group
type GroupOffsets struct {
	// Group identifier
	GroupID string

	// Offsets by topic and partition
	// Map structure: topic -> partition -> OffsetAndMetadata
	Offsets map[string]map[int32]*OffsetAndMetadata
}

// JoinGroupRequest represents a request to join a consumer group
type JoinGroupRequest struct {
	// Group identifier
	GroupID string

	// Session timeout in milliseconds
	SessionTimeoutMs int32

	// Rebalance timeout in milliseconds
	RebalanceTimeoutMs int32

	// Member identifier (empty for new members)
	MemberID string

	// Client identifier
	ClientID string

	// Protocol type (e.g., "consumer")
	ProtocolType string

	// List of supported protocols and their metadata
	Protocols []ProtocolMetadata
}

// ProtocolMetadata represents protocol and its metadata
type ProtocolMetadata struct {
	// Protocol name (e.g., "range", "roundrobin")
	Name string

	// Protocol-specific metadata
	Metadata []byte
}

// JoinGroupResponse represents response to join group request
type JoinGroupResponse struct {
	// Error code
	ErrorCode int16

	// Generation ID
	GenerationID int32

	// Selected protocol name
	ProtocolName string

	// Assigned member ID
	MemberID string

	// Leader member ID
	LeaderID string

	// All members (only populated for leader)
	Members []JoinGroupMember
}

// JoinGroupMember represents a member in join response
type JoinGroupMember struct {
	MemberID string
	Metadata []byte
}

// SyncGroupRequest represents a request to sync group assignment
type SyncGroupRequest struct {
	// Group identifier
	GroupID string

	// Generation ID
	GenerationID int32

	// Member identifier
	MemberID string

	// Assignments (only provided by leader)
	Assignments []MemberAssignmentData
}

// MemberAssignmentData represents assignment for a member
type MemberAssignmentData struct {
	MemberID   string
	Assignment []byte
}

// SyncGroupResponse represents response to sync group request
type SyncGroupResponse struct {
	// Error code
	ErrorCode int16

	// Member assignment
	Assignment []byte
}

// HeartbeatRequest represents a heartbeat from group member
type HeartbeatRequest struct {
	// Group identifier
	GroupID string

	// Generation ID
	GenerationID int32

	// Member identifier
	MemberID string
}

// HeartbeatResponse represents response to heartbeat
type HeartbeatResponse struct {
	// Error code
	ErrorCode int16
}

// LeaveGroupRequest represents a request to leave group
type LeaveGroupRequest struct {
	// Group identifier
	GroupID string

	// Member identifier
	MemberID string
}

// LeaveGroupResponse represents response to leave group
type LeaveGroupResponse struct {
	// Error code
	ErrorCode int16
}

// OffsetCommitRequest represents a request to commit offsets
type OffsetCommitRequest struct {
	// Group identifier
	GroupID string

	// Generation ID (-1 for simple consumer)
	GenerationID int32

	// Member identifier
	MemberID string

	// Offsets to commit by topic and partition
	Offsets map[string]map[int32]OffsetCommitData
}

// OffsetCommitData represents data for committing an offset
type OffsetCommitData struct {
	Offset   int64
	Metadata string
}

// OffsetCommitResponse represents response to offset commit
type OffsetCommitResponse struct {
	// Errors by topic and partition
	Errors map[string]map[int32]int16
}

// OffsetFetchRequest represents a request to fetch committed offsets
type OffsetFetchRequest struct {
	// Group identifier
	GroupID string

	// Topics and partitions to fetch (nil means all)
	Topics map[string][]int32
}

// OffsetFetchResponse represents response to offset fetch
type OffsetFetchResponse struct {
	// Offsets by topic and partition
	Offsets map[string]map[int32]OffsetFetchData
}

// OffsetFetchData represents fetched offset data
type OffsetFetchData struct {
	Offset    int64
	Metadata  string
	ErrorCode int16
}

// ErrorCode constants
const (
	ErrorCodeNone                     int16 = 0
	ErrorCodeUnknown                  int16 = -1
	ErrorCodeIllegalGeneration        int16 = 22
	ErrorCodeUnknownMemberID          int16 = 25
	ErrorCodeRebalanceInProgress      int16 = 27
	ErrorCodeInvalidSessionTimeout    int16 = 26
	ErrorCodeGroupIDNotFound          int16 = 69
	ErrorCodeGroupAuthorizationFailed int16 = 30
)
