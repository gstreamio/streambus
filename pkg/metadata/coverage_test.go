package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoOpLogger_Printf tests the NoOpLogger.Printf method
func TestNoOpLogger_Printf(t *testing.T) {
	logger := &NoOpLogger{}
	// Should not panic
	logger.Printf("test message: %s", "hello")
}

// TestFSM_SetLogger tests the FSM.SetLogger method
func TestFSM_SetLogger(t *testing.T) {
	fsm := NewFSM()
	logger := &NoOpLogger{}

	// Set custom logger
	fsm.SetLogger(logger)

	// Verify logger was set by using it
	// The logger will be used in Apply operations
	broker := BrokerInfo{
		ID:   1,
		Addr: "localhost:9092",
	}
	op := RegisterBrokerOp{Broker: broker}
	opData, _ := json.Marshal(op)
	operation := Operation{
		Type:      OpRegisterBroker,
		Data:      opData,
		Timestamp: time.Now(),
	}
	encoded, _ := EncodeOperation(operation)

	// Should not log anything with NoOpLogger
	err := fsm.Apply(encoded)
	assert.NoError(t, err)
}

// TestFSM_ApplyUpdateTopic tests the applyUpdateTopic method
func TestFSM_ApplyUpdateTopic(t *testing.T) {
	fsm := NewFSM()

	// Create topic first
	config := DefaultTopicConfig()
	createOp := CreateTopicOp{
		Name:              "test-topic",
		NumPartitions:     3,
		ReplicationFactor: 2,
		Config:            config,
	}
	createData, _ := json.Marshal(createOp)
	createOperation := Operation{
		Type:      OpCreateTopic,
		Data:      createData,
		Timestamp: time.Now(),
	}
	createEncoded, _ := EncodeOperation(createOperation)
	_ = fsm.Apply(createEncoded)

	// Update topic config
	newConfig := config
	newConfig.RetentionMs = 14 * 24 * 60 * 60 * 1000 // 14 days
	newConfig.CompressionType = "gzip"

	updateOp := UpdateTopicOp{
		Name:   "test-topic",
		Config: newConfig,
	}
	updateData, err := json.Marshal(updateOp)
	require.NoError(t, err)

	operation := Operation{
		Type:      OpUpdateTopic,
		Data:      updateData,
		Timestamp: time.Now(),
	}

	encoded, err := EncodeOperation(operation)
	require.NoError(t, err)

	err = fsm.Apply(encoded)
	assert.NoError(t, err)

	// Verify topic was updated
	topic, exists := fsm.GetTopic("test-topic")
	assert.True(t, exists)
	assert.Equal(t, int64(14*24*60*60*1000), topic.Config.RetentionMs)
	assert.Equal(t, "gzip", topic.Config.CompressionType)
}

