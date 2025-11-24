package group

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupCoordinator_JoinGroup(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	coordinator := NewGroupCoordinator(storage, config)
	defer func() { _ = coordinator.Stop() }()

	// Test first member joining
	req1 := &JoinGroupRequest{
		GroupID:            "test-group",
		SessionTimeoutMs:   30000,
		RebalanceTimeoutMs: 60000,
		ClientID:           "client-1",
		ProtocolType:       "consumer",
		Protocols: []ProtocolMetadata{
			{Name: "range", Metadata: []byte("subscription-data")},
		},
	}

	resp1, err := coordinator.HandleJoinGroup(req1)
	require.NoError(t, err)
	assert.Equal(t, ErrorCodeNone, resp1.ErrorCode)
	assert.NotEmpty(t, resp1.MemberID)
	assert.Equal(t, resp1.MemberID, resp1.LeaderID) // First member is leader
	assert.Equal(t, int32(1), resp1.GenerationID)
	assert.Len(t, resp1.Members, 1) // Leader gets all members

	// Test second member joining
	req2 := &JoinGroupRequest{
		GroupID:            "test-group",
		SessionTimeoutMs:   30000,
		RebalanceTimeoutMs: 60000,
		ClientID:           "client-2",
		ProtocolType:       "consumer",
		Protocols: []ProtocolMetadata{
			{Name: "range", Metadata: []byte("subscription-data")},
		},
	}

	// First, sync the first member to make group stable
	syncReq1 := &SyncGroupRequest{
		GroupID:      "test-group",
		GenerationID: resp1.GenerationID,
		MemberID:     resp1.MemberID,
		Assignments: []MemberAssignmentData{
			{
				MemberID:   resp1.MemberID,
				Assignment: []byte("assignment-data"),
			},
		},
	}
	_, err = coordinator.HandleSyncGroup(syncReq1)
	require.NoError(t, err)

	// Now add second member (should trigger rebalance)
	resp2, err := coordinator.HandleJoinGroup(req2)
	require.NoError(t, err)
	assert.Equal(t, ErrorCodeNone, resp2.ErrorCode)
	assert.NotEmpty(t, resp2.MemberID)
	assert.NotEqual(t, resp2.MemberID, resp2.LeaderID) // Not the leader
	assert.Equal(t, int32(2), resp2.GenerationID)       // Generation incremented
}

func TestGroupCoordinator_SyncGroup(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	coordinator := NewGroupCoordinator(storage, config)
	defer func() { _ = coordinator.Stop() }()

	// Join group
	joinReq := &JoinGroupRequest{
		GroupID:            "test-group",
		SessionTimeoutMs:   30000,
		RebalanceTimeoutMs: 60000,
		ClientID:           "client-1",
		ProtocolType:       "consumer",
		Protocols: []ProtocolMetadata{
			{Name: "range", Metadata: []byte("subscription-data")},
		},
	}

	joinResp, err := coordinator.HandleJoinGroup(joinReq)
	require.NoError(t, err)
	require.Equal(t, ErrorCodeNone, joinResp.ErrorCode)

	// Sync group
	syncReq := &SyncGroupRequest{
		GroupID:      "test-group",
		GenerationID: joinResp.GenerationID,
		MemberID:     joinResp.MemberID,
		Assignments: []MemberAssignmentData{
			{
				MemberID:   joinResp.MemberID,
				Assignment: []byte("assignment-data"),
			},
		},
	}

	syncResp, err := coordinator.HandleSyncGroup(syncReq)
	require.NoError(t, err)
	assert.Equal(t, ErrorCodeNone, syncResp.ErrorCode)
	assert.Equal(t, []byte("assignment-data"), syncResp.Assignment)
}

func TestGroupCoordinator_Heartbeat(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	coordinator := NewGroupCoordinator(storage, config)
	defer func() { _ = coordinator.Stop() }()

	// Join group
	joinReq := &JoinGroupRequest{
		GroupID:            "test-group",
		SessionTimeoutMs:   30000,
		RebalanceTimeoutMs: 60000,
		ClientID:           "client-1",
		ProtocolType:       "consumer",
		Protocols: []ProtocolMetadata{
			{Name: "range", Metadata: []byte("subscription-data")},
		},
	}

	joinResp, err := coordinator.HandleJoinGroup(joinReq)
	require.NoError(t, err)

	// Sync to make group stable
	syncReq := &SyncGroupRequest{
		GroupID:      "test-group",
		GenerationID: joinResp.GenerationID,
		MemberID:     joinResp.MemberID,
		Assignments: []MemberAssignmentData{
			{
				MemberID:   joinResp.MemberID,
				Assignment: []byte("assignment-data"),
			},
		},
	}

	_, err = coordinator.HandleSyncGroup(syncReq)
	require.NoError(t, err)

	// Send heartbeat
	hbReq := &HeartbeatRequest{
		GroupID:      "test-group",
		GenerationID: joinResp.GenerationID,
		MemberID:     joinResp.MemberID,
	}

	hbResp, err := coordinator.HandleHeartbeat(hbReq)
	require.NoError(t, err)
	assert.Equal(t, ErrorCodeNone, hbResp.ErrorCode)
}

