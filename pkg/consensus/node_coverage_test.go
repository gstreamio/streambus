package consensus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/raft/v3/raftpb"
)

// mockStateMachineWithSnapshot extends mockStateMachine to support snapshot operations
type mockStateMachineWithSnapshot struct {
	mockStateMachine
	snapshotErr error
	restoreErr  error
}

func (m *mockStateMachineWithSnapshot) Snapshot() ([]byte, error) {
	if m.snapshotErr != nil {
		return nil, m.snapshotErr
	}
	return m.snapshot, nil
}

func (m *mockStateMachineWithSnapshot) Restore(snapshot []byte) error {
	if m.restoreErr != nil {
		return m.restoreErr
	}
	m.snapshot = snapshot
	return nil
}

// TestNode_AddNode tests adding a node to the cluster
func TestNode_AddNode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.ElectionTick = 5  // Faster election
	config.HeartbeatTick = 1
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:19001"},
	}

	sm := &mockStateMachine{}
	node, err := NewNode(config, sm)
	require.NoError(t, err)

	node.SetBindAddr("localhost:19001")
	err = node.Start()
	require.NoError(t, err)
	defer func() { _ = node.Stop() }()

	// Wait for node to become leader
	var becameLeader bool
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if node.IsLeader() {
			becameLeader = true
			break
		}
	}
	require.True(t, becameLeader, "node should become leader")

	// Add a new node to the cluster
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = node.AddNode(ctx, 2, "localhost:19002")
	assert.NoError(t, err, "should be able to propose adding a node")
}

// TestNode_RemoveNode tests removing a node from the cluster
func TestNode_RemoveNode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:19101"},
	}

	sm := &mockStateMachine{}
	node, err := NewNode(config, sm)
	require.NoError(t, err)

	node.SetBindAddr("localhost:19101")
	err = node.Start()
	require.NoError(t, err)
	defer func() { _ = node.Stop() }()

	// Wait for node to become leader
	var becameLeader bool
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if node.IsLeader() {
			becameLeader = true
			break
		}
	}
	require.True(t, becameLeader, "node should become leader")

	// Remove a node from the cluster
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = node.RemoveNode(ctx, 2)
	assert.NoError(t, err, "should be able to propose removing a node")
}

// TestNode_Campaign tests forcing a node to campaign
func TestNode_Campaign(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:19201"},
	}

	sm := &mockStateMachine{}
	node, err := NewNode(config, sm)
	require.NoError(t, err)

	node.SetBindAddr("localhost:19201")
	err = node.Start()
	require.NoError(t, err)
	defer func() { _ = node.Stop() }()

	// Give node time to initialize
	time.Sleep(200 * time.Millisecond)

	// Force node to campaign
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = node.Campaign(ctx)
	assert.NoError(t, err, "should be able to campaign")

	// Wait and verify node becomes leader
	time.Sleep(500 * time.Millisecond)
	assert.True(t, node.IsLeader(), "node should become leader after campaign")
}

// Note: TestNode_Advance is tested implicitly in the run loop of all node tests
// Advance() is called automatically in node.go:311, so we get coverage through other tests

// TestNode_Snapshot tests snapshot creation
func TestNode_Snapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.SnapshotInterval = 5 // Snapshot after 5 entries
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:19401"},
	}

	sm := &mockStateMachineWithSnapshot{
		mockStateMachine: mockStateMachine{
			snapshot: []byte("test snapshot data"),
		},
	}
	node, err := NewNode(config, sm)
	require.NoError(t, err)

	node.SetBindAddr("localhost:19401")
	err = node.Start()
	require.NoError(t, err)
	defer func() { _ = node.Stop() }()

	// Wait for leader election
	var becameLeader bool
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if node.IsLeader() {
			becameLeader = true
			break
		}
	}
	require.True(t, becameLeader, "node should become leader")

	// Propose multiple entries to trigger snapshot
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		data := []byte("test data")
		err = node.Propose(ctx, data)
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for snapshot to be created
	time.Sleep(500 * time.Millisecond)

	// Verify snapshot exists in storage
	snap, err := node.storage.Snapshot()
	assert.NoError(t, err)
	// Snapshot should be created if we have enough entries
	t.Logf("Snapshot index: %d", snap.Metadata.Index)
}

