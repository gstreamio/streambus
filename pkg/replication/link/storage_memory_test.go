package link

import (
	"testing"
	"time"
)

func TestMemoryStorage_LoadLink(t *testing.T) {
	storage := NewMemoryStorage()

	// Create and save a link
	link := createTestLink("load-test", "Load Test")
	if err := storage.SaveLink(link); err != nil {
		t.Fatalf("SaveLink failed: %v", err)
	}

	// Load the link
	loaded, err := storage.LoadLink("load-test")
	if err != nil {
		t.Fatalf("LoadLink failed: %v", err)
	}

	if loaded.ID != "load-test" {
		t.Errorf("LoadLink ID = %s, want load-test", loaded.ID)
	}

	if loaded.Name != "Load Test" {
		t.Errorf("LoadLink Name = %s, want Load Test", loaded.Name)
	}
}

func TestMemoryStorage_LoadLink_NotFound(t *testing.T) {
	storage := NewMemoryStorage()

	_, err := storage.LoadLink("non-existent")
	if err == nil {
		t.Error("Expected error when loading non-existent link")
	}
}

func TestMemoryStorage_LoadCheckpoint(t *testing.T) {
	storage := NewMemoryStorage()

	// Create and save a checkpoint
	checkpoint := &Checkpoint{
		LinkID:       "cp-test",
		Topic:        "test-topic",
		Partition:    0,
		SourceOffset: 100,
		TargetOffset: 95,
		Timestamp:    time.Now(),
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	if err := storage.SaveCheckpoint(checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	// Load the checkpoint
	loaded, err := storage.LoadCheckpoint("cp-test", "test-topic", 0)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}

	if loaded.LinkID != "cp-test" {
		t.Errorf("LinkID = %s, want cp-test", loaded.LinkID)
	}

	if loaded.SourceOffset != 100 {
		t.Errorf("SourceOffset = %d, want 100", loaded.SourceOffset)
	}

	if loaded.TargetOffset != 95 {
		t.Errorf("TargetOffset = %d, want 95", loaded.TargetOffset)
	}

	if loaded.Metadata["key1"] != "value1" {
		t.Errorf("Metadata[key1] = %s, want value1", loaded.Metadata["key1"])
	}

	if loaded.Metadata["key2"] != "value2" {
		t.Errorf("Metadata[key2] = %s, want value2", loaded.Metadata["key2"])
	}
}

func TestMemoryStorage_LoadCheckpoint_NotFound(t *testing.T) {
	storage := NewMemoryStorage()

	// Test loading checkpoint for non-existent link
	_, err := storage.LoadCheckpoint("non-existent", "test-topic", 0)
	if err == nil {
		t.Error("Expected error when loading checkpoint for non-existent link")
	}

	// Create a checkpoint to test other error paths
	checkpoint := &Checkpoint{
		LinkID:       "link-test",
		Topic:        "test-topic",
		Partition:    0,
		SourceOffset: 100,
		TargetOffset: 95,
		Timestamp:    time.Now(),
	}

	if err := storage.SaveCheckpoint(checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	// Test loading checkpoint for non-existent topic
	_, err = storage.LoadCheckpoint("link-test", "non-existent-topic", 0)
	if err == nil {
		t.Error("Expected error when loading checkpoint for non-existent topic")
	}

	// Test loading checkpoint for non-existent partition
	_, err = storage.LoadCheckpoint("link-test", "test-topic", 999)
	if err == nil {
		t.Error("Expected error when loading checkpoint for non-existent partition")
	}
}

func TestMemoryStorage_SaveOffsetMapping(t *testing.T) {
	storage := NewMemoryStorage()

	// Create and save an offset mapping
	mapping := &OffsetMapping{
		LinkID:    "mapping-test",
		Topic:     "test-topic",
		Partition: 0,
		Mappings: map[int64]int64{
			100: 200,
			101: 201,
			102: 202,
		},
		LastUpdated: time.Now(),
	}

	if err := storage.SaveOffsetMapping(mapping); err != nil {
		t.Fatalf("SaveOffsetMapping failed: %v", err)
	}

	// Load and verify
	loaded, err := storage.LoadOffsetMapping("mapping-test", "test-topic", 0)
	if err != nil {
		t.Fatalf("LoadOffsetMapping failed: %v", err)
	}

	if loaded.LinkID != "mapping-test" {
		t.Errorf("LinkID = %s, want mapping-test", loaded.LinkID)
	}

	if loaded.Mappings[100] != 200 {
		t.Errorf("Mappings[100] = %d, want 200", loaded.Mappings[100])
	}

	if loaded.Mappings[101] != 201 {
		t.Errorf("Mappings[101] = %d, want 201", loaded.Mappings[101])
	}

	if loaded.Mappings[102] != 202 {
		t.Errorf("Mappings[102] = %d, want 202", loaded.Mappings[102])
	}
}

func TestMemoryStorage_SaveOffsetMapping_Nil(t *testing.T) {
	storage := NewMemoryStorage()

	err := storage.SaveOffsetMapping(nil)
	if err == nil {
		t.Error("Expected error when saving nil offset mapping")
	}
}

func TestMemoryStorage_LoadOffsetMapping(t *testing.T) {
	storage := NewMemoryStorage()

	// Create and save an offset mapping
	mapping := &OffsetMapping{
		LinkID:    "load-mapping-test",
		Topic:     "test-topic",
		Partition: 1,
		Mappings: map[int64]int64{
			200: 300,
			201: 301,
		},
		LastUpdated: time.Now(),
	}

	if err := storage.SaveOffsetMapping(mapping); err != nil {
		t.Fatalf("SaveOffsetMapping failed: %v", err)
	}

	// Load the mapping
	loaded, err := storage.LoadOffsetMapping("load-mapping-test", "test-topic", 1)
	if err != nil {
		t.Fatalf("LoadOffsetMapping failed: %v", err)
	}

	if loaded.Topic != "test-topic" {
		t.Errorf("Topic = %s, want test-topic", loaded.Topic)
	}

	if loaded.Partition != 1 {
		t.Errorf("Partition = %d, want 1", loaded.Partition)
	}

	if len(loaded.Mappings) != 2 {
		t.Errorf("Mappings count = %d, want 2", len(loaded.Mappings))
	}
}

func TestMemoryStorage_LoadOffsetMapping_NotFound(t *testing.T) {
	storage := NewMemoryStorage()

	// Test loading mapping for non-existent link
	_, err := storage.LoadOffsetMapping("non-existent", "test-topic", 0)
	if err == nil {
		t.Error("Expected error when loading mapping for non-existent link")
	}

	// Create a mapping to test other error paths
	mapping := &OffsetMapping{
		LinkID:      "link-test",
		Topic:       "test-topic",
		Partition:   0,
		Mappings:    map[int64]int64{100: 200},
		LastUpdated: time.Now(),
	}

	if err := storage.SaveOffsetMapping(mapping); err != nil {
		t.Fatalf("SaveOffsetMapping failed: %v", err)
	}

	// Test loading mapping for non-existent topic
	_, err = storage.LoadOffsetMapping("link-test", "non-existent-topic", 0)
	if err == nil {
		t.Error("Expected error when loading mapping for non-existent topic")
	}

	// Test loading mapping for non-existent partition
	_, err = storage.LoadOffsetMapping("link-test", "test-topic", 999)
	if err == nil {
		t.Error("Expected error when loading mapping for non-existent partition")
	}
}

func TestMemoryStorage_DeleteLink_CleansUpAll(t *testing.T) {
	storage := NewMemoryStorage()

	// Create link, checkpoint, and offset mapping
	link := createTestLink("cleanup-test", "Cleanup Test")
	if err := storage.SaveLink(link); err != nil {
		t.Fatalf("SaveLink failed: %v", err)
	}

	checkpoint := &Checkpoint{
		LinkID:       "cleanup-test",
		Topic:        "test-topic",
		Partition:    0,
		SourceOffset: 100,
		TargetOffset: 95,
		Timestamp:    time.Now(),
	}
	if err := storage.SaveCheckpoint(checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	mapping := &OffsetMapping{
		LinkID:      "cleanup-test",
		Topic:       "test-topic",
		Partition:   0,
		Mappings:    map[int64]int64{100: 200},
		LastUpdated: time.Now(),
	}
	if err := storage.SaveOffsetMapping(mapping); err != nil {
		t.Fatalf("SaveOffsetMapping failed: %v", err)
	}

	// Verify everything exists
	if _, err := storage.LoadLink("cleanup-test"); err != nil {
		t.Fatalf("Link should exist before delete: %v", err)
	}
	if _, err := storage.LoadCheckpoint("cleanup-test", "test-topic", 0); err != nil {
		t.Fatalf("Checkpoint should exist before delete: %v", err)
	}
	if _, err := storage.LoadOffsetMapping("cleanup-test", "test-topic", 0); err != nil {
		t.Fatalf("Mapping should exist before delete: %v", err)
	}

	// Delete the link
	if err := storage.DeleteLink("cleanup-test"); err != nil {
		t.Fatalf("DeleteLink failed: %v", err)
	}

	// Verify everything is deleted
	if _, err := storage.LoadLink("cleanup-test"); err == nil {
		t.Error("Link should not exist after delete")
	}
	if _, err := storage.LoadCheckpoint("cleanup-test", "test-topic", 0); err == nil {
		t.Error("Checkpoint should not exist after delete")
	}
	if _, err := storage.LoadOffsetMapping("cleanup-test", "test-topic", 0); err == nil {
		t.Error("Mapping should not exist after delete")
	}
}

func TestMemoryStorage_SaveLink_Nil(t *testing.T) {
	storage := NewMemoryStorage()

	err := storage.SaveLink(nil)
	if err == nil {
		t.Error("Expected error when saving nil link")
	}
}

func TestMemoryStorage_SaveCheckpoint_Nil(t *testing.T) {
	storage := NewMemoryStorage()

	err := storage.SaveCheckpoint(nil)
	if err == nil {
		t.Error("Expected error when saving nil checkpoint")
	}
}

func TestMemoryStorage_ConcurrentAccess(t *testing.T) {
	storage := NewMemoryStorage()

	// Test concurrent saves and loads
	done := make(chan bool, 10)

	for i := 0; i < 5; i++ {
		go func(id int) {
			link := createTestLink("concurrent-test", "Concurrent Test")
			storage.SaveLink(link)
			storage.LoadLink("concurrent-test")
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		go func(id int) {
			storage.ListLinks()
			done <- true
		}(i)
	}

	// Wait for all goroutines to finish
	for i := 0; i < 10; i++ {
		<-done
	}

	t.Log("Concurrent access test passed")
}
