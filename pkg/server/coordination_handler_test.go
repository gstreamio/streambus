package server

import (
	"testing"

	"github.com/gstreamio/streambus/pkg/consumer/group"
	"github.com/gstreamio/streambus/pkg/protocol"
)

// passthroughHandler records that a request fell through to the base handler.
type passthroughHandler struct {
	called bool
}

func (h *passthroughHandler) Handle(req *protocol.Request) *protocol.Response {
	h.called = true
	return &protocol.Response{
		Header: protocol.ResponseHeader{RequestID: req.Header.RequestID, Status: protocol.StatusOK},
	}
}

// newCoordinationTestHandler builds a handler over a live group coordinator.
func newCoordinationTestHandler(t *testing.T) (*CoordinationHandler, *passthroughHandler) {
	t.Helper()

	coordinator := group.NewGroupCoordinator(group.NewMemoryOffsetStorage(), group.DefaultCoordinatorConfig())
	t.Cleanup(func() { _ = coordinator.Stop() })

	base := &passthroughHandler{}
	return NewCoordinationHandler(base, coordinator), base
}

// request wraps a payload in a request of the given type.
func request(reqType protocol.RequestType, payload interface{}) *protocol.Request {
	return &protocol.Request{
		Header:  protocol.RequestHeader{RequestID: 1, Type: reqType},
		Payload: payload,
	}
}

func TestCoordinationHandler_JoinAndSync(t *testing.T) {
	handler, _ := newCoordinationTestHandler(t)

	joinResp := handler.Handle(request(protocol.RequestTypeJoinGroup, &protocol.JoinGroupRequest{
		GroupID:            "analytics",
		ClientID:           "client-a",
		ProtocolType:       "consumer",
		SessionTimeoutMs:   30000,
		RebalanceTimeoutMs: 60000,
		Protocols: []protocol.GroupProtocol{
			{Name: "range", Metadata: protocol.EncodeSubscription(&protocol.Subscription{Topics: []string{"orders"}})},
		},
	}))

	if joinResp.Header.Status != protocol.StatusOK {
		t.Fatalf("JoinGroup status = %v, want OK", joinResp.Header.Status)
	}
	join, ok := joinResp.Payload.(*protocol.JoinGroupResponse)
	if !ok {
		t.Fatalf("JoinGroup payload = %T, want *protocol.JoinGroupResponse", joinResp.Payload)
	}
	if join.ErrorCode != protocol.ErrNone {
		t.Fatalf("JoinGroup error = %v", join.ErrorCode)
	}
	if join.MemberID == "" {
		t.Fatal("JoinGroup returned no member ID")
	}
	if !join.IsLeader() {
		t.Fatal("the only member should be the leader")
	}
	if len(join.Members) != 1 {
		t.Fatalf("leader received %d members, want 1", len(join.Members))
	}

	assignment := protocol.EncodeMemberAssignment(&protocol.MemberAssignment{
		Partitions: map[string][]int32{"orders": {0, 1}},
	})

	syncResp := handler.Handle(request(protocol.RequestTypeSyncGroup, &protocol.SyncGroupRequest{
		GroupID:      "analytics",
		GenerationID: join.GenerationID,
		MemberID:     join.MemberID,
		Assignments: []protocol.SyncGroupAssignment{
			{MemberID: join.MemberID, Assignment: assignment},
		},
	}))

	sync, ok := syncResp.Payload.(*protocol.SyncGroupResponse)
	if !ok {
		t.Fatalf("SyncGroup payload = %T, want *protocol.SyncGroupResponse", syncResp.Payload)
	}
	if sync.ErrorCode != protocol.ErrNone {
		t.Fatalf("SyncGroup error = %v", sync.ErrorCode)
	}

	decoded, err := protocol.DecodeMemberAssignment(sync.Assignment)
	if err != nil {
		t.Fatalf("decoding assignment: %v", err)
	}
	if len(decoded.Partitions["orders"]) != 2 {
		t.Errorf("assignment = %v, want 2 partitions of orders", decoded.Partitions)
	}
}

