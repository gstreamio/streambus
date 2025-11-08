package consensus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
)

// RaftNode wraps etcd/raft and implements the Node interface.
type RaftNode struct {
	mu sync.RWMutex

	config       *Config
	node         raft.Node
	storage      *DiskStorage
	transport    *BidirectionalTransport
	stateMachine StateMachine

	// Channels
	readyCh   chan Ready
	commitCh  chan []raftpb.Entry
	errorCh   chan error
	stopCh    chan struct{}
	doneCh    chan struct{}

	// State
	appliedIndex  uint64
	snapshotIndex uint64

	// Soft state tracking
	lead      uint64
	raftState StateType
}

// NewNode creates a new Raft consensus node.
func NewNode(config *Config, sm StateMachine) (*RaftNode, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Create storage
	storage, err := NewDiskStorage(config.DataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	rn := &RaftNode{
		config:       config,
		storage:      storage,
		stateMachine: sm,
		readyCh:      make(chan Ready, 16),
		commitCh:     make(chan []raftpb.Entry, 16),
		errorCh:      make(chan error, 1),
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
		raftState:    StateFollower,
	}

	// Create transport
	rn.transport = NewBidirectionalTransport(config.NodeID, "", rn.handleMessage)

	// Add initial peers to transport
	for _, peer := range config.Peers {
		if peer.ID != config.NodeID {
			if err := rn.transport.AddPeer(peer.ID, peer.Addr); err != nil {
				return nil, fmt.Errorf("failed to add peer %d: %w", peer.ID, err)
			}
		}
	}

	return rn, nil
}

// SetBindAddr sets the bind address for the transport (for testing).
func (rn *RaftNode) SetBindAddr(addr string) {
	rn.transport.bindAddr = addr
}

// Start starts the Raft node.
func (rn *RaftNode) Start() error {
	// Start transport
	if err := rn.transport.Start(); err != nil {
		return fmt.Errorf("failed to start transport: %w", err)
	}

	// Load initial state
	_, confState, err := rn.storage.InitialState()
	if err != nil {
		return fmt.Errorf("failed to get initial state: %w", err)
	}

	// Create Raft configuration
	c := &raft.Config{
		ID:                        rn.config.NodeID,
		ElectionTick:              rn.config.ElectionTick,
		HeartbeatTick:             rn.config.HeartbeatTick,
		Storage:                   &raftStorageAdapter{rn.storage},
		MaxSizePerMsg:             rn.config.MaxSizePerMsg,
		MaxInflightMsgs:           rn.config.MaxInflightMsgs,
		CheckQuorum:               rn.config.CheckQuorum,
		PreVote:                   rn.config.PreVote,
		DisableProposalForwarding: rn.config.DisableProposalForwarding,
	}

	// Start or restart node
	if len(confState.Voters) == 0 {
		// New cluster - build initial peer list
		peers := make([]raft.Peer, 0, len(rn.config.Peers))
		for _, p := range rn.config.Peers {
			peers = append(peers, raft.Peer{ID: p.ID})
		}
		rn.node = raft.StartNode(c, peers)
	} else {
		// Restarting node
		rn.node = raft.RestartNode(c)
	}

	// Start background goroutines
	go rn.run()
	go rn.ticker()

	return nil
}

// Stop stops the Raft node.
func (rn *RaftNode) Stop() error {
	close(rn.stopCh)

	// Wait for goroutines to finish
	<-rn.doneCh

	// Stop Raft node
	rn.node.Stop()

	// Stop transport
	if err := rn.transport.Stop(); err != nil {
		return fmt.Errorf("failed to stop transport: %w", err)
	}

	// Close storage
	if err := rn.storage.Close(); err != nil {
		return fmt.Errorf("failed to close storage: %w", err)
	}

	return nil
}

// Propose proposes data to be appended to the Raft log.
func (rn *RaftNode) Propose(ctx context.Context, data []byte) error {
	return rn.node.Propose(ctx, data)
}

// IsLeader returns true if this node is currently the Raft leader.
func (rn *RaftNode) IsLeader() bool {
	rn.mu.RLock()
	defer rn.mu.RUnlock()
	return rn.raftState == StateLeader
}

// Leader returns the ID of the current leader.
func (rn *RaftNode) Leader() uint64 {
	rn.mu.RLock()
	defer rn.mu.RUnlock()
	return rn.lead
}

// AddNode adds a new node to the Raft cluster.
func (rn *RaftNode) AddNode(ctx context.Context, nodeID uint64, addr string) error {
	// Add to transport first
	if err := rn.transport.AddPeer(nodeID, addr); err != nil {
		return fmt.Errorf("failed to add peer to transport: %w", err)
	}

	// Propose configuration change
	return rn.node.ProposeConfChange(ctx, raftpb.ConfChange{
		Type:    raftpb.ConfChangeAddNode,
		NodeID:  nodeID,
		Context: []byte(addr),
	})
}

// RemoveNode removes a node from the Raft cluster.
func (rn *RaftNode) RemoveNode(ctx context.Context, nodeID uint64) error {
	return rn.node.ProposeConfChange(ctx, raftpb.ConfChange{
		Type:   raftpb.ConfChangeRemoveNode,
		NodeID: nodeID,
	})
}

// Campaign causes the node to transition to candidate state.
func (rn *RaftNode) Campaign(ctx context.Context) error {
	return rn.node.Campaign(ctx)
}

// Ready returns a channel that receives Ready structs.
func (rn *RaftNode) Ready() <-chan Ready {
	return rn.readyCh
}

// Advance notifies the Raft node that the application has applied entries.
func (rn *RaftNode) Advance() {
	rn.node.Advance()
}

// run is the main event loop that processes Raft ready events.
func (rn *RaftNode) run() {
	defer close(rn.doneCh)

	for {
		select {
		case <-rn.stopCh:
			return

		case rd := <-rn.node.Ready():
			// Update soft state
			if rd.SoftState != nil {
				rn.updateSoftState(rd.SoftState)
			}

			// Save entries and hard state to storage
			if !raft.IsEmptyHardState(rd.HardState) {
				if err := rn.storage.SaveState(rd.HardState); err != nil {
					rn.errorCh <- fmt.Errorf("failed to save hard state: %w", err)
					continue
				}
			}

			if len(rd.Entries) > 0 {
				if err := rn.storage.SaveEntries(rd.Entries); err != nil {
					rn.errorCh <- fmt.Errorf("failed to save entries: %w", err)
					continue
				}
			}

			// Apply snapshot if present
			if !raft.IsEmptySnap(rd.Snapshot) {
				if err := rn.applySnapshot(rd.Snapshot); err != nil {
					rn.errorCh <- fmt.Errorf("failed to apply snapshot: %w", err)
					continue
				}
			}

			// Send messages to other nodes
			for _, msg := range rd.Messages {
				if err := rn.transport.Send(msg); err != nil {
					// Log error but continue
				}
			}

			// Apply committed entries to state machine
			if len(rd.CommittedEntries) > 0 {
				if err := rn.applyCommittedEntries(rd.CommittedEntries); err != nil {
					rn.errorCh <- fmt.Errorf("failed to apply entries: %w", err)
					continue
				}
			}

			// Check if snapshot is needed
			if rn.shouldSnapshot() {
				if err := rn.createSnapshot(); err != nil {
					rn.errorCh <- fmt.Errorf("failed to create snapshot: %w", err)
				}
			}

			// Convert to our Ready type and send to channel
			ready := rn.convertReady(rd)
			select {
			case rn.readyCh <- ready:
			case <-rn.stopCh:
				return
			}

			// Notify Raft that we've processed this ready
			rn.node.Advance()
		}
	}
}

// ticker sends periodic ticks to the Raft node.
func (rn *RaftNode) ticker() {
	ticker := time.NewTicker(rn.config.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rn.node.Tick()
		case <-rn.stopCh:
			return
		}
	}
}

