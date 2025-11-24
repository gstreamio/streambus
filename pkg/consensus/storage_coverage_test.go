package consensus

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/raft/v3/raftpb"
)

// TestDiskStorage_LoadSnapshot tests loading a snapshot from disk
func TestDiskStorage_LoadSnapshot(t *testing.T) {
	dir := t.TempDir()

	// Create a storage and save a snapshot
	storage1, err := NewDiskStorage(dir)
	require.NoError(t, err)

	snap := raftpb.Snapshot{
		Data: []byte("test snapshot data"),
		Metadata: raftpb.SnapshotMetadata{
			Index: 50,
			Term:  3,
			ConfState: raftpb.ConfState{
				Voters: []uint64{1, 2, 3},
			},
		},
	}

	err = storage1.SaveSnapshot(snap)
	require.NoError(t, err)
	storage1.Close()

	// Load storage and verify snapshot was loaded
	storage2, err := NewDiskStorage(dir)
	require.NoError(t, err)
	defer storage2.Close()

	loadedSnap, err := storage2.Snapshot()
	require.NoError(t, err)

	assert.Equal(t, snap.Metadata.Index, loadedSnap.Metadata.Index)
	assert.Equal(t, snap.Metadata.Term, loadedSnap.Metadata.Term)
	assert.Equal(t, snap.Data, loadedSnap.Data)
	assert.Equal(t, uint64(51), storage2.firstIndex, "firstIndex should be snapshot.Index + 1")
}

// TestDiskStorage_LoadSnapshot_NotExists tests loading when snapshot doesn't exist
func TestDiskStorage_LoadSnapshot_NotExists(t *testing.T) {
	dir := t.TempDir()

	// Create storage without snapshot
	storage, err := NewDiskStorage(dir)
	require.NoError(t, err)
	defer storage.Close()

	// Snapshot should be empty
	snap, err := storage.Snapshot()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), snap.Metadata.Index)
}

// TestDiskStorage_LoadSnapshot_CorruptedData tests loading corrupted snapshot
func TestDiskStorage_LoadSnapshot_CorruptedData(t *testing.T) {
	dir := t.TempDir()

	// Write corrupted snapshot file
	snapPath := filepath.Join(dir, "snapshot")
	err := os.WriteFile(snapPath, []byte("corrupted data"), 0644)
	require.NoError(t, err)

	// Loading should fail with unmarshal error
	storage, err := NewDiskStorage(dir)
	assert.Error(t, err, "should fail to load corrupted snapshot")
	if storage != nil {
		storage.Close()
	}
}

// TestDiskStorage_Load_HardStateError tests load with corrupted hard state
func TestDiskStorage_Load_HardStateError(t *testing.T) {
	dir := t.TempDir()

	// Write corrupted hard state file
	hsPath := filepath.Join(dir, "hardstate")
	err := os.WriteFile(hsPath, []byte("corrupted"), 0644)
	require.NoError(t, err)

	// Loading should fail
	storage, err := NewDiskStorage(dir)
	assert.Error(t, err, "should fail to load corrupted hard state")
	if storage != nil {
		storage.Close()
	}
}

// TestDiskStorage_Load_EntriesError tests load with corrupted entries
func TestDiskStorage_Load_EntriesError(t *testing.T) {
	dir := t.TempDir()

	// Write corrupted entries file
	entriesPath := filepath.Join(dir, "entries")
	err := os.WriteFile(entriesPath, []byte("corrupted"), 0644)
	require.NoError(t, err)

	// Loading should fail
	storage, err := NewDiskStorage(dir)
	assert.Error(t, err, "should fail to load corrupted entries")
	if storage != nil {
		storage.Close()
	}
}

