package group

import (
	"errors"
	"testing"
)

func TestGroupCoordinator_ListGroups(t *testing.T) {
	offsetStorage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	gc := NewGroupCoordinator(offsetStorage, config)
	defer func() { _ = gc.Stop() }()

	// Initially empty
	groups := gc.ListGroups()
	if len(groups) != 0 {
		t.Errorf("Expected 0 groups, got %d", len(groups))
	}

	// Create some groups by joining
	for i := 0; i < 3; i++ {
		req := &JoinGroupRequest{
			GroupID:            "test-group-" + string(rune('a'+i)),
			SessionTimeoutMs:   30000,
			RebalanceTimeoutMs: 60000,
			ClientID:           "client-" + string(rune('a'+i)),
			ProtocolType:       "consumer",
			Protocols: []ProtocolMetadata{
				{Name: "range", Metadata: []byte("test")},
			},
		}
		_, _ = gc.HandleJoinGroup(req)
	}

	// List groups
	groups = gc.ListGroups()
	if len(groups) != 3 {
		t.Errorf("Expected 3 groups, got %d", len(groups))
	}

	// Verify group IDs
	groupIDs := make(map[string]bool)
	for _, group := range groups {
		groupIDs[group.GroupID] = true
	}

	for i := 0; i < 3; i++ {
		expectedID := "test-group-" + string(rune('a'+i))
		if !groupIDs[expectedID] {
			t.Errorf("Group %s not found in list", expectedID)
		}
	}
}

func TestGroupCoordinator_GetGroup(t *testing.T) {
	offsetStorage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	gc := NewGroupCoordinator(offsetStorage, config)
	defer func() { _ = gc.Stop() }()

	// Get non-existent group
	group := gc.GetGroup("non-existent")
	if group != nil {
		t.Error("Expected nil for non-existent group")
	}

	// Create a group
	req := &JoinGroupRequest{
		GroupID:            "test-group",
		SessionTimeoutMs:   30000,
		RebalanceTimeoutMs: 60000,
		ClientID:           "client-1",
		ProtocolType:       "consumer",
		Protocols: []ProtocolMetadata{
			{Name: "range", Metadata: []byte("test")},
		},
	}
	_, _ = gc.HandleJoinGroup(req)

	// Get existing group
	group = gc.GetGroup("test-group")
	if group == nil {
		t.Fatal("Expected group to exist")
	}

	if group.GroupID != "test-group" {
		t.Errorf("GroupID = %s, want test-group", group.GroupID)
	}

	if group.ProtocolType != "consumer" {
		t.Errorf("ProtocolType = %s, want consumer", group.ProtocolType)
	}

	if group.State != GroupStatePreparingRebalance {
		t.Errorf("State = %v, want GroupStatePreparingRebalance", group.State)
	}
}

func TestGroupCoordinator_GetGroup_ImmutabilityCopy(t *testing.T) {
	offsetStorage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	gc := NewGroupCoordinator(offsetStorage, config)
	defer func() { _ = gc.Stop() }()

	// Create a group
	req := &JoinGroupRequest{
		GroupID:            "test-group",
		SessionTimeoutMs:   30000,
		RebalanceTimeoutMs: 60000,
		ClientID:           "client-1",
		ProtocolType:       "consumer",
		Protocols: []ProtocolMetadata{
			{Name: "range", Metadata: []byte("test")},
		},
	}
	_, _ = gc.HandleJoinGroup(req)

	// Get the group twice
	group1 := gc.GetGroup("test-group")
	group2 := gc.GetGroup("test-group")

	// They should be different pointers (copies)
	if group1 == group2 {
		t.Error("GetGroup should return a copy, not the same pointer")
	}

	// But have the same data
	if group1.GroupID != group2.GroupID {
		t.Error("Group copies should have the same GroupID")
	}
}

func TestGroupCoordinator_GetCommittedOffset(t *testing.T) {
	offsetStorage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	gc := NewGroupCoordinator(offsetStorage, config)
	defer func() { _ = gc.Stop() }()

	// Get offset before committing - should return 0
	offset, err := gc.GetCommittedOffset("test-group", "test-topic", 0)
	if err != nil {
		t.Fatalf("GetCommittedOffset failed: %v", err)
	}
	if offset != 0 {
		t.Errorf("Offset = %d, want 0 for uncommitted offset", offset)
	}

	// Commit an offset
	offsetMeta := &OffsetAndMetadata{
		Offset:   100,
		Metadata: "test metadata",
	}
	err = offsetStorage.StoreOffset("test-group", "test-topic", 0, offsetMeta)
	if err != nil {
		t.Fatalf("StoreOffset failed: %v", err)
	}

	// Get committed offset
	offset, err = gc.GetCommittedOffset("test-group", "test-topic", 0)
	if err != nil {
		t.Fatalf("GetCommittedOffset failed: %v", err)
	}
	if offset != 100 {
		t.Errorf("Offset = %d, want 100", offset)
	}
}

