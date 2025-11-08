package consensus

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.etcd.io/raft/v3/raftpb"
)

// DiskStorage implements the Storage interface with file-based persistence.
type DiskStorage struct {
	mu sync.RWMutex

	// Directory where Raft state is stored
	dir string

	// In-memory cache of log entries
	entries []raftpb.Entry

	// Raft hard state (term, vote, commit)
	hardState raftpb.HardState

	// Cluster configuration state
	confState raftpb.ConfState

	// Current snapshot
	snapshot raftpb.Snapshot

	// Index of the first log entry
	firstIndex uint64
}

// NewDiskStorage creates a new disk-based storage.
func NewDiskStorage(dir string) (*DiskStorage, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	s := &DiskStorage{
		dir:        dir,
		entries:    make([]raftpb.Entry, 0),
		firstIndex: 1,
	}

	// Load existing state from disk
	if err := s.load(); err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	return s, nil
}

// InitialState returns the saved HardState and ConfState.
func (s *DiskStorage) InitialState() (raftpb.HardState, raftpb.ConfState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hardState, s.confState, nil
}

// SaveState saves the current HardState.
func (s *DiskStorage) SaveState(st raftpb.HardState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.hardState = st
	return s.persistHardState()
}

// Entries returns a slice of log entries in the range [lo, hi).
func (s *DiskStorage) Entries(lo, hi uint64) ([]raftpb.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if lo < s.firstIndex {
		return nil, fmt.Errorf("entries[%d] has been compacted", lo)
	}

	if hi > s.lastIndexLocked()+1 {
		return nil, fmt.Errorf("entries[%d:%d] out of range [%d:%d]",
			lo, hi, s.firstIndex, s.lastIndexLocked()+1)
	}

	if len(s.entries) == 0 {
		return nil, nil
	}

	offset := s.firstIndex
	return s.entries[lo-offset : hi-offset], nil
}

// Term returns the term of entry i.
func (s *DiskStorage) Term(i uint64) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Entry 0 is a dummy entry in Raft with term 0
	if i == 0 {
		return 0, nil
	}

	// Check if entry is in snapshot
	if i == s.snapshot.Metadata.Index {
		return s.snapshot.Metadata.Term, nil
	}

	if i < s.firstIndex {
		return 0, fmt.Errorf("entry[%d] has been compacted", i)
	}

	if i > s.lastIndexLocked() {
		return 0, fmt.Errorf("entry[%d] out of range", i)
	}

	if len(s.entries) == 0 {
		return 0, nil
	}

	offset := s.firstIndex
	return s.entries[i-offset].Term, nil
}

// LastIndex returns the index of the last entry in the log.
func (s *DiskStorage) LastIndex() (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastIndexLocked(), nil
}

// lastIndexLocked returns the last index without locking (caller must hold lock).
func (s *DiskStorage) lastIndexLocked() uint64 {
	if len(s.entries) == 0 {
		return s.firstIndex - 1
	}
	return s.entries[len(s.entries)-1].Index
}

// FirstIndex returns the index of the first log entry that is available.
func (s *DiskStorage) FirstIndex() (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.firstIndex, nil
}

// Snapshot returns the most recent snapshot.
func (s *DiskStorage) Snapshot() (raftpb.Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot, nil
}

// SaveEntries saves entries to stable storage.
func (s *DiskStorage) SaveEntries(entries []raftpb.Entry) error {
	if len(entries) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate entries are contiguous with existing log
	firstNewEntry := entries[0].Index
	lastIndex := s.lastIndexLocked()

	if firstNewEntry > lastIndex+1 {
		return fmt.Errorf("entries[%d] is out of order, last is %d", firstNewEntry, lastIndex)
	}

	// Truncate any conflicting entries
	if firstNewEntry <= lastIndex {
		offset := s.firstIndex
		s.entries = s.entries[:firstNewEntry-offset]
	}

	// Append new entries
	s.entries = append(s.entries, entries...)

	// Persist to disk
	return s.persistEntries()
}

// SaveSnapshot saves the snapshot to stable storage.
func (s *DiskStorage) SaveSnapshot(snap raftpb.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate snapshot is newer than current
	if snap.Metadata.Index <= s.snapshot.Metadata.Index {
		return fmt.Errorf("snapshot is older than current snapshot")
	}

	s.snapshot = snap
	s.confState = snap.Metadata.ConfState

	// Compact log entries up to snapshot
	if snap.Metadata.Index >= s.firstIndex {
		compactIndex := snap.Metadata.Index - s.firstIndex + 1
		if compactIndex < uint64(len(s.entries)) {
			s.entries = s.entries[compactIndex:]
		} else {
			s.entries = nil
		}
		s.firstIndex = snap.Metadata.Index + 1
	}

	return s.persistSnapshot()
}

