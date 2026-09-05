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
	return NewCoordinationHandler(base, coordinator, nil), base
}

// stubLocator is a CoordinatorLocator with a canned answer, for tests that
// only exercise FindCoordinator routing rather than real broker selection
// (that logic lives in pkg/broker and is tested there).
type stubLocator struct {
	nodeID  int32
	host    string
	port    int32
	errCode protocol.ErrorCode

	lastKeyType protocol.CoordinatorKeyType
	lastKey     string
}

func (l *stubLocator) FindCoordinator(keyType protocol.CoordinatorKeyType, key string) (int32, string, int32, protocol.ErrorCode) {
	l.lastKeyType = keyType
	l.lastKey = key
	return l.nodeID, l.host, l.port, l.errCode
}

// request wraps a payload in a request of the given type.
func request(reqType protocol.RequestType, payload any) *protocol.Request {
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
	handler := NewCoordinationHandler(base, nil, nil)

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

func TestCoordinationHandler_FindCoordinator(t *testing.T) {
	base := &passthroughHandler{}
	locator := &stubLocator{nodeID: 2, host: "broker-2", port: 9092}
	handler := NewCoordinationHandler(base, nil, locator)

	resp := handler.Handle(request(protocol.RequestTypeFindCoordinator, &protocol.FindCoordinatorRequest{
		Key:     "analytics",
		KeyType: protocol.CoordinatorKeyTypeGroup,
	}))

	if resp.Header.Status != protocol.StatusOK {
		t.Fatalf("status = %v, want OK", resp.Header.Status)
	}
	found, ok := resp.Payload.(*protocol.FindCoordinatorResponse)
	if !ok {
		t.Fatalf("payload = %T, want *protocol.FindCoordinatorResponse", resp.Payload)
	}
	if found.ErrorCode != protocol.ErrNone || found.NodeID != 2 || found.Host != "broker-2" || found.Port != 9092 {
		t.Errorf("response = %+v, want {ErrNone 2 broker-2 9092}", found)
	}
	if locator.lastKeyType != protocol.CoordinatorKeyTypeGroup || locator.lastKey != "analytics" {
		t.Errorf("locator called with (%v, %q), want (Group, analytics)", locator.lastKeyType, locator.lastKey)
	}
	if base.called {
		t.Error("FindCoordinator must not fall through to the base handler")
	}
}

func TestCoordinationHandler_FindCoordinator_LocatorError(t *testing.T) {
	locator := &stubLocator{errCode: protocol.ErrNotCoordinator}
	handler := NewCoordinationHandler(&passthroughHandler{}, nil, locator)

	resp := handler.Handle(request(protocol.RequestTypeFindCoordinator, &protocol.FindCoordinatorRequest{Key: "txn-1"}))

	// The locator's answer travels in the payload's ErrorCode, not as a
	// transport-level failure: FindCoordinator succeeded at asking, it just
	// has no coordinator to report.
	found, ok := resp.Payload.(*protocol.FindCoordinatorResponse)
	if !ok {
		t.Fatalf("payload = %T, want *protocol.FindCoordinatorResponse", resp.Payload)
	}
	if resp.Header.Status != protocol.StatusOK {
		t.Fatalf("status = %v, want OK", resp.Header.Status)
	}
	if found.ErrorCode != protocol.ErrNotCoordinator {
		t.Errorf("ErrorCode = %v, want ErrNotCoordinator", found.ErrorCode)
	}
}

func TestCoordinationHandler_FindCoordinator_NoLocator(t *testing.T) {
	handler := NewCoordinationHandler(&passthroughHandler{}, nil, nil)

	resp := handler.Handle(request(protocol.RequestTypeFindCoordinator, &protocol.FindCoordinatorRequest{Key: "analytics"}))

	found, ok := resp.Payload.(*protocol.FindCoordinatorResponse)
	if !ok {
		t.Fatalf("payload = %T, want *protocol.FindCoordinatorResponse", resp.Payload)
	}
	if found.ErrorCode != protocol.ErrNotCoordinator {
		t.Errorf("ErrorCode = %v, want ErrNotCoordinator", found.ErrorCode)
	}
}

func TestCoordinationHandler_FindCoordinator_WrongPayloadType(t *testing.T) {
	handler := NewCoordinationHandler(&passthroughHandler{}, nil, &stubLocator{})

	resp := handler.Handle(request(protocol.RequestTypeFindCoordinator, &protocol.HeartbeatRequest{}))

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