func TestGroupCoordinator_LeaveGroup(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	coordinator := NewGroupCoordinator(storage, config)
	defer func() { _ = coordinator.Stop() }()

	// Join group
	joinReq := &JoinGroupRequest{
		GroupID:            "test-group",
		SessionTimeoutMs:   30000,
		RebalanceTimeoutMs: 60000,
		ClientID:           "client-1",
		ProtocolType:       "consumer",
		Protocols: []ProtocolMetadata{
			{Name: "range", Metadata: []byte("subscription-data")},
		},
	}

	joinResp, err := coordinator.HandleJoinGroup(joinReq)
	require.NoError(t, err)

	// Leave group
	leaveReq := &LeaveGroupRequest{
		GroupID:  "test-group",
		MemberID: joinResp.MemberID,
	}

	leaveResp, err := coordinator.HandleLeaveGroup(leaveReq)
	require.NoError(t, err)
	assert.Equal(t, ErrorCodeNone, leaveResp.ErrorCode)

	// Verify group is empty
	coordinator.mu.RLock()
	group := coordinator.groups["test-group"]
	coordinator.mu.RUnlock()
	assert.Equal(t, GroupStateEmpty, group.State)
	assert.Len(t, group.Members, 0)
}

func TestGroupCoordinator_OffsetCommit(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	coordinator := NewGroupCoordinator(storage, config)
	defer func() { _ = coordinator.Stop() }()

	// Commit offset (without group membership, like simple consumer)
	commitReq := &OffsetCommitRequest{
		GroupID:      "test-group",
		GenerationID: -1, // Simple consumer
		MemberID:     "",
		Offsets: map[string]map[int32]OffsetCommitData{
			"test-topic": {
				0: {Offset: 100, Metadata: "test-metadata"},
				1: {Offset: 200, Metadata: "test-metadata"},
			},
		},
	}

	commitResp, err := coordinator.HandleOffsetCommit(commitReq)
	require.NoError(t, err)
	assert.Equal(t, ErrorCodeNone, commitResp.Errors["test-topic"][0])
	assert.Equal(t, ErrorCodeNone, commitResp.Errors["test-topic"][1])

	// Fetch offsets
	fetchReq := &OffsetFetchRequest{
		GroupID: "test-group",
		Topics: map[string][]int32{
			"test-topic": {0, 1},
		},
	}

	fetchResp, err := coordinator.HandleOffsetFetch(fetchReq)
	require.NoError(t, err)
	assert.Equal(t, int64(100), fetchResp.Offsets["test-topic"][0].Offset)
	assert.Equal(t, int64(200), fetchResp.Offsets["test-topic"][1].Offset)
	assert.Equal(t, "test-metadata", fetchResp.Offsets["test-topic"][0].Metadata)
}

func TestGroupCoordinator_ExpiredMembers(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	config.HeartbeatCheckIntervalMs = 100 // Check frequently
	coordinator := NewGroupCoordinator(storage, config)
	defer func() { _ = coordinator.Stop() }()

	// Join group with short timeout
	joinReq := &JoinGroupRequest{
		GroupID:            "test-group",
		SessionTimeoutMs:   500, // 500ms timeout
		RebalanceTimeoutMs: 1000,
		ClientID:           "client-1",
		ProtocolType:       "consumer",
		Protocols: []ProtocolMetadata{
			{Name: "range", Metadata: []byte("subscription-data")},
		},
	}

	joinResp, err := coordinator.HandleJoinGroup(joinReq)
	require.NoError(t, err)

	// Make group stable
	syncReq := &SyncGroupRequest{
		GroupID:      "test-group",
		GenerationID: joinResp.GenerationID,
		MemberID:     joinResp.MemberID,
		Assignments: []MemberAssignmentData{
			{
				MemberID:   joinResp.MemberID,
				Assignment: []byte("assignment-data"),
			},
		},
	}

	_, err = coordinator.HandleSyncGroup(syncReq)
	require.NoError(t, err)

	// Wait for member to expire
	time.Sleep(800 * time.Millisecond)

	// Verify member was removed
	coordinator.mu.RLock()
	group := coordinator.groups["test-group"]
	coordinator.mu.RUnlock()

	// Group might be nil if cleaned up, or empty
	if group != nil {
		assert.Len(t, group.Members, 0)
		assert.Equal(t, GroupStateEmpty, group.State)
	}
}