func TestCoordinationHandler_FollowerReceivesLeadersAssignment(t *testing.T) {
	handler, _ := newCoordinationTestHandler(t)

	subscription := protocol.EncodeSubscription(&protocol.Subscription{Topics: []string{"orders"}})

	join := func(memberID string) *protocol.JoinGroupResponse {
		resp := handler.Handle(request(protocol.RequestTypeJoinGroup, &protocol.JoinGroupRequest{
			GroupID:            "analytics",
			MemberID:           memberID,
			ClientID:           "client",
			ProtocolType:       "consumer",
			SessionTimeoutMs:   30000,
			RebalanceTimeoutMs: 60000,
			Protocols:          []protocol.GroupProtocol{{Name: "range", Metadata: subscription}},
		}))
		payload, ok := resp.Payload.(*protocol.JoinGroupResponse)
		if !ok {
			t.Fatalf("JoinGroup payload = %T", resp.Payload)
		}
		if payload.ErrorCode != protocol.ErrNone {
			t.Fatalf("JoinGroup error = %v", payload.ErrorCode)
		}
		return payload
	}

	leader := join("")
	follower := join("")

	// The follower's join bumped the generation, so the leader rejoins and
	// publishes an assignment covering both members.
	leaderRejoin := join(leader.MemberID)
	if !leaderRejoin.IsLeader() {
		t.Fatal("the first member should still be the leader")
	}

	leaderAssignment := protocol.EncodeMemberAssignment(&protocol.MemberAssignment{
		Partitions: map[string][]int32{"orders": {0}},
	})
	followerAssignment := protocol.EncodeMemberAssignment(&protocol.MemberAssignment{
		Partitions: map[string][]int32{"orders": {1}},
	})

	handler.Handle(request(protocol.RequestTypeSyncGroup, &protocol.SyncGroupRequest{
		GroupID:      "analytics",
		GenerationID: leaderRejoin.GenerationID,
		MemberID:     leaderRejoin.MemberID,
		Assignments: []protocol.SyncGroupAssignment{
			{MemberID: leader.MemberID, Assignment: leaderAssignment},
			{MemberID: follower.MemberID, Assignment: followerAssignment},
		},
	}))

	// The follower sends no assignments of its own and must still receive the
	// partitions the leader assigned it.
	resp := handler.Handle(request(protocol.RequestTypeSyncGroup, &protocol.SyncGroupRequest{
		GroupID:      "analytics",
		GenerationID: leaderRejoin.GenerationID,
		MemberID:     follower.MemberID,
	}))

	sync, ok := resp.Payload.(*protocol.SyncGroupResponse)
	if !ok {
		t.Fatalf("SyncGroup payload = %T", resp.Payload)
	}
	if sync.ErrorCode != protocol.ErrNone {
		t.Fatalf("follower SyncGroup error = %v", sync.ErrorCode)
	}

	decoded, err := protocol.DecodeMemberAssignment(sync.Assignment)
	if err != nil {
		t.Fatalf("decoding assignment: %v", err)
	}
	if len(decoded.Partitions["orders"]) != 1 || decoded.Partitions["orders"][0] != 1 {
		t.Errorf("follower assignment = %v, want orders partition 1", decoded.Partitions)
	}
}

func TestCoordinationHandler_SyncBeforeLeaderReportsRebalance(t *testing.T) {
	handler, _ := newCoordinationTestHandler(t)

	subscription := protocol.EncodeSubscription(&protocol.Subscription{Topics: []string{"orders"}})

	joinResp := handler.Handle(request(protocol.RequestTypeJoinGroup, &protocol.JoinGroupRequest{
		GroupID:            "analytics",
		ClientID:           "client",
		ProtocolType:       "consumer",
		SessionTimeoutMs:   30000,
		RebalanceTimeoutMs: 60000,
		Protocols:          []protocol.GroupProtocol{{Name: "range", Metadata: subscription}},
	}))
	join := joinResp.Payload.(*protocol.JoinGroupResponse)

	// A member syncing with no assignments before any leader has published
	// one must be told to wait, not handed an empty assignment it would read
	// as "no partitions for you".
	resp := handler.Handle(request(protocol.RequestTypeSyncGroup, &protocol.SyncGroupRequest{
		GroupID:      "analytics",
		GenerationID: join.GenerationID,
		MemberID:     join.MemberID,
	}))

	sync := resp.Payload.(*protocol.SyncGroupResponse)
	if sync.ErrorCode != protocol.ErrRebalanceInProgress {
		t.Errorf("SyncGroup error = %v, want ErrRebalanceInProgress", sync.ErrorCode)
	}
}

