package replication

import (
	"testing"
)

func TestErrorCode_String(t *testing.T) {
	tests := []struct {
		code ErrorCode
		want string
	}{
		{ErrorNone, "None"},
		{ErrorNotLeader, "NotLeader"},
		{ErrorOffsetOutOfRange, "OffsetOutOfRange"},
		{ErrorStaleEpoch, "StaleEpoch"},
		{ErrorPartitionNotFound, "PartitionNotFound"},
		{ErrorReplicaNotInReplicas, "ReplicaNotInReplicas"},
		{ErrorUnknown, "Unknown"},
		{ErrorCode(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.code.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReplicationState(t *testing.T) {
	state := &ReplicationState{
		ReplicaID:     1,
		LogEndOffset:  100,
		HighWaterMark: 90,
		FetchLag:      10,
		InSync:        true,
	}

	if state.ReplicaID != 1 {
		t.Errorf("ReplicaID = %d, want 1", state.ReplicaID)
	}
	if state.LogEndOffset != 100 {
		t.Errorf("LogEndOffset = %d, want 100", state.LogEndOffset)
	}
	if state.HighWaterMark != 90 {
		t.Errorf("HighWaterMark = %d, want 90", state.HighWaterMark)
	}
	if state.FetchLag != 10 {
		t.Errorf("FetchLag = %d, want 10", state.FetchLag)
	}
	if !state.InSync {
		t.Error("InSync should be true")
	}
}

func TestPartitionReplicationState(t *testing.T) {
	state := &PartitionReplicationState{
		Topic:         "test-topic",
		PartitionID:   0,
		Leader:        1,
		LeaderEpoch:   5,
		Replicas:      []ReplicaID{1, 2, 3},
		ISR:           []ReplicaID{1, 2},
		HighWaterMark: 100,
		LogEndOffset:  110,
		ReplicaStates: make(map[ReplicaID]*ReplicationState),
	}

	if state.Topic != "test-topic" {
		t.Errorf("Topic = %s, want test-topic", state.Topic)
	}
	if state.Leader != 1 {
		t.Errorf("Leader = %d, want 1", state.Leader)
	}
	if len(state.Replicas) != 3 {
		t.Errorf("Replicas count = %d, want 3", len(state.Replicas))
	}
	if len(state.ISR) != 2 {
		t.Errorf("ISR count = %d, want 2", len(state.ISR))
	}
}

func TestFetchRequest(t *testing.T) {
	req := &FetchRequest{
		ReplicaID:   2,
		Topic:       "test-topic",
		PartitionID: 0,
		FetchOffset: 100,
		MaxBytes:    1024 * 1024,
		LeaderEpoch: 5,
		MinBytes:    1,
		MaxWaitMs:   500,
	}

	if req.ReplicaID != 2 {
		t.Errorf("ReplicaID = %d, want 2", req.ReplicaID)
	}
	if req.FetchOffset != 100 {
		t.Errorf("FetchOffset = %d, want 100", req.FetchOffset)
	}
	if req.MaxBytes != 1024*1024 {
		t.Errorf("MaxBytes = %d, want 1MB", req.MaxBytes)
	}
}

func TestFetchResponse(t *testing.T) {
	resp := &FetchResponse{
		Topic:         "test-topic",
		PartitionID:   0,
		LeaderEpoch:   5,
		HighWaterMark: 100,
		LogEndOffset:  110,
		Messages: []*Message{
			{Offset: 100, Key: []byte("key1"), Value: []byte("value1")},
			{Offset: 101, Key: []byte("key2"), Value: []byte("value2")},
		},
		ErrorCode: ErrorNone,
		Error:     nil,
	}

	if resp.LeaderEpoch != 5 {
		t.Errorf("LeaderEpoch = %d, want 5", resp.LeaderEpoch)
	}
	if resp.HighWaterMark != 100 {
		t.Errorf("HighWaterMark = %d, want 100", resp.HighWaterMark)
	}
	if len(resp.Messages) != 2 {
		t.Errorf("Messages count = %d, want 2", len(resp.Messages))
	}
	if resp.ErrorCode != ErrorNone {
		t.Errorf("ErrorCode = %v, want ErrorNone", resp.ErrorCode)
	}
}

func TestMessage(t *testing.T) {
	msg := &Message{
		Offset:    100,
		Key:       []byte("test-key"),
		Value:     []byte("test-value"),
		Timestamp: 1234567890,
		Headers: map[string][]byte{
			"header1": []byte("value1"),
		},
	}

	if msg.Offset != 100 {
		t.Errorf("Offset = %d, want 100", msg.Offset)
	}
	if string(msg.Key) != "test-key" {
		t.Errorf("Key = %s, want test-key", msg.Key)
	}
	if string(msg.Value) != "test-value" {
		t.Errorf("Value = %s, want test-value", msg.Value)
	}
	if len(msg.Headers) != 1 {
		t.Errorf("Headers count = %d, want 1", len(msg.Headers))
	}
}

func TestISRChangeNotification(t *testing.T) {
	notif := &ISRChangeNotification{
		Topic:       "test-topic",
		PartitionID: 0,
		OldISR:      []ReplicaID{1, 2, 3},
		NewISR:      []ReplicaID{1, 2},
		Reason:      "Replica 3 fell behind",
	}

	if notif.Topic != "test-topic" {
		t.Errorf("Topic = %s, want test-topic", notif.Topic)
	}
	if len(notif.OldISR) != 3 {
		t.Errorf("OldISR count = %d, want 3", len(notif.OldISR))
	}
	if len(notif.NewISR) != 2 {
		t.Errorf("NewISR count = %d, want 2", len(notif.NewISR))
	}
	if notif.Reason == "" {
		t.Error("Reason should not be empty")
	}
}

func TestReplicationMetrics(t *testing.T) {
	metrics := &ReplicationMetrics{
		FetchRequestRate:       100.5,
		FetchBytesRate:         1024000.0,
		ReplicationLagMessages: 50,
		ReplicationLagMs:       100,
		LastFetchLatencyMs:     10,
		ISRShrinkRate:          0.5,
		ISRExpandRate:          0.3,
	}

	if metrics.FetchRequestRate != 100.5 {
		t.Errorf("FetchRequestRate = %f, want 100.5", metrics.FetchRequestRate)
	}
	if metrics.ReplicationLagMessages != 50 {
		t.Errorf("ReplicationLagMessages = %d, want 50", metrics.ReplicationLagMessages)
	}
}