func TestGroupCoordinator_MultipleGroups(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	coordinator := NewGroupCoordinator(storage, config)
	defer func() { _ = coordinator.Stop() }()

	// Create group 1
	joinReq1 := &JoinGroupRequest{
		GroupID:            "group-1",
		SessionTimeoutMs:   30000,
		RebalanceTimeoutMs: 60000,
		ClientID:           "client-1",
		ProtocolType:       "consumer",
		Protocols: []ProtocolMetadata{
			{Name: "range", Metadata: []byte("subscription-data")},
		},
	}

	resp1, err := coordinator.HandleJoinGroup(joinReq1)
	require.NoError(t, err)
	assert.Equal(t, ErrorCodeNone, resp1.ErrorCode)

	// Create group 2
	joinReq2 := &JoinGroupRequest{
		GroupID:            "group-2",
		SessionTimeoutMs:   30000,
		RebalanceTimeoutMs: 60000,
		ClientID:           "client-2",
		ProtocolType:       "consumer",
		Protocols: []ProtocolMetadata{
			{Name: "roundrobin", Metadata: []byte("subscription-data")},
		},
	}

	resp2, err := coordinator.HandleJoinGroup(joinReq2)
	require.NoError(t, err)
	assert.Equal(t, ErrorCodeNone, resp2.ErrorCode)

	// Verify both groups exist
	coordinator.mu.RLock()
	assert.Len(t, coordinator.groups, 2)
	assert.NotNil(t, coordinator.groups["group-1"])
	assert.NotNil(t, coordinator.groups["group-2"])
	coordinator.mu.RUnlock()
}

func TestOffsetStorage_MemoryStorage(t *testing.T) {
	storage := NewMemoryOffsetStorage()

	// Store offset
	offset1 := &OffsetAndMetadata{
		Offset:   100,
		Metadata: "test-metadata",
	}

	err := storage.StoreOffset("group-1", "topic-1", 0, offset1)
	require.NoError(t, err)

	// Fetch offset
	fetchedOffset, err := storage.FetchOffset("group-1", "topic-1", 0)
	require.NoError(t, err)
	require.NotNil(t, fetchedOffset)
	assert.Equal(t, int64(100), fetchedOffset.Offset)
	assert.Equal(t, "test-metadata", fetchedOffset.Metadata)

	// Fetch non-existent offset
	fetchedOffset, err = storage.FetchOffset("group-1", "topic-1", 1)
	require.NoError(t, err)
	assert.Nil(t, fetchedOffset)

	// Fetch all offsets
	groupOffsets, err := storage.FetchOffsets("group-1")
	require.NoError(t, err)
	assert.Len(t, groupOffsets.Offsets, 1)
	assert.Len(t, groupOffsets.Offsets["topic-1"], 1)

	// Delete offsets
	err = storage.DeleteOffsets("group-1")
	require.NoError(t, err)

	// Verify deleted
	fetchedOffset, err = storage.FetchOffset("group-1", "topic-1", 0)
	require.NoError(t, err)
	assert.Nil(t, fetchedOffset)
}

// TestGroupError_Error tests the Error() method
func TestGroupError_Error(t *testing.T) {
	err := &GroupError{Code: ErrorCodeUnknownMemberID}
	expected := "group error: code=25"
	assert.Equal(t, expected, err.Error())

	err = &GroupError{Code: ErrorCodeIllegalGeneration}
	expected = "group error: code=22"
	assert.Equal(t, expected, err.Error())
}

// mockMetadataStore is a mock implementation of MetadataStore for testing
type mockMetadataStore struct {
	offsets map[string]map[string]map[int32]*OffsetAndMetadata
	mu      sync.RWMutex
}

func newMockMetadataStore() *mockMetadataStore {
	return &mockMetadataStore{
		offsets: make(map[string]map[string]map[int32]*OffsetAndMetadata),
	}
}

func (m *mockMetadataStore) StoreGroupOffset(groupID string, topic string, partition int32, offset *OffsetAndMetadata) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.offsets[groupID]; !exists {
		m.offsets[groupID] = make(map[string]map[int32]*OffsetAndMetadata)
	}
	if _, exists := m.offsets[groupID][topic]; !exists {
		m.offsets[groupID][topic] = make(map[int32]*OffsetAndMetadata)
	}

	m.offsets[groupID][topic][partition] = offset
	return nil
}

func (m *mockMetadataStore) FetchGroupOffset(groupID string, topic string, partition int32) (*OffsetAndMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	groupOffsets, exists := m.offsets[groupID]
	if !exists {
		return nil, nil
	}

	topicOffsets, exists := groupOffsets[topic]
	if !exists {
		return nil, nil
	}

	offset, exists := topicOffsets[partition]
	if !exists {
		return nil, nil
	}

	return offset, nil
}