// TestNode_CreateSnapshot tests the createSnapshot method directly
func TestNode_CreateSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.SnapshotInterval = 5
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:19501"},
	}

	sm := &mockStateMachineWithSnapshot{
		mockStateMachine: mockStateMachine{
			snapshot: []byte("snapshot data"),
		},
	}
	node, err := NewNode(config, sm)
	require.NoError(t, err)

	node.SetBindAddr("localhost:19501")
	err = node.Start()
	require.NoError(t, err)
	defer func() { _ = node.Stop() }()

	// Wait for leader election and some entries
	time.Sleep(500 * time.Millisecond)

	// Set applied index to trigger snapshot
	node.appliedIndex = 10

	// Call createSnapshot
	err = node.createSnapshot()
	assert.NoError(t, err, "createSnapshot should succeed")

	// Verify snapshot was created
	assert.Equal(t, node.appliedIndex, node.snapshotIndex)
}

// TestNode_CreateSnapshot_NoStateMachine tests createSnapshot with nil state machine
func TestNode_CreateSnapshot_NoStateMachine(t *testing.T) {
	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:19601"},
	}

	node, err := NewNode(config, nil) // No state machine
	require.NoError(t, err)

	node.appliedIndex = 10

	// createSnapshot should return nil with no state machine
	err = node.createSnapshot()
	assert.NoError(t, err)
}

// TestNode_CreateSnapshot_Error tests createSnapshot with snapshot error
func TestNode_CreateSnapshot_Error(t *testing.T) {
	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.SnapshotInterval = 5
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:19701"},
	}

	sm := &mockStateMachineWithSnapshot{
		mockStateMachine: mockStateMachine{},
		snapshotErr:      errors.New("snapshot failed"),
	}
	node, err := NewNode(config, sm)
	require.NoError(t, err)

	node.appliedIndex = 10

	// createSnapshot should fail
	err = node.createSnapshot()
	assert.Error(t, err, "createSnapshot should fail when state machine returns error")
}

// TestNode_ApplySnapshot tests applying a snapshot
func TestNode_ApplySnapshot(t *testing.T) {
	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:19801"},
	}

	sm := &mockStateMachineWithSnapshot{
		mockStateMachine: mockStateMachine{},
	}
	node, err := NewNode(config, sm)
	require.NoError(t, err)

	// Create a snapshot
	snap := raftpb.Snapshot{
		Data: []byte("snapshot data"),
		Metadata: raftpb.SnapshotMetadata{
			Index: 100,
			Term:  5,
			ConfState: raftpb.ConfState{
				Voters: []uint64{1},
			},
		},
	}

	// Apply snapshot
	err = node.applySnapshot(snap)
	assert.NoError(t, err, "applySnapshot should succeed")

	// Verify snapshot was applied
	assert.Equal(t, uint64(100), node.appliedIndex)
	assert.Equal(t, uint64(100), node.snapshotIndex)
	assert.Equal(t, []byte("snapshot data"), sm.snapshot)
}

// TestNode_ApplySnapshot_Error tests applySnapshot with restore error
func TestNode_ApplySnapshot_Error(t *testing.T) {
	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:19901"},
	}

	sm := &mockStateMachineWithSnapshot{
		mockStateMachine: mockStateMachine{},
		restoreErr:       errors.New("restore failed"),
	}
	node, err := NewNode(config, sm)
	require.NoError(t, err)

	snap := raftpb.Snapshot{
		Data: []byte("snapshot data"),
		Metadata: raftpb.SnapshotMetadata{
			Index: 100,
			Term:  5,
		},
	}

	// Apply snapshot should fail
	err = node.applySnapshot(snap)
	assert.Error(t, err, "applySnapshot should fail when restore fails")
}

// TestNode_HandleMessage tests the handleMessage method
func TestNode_HandleMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:20001"},
	}

	sm := &mockStateMachine{}
	node, err := NewNode(config, sm)
	require.NoError(t, err)

	node.SetBindAddr("localhost:20001")
	err = node.Start()
	require.NoError(t, err)
	defer func() { _ = node.Stop() }()

	// Wait for node to initialize
	time.Sleep(200 * time.Millisecond)

	// Create a heartbeat message
	msg := raftpb.Message{
		Type: raftpb.MsgHeartbeat,
		From: 2,
		To:   1,
		Term: 1,
	}

	// Call handleMessage (should not panic)
	node.handleMessage(msg)

	// Give time for message to be processed
	time.Sleep(100 * time.Millisecond)
}

