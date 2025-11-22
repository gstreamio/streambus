package metadata

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// Logger defines the interface for FSM logging
type Logger interface {
	Printf(format string, v ...interface{})
}

// stdLogger implements Logger using the standard log package
type stdLogger struct{}

func (l *stdLogger) Printf(format string, v ...interface{}) {
	log.Printf(format, v...)
}

// NoOpLogger implements Logger but does nothing (for testing)
type NoOpLogger struct{}

func (l *NoOpLogger) Printf(format string, v ...interface{}) {}

// FSM implements the Finite State Machine for cluster metadata.
// It applies operations to the metadata state and supports snapshots.
type FSM struct {
	mu     sync.RWMutex
	state  *ClusterMetadata
	logger Logger
}

// NewFSM creates a new metadata FSM.
func NewFSM() *FSM {
	return &FSM{
		state:  NewClusterMetadata(),
		logger: &stdLogger{},
	}
}

// SetLogger sets the logger for the FSM
func (f *FSM) SetLogger(l Logger) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logger = l
}

// Apply applies a committed log entry to the state machine.
// This method is called by the consensus layer when entries are committed.
func (f *FSM) Apply(data []byte) error {
	// Decode operation
	op, err := DecodeOperation(data)
	if err != nil {
		return fmt.Errorf("failed to decode operation: %w", err)
	}

	f.logger.Printf("[FSM] Applying operation: type=%s, timestamp=%v", op.Type, op.Timestamp)

	// Apply operation based on type
	f.mu.Lock()
	defer f.mu.Unlock()

	var applyErr error
	switch op.Type {
	case OpRegisterBroker:
		applyErr = f.applyRegisterBroker(op.Data)
	case OpUnregisterBroker:
		applyErr = f.applyUnregisterBroker(op.Data)
	case OpUpdateBroker:
		applyErr = f.applyUpdateBroker(op.Data)
	case OpCreateTopic:
		applyErr = f.applyCreateTopic(op.Data)
	case OpDeleteTopic:
		applyErr = f.applyDeleteTopic(op.Data)
	case OpUpdateTopic:
		applyErr = f.applyUpdateTopic(op.Data)
	case OpCreatePartition:
		applyErr = f.applyCreatePartition(op.Data)
	case OpUpdatePartition:
		applyErr = f.applyUpdatePartition(op.Data)
	case OpUpdateLeader:
		applyErr = f.applyUpdateLeader(op.Data)
		f.logger.Printf("[FSM] Applied UpdateLeader operation, error=%v", applyErr)
	case OpUpdateISR:
		applyErr = f.applyUpdateISR(op.Data)
		f.logger.Printf("[FSM] Applied UpdateISR operation, error=%v", applyErr)
	case OpUpdateReplicaList:
		applyErr = f.applyUpdateReplicaList(op.Data)
	case OpBatchCreatePartitions:
		applyErr = f.applyBatchCreatePartitions(op.Data)
	default:
		applyErr = fmt.Errorf("unknown operation type: %s", op.Type)
	}

	if applyErr != nil {
		f.logger.Printf("[FSM] Operation failed: type=%s, error=%v", op.Type, applyErr)
	} else {
		// Note: version is already incremented by individual operation handlers
		f.logger.Printf("[FSM] Operation succeeded: type=%s, new version=%d", op.Type, f.state.Version)
	}

	return applyErr
}

// Snapshot returns the current state as a snapshot.
func (f *FSM) Snapshot() ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return json.Marshal(f.state)
}