func (m *mockMetadataStore) FetchGroupOffsets(groupID string) (*GroupOffsets, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	groupOffsets, exists := m.offsets[groupID]
	if !exists {
		return &GroupOffsets{
			GroupID: groupID,
			Offsets: make(map[string]map[int32]*OffsetAndMetadata),
		}, nil
	}

	result := &GroupOffsets{
		GroupID: groupID,
		Offsets: make(map[string]map[int32]*OffsetAndMetadata),
	}

	for topic, partitions := range groupOffsets {
		result.Offsets[topic] = make(map[int32]*OffsetAndMetadata)
		for partition, offset := range partitions {
			offsetCopy := *offset
			result.Offsets[topic][partition] = &offsetCopy
		}
	}

	return result, nil
}

func (m *mockMetadataStore) DeleteGroupOffsets(groupID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.offsets, groupID)
	return nil
}

// TestPersistentOffsetStorage tests the persistent offset storage implementation
func TestPersistentOffsetStorage(t *testing.T) {
	store := newMockMetadataStore()
	storage := NewPersistentOffsetStorage(store)

	// Test StoreOffset
	offset1 := &OffsetAndMetadata{
		Offset:   200,
		Metadata: "persistent-metadata",
	}

	err := storage.StoreOffset("group-1", "topic-1", 0, offset1)
	require.NoError(t, err)

	// Test FetchOffset
	fetchedOffset, err := storage.FetchOffset("group-1", "topic-1", 0)
	require.NoError(t, err)
	require.NotNil(t, fetchedOffset)
	assert.Equal(t, int64(200), fetchedOffset.Offset)
	assert.Equal(t, "persistent-metadata", fetchedOffset.Metadata)

	// Test FetchOffset for non-existent data
	fetchedOffset, err = storage.FetchOffset("group-1", "topic-1", 1)
	require.NoError(t, err)
	assert.Nil(t, fetchedOffset)

	// Store more offsets
	offset2 := &OffsetAndMetadata{
		Offset:   300,
		Metadata: "metadata-2",
	}
	err = storage.StoreOffset("group-1", "topic-1", 1, offset2)
	require.NoError(t, err)

	offset3 := &OffsetAndMetadata{
		Offset:   400,
		Metadata: "metadata-3",
	}
	err = storage.StoreOffset("group-1", "topic-2", 0, offset3)
	require.NoError(t, err)

	// Test FetchOffsets
	groupOffsets, err := storage.FetchOffsets("group-1")
	require.NoError(t, err)
	assert.Len(t, groupOffsets.Offsets, 2) // Two topics
	assert.Len(t, groupOffsets.Offsets["topic-1"], 2) // Two partitions in topic-1
	assert.Len(t, groupOffsets.Offsets["topic-2"], 1) // One partition in topic-2

	// Test DeleteOffsets
	err = storage.DeleteOffsets("group-1")
	require.NoError(t, err)

	// Verify deleted
	fetchedOffset, err = storage.FetchOffset("group-1", "topic-1", 0)
	require.NoError(t, err)
	assert.Nil(t, fetchedOffset)

	// Verify FetchOffsets returns empty result for deleted group
	groupOffsets, err = storage.FetchOffsets("group-1")
	require.NoError(t, err)
	assert.Empty(t, groupOffsets.Offsets)
}

