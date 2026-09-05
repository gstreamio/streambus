package link

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// fileStorage persists replication links, checkpoints and offset mappings to
// disk so that link definitions survive a broker restart instead of having
// to be recreated from scratch (see the "Known Limitations" note this
// package used to carry).
//
// All three kinds of state are small and change infrequently relative to the
// replication data stream itself, so -- exactly like
// group.FileOffsetStorage -- they are kept as a single JSON snapshot that is
// rewritten in full on every mutation, rather than as an append-only log.
// This package intentionally does not import group.writeFileAtomic: the two
// packages have no other dependency on each other, and duplicating roughly
// thirty lines of local file-write logic is cheaper to reason about than
// introducing a shared internal package for it.
type fileStorage struct {
	mu   sync.RWMutex
	path string

	links map[string]*ReplicationLink
	// checkpoints: linkID -> topic -> partition -> Checkpoint
	checkpoints map[string]map[string]map[int32]*Checkpoint
	// offsetMappings: linkID -> topic -> partition -> OffsetMapping
	offsetMappings map[string]map[string]map[int32]*OffsetMapping
}

// linkSnapshot is the on-disk file format. ReplicationLink, Checkpoint and
// OffsetMapping are plain data structs (no funcs or channels), so they
// round-trip through encoding/json without any custom marshaling.
type linkSnapshot struct {
	Version        int                `json:"version"`
	Links          []*ReplicationLink `json:"links,omitempty"`
	Checkpoints    []*Checkpoint      `json:"checkpoints,omitempty"`
	OffsetMappings []*OffsetMapping   `json:"offset_mappings,omitempty"`
}

// linkSnapshotVersion is the current on-disk format version.
const linkSnapshotVersion = 1

// Link definitions include their clusters' SecurityConfig, which carries a
// SASL password in plain text. The snapshot is therefore written 0600 in a
// 0700 directory; treat the broker's data directory as holding credentials.
// NewFileStorage opens (and creates if needed) replication link storage
// rooted at dir. Existing links, checkpoints and offset mappings are loaded
// immediately, so a Manager built on it (via NewManager, which calls
// loadLinksFromStorage) sees links from before the restart.
func NewFileStorage(dir string) (Storage, error) {
	if dir == "" {
		return nil, fmt.Errorf("replication link storage directory is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating replication link storage directory: %w", err)
	}

	s := &fileStorage{
		path:           filepath.Join(dir, "replication-links.json"),
		links:          make(map[string]*ReplicationLink),
		checkpoints:    make(map[string]map[string]map[int32]*Checkpoint),
		offsetMappings: make(map[string]map[string]map[int32]*OffsetMapping),
	}

	if err := s.load(); err != nil {
		return nil, err
	}

	return s, nil
}

// load reads the snapshot into memory. A missing file is not an error: it
// simply means no links have been created yet.
func (s *fileStorage) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading replication link snapshot: %w", err)
	}
	if len(data) == 0 {
		return nil
	}

	var snapshot linkSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("parsing replication link snapshot %s: %w", s.path, err)
	}
	if snapshot.Version != linkSnapshotVersion {
		return fmt.Errorf("replication link snapshot %s has unsupported version %d (expected %d)",
			s.path, snapshot.Version, linkSnapshotVersion)
	}

	for _, l := range snapshot.Links {
		s.links[l.ID] = l
	}
	for _, cp := range snapshot.Checkpoints {
		s.putCheckpoint(cp)
	}
	for _, om := range snapshot.OffsetMappings {
		s.putOffsetMapping(om)
	}

	return nil
}

func (s *fileStorage) putCheckpoint(cp *Checkpoint) {
	if s.checkpoints[cp.LinkID] == nil {
		s.checkpoints[cp.LinkID] = make(map[string]map[int32]*Checkpoint)
	}
	if s.checkpoints[cp.LinkID][cp.Topic] == nil {
		s.checkpoints[cp.LinkID][cp.Topic] = make(map[int32]*Checkpoint)
	}
	s.checkpoints[cp.LinkID][cp.Topic][cp.Partition] = cp
}

