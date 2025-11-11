package server

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestTopicManager_DeleteTopic(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()
	tm := NewTopicManager(tempDir)
	defer tm.Close()

	// Create a topic
	err := tm.CreateTopic("test-topic", 2)
	if err != nil {
		t.Fatalf("CreateTopic failed: %v", err)
	}

	// Verify topic exists
	if !tm.TopicExists("test-topic") {
		t.Error("Topic should exist after creation")
	}

	// Delete the topic
	err = tm.DeleteTopic("test-topic")
	if err != nil {
		t.Fatalf("DeleteTopic failed: %v", err)
	}

	// Verify topic no longer exists
	if tm.TopicExists("test-topic") {
		t.Error("Topic should not exist after deletion")
	}

	// Verify topic directory was removed
	topicDir := filepath.Join(tm.storageDir, "test-topic")
	if _, err := os.Stat(topicDir); !os.IsNotExist(err) {
		t.Error("Topic directory should be removed after deletion")
	}
}

func TestTopicManager_DeleteTopic_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	tm := NewTopicManager(tempDir)
	defer tm.Close()

	// Try to delete non-existent topic
	err := tm.DeleteTopic("non-existent")
	if err == nil {
		t.Error("Expected error when deleting non-existent topic")
	}
}

func TestTopicManager_ListTopics(t *testing.T) {
	tempDir := t.TempDir()
	tm := NewTopicManager(tempDir)
	defer tm.Close()

	// Initially should be empty
	topics := tm.ListTopics()
	if len(topics) != 0 {
		t.Errorf("Expected 0 topics, got %d", len(topics))
	}

	// Create multiple topics
	topicNames := []string{"topic-a", "topic-b", "topic-c"}
	partitionCounts := []uint32{1, 2, 3}

	for i, name := range topicNames {
		err := tm.CreateTopic(name, partitionCounts[i])
		if err != nil {
			t.Fatalf("CreateTopic %s failed: %v", name, err)
		}
	}

	// List topics
	topics = tm.ListTopics()
	if len(topics) != 3 {
		t.Errorf("Expected 3 topics, got %d", len(topics))
	}

	// Verify topic info
	topicMap := make(map[string]TopicInfo)
	for _, info := range topics {
		topicMap[info.Name] = info
	}

	for i, name := range topicNames {
		info, exists := topicMap[name]
		if !exists {
			t.Errorf("Topic %s not in list", name)
			continue
		}
		if info.NumPartitions != partitionCounts[i] {
			t.Errorf("Topic %s has %d partitions, want %d",
				name, info.NumPartitions, partitionCounts[i])
		}
	}
}

func TestTopicManager_TopicExists(t *testing.T) {
	tempDir := t.TempDir()
	tm := NewTopicManager(tempDir)
	defer tm.Close()

	// Check non-existent topic
	if tm.TopicExists("non-existent") {
		t.Error("TopicExists returned true for non-existent topic")
	}

	// Create topic
	err := tm.CreateTopic("existing-topic", 1)
	if err != nil {
		t.Fatalf("CreateTopic failed: %v", err)
	}

	// Check existing topic
	if !tm.TopicExists("existing-topic") {
		t.Error("TopicExists returned false for existing topic")
	}

	// Delete topic
	err = tm.DeleteTopic("existing-topic")
	if err != nil {
		t.Fatalf("DeleteTopic failed: %v", err)
	}

	// Check deleted topic
	if tm.TopicExists("existing-topic") {
		t.Error("TopicExists returned true for deleted topic")
	}
}

func TestTopicManager_Close(t *testing.T) {
	tempDir := t.TempDir()
	tm := NewTopicManager(tempDir)

	// Create multiple topics
	for i := 0; i < 3; i++ {
		err := tm.CreateTopic(fmt.Sprintf("topic-%d", i), 2)
		if err != nil {
			t.Fatalf("CreateTopic failed: %v", err)
		}
	}

	// Close should succeed
	err := tm.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Close again should still work (idempotent)
	err = tm.Close()
	if err != nil {
		t.Errorf("Second Close failed: %v", err)
	}
}

func TestTopicManager_CreateTopic_DuplicateError(t *testing.T) {
	tempDir := t.TempDir()
	tm := NewTopicManager(tempDir)
	defer tm.Close()

	// Create topic
	err := tm.CreateTopic("duplicate-topic", 1)
	if err != nil {
		t.Fatalf("CreateTopic failed: %v", err)
	}

	// Try to create duplicate
	err = tm.CreateTopic("duplicate-topic", 1)
	if err == nil {
		t.Error("Expected error when creating duplicate topic")
	}
}

func TestTopicManager_MultiplePartitions(t *testing.T) {
	tempDir := t.TempDir()
	tm := NewTopicManager(tempDir)
	defer tm.Close()

	// Create topic with multiple partitions
	numPartitions := uint32(5)
	err := tm.CreateTopic("multi-partition-topic", numPartitions)
	if err != nil {
		t.Fatalf("CreateTopic failed: %v", err)
	}

	// Verify all partitions exist
	for i := uint32(0); i < numPartitions; i++ {
		partition, err := tm.GetPartition("multi-partition-topic", i)
		if err != nil {
			t.Errorf("GetPartition %d failed: %v", i, err)
		}
		if partition == nil {
			t.Errorf("Partition %d is nil", i)
		}
		if partition.id != i {
			t.Errorf("Partition ID = %d, want %d", partition.id, i)
		}
	}

	// Verify partition count in ListTopics
	topics := tm.ListTopics()
	if len(topics) != 1 {
		t.Fatalf("Expected 1 topic, got %d", len(topics))
	}
	if topics[0].NumPartitions != numPartitions {
		t.Errorf("NumPartitions = %d, want %d", topics[0].NumPartitions, numPartitions)
	}
}

func TestTopicManager_ConcurrentOperations(t *testing.T) {
	tempDir := t.TempDir()
	tm := NewTopicManager(tempDir)
	defer tm.Close()

	// Create a topic
	err := tm.CreateTopic("concurrent-topic", 2)
	if err != nil {
		t.Fatalf("CreateTopic failed: %v", err)
	}

	// Test concurrent reads
	done := make(chan bool, 10)

	for i := 0; i < 5; i++ {
		go func() {
			tm.TopicExists("concurrent-topic")
			tm.ListTopics()
			done <- true
		}()
	}

	for i := 0; i < 5; i++ {
		go func(id int) {
			_, _ = tm.GetPartition("concurrent-topic", uint32(id%2))
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	t.Log("Concurrent operations completed successfully")
}