// TestHandleOffsetCommit_ErrorPaths tests error paths in HandleOffsetCommit
func TestHandleOffsetCommit_ErrorPaths(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	coordinator := NewGroupCoordinator(storage, config)
	defer func() { _ = coordinator.Stop() }()

	t.Run("commit with invalid group", func(t *testing.T) {
		// Commit offset with generation ID but non-existent group
		commitReq := &OffsetCommitRequest{
			GroupID:      "non-existent-group",
			GenerationID: 1, // Valid generation ID
			MemberID:     "member-1",
			Offsets: map[string]map[int32]OffsetCommitData{
				"test-topic": {
					0: {Offset: 100, Metadata: "test-metadata"},
				},
			},
		}

		commitResp, err := coordinator.HandleOffsetCommit(commitReq)
		require.NoError(t, err)
		assert.Equal(t, ErrorCodeGroupIDNotFound, commitResp.Errors["test-topic"][0])
	})

	t.Run("commit with invalid member", func(t *testing.T) {
		// Create a group first
		joinReq := &JoinGroupRequest{
			GroupID:            "test-group",
			SessionTimeoutMs:   30000,
			RebalanceTimeoutMs: 60000,
			ClientID:           "client-1",
			ProtocolType:       "consumer",
			Protocols: []ProtocolMetadata{
				{Name: "range", Metadata: []byte("subscription-data")},
			},
		}

		joinResp, err := coordinator.HandleJoinGroup(joinReq)
		require.NoError(t, err)

		// Try to commit with invalid member ID
		commitReq := &OffsetCommitRequest{
			GroupID:      "test-group",
			GenerationID: joinResp.GenerationID,
			MemberID:     "invalid-member-id",
			Offsets: map[string]map[int32]OffsetCommitData{
				"test-topic": {
					0: {Offset: 100, Metadata: "test-metadata"},
				},
			},
		}

		commitResp, err := coordinator.HandleOffsetCommit(commitReq)
		require.NoError(t, err)
		assert.Equal(t, ErrorCodeUnknownMemberID, commitResp.Errors["test-topic"][0])
	})

	t.Run("commit with valid group member", func(t *testing.T) {
		// Join group
		joinReq := &JoinGroupRequest{
			GroupID:            "test-group-2",
			SessionTimeoutMs:   30000,
			RebalanceTimeoutMs: 60000,
			ClientID:           "client-2",
			ProtocolType:       "consumer",
			Protocols: []ProtocolMetadata{
				{Name: "range", Metadata: []byte("subscription-data")},
			},
		}

		joinResp, err := coordinator.HandleJoinGroup(joinReq)
		require.NoError(t, err)

		// Commit with valid member
		commitReq := &OffsetCommitRequest{
			GroupID:      "test-group-2",
			GenerationID: joinResp.GenerationID,
			MemberID:     joinResp.MemberID,
			Offsets: map[string]map[int32]OffsetCommitData{
				"test-topic": {
					0: {Offset: 100, Metadata: "test-metadata"},
					1: {Offset: 200, Metadata: "test-metadata-2"},
				},
				"test-topic-2": {
					0: {Offset: 300, Metadata: "test-metadata-3"},
				},
			},
		}

		commitResp, err := coordinator.HandleOffsetCommit(commitReq)
		require.NoError(t, err)
		assert.Equal(t, ErrorCodeNone, commitResp.Errors["test-topic"][0])
		assert.Equal(t, ErrorCodeNone, commitResp.Errors["test-topic"][1])
		assert.Equal(t, ErrorCodeNone, commitResp.Errors["test-topic-2"][0])
	})
}

// TestHandleOffsetFetch_AllScenarios tests all paths in HandleOffsetFetch
func TestHandleOffsetFetch_AllScenarios(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	coordinator := NewGroupCoordinator(storage, config)
	defer func() { _ = coordinator.Stop() }()

	// Store some offsets first
	commitReq := &OffsetCommitRequest{
		GroupID:      "test-group",
		GenerationID: -1, // Simple consumer
		MemberID:     "",
		Offsets: map[string]map[int32]OffsetCommitData{
			"topic-1": {
				0: {Offset: 100, Metadata: "meta-1"},
				1: {Offset: 200, Metadata: "meta-2"},
			},
			"topic-2": {
				0: {Offset: 300, Metadata: "meta-3"},
			},
		},
	}

	_, err := coordinator.HandleOffsetCommit(commitReq)
	require.NoError(t, err)

	t.Run("fetch all offsets", func(t *testing.T) {
		// Fetch all offsets (empty Topics map)
		fetchReq := &OffsetFetchRequest{
			GroupID: "test-group",
			Topics:  map[string][]int32{}, // Empty means fetch all
		}

		fetchResp, err := coordinator.HandleOffsetFetch(fetchReq)
		require.NoError(t, err)
		assert.Len(t, fetchResp.Offsets, 2) // Two topics
		assert.Equal(t, int64(100), fetchResp.Offsets["topic-1"][0].Offset)
		assert.Equal(t, int64(200), fetchResp.Offsets["topic-1"][1].Offset)
		assert.Equal(t, int64(300), fetchResp.Offsets["topic-2"][0].Offset)
	})

	t.Run("fetch specific topics", func(t *testing.T) {
		// Fetch specific topics
		fetchReq := &OffsetFetchRequest{
			GroupID: "test-group",
			Topics: map[string][]int32{
				"topic-1": {0, 1},
			},
		}

		fetchResp, err := coordinator.HandleOffsetFetch(fetchReq)
		require.NoError(t, err)
		assert.Len(t, fetchResp.Offsets, 1) // Only one topic
		assert.Equal(t, int64(100), fetchResp.Offsets["topic-1"][0].Offset)
		assert.Equal(t, "meta-1", fetchResp.Offsets["topic-1"][0].Metadata)
		assert.Equal(t, ErrorCodeNone, fetchResp.Offsets["topic-1"][0].ErrorCode)
	})

	t.Run("fetch non-existent partition", func(t *testing.T) {
		// Fetch partition with no committed offset
		fetchReq := &OffsetFetchRequest{
			GroupID: "test-group",
			Topics: map[string][]int32{
				"topic-1": {99}, // Non-existent partition
			},
		}

		fetchResp, err := coordinator.HandleOffsetFetch(fetchReq)
		require.NoError(t, err)
		assert.Equal(t, int64(-1), fetchResp.Offsets["topic-1"][99].Offset) // -1 for no offset
		assert.Equal(t, ErrorCodeNone, fetchResp.Offsets["topic-1"][99].ErrorCode)
	})

	t.Run("fetch from non-existent group", func(t *testing.T) {
		// Fetch from group with no offsets
		fetchReq := &OffsetFetchRequest{
			GroupID: "non-existent-group",
			Topics:  map[string][]int32{},
		}

		fetchResp, err := coordinator.HandleOffsetFetch(fetchReq)
		require.NoError(t, err)
		assert.Empty(t, fetchResp.Offsets)
	})
}

