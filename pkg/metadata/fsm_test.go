package metadata

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFSM(t *testing.T) {
	fsm := NewFSM()
	require.NotNil(t, fsm)

	state := fsm.GetState()
	assert.NotNil(t, state)
	assert.Equal(t, uint64(0), state.Version)
	assert.Empty(t, state.Brokers)
	assert.Empty(t, state.Topics)
	assert.Empty(t, state.Partitions)
}

func TestFSM_RegisterBroker(t *testing.T) {
	fsm := NewFSM()

	broker := BrokerInfo{
		ID:   1,
		Addr: "localhost:9092",
		Rack: "rack1",
	}

	op := RegisterBrokerOp{Broker: broker}
	opData, err := json.Marshal(op)
	require.NoError(t, err)

	operation := Operation{
		Type:      OpRegisterBroker,
		Data:      opData,
		Timestamp: time.Now(),
	}

	encoded, err := EncodeOperation(operation)
	require.NoError(t, err)

	err = fsm.Apply(encoded)
	assert.NoError(t, err)

	// Verify broker was registered
	registered, exists := fsm.GetBroker(1)
	assert.True(t, exists)
	assert.Equal(t, uint64(1), registered.ID)
	assert.Equal(t, "localhost:9092", registered.Addr)
	assert.Equal(t, "rack1", registered.Rack)
	assert.Equal(t, BrokerStatusAlive, registered.Status)

	// Verify state version incremented
	state := fsm.GetState()
	assert.Equal(t, uint64(1), state.Version)
}