func (s *fileStorage) putOffsetMapping(om *OffsetMapping) {
	if s.offsetMappings[om.LinkID] == nil {
		s.offsetMappings[om.LinkID] = make(map[string]map[int32]*OffsetMapping)
	}
	if s.offsetMappings[om.LinkID][om.Topic] == nil {
		s.offsetMappings[om.LinkID][om.Topic] = make(map[int32]*OffsetMapping)
	}
	s.offsetMappings[om.LinkID][om.Topic][om.Partition] = om
}

// flush writes the current tables to disk atomically. Callers hold the lock.
//
// Like group.FileOffsetStorage.flush, it reports whether the new content
// became visible on disk (the rename succeeded) separately from the error,
// so a caller that rolls back in-memory state on failure does not do so once
// the rename has already made the new snapshot the one a fresh load() would
// see.
func (s *fileStorage) flush() (committed bool, err error) {
	snapshot := linkSnapshot{Version: linkSnapshotVersion}

	for _, l := range s.links {
		snapshot.Links = append(snapshot.Links, l)
	}
	for _, topics := range s.checkpoints {
		for _, partitions := range topics {
			for _, cp := range partitions {
				snapshot.Checkpoints = append(snapshot.Checkpoints, cp)
			}
		}
	}
	for _, topics := range s.offsetMappings {
		for _, partitions := range topics {
			for _, om := range partitions {
				snapshot.OffsetMappings = append(snapshot.OffsetMappings, om)
			}
		}
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encoding replication link snapshot: %w", err)
	}

	return writeFileAtomic(s.path, data)
}

// writeFileAtomic writes data to path via a temporary file that is fsynced
// and renamed, so a crash never leaves a partially written snapshot in
// place. This mirrors group.writeFileAtomic; see the fileStorage doc comment
// for why the logic is duplicated instead of shared.
func writeFileAtomic(path string, data []byte) (committed bool, err error) {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return false, fmt.Errorf("creating temporary snapshot: %w", err)
	}
	tmpName := tmp.Name()

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
	// 0600, not 0640: a link's SecurityConfig carries a SASL password and TLS
	// key path, so this snapshot must not be readable by the broker's group.
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

// SaveLink saves a replication link.
func (s *fileStorage) SaveLink(l *ReplicationLink) error {
	if l == nil {
		return fmt.Errorf("link cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	previous, existed := s.links[l.ID]
	s.links[l.ID] = l.Clone()

	committed, err := s.flush()
	if err != nil && !committed {
		if existed {
			s.links[l.ID] = previous
		} else {
			delete(s.links, l.ID)
		}
	}
	return err
}

// LoadLink loads a replication link.
func (s *fileStorage) LoadLink(linkID string) (*ReplicationLink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	l, exists := s.links[linkID]
	if !exists {
		return nil, fmt.Errorf("link %s not found", linkID)
	}
	return l.Clone(), nil
}

// DeleteLink deletes a replication link along with its checkpoints and
// offset mappings.
func (s *fileStorage) DeleteLink(linkID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	previousLink, linkExisted := s.links[linkID]
	previousCheckpoints, checkpointsExisted := s.checkpoints[linkID]
	previousMappings, mappingsExisted := s.offsetMappings[linkID]

	delete(s.links, linkID)
	delete(s.checkpoints, linkID)
	delete(s.offsetMappings, linkID)

	committed, err := s.flush()
	if err != nil && !committed {
		if linkExisted {
			s.links[linkID] = previousLink
		}
		if checkpointsExisted {
			s.checkpoints[linkID] = previousCheckpoints
		}
		if mappingsExisted {
			s.offsetMappings[linkID] = previousMappings
		}
	}
	return err
}

// ListLinks lists all replication links.
func (s *fileStorage) ListLinks() ([]*ReplicationLink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]*ReplicationLink, 0, len(s.links))
	for _, l := range s.links {
		links = append(links, l.Clone())
	}
	return links, nil
}