func TestCoordinationHandler_OffsetCommitAndFetch(t *testing.T) {
	handler, _ := newCoordinationTestHandler(t)

	commitResp := handler.Handle(request(protocol.RequestTypeOffsetCommit, &protocol.OffsetCommitRequest{
		GroupID:      "analytics",
		GenerationID: -1,
		Topics: []protocol.OffsetCommitTopic{
			{Topic: "orders", Partitions: []protocol.OffsetCommitPartition{
				{Partition: 0, Offset: 100, Metadata: "cp"},
				{Partition: 1, Offset: 250},
			}},
		},
	}))

	commit, ok := commitResp.Payload.(*protocol.OffsetCommitResponse)
	if !ok {
		t.Fatalf("OffsetCommit payload = %T", commitResp.Payload)
	}
	if code := commit.FirstError(); code != protocol.ErrNone {
		t.Fatalf("OffsetCommit error = %v", code)
	}

	fetchResp := handler.Handle(request(protocol.RequestTypeOffsetFetch, &protocol.OffsetFetchRequest{
		GroupID: "analytics",
		Topics:  []protocol.OffsetFetchTopic{{Topic: "orders", Partitions: []int32{0, 1, 2}}},
	}))

	fetch, ok := fetchResp.Payload.(*protocol.OffsetFetchResponse)
	if !ok {
		t.Fatalf("OffsetFetch payload = %T", fetchResp.Payload)
	}
	if len(fetch.Topics) != 1 {
		t.Fatalf("fetched %d topics, want 1", len(fetch.Topics))
	}

	byPartition := make(map[int32]protocol.OffsetFetchPartition)
	for _, p := range fetch.Topics[0].Partitions {
		byPartition[p.Partition] = p
	}

	if byPartition[0].Offset != 100 {
		t.Errorf("partition 0 offset = %d, want 100", byPartition[0].Offset)
	}
	if byPartition[0].Metadata != "cp" {
		t.Errorf("partition 0 metadata = %q, want cp", byPartition[0].Metadata)
	}
	if byPartition[1].Offset != 250 {
		t.Errorf("partition 1 offset = %d, want 250", byPartition[1].Offset)
	}
	// An uncommitted partition must be distinguishable from a committed 0.
	if byPartition[2].Offset != protocol.OffsetNoCommittedValue {
		t.Errorf("partition 2 offset = %d, want OffsetNoCommittedValue", byPartition[2].Offset)
	}
}

func TestCoordinationHandler_HeartbeatAndLeave(t *testing.T) {
	handler, _ := newCoordinationTestHandler(t)

	subscription := protocol.EncodeSubscription(&protocol.Subscription{Topics: []string{"orders"}})

	joinResp := handler.Handle(request(protocol.RequestTypeJoinGroup, &protocol.JoinGroupRequest{
		GroupID:            "analytics",
		ClientID:           "client",
		ProtocolType:       "consumer",
		SessionTimeoutMs:   30000,
		RebalanceTimeoutMs: 60000,
		Protocols:          []protocol.GroupProtocol{{Name: "range", Metadata: subscription}},
	}))
	join := joinResp.Payload.(*protocol.JoinGroupResponse)

	handler.Handle(request(protocol.RequestTypeSyncGroup, &protocol.SyncGroupRequest{
		GroupID:      "analytics",
		GenerationID: join.GenerationID,
		MemberID:     join.MemberID,
		Assignments: []protocol.SyncGroupAssignment{{
			MemberID:   join.MemberID,
			Assignment: protocol.EncodeMemberAssignment(&protocol.MemberAssignment{Partitions: map[string][]int32{"orders": {0}}}),
		}},
	}))

	hbResp := handler.Handle(request(protocol.RequestTypeHeartbeat, &protocol.HeartbeatRequest{
		GroupID:      "analytics",
		GenerationID: join.GenerationID,
		MemberID:     join.MemberID,
	}))
	hb := hbResp.Payload.(*protocol.HeartbeatResponse)
	if hb.ErrorCode != protocol.ErrNone {
		t.Errorf("Heartbeat error = %v", hb.ErrorCode)
	}

	// A stale generation must be rejected rather than silently accepted.
	staleResp := handler.Handle(request(protocol.RequestTypeHeartbeat, &protocol.HeartbeatRequest{
		GroupID:      "analytics",
		GenerationID: join.GenerationID + 99,
		MemberID:     join.MemberID,
	}))
	stale := staleResp.Payload.(*protocol.HeartbeatResponse)
	if stale.ErrorCode != protocol.ErrIllegalGeneration {
		t.Errorf("stale Heartbeat error = %v, want ErrIllegalGeneration", stale.ErrorCode)
	}

	leaveResp := handler.Handle(request(protocol.RequestTypeLeaveGroup, &protocol.LeaveGroupRequest{
		GroupID:  "analytics",
		MemberID: join.MemberID,
	}))
	leave := leaveResp.Payload.(*protocol.LeaveGroupResponse)
	if leave.ErrorCode != protocol.ErrNone {
		t.Errorf("LeaveGroup error = %v", leave.ErrorCode)
	}
}

