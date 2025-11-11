package group

import (
	"testing"
)

func TestNewOffsetManager(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	manager := NewOffsetManager(storage)

	if manager == nil {
		t.Fatal("NewOffsetManager returned nil")
	}

	if manager.storage == nil {
		t.Error("OffsetManager should have storage")
	}
}

func TestOffsetManager_CommitOffset(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	manager := NewOffsetManager(storage)

	// Commit an offset
	err := manager.CommitOffset("test-group", "test-topic", 0, 100, "test metadata")
	if err != nil {
		t.Fatalf("CommitOffset failed: %v", err)
	}

	// Verify it was stored
	offsetMeta, err := storage.FetchOffset("test-group", "test-topic", 0)
	if err != nil {
		t.Fatalf("FetchOffset failed: %v", err)
	}

	if offsetMeta == nil {
		t.Fatal("Expected offset to be stored")
	}

	if offsetMeta.Offset != 100 {
		t.Errorf("Offset = %d, want 100", offsetMeta.Offset)
	}

	if offsetMeta.Metadata != "test metadata" {
		t.Errorf("Metadata = %s, want 'test metadata'", offsetMeta.Metadata)
	}
}

func TestOffsetManager_GetOffset(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	manager := NewOffsetManager(storage)

	// Get non-existent offset - should return -1
	offset, err := manager.GetOffset("test-group", "test-topic", 0)
	if err != nil {
		t.Fatalf("GetOffset failed: %v", err)
	}
	if offset != -1 {
		t.Errorf("Offset = %d, want -1 for non-existent offset", offset)
	}

	// Commit an offset
	err = manager.CommitOffset("test-group", "test-topic", 0, 100, "")
	if err != nil {
		t.Fatalf("CommitOffset failed: %v", err)
	}

	// Get committed offset
	offset, err = manager.GetOffset("test-group", "test-topic", 0)
	if err != nil {
		t.Fatalf("GetOffset failed: %v", err)
	}
	if offset != 100 {
		t.Errorf("Offset = %d, want 100", offset)
	}
}

func TestOffsetManager_GetAllOffsets(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	manager := NewOffsetManager(storage)

	// Get offsets for non-existent group - should return empty map
	offsets, err := manager.GetAllOffsets("non-existent-group")
	if err != nil {
		t.Fatalf("GetAllOffsets failed: %v", err)
	}
	if len(offsets) != 0 {
		t.Errorf("Expected empty map, got %d topics", len(offsets))
	}

	// Commit multiple offsets
	testOffsets := map[string]map[int32]int64{
		"topic-a": {
			0: 100,
			1: 200,
			2: 300,
		},
		"topic-b": {
			0: 400,
			1: 500,
		},
	}

	for topic, partitions := range testOffsets {
		for partition, offset := range partitions {
			err := manager.CommitOffset("test-group", topic, partition, offset, "")
			if err != nil {
				t.Fatalf("CommitOffset failed for %s:%d: %v", topic, partition, err)
			}
		}
	}

	// Get all offsets
	allOffsets, err := manager.GetAllOffsets("test-group")
	if err != nil {
		t.Fatalf("GetAllOffsets failed: %v", err)
	}

	if len(allOffsets) != 2 {
		t.Errorf("Expected 2 topics, got %d", len(allOffsets))
	}

	// Verify all offsets match
	for topic, partitions := range testOffsets {
		retrievedPartitions, exists := allOffsets[topic]
		if !exists {
			t.Errorf("Topic %s not found in result", topic)
			continue
		}

		for partition, expectedOffset := range partitions {
			retrievedOffset, exists := retrievedPartitions[partition]
			if !exists {
				t.Errorf("Partition %d not found for topic %s", partition, topic)
				continue
			}

			if retrievedOffset != expectedOffset {
				t.Errorf("Topic %s, Partition %d: offset = %d, want %d",
					topic, partition, retrievedOffset, expectedOffset)
			}
		}
	}
}

func TestOffsetManager_ResetOffsets(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	manager := NewOffsetManager(storage)

	// Commit some offsets
	err := manager.CommitOffset("test-group", "test-topic", 0, 100, "")
	if err != nil {
		t.Fatalf("CommitOffset failed: %v", err)
	}

	err = manager.CommitOffset("test-group", "test-topic", 1, 200, "")
	if err != nil {
		t.Fatalf("CommitOffset failed: %v", err)
	}

	// Verify offsets exist
	offset, err := manager.GetOffset("test-group", "test-topic", 0)
	if err != nil {
		t.Fatalf("GetOffset failed: %v", err)
	}
	if offset != 100 {
		t.Errorf("Offset = %d, want 100", offset)
	}

	// Reset offsets
	err = manager.ResetOffsets("test-group")
	if err != nil {
		t.Fatalf("ResetOffsets failed: %v", err)
	}

	// Verify offsets are gone
	offset, err = manager.GetOffset("test-group", "test-topic", 0)
	if err != nil {
		t.Fatalf("GetOffset failed: %v", err)
	}
	if offset != -1 {
		t.Errorf("Offset = %d, want -1 after reset", offset)
	}

	offset, err = manager.GetOffset("test-group", "test-topic", 1)
	if err != nil {
		t.Fatalf("GetOffset failed: %v", err)
	}
	if offset != -1 {
		t.Errorf("Offset = %d, want -1 after reset", offset)
	}
}

