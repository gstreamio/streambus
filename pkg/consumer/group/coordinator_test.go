package group

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupCoordinator_JoinGroup(t *testing.T) {
	storage := NewMemoryOffsetStorage()
	config := DefaultCoordinatorConfig()
	coordinator := NewGroupCoordinator(storage, config)
	defer coordinator.Stop()

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
	defer coordinator.Stop()

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
	defer coordinator.Stop()

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
	defer coordinator.Stop()

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
	defer coordinator.Stop()

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
	defer coordinator.Stop()

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
	defer coordinator.Stop()

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
