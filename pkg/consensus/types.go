package consensus

import (
	"context"

	"go.etcd.io/raft/v3/raftpb"
)

// Node represents a consensus node in the cluster.
// It wraps the etcd/raft implementation and provides a clean interface
// for StreamBus to interact with the consensus layer.
type Node interface {
	// Propose proposes data to be appended to the Raft log.
	// This is used for any state changes that need to be replicated.
	Propose(ctx context.Context, data []byte) error

	// Start starts the Raft node and begins participating in the cluster.
	Start() error

	// Stop gracefully stops the Raft node.
	Stop() error

	// IsLeader returns true if this node is currently the Raft leader.
	IsLeader() bool

	// Leader returns the ID of the current leader, or 0 if unknown.
	Leader() uint64

	// AddNode adds a new node to the Raft cluster.
	AddNode(ctx context.Context, nodeID uint64, addr string) error

	// RemoveNode removes a node from the Raft cluster.
	RemoveNode(ctx context.Context, nodeID uint64) error

	// Campaign causes the node to transition to candidate state and attempt to become leader.
	Campaign(ctx context.Context) error

	// Ready returns a channel that receives Ready structs when the Raft state changes.
	Ready() <-chan Ready

	// Advance notifies the Raft node that the application has applied the committed entries.
	Advance()
}

// Ready encapsulates the state changes that need to be processed by the application.
// This is similar to raft.Ready but simplified for StreamBus use cases.
type Ready struct {
	// SoftState provides state that is useful for logging and debugging.
	// It does not need to be persisted.
	SoftState *SoftState

	// HardState contains the durable state that must be persisted to stable storage.
	HardState raftpb.HardState

	// Entries specifies entries to be saved to stable storage.
	Entries []raftpb.Entry

	// Snapshot specifies the snapshot to be saved to stable storage.
	Snapshot raftpb.Snapshot

	// CommittedEntries specifies entries to be committed to the state machine.
	CommittedEntries []raftpb.Entry

	// Messages specifies outgoing messages to be sent to other nodes.
	Messages []raftpb.Message

	// MustSync indicates whether the HardState and Entries must be synchronously
	// written to disk before proceeding.
	MustSync bool
}

// SoftState provides state that is useful for logging and debugging.
type SoftState struct {
	Lead      uint64 // Lead is the ID of the current leader
	RaftState StateType
}

// StateType represents the role of the node in the Raft cluster.
type StateType uint64

const (
	StateFollower StateType = iota
	StateCandidate
	StateLeader
)

func (st StateType) String() string {
	switch st {
	case StateFollower:
		return "Follower"
	case StateCandidate:
		return "Candidate"
	case StateLeader:
		return "Leader"
	default:
		return "Unknown"
	}
}

// Storage defines the interface for persisting Raft state.
type Storage interface {
	// InitialState returns the saved HardState and ConfState.
	InitialState() (raftpb.HardState, raftpb.ConfState, error)

	// SaveState saves the current HardState.
	SaveState(st raftpb.HardState) error

	// Entries returns a slice of log entries in the range [lo, hi).
	Entries(lo, hi uint64) ([]raftpb.Entry, error)

	// Term returns the term of entry i.
	Term(i uint64) (uint64, error)

	// LastIndex returns the index of the last entry in the log.
	LastIndex() (uint64, error)

	// FirstIndex returns the index of the first log entry that is available.
	FirstIndex() (uint64, error)

	// Snapshot returns the most recent snapshot.
	Snapshot() (raftpb.Snapshot, error)

	// SaveEntries saves entries to stable storage.
	SaveEntries(entries []raftpb.Entry) error

	// SaveSnapshot saves the snapshot to stable storage.
	SaveSnapshot(snap raftpb.Snapshot) error

	// Compact compacts the log up to compactIndex.
	Compact(compactIndex uint64) error

	// Close closes the storage.
	Close() error
}

// Transport defines the interface for sending Raft messages to other nodes.
type Transport interface {
	// Send sends a message to the specified node.
	Send(msg raftpb.Message) error

	// Start starts the transport layer.
	Start() error

	// Stop stops the transport layer.
	Stop() error

	// AddPeer adds a peer to the transport.
	AddPeer(nodeID uint64, addr string) error

	// RemovePeer removes a peer from the transport.
	RemovePeer(nodeID uint64) error
}

// StateMachine defines the interface for applying committed log entries.
// This is the application-specific logic that processes Raft commands.
type StateMachine interface {
	// Apply applies a committed log entry to the state machine.
	Apply(entry []byte) error

	// Snapshot returns the current state of the state machine as a snapshot.
	Snapshot() ([]byte, error)

	// Restore restores the state machine from a snapshot.
	Restore(snapshot []byte) error
}

// Peer represents a peer node in the Raft cluster.
type Peer struct {
	ID   uint64 // Raft node ID
	Addr string // Network address (host:port)
}
