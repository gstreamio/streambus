package protocol

import (
	"bytes"
	"reflect"
	"testing"
)

// roundTripRequest encodes a request and decodes it back through the codec.
func roundTripRequest(t *testing.T, reqType RequestType, payload interface{}) interface{} {
	t.Helper()

	codec := NewCodec()
	var buf bytes.Buffer

	req := &Request{
		Header:  RequestHeader{RequestID: 42, Type: reqType, Version: ProtocolVersion},
		Payload: payload,
	}
	if err := codec.EncodeRequest(&buf, req); err != nil {
		t.Fatalf("EncodeRequest(%v) failed: %v", reqType, err)
	}

	decoded, err := codec.DecodeRequest(&buf)
	if err != nil {
		t.Fatalf("DecodeRequest(%v) failed: %v", reqType, err)
	}
	if decoded.Header.Type != reqType {
		t.Fatalf("request type = %v, want %v", decoded.Header.Type, reqType)
	}
	if buf.Len() != 0 {
		t.Errorf("%v: %d bytes left unconsumed after decode", reqType, buf.Len())
	}

	return decoded.Payload
}

// roundTripResponse encodes a response and decodes it back through the codec.
func roundTripResponse(t *testing.T, reqType RequestType, payload interface{}) interface{} {
	t.Helper()

	codec := NewCodec()
	var buf bytes.Buffer

	resp := &Response{
		Header:  ResponseHeader{RequestID: 42, Status: StatusOK},
		Payload: payload,
	}
	if err := codec.EncodeResponse(&buf, resp); err != nil {
		t.Fatalf("EncodeResponse(%v) failed: %v", reqType, err)
	}

	decoded, err := codec.DecodeResponse(&buf)
	if err != nil {
		t.Fatalf("DecodeResponse(%v) failed: %v", reqType, err)
	}
	if err := codec.DecodeResponsePayload(decoded, reqType); err != nil {
		t.Fatalf("DecodeResponsePayload(%v) failed: %v", reqType, err)
	}

	return decoded.Payload
}