// handleMessage handles incoming Raft messages from the transport.
func (rn *RaftNode) handleMessage(msg raftpb.Message) {
	if err := rn.node.Step(context.Background(), msg); err != nil {
		// Log error
	}
}

// applyCommittedEntries applies committed entries to the state machine.
func (rn *RaftNode) applyCommittedEntries(entries []raftpb.Entry) error {
	for _, entry := range entries {
		if entry.Index <= rn.appliedIndex {
			continue
		}

		switch entry.Type {
		case raftpb.EntryNormal:
			if len(entry.Data) > 0 && rn.stateMachine != nil {
				if err := rn.stateMachine.Apply(entry.Data); err != nil {
					return fmt.Errorf("failed to apply entry %d: %w", entry.Index, err)
				}
			}

		case raftpb.EntryConfChange:
			var cc raftpb.ConfChange
			if err := cc.Unmarshal(entry.Data); err != nil {
				return fmt.Errorf("failed to unmarshal conf change: %w", err)
			}

			rn.node.ApplyConfChange(cc)

			// Update transport peers
			switch cc.Type {
			case raftpb.ConfChangeAddNode:
				addr := string(cc.Context)
				if err := rn.transport.AddPeer(cc.NodeID, addr); err != nil {
					return fmt.Errorf("failed to add peer: %w", err)
				}

			case raftpb.ConfChangeRemoveNode:
				if err := rn.transport.RemovePeer(cc.NodeID); err != nil {
					return fmt.Errorf("failed to remove peer: %w", err)
				}
			}
		}

		rn.appliedIndex = entry.Index
	}

	return nil
}