func TestOffsetManager_ValidateOffset(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	manager := NewOffsetManager(storage)

	tests := []struct {
		name      string
		offset    int64
		minOffset int64
		maxOffset int64
		wantErr   bool
	}{
		{
			name:      "valid offset",
			offset:    50,
			minOffset: 0,
			maxOffset: 100,
			wantErr:   false,
		},
		{
			name:      "offset at minimum",
			offset:    0,
			minOffset: 0,
			maxOffset: 100,
			wantErr:   false,
		},
		{
			name:      "offset at maximum",
			offset:    100,
			minOffset: 0,
			maxOffset: 100,
			wantErr:   false,
		},
		{
			name:      "offset below minimum",
			offset:    -1,
			minOffset: 0,
			maxOffset: 100,
			wantErr:   true,
		},
		{
			name:      "offset above maximum",
			offset:    101,
			minOffset: 0,
			maxOffset: 100,
			wantErr:   true,
		},
		{
			name:      "offset far below minimum",
			offset:    -100,
			minOffset: 0,
			maxOffset: 100,
			wantErr:   true,
		},
		{
			name:      "offset far above maximum",
			offset:    1000,
			minOffset: 0,
			maxOffset: 100,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateOffset(tt.offset, tt.minOffset, tt.maxOffset)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOffset() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOffsetManager_CommitMultipleGroups(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	manager := NewOffsetManager(storage)

	// Commit offsets for multiple groups
	groups := []string{"group-a", "group-b", "group-c"}
	for i, group := range groups {
		err := manager.CommitOffset(group, "test-topic", 0, int64((i+1)*100), "")
		if err != nil {
			t.Fatalf("CommitOffset failed for %s: %v", group, err)
		}
	}

	// Verify each group has its own offset
	for i, group := range groups {
		offset, err := manager.GetOffset(group, "test-topic", 0)
		if err != nil {
			t.Fatalf("GetOffset failed for %s: %v", group, err)
		}

		expectedOffset := int64((i + 1) * 100)
		if offset != expectedOffset {
			t.Errorf("Group %s: offset = %d, want %d", group, offset, expectedOffset)
		}
	}
}

func TestOffsetManager_OverwriteOffset(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	manager := NewOffsetManager(storage)

	// Commit initial offset
	err := manager.CommitOffset("test-group", "test-topic", 0, 100, "initial")
	if err != nil {
		t.Fatalf("CommitOffset failed: %v", err)
	}

	// Verify initial offset
	offset, err := manager.GetOffset("test-group", "test-topic", 0)
	if err != nil {
		t.Fatalf("GetOffset failed: %v", err)
	}
	if offset != 100 {
		t.Errorf("Initial offset = %d, want 100", offset)
	}

	// Overwrite with new offset
	err = manager.CommitOffset("test-group", "test-topic", 0, 200, "updated")
	if err != nil {
		t.Fatalf("CommitOffset (overwrite) failed: %v", err)
	}

	// Verify new offset
	offset, err = manager.GetOffset("test-group", "test-topic", 0)
	if err != nil {
		t.Fatalf("GetOffset failed: %v", err)
	}
	if offset != 200 {
		t.Errorf("Updated offset = %d, want 200", offset)
	}
}

func TestOffsetManager_ConcurrentCommits(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	manager := NewOffsetManager(storage)

	// Concurrent commits to different partitions
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(partition int32) {
			err := manager.CommitOffset("test-group", "test-topic", partition, int64(partition*100), "")
			if err != nil {
				t.Errorf("CommitOffset failed for partition %d: %v", partition, err)
			}
			done <- true
		}(int32(i))
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all offsets were committed
	for i := int32(0); i < 10; i++ {
		offset, err := manager.GetOffset("test-group", "test-topic", i)
		if err != nil {
			t.Errorf("GetOffset failed for partition %d: %v", i, err)
		}

		expectedOffset := int64(i * 100)
		if offset != expectedOffset {
			t.Errorf("Partition %d: offset = %d, want %d", i, offset, expectedOffset)
		}
	}
}