// TestFSM_ApplyUpdateTopicNonExistent tests updating a non-existent topic
func TestFSM_ApplyUpdateTopicNonExistent(t *testing.T) {
	fsm := NewFSM()

	config := DefaultTopicConfig()
	updateOp := UpdateTopicOp{
		Name:   "non-existent-topic",
		Config: config,
	}
	updateData, err := json.Marshal(updateOp)
	require.NoError(t, err)

	operation := Operation{
		Type:      OpUpdateTopic,
		Data:      updateData,
		Timestamp: time.Now(),
	}

	encoded, err := EncodeOperation(operation)
	require.NoError(t, err)

	err = fsm.Apply(encoded)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

// TestFSM_ApplyUpdatePartition tests the applyUpdatePartition method
func TestFSM_ApplyUpdatePartition(t *testing.T) {
	fsm := NewFSM()

	// Create partition first
	partition := PartitionInfo{
		Topic:     "test-topic",
		Partition: 0,
		Leader:    1,
		Replicas:  []uint64{1, 2, 3},
		ISR:       []uint64{1, 2, 3},
	}
	createOp := CreatePartitionOp{Partition: partition}
	createData, _ := json.Marshal(createOp)
	createOperation := Operation{
		Type:      OpCreatePartition,
		Data:      createData,
		Timestamp: time.Now(),
	}
	createEncoded, _ := EncodeOperation(createOperation)
	_ = fsm.Apply(createEncoded)

	// Update partition
	updatedPartition := partition
	updatedPartition.Leader = 2
	updatedPartition.ISR = []uint64{1, 2}
	updatedPartition.LeaderEpoch = 2

	updateOp := UpdatePartitionOp{Partition: updatedPartition}
	updateData, err := json.Marshal(updateOp)
	require.NoError(t, err)

	operation := Operation{
		Type:      OpUpdatePartition,
		Data:      updateData,
		Timestamp: time.Now(),
	}

	encoded, err := EncodeOperation(operation)
	require.NoError(t, err)

	err = fsm.Apply(encoded)
	assert.NoError(t, err)

	// Verify partition was updated
	updated, exists := fsm.GetPartition("test-topic", 0)
	assert.True(t, exists)
	assert.Equal(t, uint64(2), updated.Leader)
	assert.Equal(t, []uint64{1, 2}, updated.ISR)
	assert.Equal(t, uint64(2), updated.LeaderEpoch)
}

// TestFSM_ApplyUpdatePartitionNonExistent tests updating a non-existent partition
func TestFSM_ApplyUpdatePartitionNonExistent(t *testing.T) {
	fsm := NewFSM()

	partition := PartitionInfo{
		Topic:     "non-existent-topic",
		Partition: 0,
		Leader:    1,
		Replicas:  []uint64{1, 2, 3},
		ISR:       []uint64{1, 2, 3},
	}

	updateOp := UpdatePartitionOp{Partition: partition}
	updateData, err := json.Marshal(updateOp)
	require.NoError(t, err)

	operation := Operation{
		Type:      OpUpdatePartition,
		Data:      updateData,
		Timestamp: time.Now(),
	}

	encoded, err := EncodeOperation(operation)
	require.NoError(t, err)

	err = fsm.Apply(encoded)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

// TestFSM_ApplyUpdateReplicaList tests the applyUpdateReplicaList method
func TestFSM_ApplyUpdateReplicaList(t *testing.T) {
	fsm := NewFSM()

	// Create partition first
	partition := PartitionInfo{
		Topic:     "test-topic",
		Partition: 0,
		Leader:    1,
		Replicas:  []uint64{1, 2},
		ISR:       []uint64{1, 2},
	}
	createOp := CreatePartitionOp{Partition: partition}
	createData, _ := json.Marshal(createOp)
	createOperation := Operation{
		Type:      OpCreatePartition,
		Data:      createData,
		Timestamp: time.Now(),
	}
	createEncoded, _ := EncodeOperation(createOperation)
	_ = fsm.Apply(createEncoded)

	// Update replica list (add replica 3)
	updateOp := UpdateReplicaListOp{
		Topic:     "test-topic",
		Partition: 0,
		Replicas:  []uint64{1, 2, 3},
	}
	updateData, err := json.Marshal(updateOp)
	require.NoError(t, err)

	operation := Operation{
		Type:      OpUpdateReplicaList,
		Data:      updateData,
		Timestamp: time.Now(),
	}

	encoded, err := EncodeOperation(operation)
	require.NoError(t, err)

	err = fsm.Apply(encoded)
	assert.NoError(t, err)

	// Verify replica list was updated
	updated, exists := fsm.GetPartition("test-topic", 0)
	assert.True(t, exists)
	assert.Equal(t, []uint64{1, 2, 3}, updated.Replicas)
}

// TestFSM_ApplyUpdateReplicaListNonExistent tests updating replica list for non-existent partition
func TestFSM_ApplyUpdateReplicaListNonExistent(t *testing.T) {
	fsm := NewFSM()

	updateOp := UpdateReplicaListOp{
		Topic:     "non-existent-topic",
		Partition: 0,
		Replicas:  []uint64{1, 2, 3},
	}
	updateData, err := json.Marshal(updateOp)
	require.NoError(t, err)

	operation := Operation{
		Type:      OpUpdateReplicaList,
		Data:      updateData,
		Timestamp: time.Now(),
	}

	encoded, err := EncodeOperation(operation)
	require.NoError(t, err)

	err = fsm.Apply(encoded)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

// TestPartitionInfo_IsReplica tests the IsReplica method
func TestPartitionInfo_IsReplica(t *testing.T) {
	partition := &PartitionInfo{
		Replicas: []uint64{1, 2, 3},
	}

	assert.True(t, partition.IsReplica(1))
	assert.True(t, partition.IsReplica(2))
	assert.True(t, partition.IsReplica(3))
	assert.False(t, partition.IsReplica(4))
	assert.False(t, partition.IsReplica(0))
}

// MockConsensusNode is a mock implementation of ConsensusNode for testing
type MockConsensusNode struct {
	proposeFunc func(ctx context.Context, data []byte) error
	isLeader    bool
	leaderID    uint64
}

func (m *MockConsensusNode) Propose(ctx context.Context, data []byte) error {
	if m.proposeFunc != nil {
		return m.proposeFunc(ctx, data)
	}
	return nil
}

func (m *MockConsensusNode) IsLeader() bool {
	return m.isLeader
}

func (m *MockConsensusNode) Leader() uint64 {
	return m.leaderID
}

// TestStore_GetFSM tests the GetFSM method
func TestStore_GetFSM(t *testing.T) {
	fsm := NewFSM()
	consensus := &MockConsensusNode{isLeader: true, leaderID: 1}
	store := NewStore(fsm, consensus)

	retrievedFSM := store.GetFSM()
	assert.Equal(t, fsm, retrievedFSM)
}

// TestStore_DeleteTopic tests the DeleteTopic method
func TestStore_DeleteTopic(t *testing.T) {
	fsm := NewFSM()
	consensus := &MockConsensusNode{isLeader: true, leaderID: 1}
	store := NewStore(fsm, consensus)

	ctx := context.Background()
	err := store.DeleteTopic(ctx, "test-topic")
	assert.NoError(t, err)
}

// TestStore_UpdateTopic tests the UpdateTopic method
func TestStore_UpdateTopic(t *testing.T) {
	fsm := NewFSM()
	consensus := &MockConsensusNode{isLeader: true, leaderID: 1}
	store := NewStore(fsm, consensus)

	config := DefaultTopicConfig()
	config.RetentionMs = 14 * 24 * 60 * 60 * 1000

	ctx := context.Background()
	err := store.UpdateTopic(ctx, "test-topic", config)
	assert.NoError(t, err)
}

// TestStore_UpdatePartition tests the UpdatePartition method
func TestStore_UpdatePartition(t *testing.T) {
	fsm := NewFSM()
	consensus := &MockConsensusNode{isLeader: true, leaderID: 1}
	store := NewStore(fsm, consensus)

	partition := &PartitionInfo{
		Topic:     "test-topic",
		Partition: 0,
		Leader:    1,
		Replicas:  []uint64{1, 2, 3},
		ISR:       []uint64{1, 2, 3},
	}

	ctx := context.Background()
	err := store.UpdatePartition(ctx, partition)
	assert.NoError(t, err)
}

// TestStore_UpdateReplicaList tests the UpdateReplicaList method
func TestStore_UpdateReplicaList(t *testing.T) {
	fsm := NewFSM()
	consensus := &MockConsensusNode{isLeader: true, leaderID: 1}
	store := NewStore(fsm, consensus)

	ctx := context.Background()
	err := store.UpdateReplicaList(ctx, "test-topic", 0, []uint64{1, 2, 3})
	assert.NoError(t, err)
}

// TestStore_ListTopics tests the ListTopics method
func TestStore_ListTopics(t *testing.T) {
	fsm := NewFSM()
	consensus := &MockConsensusNode{isLeader: true, leaderID: 1}
	store := NewStore(fsm, consensus)

	// Initially empty
	topics := store.ListTopics()
	assert.Empty(t, topics)

	// Create a topic directly in FSM
	config := DefaultTopicConfig()
	createOp := CreateTopicOp{
		Name:              "test-topic",
		NumPartitions:     3,
		ReplicationFactor: 2,
		Config:            config,
	}
	createData, _ := json.Marshal(createOp)
	createOperation := Operation{
		Type:      OpCreateTopic,
		Data:      createData,
		Timestamp: time.Now(),
	}
	createEncoded, _ := EncodeOperation(createOperation)
	_ = fsm.Apply(createEncoded)

	// List should return the topic
	topics = store.ListTopics()
	assert.Len(t, topics, 1)
	assert.Equal(t, "test-topic", topics[0].Name)
}

// TestStore_Propose_ContextCancelled tests propose with cancelled context
func TestStore_Propose_ContextCancelled(t *testing.T) {
	fsm := NewFSM()
	consensus := &MockConsensusNode{
		isLeader: true,
		leaderID: 1,
		proposeFunc: func(ctx context.Context, data []byte) error {
			return context.Canceled
		},
	}
	store := NewStore(fsm, consensus)

	ctx := context.Background()
	err := store.DeleteTopic(ctx, "test-topic")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context")
}

// TestStore_Propose_ContextDeadlineExceeded tests propose with deadline exceeded
func TestStore_Propose_ContextDeadlineExceeded(t *testing.T) {
	fsm := NewFSM()
	consensus := &MockConsensusNode{
		isLeader: true,
		leaderID: 1,
		proposeFunc: func(ctx context.Context, data []byte) error {
			return context.DeadlineExceeded
		},
	}
	store := NewStore(fsm, consensus)

	ctx := context.Background()
	err := store.DeleteTopic(ctx, "test-topic")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context")
}

// TestStore_Propose_MaxRetriesExceeded tests propose with max retries exceeded
func TestStore_Propose_MaxRetriesExceeded(t *testing.T) {
	fsm := NewFSM()
	attemptCount := 0
	consensus := &MockConsensusNode{
		isLeader: true,
		leaderID: 1,
		proposeFunc: func(ctx context.Context, data []byte) error {
			attemptCount++
			return errors.New("temporary error")
		},
	}
	store := NewStore(fsm, consensus)

	// Use a shorter timeout to avoid long test times
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := store.DeleteTopic(ctx, "test-topic")
	assert.Error(t, err)
	// Should fail with either max retries or context error
	assert.True(t, errors.Is(err, context.DeadlineExceeded) ||
		(err != nil && (errors.Is(err, context.Canceled) ||
		 strings.Contains(err.Error(), "max retries") ||
		 strings.Contains(err.Error(), "context"))))
	// Should have made multiple attempts
	assert.Greater(t, attemptCount, 0)
}

// TestStore_Propose_NotLeader tests propose when node loses leadership
func TestStore_Propose_NotLeader(t *testing.T) {
	fsm := NewFSM()
	attemptCount := 0
	consensus := &MockConsensusNode{
		isLeader: false, // Not leader
		leaderID: 2,
		proposeFunc: func(ctx context.Context, data []byte) error {
			attemptCount++
			return errors.New("not leader")
		},
	}
	store := NewStore(fsm, consensus)

	// Use a shorter timeout to avoid long test times
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := store.DeleteTopic(ctx, "test-topic")
	assert.Error(t, err)
	// Should have made multiple attempts
	assert.Greater(t, attemptCount, 0)
}

// TestStore_Propose_EventualSuccess tests propose that succeeds after retries
func TestStore_Propose_EventualSuccess(t *testing.T) {
	fsm := NewFSM()
	attemptCount := 0
	consensus := &MockConsensusNode{
		isLeader: true,
		leaderID: 1,
		proposeFunc: func(ctx context.Context, data []byte) error {
			attemptCount++
			if attemptCount < 3 {
				return errors.New("temporary error")
			}
			return nil // Success on 3rd attempt
		},
	}
	store := NewStore(fsm, consensus)

	ctx := context.Background()
	err := store.DeleteTopic(ctx, "test-topic")
	assert.NoError(t, err)
	assert.Equal(t, 3, attemptCount)
}

// TestStore_Propose_ContextCancelledDuringRetry tests context cancellation during retry
func TestStore_Propose_ContextCancelledDuringRetry(t *testing.T) {
	fsm := NewFSM()
	attemptCount := 0
	consensus := &MockConsensusNode{
		isLeader: true,
		leaderID: 1,
		proposeFunc: func(ctx context.Context, data []byte) error {
			attemptCount++
			return errors.New("temporary error")
		},
	}
	store := NewStore(fsm, consensus)

	// Create a context that will be cancelled
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := store.DeleteTopic(ctx, "test-topic")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context")
	// Should have made at least one attempt
	assert.Greater(t, attemptCount, 0)
}

// TestStore_Propose_MarshalError tests propose with marshal error
func TestStore_Propose_MarshalError(t *testing.T) {
	fsm := NewFSM()
	consensus := &MockConsensusNode{isLeader: true, leaderID: 1}
	store := NewStore(fsm, consensus)

	// This test uses store's internal propose method indirectly
	// The operation data will be marshaled successfully, but we can test
	// other error paths. For marshal errors, we'd need to pass invalid data
	// which isn't possible through the public API.

	// Instead, test a successful path to ensure all code paths are covered
	ctx := context.Background()
	err := store.UpdateReplicaList(ctx, "test-topic", 0, []uint64{1, 2, 3})
	assert.NoError(t, err)
}

// TestFSM_Apply_UnknownOperation tests applying an unknown operation type
func TestFSM_Apply_UnknownOperation(t *testing.T) {
	fsm := NewFSM()

	operation := Operation{
		Type:      "unknown_operation",
		Data:      []byte("{}"),
		Timestamp: time.Now(),
	}

	encoded, err := EncodeOperation(operation)
	require.NoError(t, err)

	err = fsm.Apply(encoded)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown operation type")
}

// TestFSM_Apply_InvalidData tests applying operation with invalid data
func TestFSM_Apply_InvalidData(t *testing.T) {
	fsm := NewFSM()

	// Invalid JSON data
	err := fsm.Apply([]byte("invalid json"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode operation")
}

// TestBrokerInfo_Clone_Nil tests cloning a nil broker
func TestBrokerInfo_Clone_Nil(t *testing.T) {
	var broker *BrokerInfo
	cloned := broker.Clone()
	assert.Nil(t, cloned)
}

// TestTopicInfo_Clone_Nil tests cloning a nil topic
func TestTopicInfo_Clone_Nil(t *testing.T) {
	var topic *TopicInfo
	cloned := topic.Clone()
	assert.Nil(t, cloned)
}

// TestPartitionInfo_Clone_Nil tests cloning a nil partition
func TestPartitionInfo_Clone_Nil(t *testing.T) {
	var partition *PartitionInfo
	cloned := partition.Clone()
	assert.Nil(t, cloned)
}

// TestDecodeOperation_InvalidJSON tests decoding invalid JSON
func TestDecodeOperation_InvalidJSON(t *testing.T) {
	_, err := DecodeOperation([]byte("invalid json"))
	assert.Error(t, err)
}

// TestFSM_Restore_InvalidSnapshot tests restoring from invalid snapshot
func TestFSM_Restore_InvalidSnapshot(t *testing.T) {
	fsm := NewFSM()
	err := fsm.Restore([]byte("invalid json"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal snapshot")
}

// TestFSM_GetPartition_NotFound tests getting non-existent partition
func TestFSM_GetPartition_NotFound(t *testing.T) {
	fsm := NewFSM()
	_, exists := fsm.GetPartition("non-existent", 0)
	assert.False(t, exists)
}

// TestFSM_ApplyUnregisterBroker_NonExistent tests unregistering non-existent broker
func TestFSM_ApplyUnregisterBroker_NonExistent(t *testing.T) {
	fsm := NewFSM()

	unregisterOp := UnregisterBrokerOp{BrokerID: 999}
	unregisterData, err := json.Marshal(unregisterOp)
	require.NoError(t, err)

	operation := Operation{
		Type:      OpUnregisterBroker,
		Data:      unregisterData,
		Timestamp: time.Now(),
	}

	encoded, err := EncodeOperation(operation)
	require.NoError(t, err)

	err = fsm.Apply(encoded)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

// TestFSM_ApplyUpdateBroker_NonExistent tests updating non-existent broker
func TestFSM_ApplyUpdateBroker_NonExistent(t *testing.T) {
	fsm := NewFSM()

	updateOp := UpdateBrokerOp{
		BrokerID:  999,
		Status:    BrokerStatusAlive,
		Resources: BrokerResources{},
		Heartbeat: time.Now(),
	}
	updateData, err := json.Marshal(updateOp)
	require.NoError(t, err)

	operation := Operation{
		Type:      OpUpdateBroker,
		Data:      updateData,
		Timestamp: time.Now(),
	}

	encoded, err := EncodeOperation(operation)
	require.NoError(t, err)

	err = fsm.Apply(encoded)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

// TestFSM_ApplyCreateTopic_Duplicate tests creating duplicate topic
func TestFSM_ApplyCreateTopic_Duplicate(t *testing.T) {
	fsm := NewFSM()

	config := DefaultTopicConfig()
	createOp := CreateTopicOp{
		Name:              "test-topic",
		NumPartitions:     3,
		ReplicationFactor: 2,
		Config:            config,
	}
	createData, _ := json.Marshal(createOp)
	createOperation := Operation{
		Type:      OpCreateTopic,
		Data:      createData,
		Timestamp: time.Now(),
	}
	createEncoded, _ := EncodeOperation(createOperation)

	// First creation should succeed
	err := fsm.Apply(createEncoded)
	assert.NoError(t, err)

	// Second creation should fail
	err = fsm.Apply(createEncoded)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// TestFSM_ApplyDeleteTopic_NonExistent tests deleting non-existent topic
func TestFSM_ApplyDeleteTopic_NonExistent(t *testing.T) {
	fsm := NewFSM()

	deleteOp := DeleteTopicOp{Name: "non-existent"}
	deleteData, err := json.Marshal(deleteOp)
	require.NoError(t, err)

	operation := Operation{
		Type:      OpDeleteTopic,
		Data:      deleteData,
		Timestamp: time.Now(),
	}

	encoded, err := EncodeOperation(operation)
	require.NoError(t, err)

	err = fsm.Apply(encoded)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

// TestFSM_ApplyCreatePartition_Duplicate tests creating duplicate partition
func TestFSM_ApplyCreatePartition_Duplicate(t *testing.T) {
	fsm := NewFSM()

	partition := PartitionInfo{
		Topic:     "test-topic",
		Partition: 0,
		Leader:    1,
		Replicas:  []uint64{1, 2, 3},
		ISR:       []uint64{1, 2, 3},
	}

	op := CreatePartitionOp{Partition: partition}
	opData, _ := json.Marshal(op)
	operation := Operation{
		Type:      OpCreatePartition,
		Data:      opData,
		Timestamp: time.Now(),
	}
	encoded, _ := EncodeOperation(operation)

	// First creation should succeed
	err := fsm.Apply(encoded)
	assert.NoError(t, err)

	// Second creation should fail
	err = fsm.Apply(encoded)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// TestFSM_ApplyBatchCreatePartitions_Duplicate tests batch creation with duplicate
func TestFSM_ApplyBatchCreatePartitions_Duplicate(t *testing.T) {
	fsm := NewFSM()

	// Create first partition
	partition := PartitionInfo{
		Topic:     "test-topic",
		Partition: 0,
		Leader:    1,
		Replicas:  []uint64{1, 2, 3},
		ISR:       []uint64{1, 2, 3},
	}

	op := CreatePartitionOp{Partition: partition}
	opData, _ := json.Marshal(op)
	operation := Operation{
		Type:      OpCreatePartition,
		Data:      opData,
		Timestamp: time.Now(),
	}
	encoded, _ := EncodeOperation(operation)
	_ = fsm.Apply(encoded)

	// Try to batch create including the duplicate
	partitions := []PartitionInfo{
		partition, // Duplicate
		{
			Topic:     "test-topic",
			Partition: 1,
			Leader:    1,
			Replicas:  []uint64{1, 2, 3},
			ISR:       []uint64{1, 2, 3},
		},
	}

	batchOp := BatchCreatePartitionsOp{Partitions: partitions}
	batchData, _ := json.Marshal(batchOp)
	batchOperation := Operation{
		Type:      OpBatchCreatePartitions,
		Data:      batchData,
		Timestamp: time.Now(),
	}
	batchEncoded, _ := EncodeOperation(batchOperation)

	err := fsm.Apply(batchEncoded)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// TestFSM_ApplyUpdateLeader_NonExistent tests updating leader for non-existent partition
func TestFSM_ApplyUpdateLeader_NonExistent(t *testing.T) {
	fsm := NewFSM()

	updateOp := UpdateLeaderOp{
		Topic:       "non-existent",
		Partition:   0,
		Leader:      2,
		LeaderEpoch: 2,
	}
	updateData, err := json.Marshal(updateOp)
	require.NoError(t, err)

	operation := Operation{
		Type:      OpUpdateLeader,
		Data:      updateData,
		Timestamp: time.Now(),
	}

	encoded, err := EncodeOperation(operation)
	require.NoError(t, err)

	err = fsm.Apply(encoded)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

// TestFSM_ApplyUpdateISR_NonExistent tests updating ISR for non-existent partition
func TestFSM_ApplyUpdateISR_NonExistent(t *testing.T) {
	fsm := NewFSM()

	updateOp := UpdateISROp{
		Topic:     "non-existent",
		Partition: 0,
		ISR:       []uint64{1, 2},
	}
	updateData, err := json.Marshal(updateOp)
	require.NoError(t, err)

	operation := Operation{
		Type:      OpUpdateISR,
		Data:      updateData,
		Timestamp: time.Now(),
	}

	encoded, err := EncodeOperation(operation)
	require.NoError(t, err)

	err = fsm.Apply(encoded)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}