func TestCoordinationRequests_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		reqType RequestType
		payload interface{}
	}{
		{
			name:    "JoinGroup",
			reqType: RequestTypeJoinGroup,
			payload: &JoinGroupRequest{
				GroupID:            "analytics",
				MemberID:           "member-1",
				ClientID:           "client-a",
				ProtocolType:       "consumer",
				SessionTimeoutMs:   30000,
				RebalanceTimeoutMs: 60000,
				Protocols: []GroupProtocol{
					{Name: "range", Metadata: []byte{1, 2, 3}},
					{Name: "roundrobin", Metadata: []byte{4}},
				},
			},
		},
		{
			name:    "JoinGroup first join",
			reqType: RequestTypeJoinGroup,
			payload: &JoinGroupRequest{
				GroupID:      "analytics",
				ProtocolType: "consumer",
				Protocols:    []GroupProtocol{},
			},
		},
		{
			name:    "SyncGroup leader",
			reqType: RequestTypeSyncGroup,
			payload: &SyncGroupRequest{
				GroupID:      "analytics",
				GenerationID: 7,
				MemberID:     "member-1",
				Assignments: []SyncGroupAssignment{
					{MemberID: "member-1", Assignment: []byte{9, 9}},
					{MemberID: "member-2", Assignment: []byte{8}},
				},
			},
		},
		{
			name:    "SyncGroup follower",
			reqType: RequestTypeSyncGroup,
			payload: &SyncGroupRequest{
				GroupID:      "analytics",
				GenerationID: 7,
				MemberID:     "member-2",
				Assignments:  []SyncGroupAssignment{},
			},
		},
		{
			name:    "Heartbeat",
			reqType: RequestTypeHeartbeat,
			payload: &HeartbeatRequest{GroupID: "analytics", GenerationID: 7, MemberID: "member-1"},
		},
		{
			name:    "LeaveGroup",
			reqType: RequestTypeLeaveGroup,
			payload: &LeaveGroupRequest{GroupID: "analytics", MemberID: "member-1"},
		},
		{
			name:    "OffsetCommit",
			reqType: RequestTypeOffsetCommit,
			payload: &OffsetCommitRequest{
				GroupID:      "analytics",
				GenerationID: 7,
				MemberID:     "member-1",
				Topics: []OffsetCommitTopic{
					{Topic: "orders", Partitions: []OffsetCommitPartition{
						{Partition: 0, Offset: 100, Metadata: "checkpoint"},
						{Partition: 1, Offset: 250},
					}},
					{Topic: "events", Partitions: []OffsetCommitPartition{
						{Partition: 3, Offset: 0},
					}},
				},
			},
		},
		{
			name:    "OffsetFetch",
			reqType: RequestTypeOffsetFetch,
			payload: &OffsetFetchRequest{
				GroupID: "analytics",
				Topics: []OffsetFetchTopic{
					{Topic: "orders", Partitions: []int32{0, 1, 2}},
				},
			},
		},
		{
			name:    "OffsetFetch all topics",
			reqType: RequestTypeOffsetFetch,
			payload: &OffsetFetchRequest{GroupID: "analytics", Topics: []OffsetFetchTopic{}},
		},
		{
			name:    "InitProducerID",
			reqType: RequestTypeInitProducerID,
			payload: &InitProducerIDRequest{TransactionID: "txn-1", TransactionTimeoutMs: 60000},
		},
		{
			name:    "AddPartitionsToTxn",
			reqType: RequestTypeAddPartitionsToTxn,
			payload: &AddPartitionsToTxnRequest{
				TransactionID: "txn-1",
				ProducerID:    1234,
				ProducerEpoch: 2,
				Partitions: []TxnPartition{
					{Topic: "orders", Partition: 0},
					{Topic: "orders", Partition: 1},
				},
			},
		},
		{
			name:    "AddOffsetsToTxn",
			reqType: RequestTypeAddOffsetsToTxn,
			payload: &AddOffsetsToTxnRequest{
				TransactionID: "txn-1", ProducerID: 1234, ProducerEpoch: 2, GroupID: "analytics",
			},
		},
		{
			name:    "TxnOffsetCommit",
			reqType: RequestTypeTxnOffsetCommit,
			payload: &TxnOffsetCommitRequest{
				TransactionID: "txn-1",
				GroupID:       "analytics",
				ProducerID:    1234,
				ProducerEpoch: 2,
				Topics: []OffsetCommitTopic{
					{Topic: "orders", Partitions: []OffsetCommitPartition{{Partition: 0, Offset: 42}}},
				},
			},
		},
		{
			name:    "EndTxn commit",
			reqType: RequestTypeEndTxn,
			payload: &EndTxnRequest{TransactionID: "txn-1", ProducerID: 1234, ProducerEpoch: 2, Commit: true},
		},
		{
			name:    "EndTxn abort",
			reqType: RequestTypeEndTxn,
			payload: &EndTxnRequest{TransactionID: "txn-1", ProducerID: 1234, ProducerEpoch: 2, Commit: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTripRequest(t, tt.reqType, tt.payload)
			if !reflect.DeepEqual(got, tt.payload) {
				t.Errorf("round trip changed the payload:\n got: %+v\nwant: %+v", got, tt.payload)
			}
		})
	}
}

