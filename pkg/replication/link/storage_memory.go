package link

import (
	"fmt"
	"sync"
)

// memoryStorage is an in-memory implementation of the Storage interface
type memoryStorage struct {
	mu sync.RWMutex

	// links stores replication links by ID
	links map[string]*ReplicationLink

	// checkpoints stores checkpoints by link ID -> topic -> partition
	checkpoints map[string]map[string]map[int32]*Checkpoint

	// offsetMappings stores offset mappings by link ID -> topic -> partition
	offsetMappings map[string]map[string]map[int32]*OffsetMapping
}

// NewMemoryStorage creates a new in-memory storage
func NewMemoryStorage() Storage {
	return &memoryStorage{
		links:          make(map[string]*ReplicationLink),
		checkpoints:    make(map[string]map[string]map[int32]*Checkpoint),
		offsetMappings: make(map[string]map[string]map[int32]*OffsetMapping),
	}
}

// SaveLink saves a replication link
func (s *memoryStorage) SaveLink(link *ReplicationLink) error {
	if link == nil {
		return fmt.Errorf("link cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.links[link.ID] = link.Clone()
	return nil
}

// LoadLink loads a replication link
func (s *memoryStorage) LoadLink(linkID string) (*ReplicationLink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, exists := s.links[linkID]
	if !exists {
		return nil, fmt.Errorf("link %s not found", linkID)
	}

	return link.Clone(), nil
}

// DeleteLink deletes a replication link
func (s *memoryStorage) DeleteLink(linkID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.links, linkID)
	delete(s.checkpoints, linkID)
	delete(s.offsetMappings, linkID)

	return nil
}

// ListLinks lists all replication links
func (s *memoryStorage) ListLinks() ([]*ReplicationLink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]*ReplicationLink, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link.Clone())
	}

	return links, nil
}

// SaveCheckpoint saves a checkpoint
func (s *memoryStorage) SaveCheckpoint(checkpoint *Checkpoint) error {
	if checkpoint == nil {
		return fmt.Errorf("checkpoint cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize nested maps if needed
	if s.checkpoints[checkpoint.LinkID] == nil {
		s.checkpoints[checkpoint.LinkID] = make(map[string]map[int32]*Checkpoint)
	}
	if s.checkpoints[checkpoint.LinkID][checkpoint.Topic] == nil {
		s.checkpoints[checkpoint.LinkID][checkpoint.Topic] = make(map[int32]*Checkpoint)
	}

	// Clone and store checkpoint
	s.checkpoints[checkpoint.LinkID][checkpoint.Topic][checkpoint.Partition] = &Checkpoint{
		LinkID:       checkpoint.LinkID,
		Topic:        checkpoint.Topic,
		Partition:    checkpoint.Partition,
		SourceOffset: checkpoint.SourceOffset,
		TargetOffset: checkpoint.TargetOffset,
		Timestamp:    checkpoint.Timestamp,
		Metadata:     make(map[string]string),
	}

	// Copy metadata
	for k, v := range checkpoint.Metadata {
		s.checkpoints[checkpoint.LinkID][checkpoint.Topic][checkpoint.Partition].Metadata[k] = v
	}

	return nil
}

// LoadCheckpoint loads a checkpoint
func (s *memoryStorage) LoadCheckpoint(linkID, topic string, partition int32) (*Checkpoint, error) {
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

	// Clone and return
	result := &Checkpoint{
		LinkID:       checkpoint.LinkID,
		Topic:        checkpoint.Topic,
		Partition:    checkpoint.Partition,
		SourceOffset: checkpoint.SourceOffset,
		TargetOffset: checkpoint.TargetOffset,
		Timestamp:    checkpoint.Timestamp,
		Metadata:     make(map[string]string),
	}

	for k, v := range checkpoint.Metadata {
		result.Metadata[k] = v
	}

	return result, nil
}

// SaveOffsetMapping saves an offset mapping
func (s *memoryStorage) SaveOffsetMapping(mapping *OffsetMapping) error {
	if mapping == nil {
		return fmt.Errorf("offset mapping cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize nested maps if needed
	if s.offsetMappings[mapping.LinkID] == nil {
		s.offsetMappings[mapping.LinkID] = make(map[string]map[int32]*OffsetMapping)
	}
	if s.offsetMappings[mapping.LinkID][mapping.Topic] == nil {
		s.offsetMappings[mapping.LinkID][mapping.Topic] = make(map[int32]*OffsetMapping)
	}

	// Clone and store mapping
	s.offsetMappings[mapping.LinkID][mapping.Topic][mapping.Partition] = &OffsetMapping{
		LinkID:      mapping.LinkID,
		Topic:       mapping.Topic,
		Partition:   mapping.Partition,
		Mappings:    make(map[int64]int64),
		LastUpdated: mapping.LastUpdated,
	}

	// Copy mappings
	for k, v := range mapping.Mappings {
		s.offsetMappings[mapping.LinkID][mapping.Topic][mapping.Partition].Mappings[k] = v
	}

	return nil
}

// LoadOffsetMapping loads an offset mapping
func (s *memoryStorage) LoadOffsetMapping(linkID, topic string, partition int32) (*OffsetMapping, error) {
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

	// Clone and return
	result := &OffsetMapping{
		LinkID:      mapping.LinkID,
		Topic:       mapping.Topic,
		Partition:   mapping.Partition,
		Mappings:    make(map[int64]int64),
		LastUpdated: mapping.LastUpdated,
	}

	for k, v := range mapping.Mappings {
		result.Mappings[k] = v
	}

	return result, nil
}
