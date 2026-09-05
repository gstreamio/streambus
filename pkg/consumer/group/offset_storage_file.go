package group

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileOffsetStorage persists committed consumer group offsets to disk.
//
// Offsets are the one piece of group state that must survive a broker
// restart: group membership rebuilds itself when members rejoin, but a lost
// offset silently rewinds or skips a consumer. The whole offset table is
// small (one entry per group/topic/partition), so it is written as a single
// snapshot rather than a log.
//
// Writes go to a temporary file that is fsynced and then renamed over the
// snapshot, so a crash mid-write leaves the previous snapshot intact rather
// than a truncated file.
type FileOffsetStorage struct {
	mu   sync.RWMutex
	path string
	// offsets: groupID -> topic -> partition -> OffsetAndMetadata
	offsets map[string]map[string]map[int32]*OffsetAndMetadata
}

// persistedOffset is the on-disk form of an offset entry. Times are stored as
// Unix nanoseconds so the file does not depend on Go's time formatting.
type persistedOffset struct {
	Offset         int64  `json:"offset"`
	Metadata       string `json:"metadata,omitempty"`
	CommitTimeNs   int64  `json:"commit_time_ns"`
	ExpireTimeNs   int64  `json:"expire_time_ns,omitempty"`
	LeaderEpoch    int64  `json:"leader_epoch,omitempty"`
	GroupID        string `json:"group_id"`
	Topic          string `json:"topic"`
	PartitionIndex int32  `json:"partition"`
}

// offsetSnapshot is the on-disk file format.
type offsetSnapshot struct {
	Version int               `json:"version"`
	Offsets []persistedOffset `json:"offsets"`
}

// offsetSnapshotVersion is the current on-disk format version.
const offsetSnapshotVersion = 1

// NewFileOffsetStorage opens (and creates if needed) offset storage rooted at
// dir. Existing offsets are loaded immediately, so a coordinator built on it
// serves committed offsets from before the restart.
func NewFileOffsetStorage(dir string) (*FileOffsetStorage, error) {
	if dir == "" {
		return nil, fmt.Errorf("offset storage directory is required")
	}
	// Owner-only: consumer offsets are broker-internal state, not meant to be
	// readable by other local accounts on shared hosts.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating offset storage directory: %w", err)
	}

	s := &FileOffsetStorage{
		path:    filepath.Join(dir, "consumer-offsets.json"),
		offsets: make(map[string]map[string]map[int32]*OffsetAndMetadata),
	}

	if err := s.load(); err != nil {
		return nil, err
	}

	return s, nil
}

// load reads the snapshot into memory. A missing file is not an error: it
// simply means no offsets have been committed yet.
func (s *FileOffsetStorage) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading offset snapshot: %w", err)
	}
	if len(data) == 0 {
		return nil
	}

	var snapshot offsetSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("parsing offset snapshot %s: %w", s.path, err)
	}
	if snapshot.Version != offsetSnapshotVersion {
		return fmt.Errorf("offset snapshot %s has unsupported version %d (expected %d)",
			s.path, snapshot.Version, offsetSnapshotVersion)
	}

	for _, entry := range snapshot.Offsets {
		offset := &OffsetAndMetadata{
			Offset:      entry.Offset,
			Metadata:    entry.Metadata,
			CommitTime:  time.Unix(0, entry.CommitTimeNs),
			LeaderEpoch: entry.LeaderEpoch,
		}
		if entry.ExpireTimeNs != 0 {
			offset.ExpireTime = time.Unix(0, entry.ExpireTimeNs)
		}
		s.put(entry.GroupID, entry.Topic, entry.PartitionIndex, offset)
	}

	return nil
}

// put stores an offset in the in-memory table. Callers hold the lock, except
// during load where no other goroutine can see the storage yet.
func (s *FileOffsetStorage) put(groupID, topic string, partition int32, offset *OffsetAndMetadata) {
	if s.offsets[groupID] == nil {
		s.offsets[groupID] = make(map[string]map[int32]*OffsetAndMetadata)
	}
	if s.offsets[groupID][topic] == nil {
		s.offsets[groupID][topic] = make(map[int32]*OffsetAndMetadata)
	}
	s.offsets[groupID][topic][partition] = offset
}

// flush writes the current table to disk atomically. Callers hold the lock.
//
// It reports whether the new content became visible on disk (the rename
// succeeded) separately from the error, because a caller that rolls back
// in-memory state on any error must not do so once the rename has already
// happened -- at that point a fresh load() would see the new data, and
// rolling back memory would leave it permanently disagreeing with disk.
func (s *FileOffsetStorage) flush() (committed bool, err error) {
	snapshot := offsetSnapshot{Version: offsetSnapshotVersion}

	for groupID, topics := range s.offsets {
		for topic, partitions := range topics {
			for partition, offset := range partitions {
				entry := persistedOffset{
					Offset:         offset.Offset,
					Metadata:       offset.Metadata,
					CommitTimeNs:   offset.CommitTime.UnixNano(),
					LeaderEpoch:    offset.LeaderEpoch,
					GroupID:        groupID,
					Topic:          topic,
					PartitionIndex: partition,
				}
				if !offset.ExpireTime.IsZero() {
					entry.ExpireTimeNs = offset.ExpireTime.UnixNano()
				}
				snapshot.Offsets = append(snapshot.Offsets, entry)
			}
		}
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return false, fmt.Errorf("encoding offset snapshot: %w", err)
	}

	return writeFileAtomic(s.path, data)
}