// TestHandleHeartbeat_ErrorCases tests error cases in HandleHeartbeat
func TestHandleHeartbeat_ErrorCases(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	coordinator := NewGroupCoordinator(storage, config)
	defer func() { _ = coordinator.Stop() }()

	t.Run("heartbeat for non-existent group", func(t *testing.T) {
		hbReq := &HeartbeatRequest{
			GroupID:      "non-existent-group",
			GenerationID: 1,
			MemberID:     "member-1",
		}

		hbResp, err := coordinator.HandleHeartbeat(hbReq)
		require.NoError(t, err)
		assert.Equal(t, ErrorCodeGroupIDNotFound, hbResp.ErrorCode)
	})

	t.Run("heartbeat with invalid generation", func(t *testing.T) {
		// Join group
		joinReq := &JoinGroupRequest{
			GroupID:            "test-group",
			SessionTimeoutMs:   30000,
			RebalanceTimeoutMs: 60000,
			ClientID:           "client-1",
			ProtocolType:       "consumer",
			Protocols: []ProtocolMetadata{
				{Name: "range", Metadata: []byte("subscription-data")},
			},
		}

		joinResp, err := coordinator.HandleJoinGroup(joinReq)
		require.NoError(t, err)

		// Send heartbeat with wrong generation
		hbReq := &HeartbeatRequest{
			GroupID:      "test-group",
			GenerationID: joinResp.GenerationID + 10,
			MemberID:     joinResp.MemberID,
		}

		hbResp, err := coordinator.HandleHeartbeat(hbReq)
		require.NoError(t, err)
		assert.Equal(t, ErrorCodeIllegalGeneration, hbResp.ErrorCode)
	})

	t.Run("heartbeat with invalid member", func(t *testing.T) {
		// Send heartbeat with unknown member
		hbReq := &HeartbeatRequest{
			GroupID:      "test-group",
			GenerationID: 1,
			MemberID:     "unknown-member",
		}

		hbResp, err := coordinator.HandleHeartbeat(hbReq)
		require.NoError(t, err)
		assert.Equal(t, ErrorCodeUnknownMemberID, hbResp.ErrorCode)
	})

	t.Run("heartbeat during rebalance", func(t *testing.T) {
		// Create a stable group
		joinReq := &JoinGroupRequest{
			GroupID:            "test-group-2",
			SessionTimeoutMs:   30000,
			RebalanceTimeoutMs: 60000,
			ClientID:           "client-2",
			ProtocolType:       "consumer",
			Protocols: []ProtocolMetadata{
				{Name: "range", Metadata: []byte("subscription-data")},
			},
		}

		joinResp, err := coordinator.HandleJoinGroup(joinReq)
		require.NoError(t, err)

		// Make group stable
		syncReq := &SyncGroupRequest{
			GroupID:      "test-group-2",
			GenerationID: joinResp.GenerationID,
			MemberID:     joinResp.MemberID,
			Assignments: []MemberAssignmentData{
				{
					MemberID:   joinResp.MemberID,
					Assignment: []byte("assignment-data"),
				},
			},
		}
		_, err = coordinator.HandleSyncGroup(syncReq)
		require.NoError(t, err)

		// Join another member to trigger rebalance
		joinReq2 := &JoinGroupRequest{
			GroupID:            "test-group-2",
			SessionTimeoutMs:   30000,
			RebalanceTimeoutMs: 60000,
			ClientID:           "client-3",
			ProtocolType:       "consumer",
			Protocols: []ProtocolMetadata{
				{Name: "range", Metadata: []byte("subscription-data")},
			},
		}

		joinResp2, err := coordinator.HandleJoinGroup(joinReq2)
		require.NoError(t, err)

		// Send heartbeat from first member during rebalance
		hbReq := &HeartbeatRequest{
			GroupID:      "test-group-2",
			GenerationID: joinResp.GenerationID, // Old generation
			MemberID:     joinResp.MemberID,
		}

		hbResp, err := coordinator.HandleHeartbeat(hbReq)
		require.NoError(t, err)
		// Should return either rebalance in progress or illegal generation
		assert.True(t, hbResp.ErrorCode == ErrorCodeRebalanceInProgress || hbResp.ErrorCode == ErrorCodeIllegalGeneration)

		// Clean up - verify the second member response
		assert.Equal(t, ErrorCodeNone, joinResp2.ErrorCode)
	})
}

