package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gstreamio/streambus/pkg/logger"
	"github.com/gstreamio/streambus/pkg/storage"
	"go.uber.org/zap"
)

// TopicManager manages topics and their partitions
type TopicManager struct {
	mu         sync.RWMutex
	topics     map[string]*Topic
	dataDir    string
	storageDir string
}

// Topic represents a topic with multiple partitions
type Topic struct {
	name       string
	partitions map[uint32]*Partition
	mu         sync.RWMutex
}

// Partition represents a single partition with its log
type Partition struct {
	id  uint32
	log storage.Log
}

// NewTopicManager creates a new topic manager
func NewTopicManager(dataDir string) *TopicManager {
	storageDir := filepath.Join(dataDir, "topics")
	_ = os.MkdirAll(storageDir, 0755)

	tm := &TopicManager{
		topics:     make(map[string]*Topic),
		dataDir:    dataDir,
		storageDir: storageDir,
	}

	// Load existing topics from disk
	_ = tm.loadExistingTopics()

	return tm
}

// loadExistingTopics scans the storage directory for existing topics
func (tm *TopicManager) loadExistingTopics() error {
	logger.Debug("scanning storage directory", zap.String("dir", tm.storageDir))
	entries, err := os.ReadDir(tm.storageDir)
	if err != nil {
		// Directory might not exist on first run
		logger.Debug("storage directory doesn't exist or can't be read", zap.Error(err))
		return nil
	}

	logger.Debug("found entries in storage directory", zap.Int("count", len(entries)))

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		topicName := entry.Name()
		topicDir := filepath.Join(tm.storageDir, topicName)
		logger.Debug("loading topic", zap.String("topic", topicName), zap.String("dir", topicDir))

		// Count partitions by looking for partition directories
		partitionEntries, err := os.ReadDir(topicDir)
		if err != nil {
			continue
		}

		topic := &Topic{
			name:       topicName,
			partitions: make(map[uint32]*Partition),
		}

		for _, partEntry := range partitionEntries {
			if !partEntry.IsDir() || !strings.HasPrefix(partEntry.Name(), "partition-") {
				continue
			}

			// Parse partition ID from directory name
			var partitionID uint32
			if _, err := fmt.Sscanf(partEntry.Name(), "partition-%d", &partitionID); err != nil {
				continue
			}

			// Load the partition with storage recovery
			partitionDir := filepath.Join(topicDir, partEntry.Name())
			logger.Debug("creating log for partition",
				zap.Uint32("partition", partitionID),
				zap.String("dir", partitionDir))
			config := storage.DefaultConfig()
			log, err := storage.NewLog(partitionDir, *config)
			if err != nil {
				logger.Warn("failed to create log for partition",
					zap.Uint32("partition", partitionID),
					zap.Error(err))
				continue
			}

			logger.Debug("successfully loaded partition", zap.Uint32("partition", partitionID))
			topic.partitions[partitionID] = &Partition{
				id:  partitionID,
				log: log,
			}
		}

		if len(topic.partitions) > 0 {
			tm.topics[topicName] = topic
			logger.Info("registered topic",
				zap.String("topic", topicName),
				zap.Int("partitions", len(topic.partitions)))
		} else {
			logger.Debug("skipping topic - no partitions loaded", zap.String("topic", topicName))
		}
	}

	logger.Info("topic loading complete", zap.Int("totalTopics", len(tm.topics)))
	return nil
}

// CreateTopic creates a new topic with the specified number of partitions
func (tm *TopicManager) CreateTopic(name string, numPartitions uint32) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if existing, exists := tm.topics[name]; exists {
		// Check if partition count matches
		existing.mu.RLock()
		existingPartitions := uint32(len(existing.partitions))
		existing.mu.RUnlock()

		if existingPartitions != numPartitions {
			return fmt.Errorf("topic %s already exists with %d partitions, cannot recreate with %d partitions",
				name, existingPartitions, numPartitions)
		}
		// Topic exists with same partition count - idempotent operation
		return nil
	}

	topic := &Topic{
		name:       name,
		partitions: make(map[uint32]*Partition),
	}

	// Create partitions
	for i := uint32(0); i < numPartitions; i++ {
		partition, err := tm.createPartition(name, i)
		if err != nil {
			// Clean up any created partitions
			for j := uint32(0); j < i; j++ {
				if p := topic.partitions[j]; p != nil {
					p.log.Close()
				}
			}
			return fmt.Errorf("failed to create partition %d: %w", i, err)
		}
		topic.partitions[i] = partition
	}

	tm.topics[name] = topic
	return nil
}

// DeleteTopic deletes a topic and all its partitions
func (tm *TopicManager) DeleteTopic(name string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	topic, exists := tm.topics[name]
	if !exists {
		return fmt.Errorf("topic %s does not exist", name)
	}

	// Close all partition logs
	topic.mu.Lock()
	for _, partition := range topic.partitions {
		if err := partition.log.Close(); err != nil {
			topic.mu.Unlock()
			return fmt.Errorf("failed to close partition %d: %w", partition.id, err)
		}
	}
	topic.mu.Unlock()

	// Remove topic directory
	topicDir := filepath.Join(tm.storageDir, name)
	if err := os.RemoveAll(topicDir); err != nil {
		return fmt.Errorf("failed to remove topic directory: %w", err)
	}

	delete(tm.topics, name)
	return nil
}

// GetPartition gets a partition for a topic
func (tm *TopicManager) GetPartition(topic string, partitionID uint32) (*Partition, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	t, exists := tm.topics[topic]
	if !exists {
		return nil, fmt.Errorf("topic %s does not exist", topic)
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	partition, exists := t.partitions[partitionID]
	if !exists {
		return nil, fmt.Errorf("partition %d does not exist in topic %s", partitionID, topic)
	}

	return partition, nil
}

// ListTopics returns a list of all topics
func (tm *TopicManager) ListTopics() []TopicInfo {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	topics := make([]TopicInfo, 0, len(tm.topics))
	for name, topic := range tm.topics {
		topic.mu.RLock()
		topics = append(topics, TopicInfo{
			Name:          name,
			NumPartitions: uint32(len(topic.partitions)),
		})
		topic.mu.RUnlock()
	}

	return topics
}

// TopicExists checks if a topic exists
func (tm *TopicManager) TopicExists(name string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	_, exists := tm.topics[name]

	// Only log at debug level to avoid flooding
	logger.Debug("topic existence check",
		zap.String("topic", name),
		zap.Bool("exists", exists),
		zap.Int("totalTopics", len(tm.topics)))

	return exists
}

// createPartition creates a single partition
func (tm *TopicManager) createPartition(topic string, partitionID uint32) (*Partition, error) {
	// Create partition directory
	partitionDir := filepath.Join(tm.storageDir, topic, fmt.Sprintf("partition-%d", partitionID))
	if err := os.MkdirAll(partitionDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create partition directory: %w", err)
	}

	// Create log config
	config := storage.DefaultConfig()

	// Create new log for partition
	log, err := storage.NewLog(partitionDir, *config)
	if err != nil {
		return nil, fmt.Errorf("failed to create log: %w", err)
	}

	return &Partition{
		id:  partitionID,
		log: log,
	}, nil
}

// Close closes all topic logs
func (tm *TopicManager) Close() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	var lastErr error
	for _, topic := range tm.topics {
		topic.mu.Lock()
		for _, partition := range topic.partitions {
			if err := partition.log.Close(); err != nil {
				lastErr = err
			}
		}
		topic.mu.Unlock()
	}

	return lastErr
}

// TopicInfo holds information about a topic
type TopicInfo struct {
	Name          string
	NumPartitions uint32
}