// SaveCheckpoint saves a checkpoint.
func (s *fileStorage) SaveCheckpoint(checkpoint *Checkpoint) error {
	if checkpoint == nil {
		return fmt.Errorf("checkpoint cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var previous *Checkpoint
	existed := false
	if topics, ok := s.checkpoints[checkpoint.LinkID]; ok {
		if partitions, ok := topics[checkpoint.Topic]; ok {
			previous, existed = partitions[checkpoint.Partition]
		}
	}

	clone := *checkpoint
	clone.Metadata = make(map[string]string, len(checkpoint.Metadata))
	for k, v := range checkpoint.Metadata {
		clone.Metadata[k] = v
	}
	s.putCheckpoint(&clone)

	committed, err := s.flush()
	if err != nil && !committed {
		if existed {
			s.putCheckpoint(previous)
		} else {
			s.removeCheckpoint(checkpoint.LinkID, checkpoint.Topic, checkpoint.Partition)
		}
	}
	return err
}

func (s *fileStorage) removeCheckpoint(linkID, topic string, partition int32) {
	topics, ok := s.checkpoints[linkID]
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
		delete(s.checkpoints, linkID)
	}
}

// LoadCheckpoint loads a checkpoint.
func (s *fileStorage) LoadCheckpoint(linkID, topic string, partition int32) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	linkCheckpoints, exists := s.checkpoints[linkID]
	if !exists {
		return nil, fmt.Errorf("no checkpoints for link %s", linkID)
	}
	topicCheckpoints, exists := linkCheckpoints[topic]
	if !exists {
		return nil, fmt.Errorf("no checkpoints for topic %s", topic)
	}
	checkpoint, exists := topicCheckpoints[partition]
	if !exists {
		return nil, fmt.Errorf("no checkpoint for partition %d", partition)
	}

	result := *checkpoint
	result.Metadata = make(map[string]string, len(checkpoint.Metadata))
	for k, v := range checkpoint.Metadata {
		result.Metadata[k] = v
	}
	return &result, nil
}

// SaveOffsetMapping saves an offset mapping.
func (s *fileStorage) SaveOffsetMapping(mapping *OffsetMapping) error {
	if mapping == nil {
		return fmt.Errorf("offset mapping cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var previous *OffsetMapping
	existed := false
	if topics, ok := s.offsetMappings[mapping.LinkID]; ok {
		if partitions, ok := topics[mapping.Topic]; ok {
			previous, existed = partitions[mapping.Partition]
		}
	}

	clone := *mapping
	clone.Mappings = make(map[int64]int64, len(mapping.Mappings))
	for k, v := range mapping.Mappings {
		clone.Mappings[k] = v
	}
	s.putOffsetMapping(&clone)

	committed, err := s.flush()
	if err != nil && !committed {
		if existed {
			s.putOffsetMapping(previous)
		} else {
			s.removeOffsetMapping(mapping.LinkID, mapping.Topic, mapping.Partition)
		}
	}
	return err
}

func (s *fileStorage) removeOffsetMapping(linkID, topic string, partition int32) {
	topics, ok := s.offsetMappings[linkID]
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
		delete(s.offsetMappings, linkID)
	}
}

// LoadOffsetMapping loads an offset mapping.
func (s *fileStorage) LoadOffsetMapping(linkID, topic string, partition int32) (*OffsetMapping, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	linkMappings, exists := s.offsetMappings[linkID]
	if !exists {
		return nil, fmt.Errorf("no offset mappings for link %s", linkID)
	}
	topicMappings, exists := linkMappings[topic]
	if !exists {
		return nil, fmt.Errorf("no offset mappings for topic %s", topic)
	}
	mapping, exists := topicMappings[partition]
	if !exists {
		return nil, fmt.Errorf("no offset mapping for partition %d", partition)
	}

	result := *mapping
	result.Mappings = make(map[int64]int64, len(mapping.Mappings))
	for k, v := range mapping.Mappings {
		result.Mappings[k] = v
	}
	return &result, nil
}
