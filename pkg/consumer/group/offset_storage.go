package group

import (
	"fmt"
	"sync"
)

// MemoryOffsetStorage is an in-memory implementation of OffsetStorage
type MemoryOffsetStorage struct {
	mu sync.RWMutex //nolint:unused // Protects concurrent access to offsets map
	// offsets: groupID -> topic -> partition -> OffsetAndMetadata
	offsets map[string]map[string]map[int32]*OffsetAndMetadata
}

// NewMemoryOffsetStorage creates a new in-memory offset storage
func NewMemoryOffsetStorage() *MemoryOffsetStorage {
	return &MemoryOffsetStorage{
		offsets: make(map[string]map[string]map[int32]*OffsetAndMetadata),
	}
}

// StoreOffset stores an offset for a group/topic/partition
func (s *MemoryOffsetStorage) StoreOffset(groupID string, topic string, partition int32, offset *OffsetAndMetadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize maps if needed
	if _, exists := s.offsets[groupID]; !exists {
		s.offsets[groupID] = make(map[string]map[int32]*OffsetAndMetadata)
	}
	if _, exists := s.offsets[groupID][topic]; !exists {
		s.offsets[groupID][topic] = make(map[int32]*OffsetAndMetadata)
	}

	// Store offset
	s.offsets[groupID][topic][partition] = offset

	return nil
}

// FetchOffset fetches an offset for a group/topic/partition
func (s *MemoryOffsetStorage) FetchOffset(groupID string, topic string, partition int32) (*OffsetAndMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupOffsets, exists := s.offsets[groupID]
	if !exists {
		return nil, nil // No offsets for this group
	}

	topicOffsets, exists := groupOffsets[topic]
	if !exists {
		return nil, nil // No offsets for this topic
	}

	offset, exists := topicOffsets[partition]
	if !exists {
		return nil, nil // No offset for this partition
	}

	return offset, nil
}

// FetchOffsets fetches all offsets for a group
func (s *MemoryOffsetStorage) FetchOffsets(groupID string) (*GroupOffsets, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	groupOffsets, exists := s.offsets[groupID]
	if !exists {
		return &GroupOffsets{
			GroupID: groupID,
			Offsets: make(map[string]map[int32]*OffsetAndMetadata),
		}, nil
	}

	// Deep copy to avoid concurrent modification
	result := &GroupOffsets{
		GroupID: groupID,
		Offsets: make(map[string]map[int32]*OffsetAndMetadata),
	}

	for topic, partitions := range groupOffsets {
		result.Offsets[topic] = make(map[int32]*OffsetAndMetadata)
		for partition, offset := range partitions {
			// Create a copy
			offsetCopy := *offset
			result.Offsets[topic][partition] = &offsetCopy
		}
	}

	return result, nil
}

// DeleteOffsets deletes all offsets for a group
func (s *MemoryOffsetStorage) DeleteOffsets(groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.offsets, groupID)
	return nil
}

// PersistentOffsetStorage is a persistent implementation using the metadata store
type PersistentOffsetStorage struct {
	store MetadataStore
}

// MetadataStore interface for persisting offsets
type MetadataStore interface {
	StoreGroupOffset(groupID string, topic string, partition int32, offset *OffsetAndMetadata) error
	FetchGroupOffset(groupID string, topic string, partition int32) (*OffsetAndMetadata, error)
	FetchGroupOffsets(groupID string) (*GroupOffsets, error)
	DeleteGroupOffsets(groupID string) error
}

// NewPersistentOffsetStorage creates a new persistent offset storage
func NewPersistentOffsetStorage(store MetadataStore) *PersistentOffsetStorage {
	return &PersistentOffsetStorage{
		store: store,
	}
}

// StoreOffset stores an offset persistently
func (s *PersistentOffsetStorage) StoreOffset(groupID string, topic string, partition int32, offset *OffsetAndMetadata) error {
	return s.store.StoreGroupOffset(groupID, topic, partition, offset)
}

// FetchOffset fetches an offset from persistent storage
func (s *PersistentOffsetStorage) FetchOffset(groupID string, topic string, partition int32) (*OffsetAndMetadata, error) {
	return s.store.FetchGroupOffset(groupID, topic, partition)
}

// FetchOffsets fetches all offsets for a group from persistent storage
func (s *PersistentOffsetStorage) FetchOffsets(groupID string) (*GroupOffsets, error) {
	return s.store.FetchGroupOffsets(groupID)
}

// DeleteOffsets deletes offsets from persistent storage
func (s *PersistentOffsetStorage) DeleteOffsets(groupID string) error {
	return s.store.DeleteGroupOffsets(groupID)
}

// OffsetCommitLog represents a log of offset commits for audit/replay
type OffsetCommitLog struct {
	GroupID   string
	Topic     string
	Partition int32
	Offset    int64
	Metadata  string
	Timestamp int64
	MemberID  string
}

// OffsetManager provides higher-level offset management operations
type OffsetManager struct {
	storage OffsetStorage
	mu      sync.RWMutex //nolint:unused // Reserved for future use in concurrent offset operations
}

// NewOffsetManager creates a new offset manager
func NewOffsetManager(storage OffsetStorage) *OffsetManager {
	return &OffsetManager{
		storage: storage,
	}
}

// CommitOffset commits a single offset
func (m *OffsetManager) CommitOffset(groupID string, topic string, partition int32, offset int64, metadata string) error {
	offsetMeta := &OffsetAndMetadata{
		Offset:   offset,
		Metadata: metadata,
	}
	return m.storage.StoreOffset(groupID, topic, partition, offsetMeta)
}

// GetOffset gets a committed offset
func (m *OffsetManager) GetOffset(groupID string, topic string, partition int32) (int64, error) {
	offsetMeta, err := m.storage.FetchOffset(groupID, topic, partition)
	if err != nil {
		return -1, err
	}
	if offsetMeta == nil {
		return -1, nil // No committed offset
	}
	return offsetMeta.Offset, nil
}

// GetAllOffsets gets all committed offsets for a group
func (m *OffsetManager) GetAllOffsets(groupID string) (map[string]map[int32]int64, error) {
	groupOffsets, err := m.storage.FetchOffsets(groupID)
	if err != nil {
		return nil, err
	}

	result := make(map[string]map[int32]int64)
	for topic, partitions := range groupOffsets.Offsets {
		result[topic] = make(map[int32]int64)
		for partition, offsetMeta := range partitions {
			result[topic][partition] = offsetMeta.Offset
		}
	}

	return result, nil
}

// ResetOffsets resets all offsets for a group
func (m *OffsetManager) ResetOffsets(groupID string) error {
	return m.storage.DeleteOffsets(groupID)
}

// ValidateOffset validates that an offset is within valid range
func (m *OffsetManager) ValidateOffset(offset int64, minOffset int64, maxOffset int64) error {
	if offset < minOffset {
		return fmt.Errorf("offset %d is less than minimum offset %d", offset, minOffset)
	}
	if offset > maxOffset {
		return fmt.Errorf("offset %d exceeds maximum offset %d", offset, maxOffset)
	}
	return nil
}
