package consensus

import (
	"fmt"
	"time"
)

// Config holds the configuration for a consensus node.
type Config struct {
	// NodeID is the unique identifier for this node in the cluster.
	// Must be non-zero.
	NodeID uint64

	// Peers is the initial list of peers in the cluster.
	// For bootstrapping a new cluster, include all initial members.
	// For joining an existing cluster, this can be a subset.
	Peers []Peer

	// DataDir is the directory where Raft state is persisted.
	DataDir string

	// TickInterval is the time between Raft ticks.
	// The Raft paper recommends 100-500ms for typical deployments.
	// Default: 100ms
	TickInterval time.Duration

	// ElectionTick is the number of ticks before starting an election.
	// A higher value can reduce election frequency in stable networks.
	// Must be greater than HeartbeatTick.
	// Default: 10 (1 second with 100ms tick interval)
	ElectionTick int

	// HeartbeatTick is the number of ticks between heartbeats.
	// Leaders send heartbeats to maintain their leadership.
	// Default: 1 (100ms with 100ms tick interval)
	HeartbeatTick int

	// MaxSizePerMsg is the maximum size of each Raft message.
	// Default: 1MB
	MaxSizePerMsg uint64

	// MaxInflightMsgs is the maximum number of in-flight append messages.
	// This limits the number of messages being sent to followers at once.
	// Default: 256
	MaxInflightMsgs int

	// SnapshotInterval is the number of log entries between snapshots.
	// A snapshot compacts the Raft log to prevent unbounded growth.
	// Default: 10000
	SnapshotInterval uint64

	// SnapshotCatchUpEntries is the number of entries a follower can lag
	// before the leader sends a snapshot instead of log entries.
	// Default: 5000
	SnapshotCatchUpEntries uint64

	// PreVote enables the pre-vote algorithm to reduce disruptions.
	// When enabled, candidates first check if they would win an election
	// before actually starting one.
	// Default: true
	PreVote bool

	// CheckQuorum enables leader health checking by followers.
	// If the leader doesn't receive a quorum of heartbeat responses,
	// it steps down to follower.
	// Default: true
	CheckQuorum bool

	// DisableProposalForwarding disables proposal forwarding to the leader.
	// When disabled, proposals on non-leaders will be rejected immediately.
	// Default: false
	DisableProposalForwarding bool
}

// DefaultConfig returns a Config with sensible defaults for a typical deployment.
func DefaultConfig() *Config {
	return &Config{
		TickInterval:               100 * time.Millisecond,
		ElectionTick:               10, // 1 second
		HeartbeatTick:              1,  // 100ms
		MaxSizePerMsg:              1024 * 1024,
		MaxInflightMsgs:            256,
		SnapshotInterval:           10000,
		SnapshotCatchUpEntries:     5000,
		PreVote:                    true,
		CheckQuorum:                true,
		DisableProposalForwarding:  false,
	}
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.NodeID == 0 {
		return fmt.Errorf("NodeID must be non-zero")
	}

	if c.DataDir == "" {
		return fmt.Errorf("DataDir must be specified")
	}

	if c.TickInterval <= 0 {
		return fmt.Errorf("TickInterval must be positive")
	}

	if c.ElectionTick <= c.HeartbeatTick {
		return fmt.Errorf("ElectionTick (%d) must be greater than HeartbeatTick (%d)",
			c.ElectionTick, c.HeartbeatTick)
	}

	if c.HeartbeatTick <= 0 {
		return fmt.Errorf("HeartbeatTick must be positive")
	}

	if c.MaxSizePerMsg == 0 {
		return fmt.Errorf("MaxSizePerMsg must be positive")
	}

	if c.MaxInflightMsgs == 0 {
		return fmt.Errorf("MaxInflightMsgs must be positive")
	}

	if c.SnapshotInterval == 0 {
		return fmt.Errorf("SnapshotInterval must be positive")
	}

	if c.SnapshotCatchUpEntries == 0 {
		return fmt.Errorf("SnapshotCatchUpEntries must be positive")
	}

	// Validate peer addresses
	for i, peer := range c.Peers {
		if peer.ID == 0 {
			return fmt.Errorf("peer %d: ID must be non-zero", i)
		}
		if peer.Addr == "" {
			return fmt.Errorf("peer %d: Addr must be specified", i)
		}
	}

	return nil
}

// Clone creates a deep copy of the configuration.
func (c *Config) Clone() *Config {
	clone := *c
	clone.Peers = make([]Peer, len(c.Peers))
	copy(clone.Peers, c.Peers)
	return &clone
}