func TestFSM_RegisterBrokerDuplicate(t *testing.T) {
	fsm := NewFSM()

	broker := BrokerInfo{
		ID:   1,
		Addr: "localhost:9092",
	}

	// Register first time
	op := RegisterBrokerOp{Broker: broker}
	opData, err := json.Marshal(op)
	require.NoError(t, err)

	operation := Operation{
		Type:      OpRegisterBroker,
		Data:      opData,
		Timestamp: time.Now(),
	}

	encoded, err := EncodeOperation(operation)
	require.NoError(t, err)

	err = fsm.Apply(encoded)
	assert.NoError(t, err)

	// Try to register again (should fail)
	err = fsm.Apply(encoded)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestFSM_UnregisterBroker(t *testing.T) {
	fsm := NewFSM()

	// Register broker first
	broker := BrokerInfo{
		ID:   1,
		Addr: "localhost:9092",
	}
	registerOp := RegisterBrokerOp{Broker: broker}
	registerData, _ := json.Marshal(registerOp)
	registerOperation := Operation{
		Type:      OpRegisterBroker,
		Data:      registerData,
		Timestamp: time.Now(),
	}
	registerEncoded, _ := EncodeOperation(registerOperation)
	_ = fsm.Apply(registerEncoded)

	// Unregister broker
	unregisterOp := UnregisterBrokerOp{BrokerID: 1}
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
	assert.NoError(t, err)

	// Verify broker was unregistered
	_, exists := fsm.GetBroker(1)
	assert.False(t, exists)
}

func TestFSM_CreateTopic(t *testing.T) {
	fsm := NewFSM()

	config := DefaultTopicConfig()
	op := CreateTopicOp{
		Name:              "test-topic",
		NumPartitions:     3,
		ReplicationFactor: 2,
		Config:            config,
	}

	opData, err := json.Marshal(op)
	require.NoError(t, err)

	operation := Operation{
		Type:      OpCreateTopic,
		Data:      opData,
		Timestamp: time.Now(),
	}

	encoded, err := EncodeOperation(operation)
	require.NoError(t, err)

	err = fsm.Apply(encoded)
	assert.NoError(t, err)

	// Verify topic was created
	topic, exists := fsm.GetTopic("test-topic")
	assert.True(t, exists)
	assert.Equal(t, "test-topic", topic.Name)
	assert.Equal(t, 3, topic.NumPartitions)
	assert.Equal(t, 2, topic.ReplicationFactor)
	assert.Equal(t, config.RetentionMs, topic.Config.RetentionMs)
}

func TestFSM_DeleteTopic(t *testing.T) {
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

	// Create some partitions
	for i := 0; i < 3; i++ {
		partition := PartitionInfo{
			Topic:     "test-topic",
			Partition: i,
			Leader:    1,
			Replicas:  []uint64{1, 2},
			ISR:       []uint64{1, 2},
		}
		partOp := CreatePartitionOp{Partition: partition}
		partData, _ := json.Marshal(partOp)
		partOperation := Operation{
			Type:      OpCreatePartition,
			Data:      partData,
			Timestamp: time.Now(),
		}
		partEncoded, _ := EncodeOperation(partOperation)
		_ = fsm.Apply(partEncoded)
	}

	// Delete topic
	deleteOp := DeleteTopicOp{Name: "test-topic"}
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
	assert.NoError(t, err)

	// Verify topic was deleted
	_, exists := fsm.GetTopic("test-topic")
	assert.False(t, exists)

	// Verify partitions were deleted
	partitions := fsm.ListPartitions("test-topic")
	assert.Empty(t, partitions)
}

func TestFSM_CreatePartition(t *testing.T) {
	fsm := NewFSM()

	partition := PartitionInfo{
		Topic:     "test-topic",
		Partition: 0,
		Leader:    1,
		Replicas:  []uint64{1, 2, 3},
		ISR:       []uint64{1, 2, 3},
	}

	op := CreatePartitionOp{Partition: partition}
	opData, err := json.Marshal(op)
	require.NoError(t, err)

	operation := Operation{
		Type:      OpCreatePartition,
		Data:      opData,
		Timestamp: time.Now(),
	}

	encoded, err := EncodeOperation(operation)
	require.NoError(t, err)

	err = fsm.Apply(encoded)
	assert.NoError(t, err)

	// Verify partition was created
	created, exists := fsm.GetPartition("test-topic", 0)
	assert.True(t, exists)
	assert.Equal(t, "test-topic", created.Topic)
	assert.Equal(t, 0, created.Partition)
	assert.Equal(t, uint64(1), created.Leader)
	assert.Equal(t, []uint64{1, 2, 3}, created.Replicas)
	assert.Equal(t, []uint64{1, 2, 3}, created.ISR)
	assert.Equal(t, uint64(1), created.LeaderEpoch)
}

func TestFSM_UpdateLeader(t *testing.T) {
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

	// Update leader
	updateOp := UpdateLeaderOp{
		Topic:       "test-topic",
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
	assert.NoError(t, err)

	// Verify leader was updated
	updated, exists := fsm.GetPartition("test-topic", 0)
	assert.True(t, exists)
	assert.Equal(t, uint64(2), updated.Leader)
	assert.Equal(t, uint64(2), updated.LeaderEpoch)
}

func TestFSM_UpdateISR(t *testing.T) {
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

	// Update ISR (shrink to 2 replicas)
	updateOp := UpdateISROp{
		Topic:     "test-topic",
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
	assert.NoError(t, err)

	// Verify ISR was updated
	updated, exists := fsm.GetPartition("test-topic", 0)
	assert.True(t, exists)
	assert.Equal(t, []uint64{1, 2}, updated.ISR)
}

func TestFSM_Snapshot(t *testing.T) {
	fsm := NewFSM()

	// Register a broker
	broker := BrokerInfo{
		ID:   1,
		Addr: "localhost:9092",
	}
	registerOp := RegisterBrokerOp{Broker: broker}
	registerData, _ := json.Marshal(registerOp)
	registerOperation := Operation{
		Type:      OpRegisterBroker,
		Data:      registerData,
		Timestamp: time.Now(),
	}
	registerEncoded, _ := EncodeOperation(registerOperation)
	_ = fsm.Apply(registerEncoded)

	// Create a topic
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

	// Take snapshot
	snapshot, err := fsm.Snapshot()
	require.NoError(t, err)
	assert.NotEmpty(t, snapshot)

	// Create new FSM and restore from snapshot
	fsm2 := NewFSM()
	err = fsm2.Restore(snapshot)
	require.NoError(t, err)

	// Verify state was restored
	broker2, exists := fsm2.GetBroker(1)
	assert.True(t, exists)
	assert.Equal(t, uint64(1), broker2.ID)

	topic2, exists := fsm2.GetTopic("test-topic")
	assert.True(t, exists)
	assert.Equal(t, "test-topic", topic2.Name)
	assert.Equal(t, 3, topic2.NumPartitions)
}

func TestFSM_ListOperations(t *testing.T) {
	fsm := NewFSM()

	// Register multiple brokers
	for i := uint64(1); i <= 3; i++ {
		broker := BrokerInfo{
			ID:   i,
			Addr: "localhost:909" + string(rune('0'+i)),
		}
		registerOp := RegisterBrokerOp{Broker: broker}
		registerData, _ := json.Marshal(registerOp)
		registerOperation := Operation{
			Type:      OpRegisterBroker,
			Data:      registerData,
			Timestamp: time.Now(),
		}
		registerEncoded, _ := EncodeOperation(registerOperation)
		_ = fsm.Apply(registerEncoded)
	}

	// List brokers
	brokers := fsm.ListBrokers()
	assert.Equal(t, 3, len(brokers))

	// Create multiple topics
	for i := 0; i < 3; i++ {
		config := DefaultTopicConfig()
		createOp := CreateTopicOp{
			Name:              "topic-" + string(rune('0'+i)),
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
	}

	// List topics
	topics := fsm.ListTopics()
	assert.Equal(t, 3, len(topics))
}

func TestPartitionID(t *testing.T) {
	tests := []struct {
		topic     string
		partition int
		want      string
	}{
		{"test-topic", 0, "test-topic:0"},
		{"test-topic", 1, "test-topic:1"},
		{"events", 42, "events:42"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := PartitionID(tt.topic, tt.partition)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBrokerInfo_IsAlive(t *testing.T) {
	broker := &BrokerInfo{
		ID:            1,
		Status:        BrokerStatusAlive,
		LastHeartbeat: time.Now(),
	}

	// Should be alive with recent heartbeat
	assert.True(t, broker.IsAlive(10*time.Second))

	// Should be dead if heartbeat is too old
	broker.LastHeartbeat = time.Now().Add(-20 * time.Second)
	assert.False(t, broker.IsAlive(10*time.Second))

	// Should be dead if status is not alive
	broker.LastHeartbeat = time.Now()
	broker.Status = BrokerStatusDead
	assert.False(t, broker.IsAlive(10*time.Second))
}

func TestPartitionInfo_IsLeader(t *testing.T) {
	partition := &PartitionInfo{
		Leader: 2,
	}

	assert.False(t, partition.IsLeader(1))
	assert.True(t, partition.IsLeader(2))
	assert.False(t, partition.IsLeader(3))
}

func TestPartitionInfo_IsInISR(t *testing.T) {
	partition := &PartitionInfo{
		ISR: []uint64{1, 2, 3},
	}

	assert.True(t, partition.IsInISR(1))
	assert.True(t, partition.IsInISR(2))
	assert.True(t, partition.IsInISR(3))
	assert.False(t, partition.IsInISR(4))
}