// Restore restores the state from a snapshot.
func (f *FSM) Restore(snapshot []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	state := NewClusterMetadata()
	if err := json.Unmarshal(snapshot, state); err != nil {
		return fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	f.state = state
	return nil
}

// GetState returns a clone of the current state.
// This is safe to call concurrently with Apply.
func (f *FSM) GetState() *ClusterMetadata {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.state.Clone()
}

// GetBroker returns a broker by ID.
func (f *FSM) GetBroker(id uint64) (*BrokerInfo, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	broker, exists := f.state.Brokers[id]
	if !exists {
		return nil, false
	}
	return broker.Clone(), true
}

// GetTopic returns a topic by name.
func (f *FSM) GetTopic(name string) (*TopicInfo, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	topic, exists := f.state.Topics[name]
	if !exists {
		return nil, false
	}
	return topic.Clone(), true
}

// GetPartition returns a partition by topic and partition number.
func (f *FSM) GetPartition(topic string, partition int) (*PartitionInfo, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	id := PartitionID(topic, partition)
	part, exists := f.state.Partitions[id]
	if !exists {
		return nil, false
	}
	return part.Clone(), true
}

// ListBrokers returns all brokers.
func (f *FSM) ListBrokers() []*BrokerInfo {
	f.mu.RLock()
	defer f.mu.RUnlock()

	brokers := make([]*BrokerInfo, 0, len(f.state.Brokers))
	for _, broker := range f.state.Brokers {
		brokers = append(brokers, broker.Clone())
	}
	return brokers
}

// ListTopics returns all topics.
func (f *FSM) ListTopics() []*TopicInfo {
	f.mu.RLock()
	defer f.mu.RUnlock()

	topics := make([]*TopicInfo, 0, len(f.state.Topics))
	for _, topic := range f.state.Topics {
		topics = append(topics, topic.Clone())
	}
	return topics
}

// ListPartitions returns all partitions for a topic.
func (f *FSM) ListPartitions(topic string) []*PartitionInfo {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var partitions []*PartitionInfo
	for _, part := range f.state.Partitions {
		if part.Topic == topic {
			partitions = append(partitions, part.Clone())
		}
	}
	return partitions
}

// applyRegisterBroker applies a register broker operation.
func (f *FSM) applyRegisterBroker(data []byte) error {
	var op RegisterBrokerOp
	if err := json.Unmarshal(data, &op); err != nil {
		return fmt.Errorf("failed to unmarshal register broker op: %w", err)
	}

	// Check if broker already exists
	if _, exists := f.state.Brokers[op.Broker.ID]; exists {
		return fmt.Errorf("broker %d already exists", op.Broker.ID)
	}

	// Register broker
	broker := op.Broker.Clone()
	broker.RegisteredAt = time.Now()
	broker.LastHeartbeat = time.Now()
	broker.Status = BrokerStatusAlive

	f.state.Brokers[broker.ID] = broker
	f.state.Version++
	f.state.LastModified = time.Now()

	return nil
}

// applyUnregisterBroker applies an unregister broker operation.
func (f *FSM) applyUnregisterBroker(data []byte) error {
	var op UnregisterBrokerOp
	if err := json.Unmarshal(data, &op); err != nil {
		return fmt.Errorf("failed to unmarshal unregister broker op: %w", err)
	}

	// Check if broker exists
	if _, exists := f.state.Brokers[op.BrokerID]; !exists {
		return fmt.Errorf("broker %d does not exist", op.BrokerID)
	}

	// Remove broker
	delete(f.state.Brokers, op.BrokerID)
	f.state.Version++
	f.state.LastModified = time.Now()

	return nil
}

// applyUpdateBroker applies an update broker operation.
func (f *FSM) applyUpdateBroker(data []byte) error {
	var op UpdateBrokerOp
	if err := json.Unmarshal(data, &op); err != nil {
		return fmt.Errorf("failed to unmarshal update broker op: %w", err)
	}

	// Check if broker exists
	broker, exists := f.state.Brokers[op.BrokerID]
	if !exists {
		return fmt.Errorf("broker %d does not exist", op.BrokerID)
	}

	// Update broker
	broker.Status = op.Status
	broker.Resources = op.Resources
	broker.LastHeartbeat = op.Heartbeat

	f.state.Version++
	f.state.LastModified = time.Now()

	return nil
}

// applyCreateTopic applies a create topic operation.
func (f *FSM) applyCreateTopic(data []byte) error {
	var op CreateTopicOp
	if err := json.Unmarshal(data, &op); err != nil {
		return fmt.Errorf("failed to unmarshal create topic op: %w", err)
	}

	// Check if topic already exists
	if _, exists := f.state.Topics[op.Name]; exists {
		return fmt.Errorf("topic %s already exists", op.Name)
	}

	// Create topic
	now := time.Now()
	topic := &TopicInfo{
		Name:              op.Name,
		NumPartitions:     op.NumPartitions,
		ReplicationFactor: op.ReplicationFactor,
		Config:            op.Config,
		CreatedAt:         now,
		ModifiedAt:        now,
	}

	f.state.Topics[op.Name] = topic
	f.state.Version++
	f.state.LastModified = time.Now()

	return nil
}

// applyDeleteTopic applies a delete topic operation.
func (f *FSM) applyDeleteTopic(data []byte) error {
	var op DeleteTopicOp
	if err := json.Unmarshal(data, &op); err != nil {
		return fmt.Errorf("failed to unmarshal delete topic op: %w", err)
	}

	// Check if topic exists
	if _, exists := f.state.Topics[op.Name]; !exists {
		return fmt.Errorf("topic %s does not exist", op.Name)
	}

	// Delete topic
	delete(f.state.Topics, op.Name)

	// Delete all partitions for this topic
	for id, part := range f.state.Partitions {
		if part.Topic == op.Name {
			delete(f.state.Partitions, id)
		}
	}

	f.state.Version++
	f.state.LastModified = time.Now()

	return nil
}

// applyUpdateTopic applies an update topic operation.
func (f *FSM) applyUpdateTopic(data []byte) error {
	var op UpdateTopicOp
	if err := json.Unmarshal(data, &op); err != nil {
		return fmt.Errorf("failed to unmarshal update topic op: %w", err)
	}

	// Check if topic exists
	topic, exists := f.state.Topics[op.Name]
	if !exists {
		return fmt.Errorf("topic %s does not exist", op.Name)
	}

	// Update topic
	topic.Config = op.Config
	topic.ModifiedAt = time.Now()

	f.state.Version++
	f.state.LastModified = time.Now()

	return nil
}

// applyCreatePartition applies a create partition operation.
func (f *FSM) applyCreatePartition(data []byte) error {
	var op CreatePartitionOp
	if err := json.Unmarshal(data, &op); err != nil {
		return fmt.Errorf("failed to unmarshal create partition op: %w", err)
	}

	id := op.Partition.PartitionID()

	// Check if partition already exists
	if _, exists := f.state.Partitions[id]; exists {
		return fmt.Errorf("partition %s already exists", id)
	}

	// Create partition
	now := time.Now()
	partition := op.Partition.Clone()
	partition.CreatedAt = now
	partition.ModifiedAt = now
	partition.LeaderEpoch = 1

	f.state.Partitions[id] = partition
	f.state.Version++
	f.state.LastModified = time.Now()

	return nil
}

// applyBatchCreatePartitions applies a batch create partitions operation.
// This creates multiple partitions atomically in a single Raft proposal.
func (f *FSM) applyBatchCreatePartitions(data []byte) error {
	var op BatchCreatePartitionsOp
	if err := json.Unmarshal(data, &op); err != nil {
		return fmt.Errorf("failed to unmarshal batch create partitions op: %w", err)
	}

	now := time.Now()

	// Validate all partitions first (fail fast if any are invalid)
	for _, partition := range op.Partitions {
		id := partition.PartitionID()
		if _, exists := f.state.Partitions[id]; exists {
			return fmt.Errorf("partition %s already exists", id)
		}
	}

	// Create all partitions
	for _, partition := range op.Partitions {
		id := partition.PartitionID()
		p := partition.Clone()
		p.CreatedAt = now
		p.ModifiedAt = now
		p.LeaderEpoch = 1
		f.state.Partitions[id] = p
	}

	f.state.Version++
	f.state.LastModified = time.Now()

	return nil
}

// applyUpdatePartition applies an update partition operation.
func (f *FSM) applyUpdatePartition(data []byte) error {
	var op UpdatePartitionOp
	if err := json.Unmarshal(data, &op); err != nil {
		return fmt.Errorf("failed to unmarshal update partition op: %w", err)
	}

	id := op.Partition.PartitionID()

	// Check if partition exists
	if _, exists := f.state.Partitions[id]; !exists {
		return fmt.Errorf("partition %s does not exist", id)
	}

	// Update partition
	partition := op.Partition.Clone()
	partition.ModifiedAt = time.Now()

	f.state.Partitions[id] = partition
	f.state.Version++
	f.state.LastModified = time.Now()

	return nil
}

// applyUpdateLeader applies an update leader operation.
func (f *FSM) applyUpdateLeader(data []byte) error {
	var op UpdateLeaderOp
	if err := json.Unmarshal(data, &op); err != nil {
		return fmt.Errorf("failed to unmarshal update leader op: %w", err)
	}

	id := PartitionID(op.Topic, op.Partition)

	// Check if partition exists
	partition, exists := f.state.Partitions[id]
	if !exists {
		return fmt.Errorf("partition %s does not exist", id)
	}

	// Update leader
	partition.Leader = op.Leader
	partition.LeaderEpoch = op.LeaderEpoch
	partition.ModifiedAt = time.Now()

	f.state.Version++
	f.state.LastModified = time.Now()

	return nil
}

// applyUpdateISR applies an update ISR operation.
func (f *FSM) applyUpdateISR(data []byte) error {
	var op UpdateISROp
	if err := json.Unmarshal(data, &op); err != nil {
		return fmt.Errorf("failed to unmarshal update ISR op: %w", err)
	}

	id := PartitionID(op.Topic, op.Partition)

	// Check if partition exists
	partition, exists := f.state.Partitions[id]
	if !exists {
		return fmt.Errorf("partition %s does not exist", id)
	}

	// Update ISR
	partition.ISR = append([]uint64(nil), op.ISR...)
	partition.ModifiedAt = time.Now()

	f.state.Version++
	f.state.LastModified = time.Now()

	return nil
}

// applyUpdateReplicaList applies an update replica list operation.
func (f *FSM) applyUpdateReplicaList(data []byte) error {
	var op UpdateReplicaListOp
	if err := json.Unmarshal(data, &op); err != nil {
		return fmt.Errorf("failed to unmarshal update replica list op: %w", err)
	}

	id := PartitionID(op.Topic, op.Partition)

	// Check if partition exists
	partition, exists := f.state.Partitions[id]
	if !exists {
		return fmt.Errorf("partition %s does not exist", id)
	}

	// Update replicas
	partition.Replicas = append([]uint64(nil), op.Replicas...)
	partition.ModifiedAt = time.Now()

	f.state.Version++
	f.state.LastModified = time.Now()

	return nil
}
