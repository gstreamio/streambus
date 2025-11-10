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