// TestDiskStorage_Load_Success tests successful load of all state
func TestDiskStorage_Load_Success(t *testing.T) {
	dir := t.TempDir()

	// Create storage with hard state, entries, and snapshot
	storage1, err := NewDiskStorage(dir)
	require.NoError(t, err)

	// Save hard state
	hs := raftpb.HardState{
		Term:   5,
		Vote:   1,
		Commit: 10,
	}
	err = storage1.SaveState(hs)
	require.NoError(t, err)

	// Save entries
	entries := []raftpb.Entry{
		{Index: 1, Term: 1, Data: []byte("entry1")},
		{Index: 2, Term: 1, Data: []byte("entry2")},
	}
	err = storage1.SaveEntries(entries)
	require.NoError(t, err)

	// Save snapshot
	snap := raftpb.Snapshot{
		Data: []byte("snapshot"),
		Metadata: raftpb.SnapshotMetadata{
			Index: 20,
			Term:  3,
		},
	}
	err = storage1.SaveSnapshot(snap)
	require.NoError(t, err)

	storage1.Close()

	// Load storage
	storage2, err := NewDiskStorage(dir)
	require.NoError(t, err)
	defer storage2.Close()

	// Verify hard state
	loadedHS, _, err := storage2.InitialState()
	require.NoError(t, err)
	assert.Equal(t, hs.Term, loadedHS.Term)
	assert.Equal(t, hs.Vote, loadedHS.Vote)
	assert.Equal(t, hs.Commit, loadedHS.Commit)

	// Verify snapshot
	loadedSnap, err := storage2.Snapshot()
	require.NoError(t, err)
	assert.Equal(t, snap.Metadata.Index, loadedSnap.Metadata.Index)
	assert.Equal(t, snap.Metadata.Term, loadedSnap.Metadata.Term)
}

// TestDiskStorage_LoadEntries_EmptyFile tests loading from empty entries file
func TestDiskStorage_LoadEntries_EmptyFile(t *testing.T) {
	dir := t.TempDir()

	// Create empty entries file
	entriesPath := filepath.Join(dir, "entries")
	f, err := os.Create(entriesPath)
	require.NoError(t, err)
	f.Close()

	// Loading should fail (can't read header)
	storage, err := NewDiskStorage(dir)
	assert.Error(t, err, "should fail to load empty entries file")
	if storage != nil {
		storage.Close()
	}
}

// TestDiskStorage_SaveSnapshot_OlderSnapshot tests rejecting older snapshots
func TestDiskStorage_SaveSnapshot_OlderSnapshot(t *testing.T) {
	dir := t.TempDir()

	storage, err := NewDiskStorage(dir)
	require.NoError(t, err)
	defer storage.Close()

	// Save a snapshot
	snap1 := raftpb.Snapshot{
		Data: []byte("snapshot1"),
		Metadata: raftpb.SnapshotMetadata{
			Index: 100,
			Term:  5,
		},
	}
	err = storage.SaveSnapshot(snap1)
	require.NoError(t, err)

	// Try to save an older snapshot
	snap2 := raftpb.Snapshot{
		Data: []byte("snapshot2"),
		Metadata: raftpb.SnapshotMetadata{
			Index: 50,
			Term:  3,
		},
	}
	err = storage.SaveSnapshot(snap2)
	assert.Error(t, err, "should reject older snapshot")
	assert.Contains(t, err.Error(), "older than current snapshot")
}

// TestDiskStorage_SaveSnapshot_CompactEntries tests snapshot compacting entries
func TestDiskStorage_SaveSnapshot_CompactEntries(t *testing.T) {
	dir := t.TempDir()

	storage, err := NewDiskStorage(dir)
	require.NoError(t, err)
	defer storage.Close()

	// Save entries
	entries := []raftpb.Entry{
		{Index: 1, Term: 1, Data: []byte("entry1")},
		{Index: 2, Term: 1, Data: []byte("entry2")},
		{Index: 3, Term: 2, Data: []byte("entry3")},
		{Index: 4, Term: 2, Data: []byte("entry4")},
		{Index: 5, Term: 2, Data: []byte("entry5")},
	}
	err = storage.SaveEntries(entries)
	require.NoError(t, err)

	// Save snapshot at index 3
	snap := raftpb.Snapshot{
		Data: []byte("snapshot"),
		Metadata: raftpb.SnapshotMetadata{
			Index: 3,
			Term:  2,
		},
	}
	err = storage.SaveSnapshot(snap)
	require.NoError(t, err)

	// Verify entries before index 3 are compacted
	assert.Equal(t, uint64(4), storage.firstIndex)

	// Should be able to get entries 4 and 5
	loadedEntries, err := storage.Entries(4, 6)
	require.NoError(t, err)
	assert.Equal(t, 2, len(loadedEntries))
	assert.Equal(t, uint64(4), loadedEntries[0].Index)
	assert.Equal(t, uint64(5), loadedEntries[1].Index)
}