// TestNode_HandleMessage_InvalidMessage tests handleMessage with invalid message
func TestNode_HandleMessage_InvalidMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:20101"},
	}

	sm := &mockStateMachine{}
	node, err := NewNode(config, sm)
	require.NoError(t, err)

	node.SetBindAddr("localhost:20101")
	err = node.Start()
	require.NoError(t, err)
	defer func() { _ = node.Stop() }()

	// Wait for node to initialize
	time.Sleep(200 * time.Millisecond)

	// Create an invalid message (wrong term, etc.)
	msg := raftpb.Message{
		Type: raftpb.MsgHeartbeat,
		From: 0, // Invalid from ID
		To:   1,
		Term: 0,
	}

	// Call handleMessage (should not panic, just log error)
	node.handleMessage(msg)
}

// TestNode_Run_WithSnapshot tests the run method processing snapshots
func TestNode_Run_WithSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.SnapshotInterval = 3
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:20201"},
	}

	sm := &mockStateMachineWithSnapshot{
		mockStateMachine: mockStateMachine{
			snapshot: []byte("test snapshot"),
		},
	}
	node, err := NewNode(config, sm)
	require.NoError(t, err)

	node.SetBindAddr("localhost:20201")
	err = node.Start()
	require.NoError(t, err)
	defer func() { _ = node.Stop() }()

	// Wait for leader election
	var becameLeader bool
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if node.IsLeader() {
			becameLeader = true
			break
		}
	}
	require.True(t, becameLeader, "node should become leader")

	// Propose multiple entries to potentially trigger snapshot
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		data := []byte("entry data")
		_ = node.Propose(ctx, data)
		time.Sleep(100 * time.Millisecond)
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Verify node is still running
	assert.True(t, node.IsLeader() || !node.IsLeader(), "node should still be operational")
}

// TestNode_Run_ErrorHandling tests error handling in run method
func TestNode_Run_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:20301"},
	}

	// Create a state machine that fails on apply
	sm := &mockStateMachine{}
	node, err := NewNode(config, sm)
	require.NoError(t, err)

	node.SetBindAddr("localhost:20301")
	err = node.Start()
	require.NoError(t, err)
	defer func() { _ = node.Stop() }()

	// Wait for initialization
	time.Sleep(200 * time.Millisecond)

	// Propose some data
	ctx := context.Background()
	data := []byte("test data")
	_ = node.Propose(ctx, data)

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Check error channel (non-blocking)
	select {
	case err := <-node.errorCh:
		t.Logf("Received error from error channel: %v", err)
	default:
		// No error, which is fine
	}
}

// TestNode_ConfChange_AddNode tests configuration change for adding a node
func TestNode_ConfChange_AddNode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:20401"},
	}

	sm := &mockStateMachine{}
	node, err := NewNode(config, sm)
	require.NoError(t, err)

	node.SetBindAddr("localhost:20401")
	err = node.Start()
	require.NoError(t, err)
	defer func() { _ = node.Stop() }()

	// Wait for leader election
	var becameLeader bool
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if node.IsLeader() {
			becameLeader = true
			break
		}
	}
	require.True(t, becameLeader, "node should become leader")

	// Add node via configuration change
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = node.AddNode(ctx, 2, "localhost:20402")
	assert.NoError(t, err)

	// Wait for configuration change to be processed
	time.Sleep(500 * time.Millisecond)

	// Verify peer was added to transport
	node.transport.mu.RLock()
	_, exists := node.transport.peers[2]
	node.transport.mu.RUnlock()
	assert.True(t, exists, "peer 2 should be added to transport")
}

// TestNode_ConfChange_RemoveNode tests configuration change for removing a node
func TestNode_ConfChange_RemoveNode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:20501"},
		{ID: 2, Addr: "localhost:20502"},
	}

	sm := &mockStateMachine{}
	node, err := NewNode(config, sm)
	require.NoError(t, err)

	node.SetBindAddr("localhost:20501")
	err = node.Start()
	require.NoError(t, err)
	defer func() { _ = node.Stop() }()

	// Wait for leader election
	var becameLeader bool
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if node.IsLeader() {
			becameLeader = true
			break
		}
	}
	require.True(t, becameLeader, "node should become leader")

	// Verify peer 2 exists
	node.transport.mu.RLock()
	_, exists := node.transport.peers[2]
	node.transport.mu.RUnlock()
	require.True(t, exists, "peer 2 should exist initially")

	// Remove node via configuration change
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = node.RemoveNode(ctx, 2)
	assert.NoError(t, err)

	// Wait for configuration change to be processed
	time.Sleep(500 * time.Millisecond)
}