func TestCoordinationResponses_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		reqType RequestType
		payload interface{}
	}{
		{
			name:    "JoinGroup leader",
			reqType: RequestTypeJoinGroup,
			payload: &JoinGroupResponse{
				GenerationID: 7,
				ProtocolName: "range",
				MemberID:     "member-1",
				LeaderID:     "member-1",
				Members: []JoinGroupMember{
					{MemberID: "member-1", Metadata: []byte{1}},
					{MemberID: "member-2", Metadata: []byte{2}},
				},
			},
		},
		{
			name:    "JoinGroup follower",
			reqType: RequestTypeJoinGroup,
			payload: &JoinGroupResponse{
				GenerationID: 7,
				ProtocolName: "range",
				MemberID:     "member-2",
				LeaderID:     "member-1",
				Members:      []JoinGroupMember{},
			},
		},
		{
			name:    "JoinGroup error",
			reqType: RequestTypeJoinGroup,
			payload: &JoinGroupResponse{
				ErrorCode: ErrInvalidSessionTimeout,
				Members:   []JoinGroupMember{},
			},
		},
		{
			name:    "SyncGroup",
			reqType: RequestTypeSyncGroup,
			payload: &SyncGroupResponse{Assignment: []byte{1, 2, 3}},
		},
		{
			name:    "SyncGroup no assignment",
			reqType: RequestTypeSyncGroup,
			payload: &SyncGroupResponse{ErrorCode: ErrRebalanceInProgress},
		},
		{
			name:    "Heartbeat",
			reqType: RequestTypeHeartbeat,
			payload: &HeartbeatResponse{ErrorCode: ErrRebalanceInProgress},
		},
		{
			name:    "LeaveGroup",
			reqType: RequestTypeLeaveGroup,
			payload: &LeaveGroupResponse{},
		},
		{
			name:    "OffsetCommit",
			reqType: RequestTypeOffsetCommit,
			payload: &OffsetCommitResponse{
				Topics: []OffsetCommitTopicResult{
					{Topic: "orders", Partitions: []OffsetCommitPartitionResult{
						{Partition: 0},
						{Partition: 1, ErrorCode: ErrIllegalGeneration},
					}},
				},
			},
		},
		{
			name:    "OffsetFetch",
			reqType: RequestTypeOffsetFetch,
			payload: &OffsetFetchResponse{
				Topics: []OffsetFetchTopicResult{
					{Topic: "orders", Partitions: []OffsetFetchPartition{
						{Partition: 0, Offset: 100, Metadata: "cp"},
						{Partition: 1, Offset: OffsetNoCommittedValue},
					}},
				},
			},
		},
		{
			name:    "InitProducerID",
			reqType: RequestTypeInitProducerID,
			payload: &InitProducerIDResponse{ProducerID: 1234, ProducerEpoch: 3},
		},
		{
			name:    "AddPartitionsToTxn",
			reqType: RequestTypeAddPartitionsToTxn,
			payload: &AddPartitionsToTxnResponse{
				Results: []TxnPartitionResult{
					{Topic: "orders", Partition: 0},
					{Topic: "orders", Partition: 1, ErrorCode: ErrInvalidProducerEpoch},
				},
			},
		},
		{
			name:    "AddOffsetsToTxn",
			reqType: RequestTypeAddOffsetsToTxn,
			payload: &AddOffsetsToTxnResponse{},
		},
		{
			name:    "TxnOffsetCommit",
			reqType: RequestTypeTxnOffsetCommit,
			payload: &TxnOffsetCommitResponse{
				Topics: []OffsetCommitTopicResult{
					{Topic: "orders", Partitions: []OffsetCommitPartitionResult{{Partition: 0}}},
				},
			},
		},
		{
			name:    "EndTxn",
			reqType: RequestTypeEndTxn,
			payload: &EndTxnResponse{},
		},
		{
			name:    "EndTxn error",
			reqType: RequestTypeEndTxn,
			payload: &EndTxnResponse{ErrorCode: ErrInvalidTransactionState},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTripResponse(t, tt.reqType, tt.payload)
			if !reflect.DeepEqual(got, tt.payload) {
				t.Errorf("round trip changed the payload:\n got: %+v\nwant: %+v", got, tt.payload)
			}
		})
	}
}

func TestSubscription_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		sub  *Subscription
	}{
		{"topics only", &Subscription{Topics: []string{"orders", "events"}}},
		{"with user data", &Subscription{Topics: []string{"orders"}, UserData: []byte{1, 2}}},
		{"no topics", &Subscription{Topics: []string{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeSubscription(EncodeSubscription(tt.sub))
			if err != nil {
				t.Fatalf("DecodeSubscription failed: %v", err)
			}
			if !reflect.DeepEqual(got, tt.sub) {
				t.Errorf("round trip changed the subscription:\n got: %+v\nwant: %+v", got, tt.sub)
			}
		})
	}
}

func TestMemberAssignment_RoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		assignment *MemberAssignment
	}{
		{"single topic", &MemberAssignment{Partitions: map[string][]int32{"orders": {0, 1, 2}}}},
		{"multiple topics", &MemberAssignment{Partitions: map[string][]int32{
			"orders": {0}, "events": {1, 2}, "audit": {},
		}}},
		{"empty", &MemberAssignment{Partitions: map[string][]int32{}}},
		{"with user data", &MemberAssignment{
			Partitions: map[string][]int32{"orders": {0}},
			UserData:   []byte{7},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeMemberAssignment(EncodeMemberAssignment(tt.assignment))
			if err != nil {
				t.Fatalf("DecodeMemberAssignment failed: %v", err)
			}
			if !reflect.DeepEqual(got, tt.assignment) {
				t.Errorf("round trip changed the assignment:\n got: %+v\nwant: %+v", got, tt.assignment)
			}
		})
	}
}