// Compact compacts the log up to compactIndex.
func (s *DiskStorage) Compact(compactIndex uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if compactIndex < s.firstIndex {
		return fmt.Errorf("compact %d is out of range [%d]", compactIndex, s.firstIndex)
	}

	if compactIndex > s.lastIndexLocked() {
		return fmt.Errorf("compact %d is out of range [%d]", compactIndex, s.lastIndexLocked())
	}

	offset := s.firstIndex
	i := compactIndex - offset
	s.entries = s.entries[i:]
	s.firstIndex = compactIndex

	return s.persistEntries()
}

// Close closes the storage.
func (s *DiskStorage) Close() error {
	return nil
}

// load loads existing state from disk.
func (s *DiskStorage) load() error {
	// Load hard state
	if err := s.loadHardState(); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load hard state: %w", err)
	}

	// Load snapshot
	if err := s.loadSnapshot(); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load snapshot: %w", err)
	}

	// Load entries
	if err := s.loadEntries(); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load entries: %w", err)
	}

	return nil
}

// persistHardState persists the hard state to disk.
func (s *DiskStorage) persistHardState() error {
	data, err := s.hardState.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal hard state: %w", err)
	}

	path := filepath.Join(s.dir, "hardstate")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write hard state: %w", err)
	}

	return nil
}

// loadHardState loads the hard state from disk.
func (s *DiskStorage) loadHardState() error {
	path := filepath.Join(s.dir, "hardstate")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var hs raftpb.HardState
	if err := hs.Unmarshal(data); err != nil {
		return fmt.Errorf("failed to unmarshal hard state: %w", err)
	}

	s.hardState = hs
	return nil
}

// persistSnapshot persists the snapshot to disk.
func (s *DiskStorage) persistSnapshot() error {
	data, err := s.snapshot.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	path := filepath.Join(s.dir, "snapshot")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write snapshot: %w", err)
	}

	return nil
}

// loadSnapshot loads the snapshot from disk.
func (s *DiskStorage) loadSnapshot() error {
	path := filepath.Join(s.dir, "snapshot")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var snap raftpb.Snapshot
	if err := snap.Unmarshal(data); err != nil {
		return fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	s.snapshot = snap
	s.confState = snap.Metadata.ConfState

	// Update firstIndex based on snapshot
	if snap.Metadata.Index > 0 {
		s.firstIndex = snap.Metadata.Index + 1
	}

	return nil
}

// persistEntries persists log entries to disk.
func (s *DiskStorage) persistEntries() error {
	path := filepath.Join(s.dir, "entries")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create entries file: %w", err)
	}
	defer f.Close()

	// Write firstIndex
	if err := binary.Write(f, binary.LittleEndian, s.firstIndex); err != nil {
		return fmt.Errorf("failed to write firstIndex: %w", err)
	}

	// Write number of entries
	numEntries := uint64(len(s.entries))
	if err := binary.Write(f, binary.LittleEndian, numEntries); err != nil {
		return fmt.Errorf("failed to write entry count: %w", err)
	}

	// Write each entry
	for _, entry := range s.entries {
		data, err := entry.Marshal()
		if err != nil {
			return fmt.Errorf("failed to marshal entry: %w", err)
		}

		// Write entry size
		size := uint64(len(data))
		if err := binary.Write(f, binary.LittleEndian, size); err != nil {
			return fmt.Errorf("failed to write entry size: %w", err)
		}

		// Write entry data
		if _, err := f.Write(data); err != nil {
			return fmt.Errorf("failed to write entry data: %w", err)
		}
	}

	return f.Sync()
}

// loadEntries loads log entries from disk.
func (s *DiskStorage) loadEntries() error {
	path := filepath.Join(s.dir, "entries")
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Read firstIndex
	var firstIndex uint64
	if err := binary.Read(f, binary.LittleEndian, &firstIndex); err != nil {
		return fmt.Errorf("failed to read firstIndex: %w", err)
	}
	s.firstIndex = firstIndex

	// Read number of entries
	var numEntries uint64
	if err := binary.Read(f, binary.LittleEndian, &numEntries); err != nil {
		return fmt.Errorf("failed to read entry count: %w", err)
	}

	// Read each entry
	s.entries = make([]raftpb.Entry, 0, numEntries)
	for i := uint64(0); i < numEntries; i++ {
		// Read entry size
		var size uint64
		if err := binary.Read(f, binary.LittleEndian, &size); err != nil {
			return fmt.Errorf("failed to read entry size: %w", err)
		}

		// Read entry data
		data := make([]byte, size)
		if _, err := f.Read(data); err != nil {
			return fmt.Errorf("failed to read entry data: %w", err)
		}

		// Unmarshal entry
		var entry raftpb.Entry
		if err := entry.Unmarshal(data); err != nil {
			return fmt.Errorf("failed to unmarshal entry: %w", err)
		}

		s.entries = append(s.entries, entry)
	}

	return nil
}