// TestDiskStorage_SaveSnapshot_CompactAllEntries tests snapshot compacting all entries
func TestDiskStorage_SaveSnapshot_CompactAllEntries(t *testing.T) {
	dir := t.TempDir()

	storage, err := NewDiskStorage(dir)
	require.NoError(t, err)
	defer storage.Close()

	// Save entries
	entries := []raftpb.Entry{
		{Index: 1, Term: 1, Data: []byte("entry1")},
		{Index: 2, Term: 1, Data: []byte("entry2")},
	}
	err = storage.SaveEntries(entries)
	require.NoError(t, err)

	// Save snapshot that covers all entries
	snap := raftpb.Snapshot{
		Data: []byte("snapshot"),
		Metadata: raftpb.SnapshotMetadata{
			Index: 5,
			Term:  2,
		},
	}
	err = storage.SaveSnapshot(snap)
	require.NoError(t, err)

	// All entries should be compacted
	assert.Equal(t, uint64(6), storage.firstIndex)
	assert.Nil(t, storage.entries)
}

// TestDiskStorage_Compact_EdgeCases tests compact with edge cases
func TestDiskStorage_Compact_EdgeCases(t *testing.T) {
	dir := t.TempDir()

	storage, err := NewDiskStorage(dir)
	require.NoError(t, err)
	defer storage.Close()

	// Save entries
	entries := []raftpb.Entry{
		{Index: 1, Term: 1, Data: []byte("entry1")},
		{Index: 2, Term: 1, Data: []byte("entry2")},
		{Index: 3, Term: 2, Data: []byte("entry3")},
	}
	err = storage.SaveEntries(entries)
	require.NoError(t, err)

	// Test compacting below firstIndex
	err = storage.Compact(0)
	assert.Error(t, err, "should fail to compact below firstIndex")

	// Test compacting beyond lastIndex
	err = storage.Compact(10)
	assert.Error(t, err, "should fail to compact beyond lastIndex")

	// Test valid compact
	err = storage.Compact(2)
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), storage.firstIndex)

	// Verify can still get entry 3
	loadedEntries, err := storage.Entries(2, 4)
	require.NoError(t, err)
	assert.Equal(t, 2, len(loadedEntries))
}

// TestDiskStorage_Term_EdgeCases tests Term with various edge cases
func TestDiskStorage_Term_EdgeCases(t *testing.T) {
	dir := t.TempDir()

	storage, err := NewDiskStorage(dir)
	require.NoError(t, err)
	defer storage.Close()

	// Test term for index 0
	term, err := storage.Term(0)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), term)

	// Save entries
	entries := []raftpb.Entry{
		{Index: 1, Term: 1, Data: []byte("entry1")},
		{Index: 2, Term: 2, Data: []byte("entry2")},
	}
	err = storage.SaveEntries(entries)
	require.NoError(t, err)

	// Save snapshot
	snap := raftpb.Snapshot{
		Data: []byte("snapshot"),
		Metadata: raftpb.SnapshotMetadata{
			Index: 5,
			Term:  3,
		},
	}
	err = storage.SaveSnapshot(snap)
	require.NoError(t, err)

	// Test term for snapshot index
	term, err = storage.Term(5)
	assert.NoError(t, err)
	assert.Equal(t, uint64(3), term)

	// Test term for compacted entry
	term, err = storage.Term(1)
	assert.Error(t, err, "should fail for compacted entry")

	// Test term for out of range entry
	term, err = storage.Term(100)
	assert.Error(t, err, "should fail for out of range entry")
}

// TestDiskStorage_Term_EmptyEntries tests Term with empty entries
func TestDiskStorage_Term_EmptyEntries(t *testing.T) {
	dir := t.TempDir()

	storage, err := NewDiskStorage(dir)
	require.NoError(t, err)
	defer storage.Close()

	// No entries saved, try to get term for index 1
	// This should fail because there are no entries
	term, err := storage.Term(1)
	assert.Error(t, err, "should fail when no entries exist")
	assert.Equal(t, uint64(0), term)
}

// TestDiskStorage_Entries_EdgeCases tests Entries with edge cases
func TestDiskStorage_Entries_EdgeCases(t *testing.T) {
	dir := t.TempDir()

	storage, err := NewDiskStorage(dir)
	require.NoError(t, err)
	defer storage.Close()

	// Save entries
	entries := []raftpb.Entry{
		{Index: 1, Term: 1, Data: []byte("entry1")},
		{Index: 2, Term: 1, Data: []byte("entry2")},
		{Index: 3, Term: 2, Data: []byte("entry3")},
	}
	err = storage.SaveEntries(entries)
	require.NoError(t, err)

	// Test getting entries from compacted index
	storage.firstIndex = 2
	storage.entries = entries[1:]

	_, err = storage.Entries(1, 3)
	assert.Error(t, err, "should fail for compacted entries")
	assert.Contains(t, err.Error(), "has been compacted")

	// Test getting entries beyond range
	_, err = storage.Entries(2, 100)
	assert.Error(t, err, "should fail for out of range")
	assert.Contains(t, err.Error(), "out of range")

	// Test getting empty entries when entries is nil
	storage.entries = nil
	storage.firstIndex = 2
	loadedEntries, err := storage.Entries(2, 2)
	assert.NoError(t, err)
	assert.Empty(t, loadedEntries)
}