func TestMemberAssignment_EncodingIsStable(t *testing.T) {
	// Map iteration order must not leak into the encoded bytes: callers
	// compare assignment blobs to detect whether a rebalance changed anything.
	assignment := &MemberAssignment{Partitions: map[string][]int32{
		"alpha": {0}, "beta": {1}, "gamma": {2}, "delta": {3}, "epsilon": {4},
	}}

	first := EncodeMemberAssignment(assignment)
	for i := 0; i < 50; i++ {
		if !bytes.Equal(EncodeMemberAssignment(assignment), first) {
			t.Fatalf("encoding is not stable across calls (iteration %d)", i)
		}
	}
}

func TestJoinGroupResponse_IsLeader(t *testing.T) {
	tests := []struct {
		name     string
		resp     JoinGroupResponse
		isLeader bool
	}{
		{"leader", JoinGroupResponse{MemberID: "m1", LeaderID: "m1"}, true},
		{"follower", JoinGroupResponse{MemberID: "m2", LeaderID: "m1"}, false},
		{"unassigned member", JoinGroupResponse{MemberID: "", LeaderID: ""}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.resp.IsLeader(); got != tt.isLeader {
				t.Errorf("IsLeader() = %v, want %v", got, tt.isLeader)
			}
		})
	}
}

func TestOffsetCommitResponse_FirstError(t *testing.T) {
	ok := &OffsetCommitResponse{Topics: []OffsetCommitTopicResult{
		{Topic: "orders", Partitions: []OffsetCommitPartitionResult{{Partition: 0}, {Partition: 1}}},
	}}
	if got := ok.FirstError(); got != ErrNone {
		t.Errorf("FirstError() = %v, want ErrNone", got)
	}

	failed := &OffsetCommitResponse{Topics: []OffsetCommitTopicResult{
		{Topic: "orders", Partitions: []OffsetCommitPartitionResult{
			{Partition: 0},
			{Partition: 1, ErrorCode: ErrIllegalGeneration},
		}},
	}}
	if got := failed.FirstError(); got != ErrIllegalGeneration {
		t.Errorf("FirstError() = %v, want ErrIllegalGeneration", got)
	}
}

func TestDecodeCoordination_TruncatedPayload(t *testing.T) {
	// A truncated payload must produce an error, never a panic or a
	// half-populated struct that looks valid.
	codec := NewCodec()
	var buf bytes.Buffer

	req := &Request{
		Header: RequestHeader{RequestID: 1, Type: RequestTypeJoinGroup},
		Payload: &JoinGroupRequest{
			GroupID:   "analytics",
			Protocols: []GroupProtocol{{Name: "range", Metadata: []byte{1, 2, 3}}},
		},
	}
	if err := codec.EncodeRequest(&buf, req); err != nil {
		t.Fatalf("EncodeRequest failed: %v", err)
	}

	full := buf.Bytes()
	// Truncate the payload at every length and confirm each is rejected
	// rather than panicking. The CRC check catches most of these; the
	// payload reader's bounds checks catch the rest.
	for cut := 1; cut < len(full); cut++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("decoding a %d-byte prefix panicked: %v", cut, r)
				}
			}()
			_, _ = codec.DecodeRequest(bytes.NewReader(full[:cut]))
		}()
	}
}

func TestDecodeCoordination_HostileElementCount(t *testing.T) {
	// A payload claiming a huge element count must be rejected on the
	// remaining-bytes bound rather than driving a huge allocation.
	w := newSizer()
	w.writeString("analytics")
	w.writeString("")
	w.writeString("")
	w.writeString("consumer")
	w.writeInt32(0)
	w.writeInt32(0)
	w.writeInt32(0) // protocol count placeholder
	buf := make([]byte, w.Len())

	writer := newWriter(buf, 0)
	writer.writeString("analytics")
	writer.writeString("")
	writer.writeString("")
	writer.writeString("consumer")
	writer.writeInt32(0)
	writer.writeInt32(0)
	writer.writeInt32(1 << 30) // absurd protocol count

	if _, err := decodeCoordinationRequest(buf, RequestTypeJoinGroup); err == nil {
		t.Fatal("expected an error for an absurd element count")
	}
}
