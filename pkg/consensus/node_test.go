package consensus

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/raft/v3/raftpb"
)

// mockStateMachine implements StateMachine for testing.
type mockStateMachine struct {
	applied  [][]byte
	snapshot []byte
}

func (m *mockStateMachine) Apply(entry []byte) error {
	m.applied = append(m.applied, entry)
	return nil
}

func (m *mockStateMachine) Snapshot() ([]byte, error) {
	return m.snapshot, nil
}

func (m *mockStateMachine) Restore(snapshot []byte) error {
	m.snapshot = snapshot
	return nil
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				NodeID:        1,
				DataDir:       "/tmp/test",
				TickInterval:  100 * time.Millisecond,
				ElectionTick:  10,
				HeartbeatTick: 1,
				MaxSizePerMsg: 1024,
				MaxInflightMsgs: 256,
				SnapshotInterval: 10000,
				SnapshotCatchUpEntries: 5000,
			},
			wantErr: false,
		},
		{
			name: "zero node ID",
			config: &Config{
				NodeID:  0,
				DataDir: "/tmp/test",
			},
			wantErr: true,
		},
		{
			name: "empty data dir",
			config: &Config{
				NodeID:  1,
				DataDir: "",
			},
			wantErr: true,
		},
		{
			name: "election tick <= heartbeat tick",
			config: &Config{
				NodeID:        1,
				DataDir:       "/tmp/test",
				TickInterval:  100 * time.Millisecond,
				ElectionTick:  1,
				HeartbeatTick: 1,
				MaxSizePerMsg: 1024,
				MaxInflightMsgs: 256,
				SnapshotInterval: 10000,
				SnapshotCatchUpEntries: 5000,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	require.NotNil(t, cfg)

	assert.Equal(t, 100*time.Millisecond, cfg.TickInterval)
	assert.Equal(t, 15, cfg.ElectionTick) // Increased to prevent premature elections
	assert.Equal(t, 1, cfg.HeartbeatTick)
	assert.Equal(t, uint64(1024*1024), cfg.MaxSizePerMsg)
	assert.Equal(t, 256, cfg.MaxInflightMsgs)
	assert.Equal(t, uint64(10000), cfg.SnapshotInterval)
	assert.Equal(t, uint64(5000), cfg.SnapshotCatchUpEntries)
	assert.True(t, cfg.PreVote)
	assert.True(t, cfg.CheckQuorum)
	assert.False(t, cfg.DisableProposalForwarding)
}

func TestConfigClone(t *testing.T) {
	original := DefaultConfig()
	original.NodeID = 1
	original.DataDir = "/tmp/test"
	original.Peers = []Peer{
		{ID: 1, Addr: "localhost:5001"},
		{ID: 2, Addr: "localhost:5002"},
	}

	clone := original.Clone()
	require.NotNil(t, clone)

	// Verify values are equal
	assert.Equal(t, original.NodeID, clone.NodeID)
	assert.Equal(t, original.DataDir, clone.DataDir)
	assert.Equal(t, original.Peers, clone.Peers)

	// Verify it's a deep copy
	clone.Peers[0].Addr = "localhost:6001"
	assert.NotEqual(t, original.Peers[0].Addr, clone.Peers[0].Addr)
}

func TestDiskStorage(t *testing.T) {
	dir := t.TempDir()

	storage, err := NewDiskStorage(dir)
	require.NoError(t, err)
	defer storage.Close()

	t.Run("initial state", func(t *testing.T) {
		hs, cs, err := storage.InitialState()
		require.NoError(t, err)
		assert.Equal(t, uint64(0), hs.Term)
		assert.Equal(t, uint64(0), hs.Vote)
		assert.Equal(t, uint64(0), hs.Commit)
		assert.Empty(t, cs.Voters)
	})

	t.Run("first and last index", func(t *testing.T) {
		first, err := storage.FirstIndex()
		require.NoError(t, err)
		assert.Equal(t, uint64(1), first)

		last, err := storage.LastIndex()
		require.NoError(t, err)
		assert.Equal(t, uint64(0), last)
	})
}

func TestDiskStoragePersistence(t *testing.T) {
	dir := t.TempDir()

	// Create storage and save some data
	storage1, err := NewDiskStorage(dir)
	require.NoError(t, err)

	// Save hard state
	hardState := makeHardState(1, 2, 3)
	err = storage1.SaveState(hardState)
	require.NoError(t, err)

	// Save entries
	entries := makeEntries(1, 3, 1) // indices 1, 2
	err = storage1.SaveEntries(entries)
	require.NoError(t, err)

	storage1.Close()

	// Load storage and verify data persisted
	storage2, err := NewDiskStorage(dir)
	require.NoError(t, err)
	defer storage2.Close()

	hs, _, err := storage2.InitialState()
	require.NoError(t, err)
	assert.Equal(t, hardState.Term, hs.Term)
	assert.Equal(t, hardState.Vote, hs.Vote)
	assert.Equal(t, hardState.Commit, hs.Commit)

	loadedEntries, err := storage2.Entries(1, 3)
	require.NoError(t, err)
	assert.Equal(t, 2, len(loadedEntries))
	assert.Equal(t, uint64(1), loadedEntries[0].Index)
	assert.Equal(t, uint64(2), loadedEntries[1].Index)
}

func TestStateTypeString(t *testing.T) {
	tests := []struct {
		state StateType
		want  string
	}{
		{StateFollower, "Follower"},
		{StateCandidate, "Candidate"},
		{StateLeader, "Leader"},
		{StateType(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.state.String())
		})}
}