// TestHandleLeaveGroup_Scenarios tests various leave group scenarios
func TestHandleLeaveGroup_Scenarios(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	coordinator := NewGroupCoordinator(storage, config)
	defer func() { _ = coordinator.Stop() }()

	t.Run("leave non-existent group", func(t *testing.T) {
		leaveReq := &LeaveGroupRequest{
			GroupID:  "non-existent-group",
			MemberID: "member-1",
		}

		leaveResp, err := coordinator.HandleLeaveGroup(leaveReq)
		require.NoError(t, err)
		assert.Equal(t, ErrorCodeGroupIDNotFound, leaveResp.ErrorCode)
	})

	t.Run("member leaves from stable group with remaining members", func(t *testing.T) {
		// Create group with two members
		joinReq1 := &JoinGroupRequest{
			GroupID:            "test-group",
			SessionTimeoutMs:   30000,
			RebalanceTimeoutMs: 60000,
			ClientID:           "client-1",
			ProtocolType:       "consumer",
			Protocols: []ProtocolMetadata{
				{Name: "range", Metadata: []byte("subscription-data")},
			},
		}

		joinResp1, err := coordinator.HandleJoinGroup(joinReq1)
		require.NoError(t, err)

		// Make group stable
		syncReq := &SyncGroupRequest{
			GroupID:      "test-group",
			GenerationID: joinResp1.GenerationID,
			MemberID:     joinResp1.MemberID,
			Assignments: []MemberAssignmentData{
				{
					MemberID:   joinResp1.MemberID,
					Assignment: []byte("assignment-data"),
				},
			},
		}
		_, err = coordinator.HandleSyncGroup(syncReq)
		require.NoError(t, err)

		// Add second member (triggers rebalance)
		joinReq2 := &JoinGroupRequest{
			GroupID:            "test-group",
			SessionTimeoutMs:   30000,
			RebalanceTimeoutMs: 60000,
			ClientID:           "client-2",
			ProtocolType:       "consumer",
			Protocols: []ProtocolMetadata{
				{Name: "range", Metadata: []byte("subscription-data")},
			},
		}

		joinResp2, err := coordinator.HandleJoinGroup(joinReq2)
		require.NoError(t, err)

		// Sync both members
		syncReq1 := &SyncGroupRequest{
			GroupID:      "test-group",
			GenerationID: joinResp2.GenerationID,
			MemberID:     joinResp1.MemberID,
			Assignments:  []MemberAssignmentData{},
		}
		_, err = coordinator.HandleSyncGroup(syncReq1)
		require.NoError(t, err)

		syncReq2 := &SyncGroupRequest{
			GroupID:      "test-group",
			GenerationID: joinResp2.GenerationID,
			MemberID:     joinResp2.MemberID,
			Assignments: []MemberAssignmentData{
				{
					MemberID:   joinResp1.MemberID,
					Assignment: []byte("assignment-1"),
				},
				{
					MemberID:   joinResp2.MemberID,
					Assignment: []byte("assignment-2"),
				},
			},
		}
		_, err = coordinator.HandleSyncGroup(syncReq2)
		require.NoError(t, err)

		// Now leave with first member
		leaveReq := &LeaveGroupRequest{
			GroupID:  "test-group",
			MemberID: joinResp1.MemberID,
		}

		leaveResp, err := coordinator.HandleLeaveGroup(leaveReq)
		require.NoError(t, err)
		assert.Equal(t, ErrorCodeNone, leaveResp.ErrorCode)

		// Verify group state - should have triggered rebalance
		coordinator.mu.RLock()
		group := coordinator.groups["test-group"]
		coordinator.mu.RUnlock()

		assert.Len(t, group.Members, 1)
		assert.Equal(t, GroupStatePreparingRebalance, group.State)
	})
}