func TestCoordinationHandler_UnknownGroup(t *testing.T) {
	handler, _ := newCoordinationTestHandler(t)

	resp := handler.Handle(request(protocol.RequestTypeHeartbeat, &protocol.HeartbeatRequest{
		GroupID:      "never-created",
		GenerationID: 1,
		MemberID:     "member-1",
	}))

	hb := resp.Payload.(*protocol.HeartbeatResponse)
	if hb.ErrorCode == protocol.ErrNone {
		t.Error("heartbeat against an unknown group reported success")
	}
}

func TestCoordinationHandler_PassesThroughOtherRequests(t *testing.T) {
	handler, base := newCoordinationTestHandler(t)

	handler.Handle(request(protocol.RequestTypeListTopics, &protocol.ListTopicsRequest{}))

	if !base.called {
		t.Error("a non-group request should reach the base handler")
	}
}

func TestCoordinationHandler_NoCoordinator(t *testing.T) {
	base := &passthroughHandler{}
	handler := NewCoordinationHandler(base, nil)

	resp := handler.Handle(request(protocol.RequestTypeJoinGroup, &protocol.JoinGroupRequest{GroupID: "analytics"}))

	if resp.Header.Status != protocol.StatusError {
		t.Fatalf("status = %v, want Error", resp.Header.Status)
	}
	if resp.Header.ErrorCode != protocol.ErrNotCoordinator {
		t.Errorf("error code = %v, want ErrNotCoordinator", resp.Header.ErrorCode)
	}
	if base.called {
		t.Error("a group request must not fall through to the base handler")
	}
}

func TestCoordinationHandler_WrongPayloadType(t *testing.T) {
	handler, _ := newCoordinationTestHandler(t)

	resp := handler.Handle(request(protocol.RequestTypeJoinGroup, &protocol.HeartbeatRequest{}))

	if resp.Header.Status != protocol.StatusError {
		t.Fatalf("status = %v, want Error", resp.Header.Status)
	}
	if resp.Header.ErrorCode != protocol.ErrInvalidRequest {
		t.Errorf("error code = %v, want ErrInvalidRequest", resp.Header.ErrorCode)
	}
}

func TestGroupErrorToProtocol(t *testing.T) {
	tests := []struct {
		name string
		code int16
		want protocol.ErrorCode
	}{
		{"none", group.ErrorCodeNone, protocol.ErrNone},
		{"illegal generation", group.ErrorCodeIllegalGeneration, protocol.ErrIllegalGeneration},
		{"unknown member", group.ErrorCodeUnknownMemberID, protocol.ErrUnknownMemberID},
		{"rebalance", group.ErrorCodeRebalanceInProgress, protocol.ErrRebalanceInProgress},
		{"group not found", group.ErrorCodeGroupIDNotFound, protocol.ErrUnknownConsumerGroupID},
		// An unmapped non-zero code must never look like success.
		{"unmapped", int16(9999), protocol.ErrInvalidRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := groupErrorToProtocol(tt.code); got != tt.want {
				t.Errorf("groupErrorToProtocol(%d) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}