func TestNodeCreation(t *testing.T) {
	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:5001"},
		{ID: 2, Addr: "localhost:5002"},
		{ID: 3, Addr: "localhost:5003"},
	}

	sm := &mockStateMachine{}
	node, err := NewNode(config, sm)
	require.NoError(t, err)
	require.NotNil(t, node)

	// Verify initial state
	assert.False(t, node.IsLeader())
	assert.Equal(t, uint64(0), node.Leader())
}

func TestNodeInvalidConfig(t *testing.T) {
	config := &Config{
		NodeID:  0, // Invalid
		DataDir: "",
	}

	sm := &mockStateMachine{}
	node, err := NewNode(config, sm)
	assert.Error(t, err)
	assert.Nil(t, node)
}

func TestSingleNodeCluster(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:15001"},
	}

	sm := &mockStateMachine{}
	node, err := NewNode(config, sm)
	require.NoError(t, err)

	// Update transport bind address
	node.SetBindAddr("localhost:15001")

	err = node.Start()
	require.NoError(t, err)
	defer node.Stop()

	// Wait for leader election (single node should elect itself)
	// Check periodically for up to 5 seconds
	var becameLeader bool
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if node.IsLeader() && node.Leader() == 1 {
			becameLeader = true
			break
		}
	}

	// Single node should become leader
	assert.True(t, becameLeader, "single node should become leader within 5 seconds")
	assert.True(t, node.IsLeader(), "should be leader")
	assert.Equal(t, uint64(1), node.Leader())

	// Test proposal
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data := []byte("test data")
	err = node.Propose(ctx, data)
	assert.NoError(t, err)

	// Wait for entry to be applied
	time.Sleep(200 * time.Millisecond)

	// Verify entry was applied to state machine
	assert.Equal(t, 1, len(sm.applied))
	assert.Equal(t, data, sm.applied[0])
}

// Helper functions

func makeHardState(term, vote, commit uint64) raftpb.HardState {
	return raftpb.HardState{
		Term:   term,
		Vote:   vote,
		Commit: commit,
	}
}

func makeEntries(start, count int, term uint64) []raftpb.Entry {
	entries := make([]raftpb.Entry, count)
	for i := 0; i < count; i++ {
		entries[i] = raftpb.Entry{
			Term:  term,
			Index: uint64(start + i),
			Type:  raftpb.EntryNormal,
			Data:  []byte("data"),
		}
	}
	return entries
}

func cleanupTestDir(dir string) {
	os.RemoveAll(dir)
}

// Benchmark tests

func BenchmarkPropose(b *testing.B) {
	dir := b.TempDir()
	defer cleanupTestDir(dir)

	config := DefaultConfig()
	config.NodeID = 1
	config.DataDir = dir
	config.Peers = []Peer{
		{ID: 1, Addr: "localhost:25001"},
	}

	sm := &mockStateMachine{}
	node, err := NewNode(config, sm)
	require.NoError(b, err)

	node.SetBindAddr("localhost:25001")
	err = node.Start()
	require.NoError(b, err)
	defer node.Stop()

	// Wait for leader election
	time.Sleep(500 * time.Millisecond)

	data := []byte("benchmark data")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = node.Propose(ctx, data)
	}
}

func BenchmarkStorage(b *testing.B) {
	dir := b.TempDir()
	defer cleanupTestDir(dir)

	storage, err := NewDiskStorage(dir)
	require.NoError(b, err)
	defer storage.Close()

	entries := makeEntries(1, 1, 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entries[0].Index = uint64(i + 1)
		_ = storage.SaveEntries(entries)
	}
}