// TestCheckExpiredMembers_Direct tests member expiration directly
func TestCheckExpiredMembers_Direct(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	coordinator := NewGroupCoordinator(storage, config)
	defer func() { _ = coordinator.Stop() }()

	// Create a group with a member
	joinReq := &JoinGroupRequest{
		GroupID:            "test-group-expire",
		SessionTimeoutMs:   6000, // 6 seconds timeout (minimum)
		RebalanceTimeoutMs: 10000,
		ClientID:           "client-1",
		ProtocolType:       "consumer",
		Protocols: []ProtocolMetadata{
			{Name: "range", Metadata: []byte("subscription-data")},
		},
	}

	joinResp, err := coordinator.HandleJoinGroup(joinReq)
	require.NoError(t, err)

	// Make group stable
	syncReq := &SyncGroupRequest{
		GroupID:      "test-group-expire",
		GenerationID: joinResp.GenerationID,
		MemberID:     joinResp.MemberID,
		Assignments: []MemberAssignmentData{
			{
				MemberID:   joinResp.MemberID,
				Assignment: []byte("assignment-data"),
			},
		},
	}
	_, err = coordinator.HandleSyncGroup(syncReq)
	require.NoError(t, err)

	// Verify member exists and set expired heartbeat
	coordinator.mu.Lock()
	group := coordinator.groups["test-group-expire"]
	require.NotNil(t, group, "Group 'test-group-expire' should exist")
	assert.Len(t, group.Members, 1)
	assert.Equal(t, GroupStateStable, group.State)

	// Manually set last heartbeat to past to simulate expiration
	member := group.Members[joinResp.MemberID]
	member.LastHeartbeat = time.Now().Add(-7 * time.Second) // 7 seconds ago (>6 second timeout)
	coordinator.mu.Unlock()

	// Manually trigger check for expired members
	coordinator.checkExpiredMembers()

	// Verify member was removed
	coordinator.mu.RLock()
	group = coordinator.groups["test-group-expire"]
	coordinator.mu.RUnlock()

	require.NotNil(t, group)
	assert.Len(t, group.Members, 0)
	assert.Equal(t, GroupStateEmpty, group.State)
}

// TestCheckExpiredMembers_MultiMember tests expiration with multiple members
func TestCheckExpiredMembers_MultiMember(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	coordinator := NewGroupCoordinator(storage, config)
	defer func() { _ = coordinator.Stop() }()

	// Create group with two members
	joinReq1 := &JoinGroupRequest{
		GroupID:            "test-group-multi",
		SessionTimeoutMs:   6000, // 6 seconds (minimum)
		RebalanceTimeoutMs: 10000,
		ClientID:           "client-1",
		ProtocolType:       "consumer",
		Protocols: []ProtocolMetadata{
			{Name: "range", Metadata: []byte("subscription-data")},
		},
	}

	joinResp1, err := coordinator.HandleJoinGroup(joinReq1)
	require.NoError(t, err)

	// Add second member
	joinReq2 := &JoinGroupRequest{
		GroupID:            "test-group-multi",
		SessionTimeoutMs:   6000, // 6 seconds (minimum)
		RebalanceTimeoutMs: 10000,
		ClientID:           "client-2",
		ProtocolType:       "consumer",
		Protocols: []ProtocolMetadata{
			{Name: "range", Metadata: []byte("subscription-data")},
		},
	}

	joinResp2, err := coordinator.HandleJoinGroup(joinReq2)
	require.NoError(t, err)

	// Sync to make stable
	syncReq := &SyncGroupRequest{
		GroupID:      "test-group-multi",
		GenerationID: joinResp2.GenerationID,
		MemberID:     joinResp1.MemberID,
		Assignments: []MemberAssignmentData{
			{
				MemberID:   joinResp1.MemberID,
				Assignment: []byte("assignment-1"),
			},
			{
				MemberID:   joinResp2.MemberID,
				Assignment: []byte("assignment-2"),
			},
		},
	}
	_, err = coordinator.HandleSyncGroup(syncReq)
	require.NoError(t, err)

	// Expire only the first member
	coordinator.mu.Lock()
	group := coordinator.groups["test-group-multi"]
	require.NotNil(t, group)
	member1 := group.Members[joinResp1.MemberID]
	member1.LastHeartbeat = time.Now().Add(-7 * time.Second) // Expired (>6 second timeout)
	coordinator.mu.Unlock()

	// Trigger expiration check
	coordinator.checkExpiredMembers()

	// Verify only first member expired
	coordinator.mu.RLock()
	group = coordinator.groups["test-group-multi"]
	coordinator.mu.RUnlock()

	require.NotNil(t, group)
	assert.Len(t, group.Members, 1)
	assert.Contains(t, group.Members, joinResp2.MemberID)
	assert.NotContains(t, group.Members, joinResp1.MemberID)
	assert.Equal(t, GroupStatePreparingRebalance, group.State)
}
