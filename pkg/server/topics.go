package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/shawntherrien/streambus/pkg/storage"
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
	os.MkdirAll(storageDir, 0755)

	tm := &TopicManager{
		topics:     make(map[string]*Topic),
		dataDir:    dataDir,
		storageDir: storageDir,
	}

	// Load existing topics from disk
	tm.loadExistingTopics()

	return tm
}

// loadExistingTopics scans the storage directory for existing topics
func (tm *TopicManager) loadExistingTopics() error {
	fmt.Printf("[TOPIC-LOAD] Scanning storage directory: %s\n", tm.storageDir)
	entries, err := os.ReadDir(tm.storageDir)
	if err != nil {
		// Directory might not exist on first run
		fmt.Printf("[TOPIC-LOAD] Storage directory doesn't exist or can't be read: %v\n", err)
		return nil
	}

	fmt.Printf("[TOPIC-LOAD] Found %d entries in storage directory\n", len(entries))

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		topicName := entry.Name()
		topicDir := filepath.Join(tm.storageDir, topicName)
		fmt.Printf("[TOPIC-LOAD] Loading topic: %s from %s\n", topicName, topicDir)

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
			fmt.Printf("[TOPIC-LOAD] Creating log for partition %d at %s\n", partitionID, partitionDir)
			config := storage.DefaultConfig()
			log, err := storage.NewLog(partitionDir, *config)
			if err != nil {
				fmt.Printf("[TOPIC-LOAD] Failed to create log for partition %d: %v\n", partitionID, err)
				continue
			}

			fmt.Printf("[TOPIC-LOAD] Successfully loaded partition %d\n", partitionID)
			topic.partitions[partitionID] = &Partition{
				id:  partitionID,
				log: log,
			}
		}

		if len(topic.partitions) > 0 {
			tm.topics[topicName] = topic
			fmt.Printf("[TOPIC-LOAD] Successfully registered topic '%s' with %d partitions\n", topicName, len(topic.partitions))
		} else {
			fmt.Printf("[TOPIC-LOAD] Skipping topic '%s' - no partitions loaded\n", topicName)
		}
	}

	fmt.Printf("[TOPIC-LOAD] Loading complete. Total topics loaded: %d\n", len(tm.topics))
	for name := range tm.topics {
		fmt.Printf("[TOPIC-LOAD]   - %s\n", name)
	}

	return nil
}

// CreateTopic creates a new topic with the specified number of partitions
func (tm *TopicManager) CreateTopic(name string, numPartitions uint32) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.topics[name]; exists {
		// Topic already loaded, nothing to do
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

	fmt.Printf("[TOPIC-EXISTS] Checking for topic '%s': exists=%v (total topics: %d)\n", name, exists, len(tm.topics))
	if !exists && len(tm.topics) > 0 {
		fmt.Printf("[TOPIC-EXISTS] Available topics:\n")
		for topicName := range tm.topics {
			fmt.Printf("[TOPIC-EXISTS]   - %s\n", topicName)
		}
	}

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