// writeFileAtomic writes data to path via a temporary file that is fsynced and
// renamed, so a crash never leaves a partially written snapshot in place.
//
// The returned committed flag is true from the moment the rename succeeds,
// even if the trailing directory fsync below it fails: that fsync only
// protects the rename against being lost across a crash, it does not change
// what a reader sees right now, so callers must treat a post-rename error as
// a durability warning rather than proof that the write did not take.
func writeFileAtomic(path string, data []byte) (committed bool, err error) {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return false, fmt.Errorf("creating temporary snapshot: %w", err)
	}
	tmpName := tmp.Name()

	// Remove the temporary file on any failure path so a failed write does
	// not leave debris next to the real snapshot.
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("writing temporary snapshot: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("syncing temporary snapshot: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("closing temporary snapshot: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return false, fmt.Errorf("setting snapshot permissions: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return false, fmt.Errorf("replacing snapshot: %w", err)
	}
	tmpName = "" // renamed successfully; nothing left to clean up

	// The rename itself is durable only once the directory entry is synced,
	// but the content is already committed as far as any reader is concerned.
	// #nosec G304 -- dir is derived from the storage's own configured path, not user-supplied input
	dirFile, err := os.Open(dir)
	if err != nil {
		return true, fmt.Errorf("opening snapshot directory: %w", err)
	}
	defer func() { _ = dirFile.Close() }()
	if err := dirFile.Sync(); err != nil {
		return true, fmt.Errorf("syncing snapshot directory: %w", err)
	}

	return true, nil
}

// StoreOffset stores an offset and persists the table.
//
// The offset is kept in memory only if the write succeeds, so a caller that
// sees an error knows the offset was not committed rather than being told it
// was durable when it is not. If flush fails after its rename already made
// the new snapshot visible on disk, memory is left as-is instead of being
// rolled back -- rolling back at that point would make memory disagree with
// what a reload would see, which is the exact inconsistency this storage
// exists to prevent.
func (s *FileOffsetStorage) StoreOffset(groupID string, topic string, partition int32, offset *OffsetAndMetadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous, existed := s.lookup(groupID, topic, partition)
	s.put(groupID, topic, partition, offset)

	committed, err := s.flush()
	if err != nil && !committed {
		// The write never reached disk, so roll back so memory matches what
		// is actually on disk.
		if existed {
			s.put(groupID, topic, partition, previous)
		} else {
			s.remove(groupID, topic, partition)
		}
	}

	return err
}

// lookup returns the stored offset for a group/topic/partition.
func (s *FileOffsetStorage) lookup(groupID, topic string, partition int32) (*OffsetAndMetadata, bool) {
	topics, ok := s.offsets[groupID]
	if !ok {
		return nil, false
	}
	partitions, ok := topics[topic]
	if !ok {
		return nil, false
	}
	offset, ok := partitions[partition]
	return offset, ok
}

// remove drops one offset entry, pruning empty parents.
func (s *FileOffsetStorage) remove(groupID, topic string, partition int32) {
	topics, ok := s.offsets[groupID]
	if !ok {
		return
	}
	partitions, ok := topics[topic]
	if !ok {
		return
	}

	delete(partitions, partition)
	if len(partitions) == 0 {
		delete(topics, topic)
	}
	if len(topics) == 0 {
		delete(s.offsets, groupID)
	}
}

// FetchOffset fetches an offset for a group/topic/partition.
func (s *FileOffsetStorage) FetchOffset(groupID string, topic string, partition int32) (*OffsetAndMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	offset, ok := s.lookup(groupID, topic, partition)
	if !ok {
		return nil, nil // No committed offset
	}

	copied := *offset
	return &copied, nil
}

// FetchOffsets fetches all offsets for a group.
func (s *FileOffsetStorage) FetchOffsets(groupID string) (*GroupOffsets, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := &GroupOffsets{
		GroupID: groupID,
		Offsets: make(map[string]map[int32]*OffsetAndMetadata),
	}

	for topic, partitions := range s.offsets[groupID] {
		result.Offsets[topic] = make(map[int32]*OffsetAndMetadata, len(partitions))
		for partition, offset := range partitions {
			copied := *offset
			result.Offsets[topic][partition] = &copied
		}
	}

	return result, nil
}

// DeleteOffsets deletes all offsets for a group and persists the table.
func (s *FileOffsetStorage) DeleteOffsets(groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous, existed := s.offsets[groupID]
	delete(s.offsets, groupID)

	committed, err := s.flush()
	if err != nil && !committed && existed {
		s.offsets[groupID] = previous
	}

	return err
}