// applySnapshot applies a snapshot to the state machine.
func (rn *RaftNode) applySnapshot(snap raftpb.Snapshot) error {
	if err := rn.storage.SaveSnapshot(snap); err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}

	if rn.stateMachine != nil {
		if err := rn.stateMachine.Restore(snap.Data); err != nil {
			return fmt.Errorf("failed to restore snapshot: %w", err)
		}
	}

	rn.appliedIndex = snap.Metadata.Index
	rn.snapshotIndex = snap.Metadata.Index

	return nil
}

// shouldSnapshot checks if a snapshot should be created.
func (rn *RaftNode) shouldSnapshot() bool {
	if rn.config.SnapshotInterval == 0 {
		return false
	}
	return rn.appliedIndex-rn.snapshotIndex >= rn.config.SnapshotInterval
}

// createSnapshot creates a new snapshot.
func (rn *RaftNode) createSnapshot() error {
	if rn.stateMachine == nil {
		return nil
	}

	// Get snapshot data from state machine
	data, err := rn.stateMachine.Snapshot()
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	// Get current conf state
	_, confState, err := rn.storage.InitialState()
	if err != nil {
		return fmt.Errorf("failed to get conf state: %w", err)
	}

	// Create snapshot
	snap := raftpb.Snapshot{
		Data: data,
		Metadata: raftpb.SnapshotMetadata{
			Index:     rn.appliedIndex,
			Term:      0, // Will be set by Raft
			ConfState: confState,
		},
	}

	// Save snapshot
	if err := rn.storage.SaveSnapshot(snap); err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}

	// Compact log
	compactIndex := rn.appliedIndex
	if compactIndex > rn.config.SnapshotCatchUpEntries {
		compactIndex -= rn.config.SnapshotCatchUpEntries
	}

	if err := rn.storage.Compact(compactIndex); err != nil {
		return fmt.Errorf("failed to compact log: %w", err)
	}

	rn.snapshotIndex = rn.appliedIndex
	return nil
}

// updateSoftState updates the cached soft state.
func (rn *RaftNode) updateSoftState(ss *raft.SoftState) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.lead = ss.Lead
	rn.raftState = StateType(ss.RaftState)
}

// convertReady converts raft.Ready to our Ready type.
func (rn *RaftNode) convertReady(rd raft.Ready) Ready {
	var softState *SoftState
	if rd.SoftState != nil {
		softState = &SoftState{
			Lead:      rd.SoftState.Lead,
			RaftState: StateType(rd.SoftState.RaftState),
		}
	}

	return Ready{
		SoftState:        softState,
		HardState:        rd.HardState,
		Entries:          rd.Entries,
		Snapshot:         rd.Snapshot,
		CommittedEntries: rd.CommittedEntries,
		Messages:         rd.Messages,
		MustSync:         rd.MustSync,
	}
}

// raftStorageAdapter adapts our DiskStorage to raft.Storage interface.
type raftStorageAdapter struct {
	*DiskStorage
}

func (a *raftStorageAdapter) InitialState() (raftpb.HardState, raftpb.ConfState, error) {
	return a.DiskStorage.InitialState()
}

func (a *raftStorageAdapter) Entries(lo, hi, maxSize uint64) ([]raftpb.Entry, error) {
	entries, err := a.DiskStorage.Entries(lo, hi)
	if err != nil {
		return nil, err
	}

	// Apply maxSize limit
	if maxSize > 0 {
		size := uint64(0)
		for i, e := range entries {
			size += uint64(e.Size())
			if size > maxSize {
				return entries[:i], nil
			}
		}
	}

	return entries, nil
}

func (a *raftStorageAdapter) Term(i uint64) (uint64, error) {
	return a.DiskStorage.Term(i)
}

func (a *raftStorageAdapter) LastIndex() (uint64, error) {
	return a.DiskStorage.LastIndex()
}

func (a *raftStorageAdapter) FirstIndex() (uint64, error) {
	return a.DiskStorage.FirstIndex()
}

func (a *raftStorageAdapter) Snapshot() (raftpb.Snapshot, error) {
	return a.DiskStorage.Snapshot()
}
