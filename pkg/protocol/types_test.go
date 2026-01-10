package protocol

import (
	"testing"
)

func TestRequestType_String(t *testing.T) {
	tests := []struct {
		name string
		rt   RequestType
		want string
	}{
		{"Produce", RequestTypeProduce, "Produce"},
		{"Fetch", RequestTypeFetch, "Fetch"},
		{"GetOffset", RequestTypeGetOffset, "GetOffset"},
		{"CreateTopic", RequestTypeCreateTopic, "CreateTopic"},
		{"DeleteTopic", RequestTypeDeleteTopic, "DeleteTopic"},
		{"ListTopics", RequestTypeListTopics, "ListTopics"},
		{"HealthCheck", RequestTypeHealthCheck, "HealthCheck"},
		{"JoinGroup", RequestTypeJoinGroup, "JoinGroup"},
		{"SyncGroup", RequestTypeSyncGroup, "SyncGroup"},
		{"Heartbeat", RequestTypeHeartbeat, "Heartbeat"},
		{"LeaveGroup", RequestTypeLeaveGroup, "LeaveGroup"},
		{"OffsetCommit", RequestTypeOffsetCommit, "OffsetCommit"},
		{"OffsetFetch", RequestTypeOffsetFetch, "OffsetFetch"},
		{"InitProducerID", RequestTypeInitProducerID, "InitProducerID"},
		{"AddPartitionsToTxn", RequestTypeAddPartitionsToTxn, "AddPartitionsToTxn"},
		{"AddOffsetsToTxn", RequestTypeAddOffsetsToTxn, "AddOffsetsToTxn"},
		{"EndTxn", RequestTypeEndTxn, "EndTxn"},
		{"TxnOffsetCommit", RequestTypeTxnOffsetCommit, "TxnOffsetCommit"},
		{"Unknown", RequestType(255), "Unknown(255)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rt.String()
			if got != tt.want {
				t.Errorf("RequestType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatusCode_String(t *testing.T) {
	tests := []struct {
		name string
		s    StatusCode
		want string
	}{
		{"OK", StatusOK, "OK"},
		{"Error", StatusError, "Error"},
		{"PartialSuccess", StatusPartialSuccess, "PartialSuccess"},
		{"Unknown", StatusCode(255), "Unknown(255)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.s.String()
			if got != tt.want {
				t.Errorf("StatusCode.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorCode_String(t *testing.T) {
	tests := []struct {
		name string
		e    ErrorCode
		want string
	}{
		{"None", ErrNone, "None"},
		{"UnknownRequest", ErrUnknownRequest, "UnknownRequest"},
		{"InvalidRequest", ErrInvalidRequest, "InvalidRequest"},
		{"OffsetOutOfRange", ErrOffsetOutOfRange, "OffsetOutOfRange"},
		{"CorruptMessage", ErrCorruptMessage, "CorruptMessage"},
		{"PartitionNotFound", ErrPartitionNotFound, "PartitionNotFound"},
		{"RequestTimeout", ErrRequestTimeout, "RequestTimeout"},
		{"StorageError", ErrStorageError, "StorageError"},
		{"TopicNotFound", ErrTopicNotFound, "TopicNotFound"},
		{"TopicExists", ErrTopicExists, "TopicExists"},
		{"ChecksumMismatch", ErrChecksumMismatch, "ChecksumMismatch"},
		{"InvalidProtocol", ErrInvalidProtocol, "InvalidProtocol"},
		{"MessageTooLarge", ErrMessageTooLarge, "MessageTooLarge"},
		{"UnknownMemberID", ErrUnknownMemberID, "UnknownMemberID"},
		{"InvalidSessionTimeout", ErrInvalidSessionTimeout, "InvalidSessionTimeout"},
		{"RebalanceInProgress", ErrRebalanceInProgress, "RebalanceInProgress"},
		{"InvalidGenerationID", ErrInvalidGenerationID, "InvalidGenerationID"},
		{"UnknownConsumerGroupID", ErrUnknownConsumerGroupID, "UnknownConsumerGroupID"},
		{"NotCoordinator", ErrNotCoordinator, "NotCoordinator"},
		{"InvalidCommitOffsetSize", ErrInvalidCommitOffsetSize, "InvalidCommitOffsetSize"},
		{"GroupAuthorizationFailed", ErrGroupAuthorizationFailed, "GroupAuthorizationFailed"},
		{"IllegalGeneration", ErrIllegalGeneration, "IllegalGeneration"},
		{"InconsistentGroupProtocol", ErrInconsistentGroupProtocol, "InconsistentGroupProtocol"},
		{"InvalidProducerEpoch", ErrInvalidProducerEpoch, "InvalidProducerEpoch"},
		{"InvalidTransactionState", ErrInvalidTransactionState, "InvalidTransactionState"},
		{"InvalidProducerIDMapping", ErrInvalidProducerIDMapping, "InvalidProducerIDMapping"},
		{"TransactionCoordinatorNotAvailable", ErrTransactionCoordinatorNotAvailable, "TransactionCoordinatorNotAvailable"},
		{"TransactionCoordinatorFenced", ErrTransactionCoordinatorFenced, "TransactionCoordinatorFenced"},
		{"ProducerFenced", ErrProducerFenced, "ProducerFenced"},
		{"InvalidTransactionTimeout", ErrInvalidTransactionTimeout, "InvalidTransactionTimeout"},
		{"ConcurrentTransactions", ErrConcurrentTransactions, "ConcurrentTransactions"},
		{"TransactionAborted", ErrTransactionAborted, "TransactionAborted"},
		{"InvalidPartitionList", ErrInvalidPartitionList, "InvalidPartitionList"},
		{"AuthenticationFailed", ErrAuthenticationFailed, "AuthenticationFailed"},
		{"AuthorizationFailed", ErrAuthorizationFailed, "AuthorizationFailed"},
		{"InvalidCredentials", ErrInvalidCredentials, "InvalidCredentials"},
		{"AccountDisabled", ErrAccountDisabled, "AccountDisabled"},
		{"Unknown", ErrorCode(9999), "Unknown(9999)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.e.String()
			if got != tt.want {
				t.Errorf("ErrorCode.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorCode_Error(t *testing.T) {
	tests := []struct {
		name string
		e    ErrorCode
		want string
	}{
		{"None", ErrNone, "None"},
		{"TopicNotFound", ErrTopicNotFound, "TopicNotFound"},
		{"AuthenticationFailed", ErrAuthenticationFailed, "AuthenticationFailed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.e.Error()
			if got != tt.want {
				t.Errorf("ErrorCode.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestOffsetTimestampConstants verifies the Kafka-compatible offset constants
func TestOffsetTimestampConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant int64
		expected int64
	}{
		{"OffsetLatest", OffsetLatest, -1},
		{"OffsetEarliest", OffsetEarliest, -2},
		{"OffsetMaxTimestamp", OffsetMaxTimestamp, -3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

// TestLeaderEpochErrorCodes verifies the leader epoch error codes
func TestLeaderEpochErrorCodes(t *testing.T) {
	tests := []struct {
		name     string
		code     ErrorCode
		expected ErrorCode
		str      string
	}{
		{"ErrFencedLeaderEpoch", ErrFencedLeaderEpoch, ErrorCode(50), "FencedLeaderEpoch"},
		{"ErrUnknownLeaderEpoch", ErrUnknownLeaderEpoch, ErrorCode(51), "UnknownLeaderEpoch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.code, tt.expected)
			}
			if tt.code.String() != tt.str {
				t.Errorf("%s.String() = %s, want %s", tt.name, tt.code.String(), tt.str)
			}
		})
	}
}

// TestGetOffsetRequest_TimestampField verifies GetOffsetRequest has Timestamp field
func TestGetOffsetRequest_TimestampField(t *testing.T) {
	req := GetOffsetRequest{
		Topic:       "test-topic",
		PartitionID: 0,
		Timestamp:   OffsetLatest,
	}

	if req.Topic != "test-topic" {
		t.Errorf("Topic = %s, want test-topic", req.Topic)
	}
	if req.PartitionID != 0 {
		t.Errorf("PartitionID = %d, want 0", req.PartitionID)
	}
	if req.Timestamp != OffsetLatest {
		t.Errorf("Timestamp = %d, want %d (OffsetLatest)", req.Timestamp, OffsetLatest)
	}
}

// TestGetOffsetResponse_NewFields verifies GetOffsetResponse has new fields
func TestGetOffsetResponse_NewFields(t *testing.T) {
	resp := GetOffsetResponse{
		Topic:         "test-topic",
		PartitionID:   0,
		StartOffset:   0,
		EndOffset:     100,
		HighWaterMark: 100,
		Offset:        50,
		Timestamp:     1234567890,
		LeaderEpoch:   5,
	}

	if resp.Offset != 50 {
		t.Errorf("Offset = %d, want 50", resp.Offset)
	}
	if resp.Timestamp != 1234567890 {
		t.Errorf("Timestamp = %d, want 1234567890", resp.Timestamp)
	}
	if resp.LeaderEpoch != 5 {
		t.Errorf("LeaderEpoch = %d, want 5", resp.LeaderEpoch)
	}
}

// TestProduceRequest_LeaderEpochField verifies ProduceRequest has LeaderEpoch field
func TestProduceRequest_LeaderEpochField(t *testing.T) {
	req := ProduceRequest{
		Topic:       "test-topic",
		PartitionID: 0,
		Messages:    []Message{{Key: []byte("k"), Value: []byte("v")}},
		Acks:        AcksAll,
		TimeoutMs:   30000,
		LeaderEpoch: 5,
	}

	if req.LeaderEpoch != 5 {
		t.Errorf("LeaderEpoch = %d, want 5", req.LeaderEpoch)
	}
}

// TestProduceResponse_LeaderEpochField verifies ProduceResponse has LeaderEpoch field
func TestProduceResponse_LeaderEpochField(t *testing.T) {
	resp := ProduceResponse{
		Topic:         "test-topic",
		PartitionID:   0,
		BaseOffset:    100,
		NumMessages:   10,
		HighWaterMark: 110,
		LeaderEpoch:   5,
	}

	if resp.LeaderEpoch != 5 {
		t.Errorf("LeaderEpoch = %d, want 5", resp.LeaderEpoch)
	}
}