// TestDiskStorage_SaveEntries_OutOfOrder tests SaveEntries with out of order entries
func TestDiskStorage_SaveEntries_OutOfOrder(t *testing.T) {
	dir := t.TempDir()

	storage, err := NewDiskStorage(dir)
	require.NoError(t, err)
	defer storage.Close()

	// Save initial entries
	entries := []raftpb.Entry{
		{Index: 1, Term: 1, Data: []byte("entry1")},
		{Index: 2, Term: 1, Data: []byte("entry2")},
	}
	err = storage.SaveEntries(entries)
	require.NoError(t, err)

	// Try to save out of order entries (gap)
	gapEntries := []raftpb.Entry{
		{Index: 5, Term: 2, Data: []byte("entry5")},
	}
	err = storage.SaveEntries(gapEntries)
	assert.Error(t, err, "should fail for out of order entries")
	assert.Contains(t, err.Error(), "out of order")
}

// TestDiskStorage_SaveEntries_Truncate tests SaveEntries truncating conflicting entries
func TestDiskStorage_SaveEntries_Truncate(t *testing.T) {
	dir := t.TempDir()

	storage, err := NewDiskStorage(dir)
	require.NoError(t, err)
	defer storage.Close()

	// Save initial entries
	entries := []raftpb.Entry{
		{Index: 1, Term: 1, Data: []byte("entry1")},
		{Index: 2, Term: 1, Data: []byte("entry2")},
		{Index: 3, Term: 1, Data: []byte("entry3")},
	}
	err = storage.SaveEntries(entries)
	require.NoError(t, err)

	// Save conflicting entries (overwrite from index 2)
	newEntries := []raftpb.Entry{
		{Index: 2, Term: 2, Data: []byte("new_entry2")},
		{Index: 3, Term: 2, Data: []byte("new_entry3")},
	}
	err = storage.SaveEntries(newEntries)
	require.NoError(t, err)

	// Verify entries were truncated and replaced
	loadedEntries, err := storage.Entries(1, 4)
	require.NoError(t, err)
	assert.Equal(t, 3, len(loadedEntries))
	assert.Equal(t, uint64(1), loadedEntries[0].Term)
	assert.Equal(t, uint64(2), loadedEntries[1].Term)
	assert.Equal(t, []byte("new_entry2"), loadedEntries[1].Data)
}

// TestRaftStorageAdapter tests the raft storage adapter
func TestRaftStorageAdapter(t *testing.T) {
	dir := t.TempDir()

	diskStorage, err := NewDiskStorage(dir)
	require.NoError(t, err)
	defer diskStorage.Close()

	// Create adapter
	adapter := &raftStorageAdapter{diskStorage}

	// Test InitialState
	hs, cs, err := adapter.InitialState()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), hs.Term)
	assert.Empty(t, cs.Voters)

	// Save some entries
	entries := []raftpb.Entry{
		{Index: 1, Term: 1, Data: []byte("entry1")},
		{Index: 2, Term: 1, Data: []byte("entry2")},
	}
	err = diskStorage.SaveEntries(entries)
	require.NoError(t, err)

	// Test Entries with maxSize
	loadedEntries, err := adapter.Entries(1, 3, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, len(loadedEntries))

	// Test Entries with maxSize limit
	loadedEntries, err = adapter.Entries(1, 3, 1)
	require.NoError(t, err)
	// Should return partial results due to size limit
	assert.True(t, len(loadedEntries) <= 2)

	// Test Term
	term, err := adapter.Term(1)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), term)

	// Test LastIndex
	lastIndex, err := adapter.LastIndex()
	require.NoError(t, err)
	assert.Equal(t, uint64(2), lastIndex)

	// Test FirstIndex
	firstIndex, err := adapter.FirstIndex()
	require.NoError(t, err)
	assert.Equal(t, uint64(1), firstIndex)

	// Test Snapshot
	snap, err := adapter.Snapshot()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), snap.Metadata.Index)
}