func TestGroupCoordinator_GetCommittedOffset_MultiplePartitions(t *testing.T) {
	offsetStorage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	gc := NewGroupCoordinator(offsetStorage, config)
	defer func() { _ = gc.Stop() }()

	// Commit offsets for multiple partitions
	partitionOffsets := map[int32]int64{
		0: 100,
		1: 200,
		2: 300,
	}

	for partition, offset := range partitionOffsets {
		offsetMeta := &OffsetAndMetadata{
			Offset:   offset,
			Metadata: "test",
		}
		err := offsetStorage.StoreOffset("test-group", "test-topic", partition, offsetMeta)
		if err != nil {
			t.Fatalf("StoreOffset failed for partition %d: %v", partition, err)
		}
	}

	// Verify all committed offsets
	for partition, expectedOffset := range partitionOffsets {
		offset, err := gc.GetCommittedOffset("test-group", "test-topic", partition)
		if err != nil {
			t.Errorf("GetCommittedOffset failed for partition %d: %v", partition, err)
		}
		if offset != expectedOffset {
			t.Errorf("Partition %d: offset = %d, want %d", partition, offset, expectedOffset)
		}
	}
}

func TestGroupCoordinator_ListGroups_ConcurrentSafety(t *testing.T) {
	offsetStorage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	gc := NewGroupCoordinator(offsetStorage, config)
	defer func() { _ = gc.Stop() }()

	// Create a group
	req := &JoinGroupRequest{
		GroupID:            "concurrent-group",
		SessionTimeoutMs:   30000,
		RebalanceTimeoutMs: 60000,
		ClientID:           "client-1",
		ProtocolType:       "consumer",
		Protocols: []ProtocolMetadata{
			{Name: "range", Metadata: []byte("test")},
		},
	}
	_, _ = gc.HandleJoinGroup(req)

	// Concurrent reads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			groups := gc.ListGroups()
			if len(groups) == 0 {
				t.Error("Expected at least 1 group")
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestGroupCoordinator_DeleteGroup_NotFound(t *testing.T) {
	offsetStorage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	gc := NewGroupCoordinator(offsetStorage, config)
	defer func() { _ = gc.Stop() }()

	err := gc.DeleteGroup("does-not-exist")
	if !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("DeleteGroup(unknown) = %v, want ErrGroupNotFound", err)
	}
}

// TestGroupCoordinator_DeleteGroup_RefusesActiveMembers covers Kafka's
// NON_EMPTY_GROUP behavior: deleting a group with an active member would
// discard offsets that member still depends on, so it must be refused
// rather than performed.
func TestGroupCoordinator_DeleteGroup_RefusesActiveMembers(t *testing.T) {
	offsetStorage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	gc := NewGroupCoordinator(offsetStorage, config)
	defer func() { _ = gc.Stop() }()

	req := &JoinGroupRequest{
		GroupID:            "active-group",
		SessionTimeoutMs:   30000,
		RebalanceTimeoutMs: 60000,
		ClientID:           "client-1",
		ProtocolType:       "consumer",
		Protocols: []ProtocolMetadata{
			{Name: "range", Metadata: []byte("test")},
		},
	}
	_, _ = gc.HandleJoinGroup(req)

	err := gc.DeleteGroup("active-group")
	var notEmpty *GroupNotEmptyError
	if !errors.As(err, &notEmpty) {
		t.Fatalf("DeleteGroup(group with a member) = %v, want *GroupNotEmptyError", err)
	}
	if notEmpty.MemberCount != 1 {
		t.Errorf("MemberCount = %d, want 1", notEmpty.MemberCount)
	}

	// The group must still exist - refused, not partially deleted.
	if gc.GetGroup("active-group") == nil {
		t.Error("group was removed despite having an active member")
	}
}

// TestGroupCoordinator_DeleteGroup_Success covers the happy path: a group
// with no members left is deleted, and - asserted directly against the
// offset store, not assumed from DeleteGroup returning nil - its committed
// offsets are actually gone afterward.
func TestGroupCoordinator_DeleteGroup_Success(t *testing.T) {
	offsetStorage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	gc := NewGroupCoordinator(offsetStorage, config)
	defer func() { _ = gc.Stop() }()

	req := &JoinGroupRequest{
		GroupID:            "empty-group",
		SessionTimeoutMs:   30000,
		RebalanceTimeoutMs: 60000,
		ClientID:           "client-1",
		ProtocolType:       "consumer",
		Protocols: []ProtocolMetadata{
			{Name: "range", Metadata: []byte("test")},
		},
	}
	joinResp, err := gc.HandleJoinGroup(req)
	if err != nil {
		t.Fatalf("HandleJoinGroup: %v", err)
	}

	// Commit an offset for the group before it goes empty, so deletion has
	// something real to clean up.
	if err := offsetStorage.StoreOffset("empty-group", "test-topic", 0, &OffsetAndMetadata{Offset: 42}); err != nil {
		t.Fatalf("StoreOffset: %v", err)
	}

	// The member leaves, so the group stays registered (Empty state) but has
	// zero members - the case DeleteGroup must accept.
	if _, err := gc.HandleLeaveGroup(&LeaveGroupRequest{GroupID: "empty-group", MemberID: joinResp.MemberID}); err != nil {
		t.Fatalf("HandleLeaveGroup: %v", err)
	}

	if err := gc.DeleteGroup("empty-group"); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}

	if gc.GetGroup("empty-group") != nil {
		t.Error("group still present after DeleteGroup")
	}

	offset, err := offsetStorage.FetchOffset("empty-group", "test-topic", 0)
	if err != nil {
		t.Fatalf("FetchOffset after delete: %v", err)
	}
	if offset != nil {
		t.Errorf("offset still present after DeleteGroup: %+v", offset)
	}
}
