package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ConsensusNode is the interface for proposing operations via consensus.
// This matches the consensus.Node interface.
type ConsensusNode interface {
	Propose(ctx context.Context, data []byte) error
	IsLeader() bool
	Leader() uint64
}

// Store provides a high-level API for managing cluster metadata.
// It wraps the FSM and integrates with the consensus layer.
type Store struct {
	fsm       *FSM
	consensus ConsensusNode
}

// NewStore creates a new metadata store with the given FSM and consensus node.
func NewStore(fsm *FSM, consensus ConsensusNode) *Store {
	return &Store{
		fsm:       fsm,
		consensus: consensus,
	}
}

// GetFSM returns the underlying FSM (for consensus integration).
func (s *Store) GetFSM() *FSM {
	return s.fsm
}

// propose proposes an operation to the consensus layer with retry logic.
func (s *Store) propose(ctx context.Context, opType OperationType, opData interface{}) error {
	// Encode operation data
	data, err := json.Marshal(opData)
	if err != nil {
		return fmt.Errorf("failed to marshal operation data: %w", err)
	}

	// Create operation
	op := Operation{
		Type:      opType,
		Data:      data,
		Timestamp: time.Now(),
	}

	// Encode full operation
	opBytes, err := EncodeOperation(op)
	if err != nil {
		return fmt.Errorf("failed to encode operation: %w", err)
	}

	// Retry with exponential backoff for transient failures
	maxRetries := 10
	backoff := 50 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Check if we still have time
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		}

		// Try to propose
		err := s.consensus.Propose(ctx, opBytes)
		if err == nil {
			return nil // Success!
		}

		// Check if error is retriable
		if err == context.DeadlineExceeded || err == context.Canceled {
			return fmt.Errorf("proposal context error: %w", err)
		}

		// If this was the last attempt, return the error
		if attempt == maxRetries {
			return fmt.Errorf("failed to propose after %d attempts: %w", maxRetries+1, err)
		}

		// Wait before retrying with exponential backoff
		select {
		case <-time.After(backoff):
			if backoff < 2*time.Second {
				backoff *= 2
			}
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		}

		// If we're not the leader anymore, wait for new leader
		if !s.IsLeader() {
			time.Sleep(200 * time.Millisecond)
		}
	}

	return fmt.Errorf("failed to propose operation: max retries exceeded")
}

// RegisterBroker registers a new broker in the cluster.
func (s *Store) RegisterBroker(ctx context.Context, broker *BrokerInfo) error {
	op := RegisterBrokerOp{
		Broker: *broker,
	}
	return s.propose(ctx, OpRegisterBroker, op)
}

// UnregisterBroker unregisters a broker from the cluster.
func (s *Store) UnregisterBroker(ctx context.Context, brokerID uint64) error {
	op := UnregisterBrokerOp{
		BrokerID: brokerID,
	}
	return s.propose(ctx, OpUnregisterBroker, op)
}

// UpdateBroker updates broker information.
func (s *Store) UpdateBroker(ctx context.Context, brokerID uint64, status BrokerStatus, resources BrokerResources) error {
	op := UpdateBrokerOp{
		BrokerID:  brokerID,
		Status:    status,
		Resources: resources,
		Heartbeat: time.Now(),
	}
	return s.propose(ctx, OpUpdateBroker, op)
}

// CreateTopic creates a new topic.
func (s *Store) CreateTopic(ctx context.Context, name string, numPartitions, replicationFactor int, config TopicConfig) error {
	op := CreateTopicOp{
		Name:              name,
		NumPartitions:     numPartitions,
		ReplicationFactor: replicationFactor,
		Config:            config,
	}
	return s.propose(ctx, OpCreateTopic, op)
}

// DeleteTopic deletes a topic.
func (s *Store) DeleteTopic(ctx context.Context, name string) error {
	op := DeleteTopicOp{
		Name: name,
	}
	return s.propose(ctx, OpDeleteTopic, op)
}

// UpdateTopic updates topic configuration.
func (s *Store) UpdateTopic(ctx context.Context, name string, config TopicConfig) error {
	op := UpdateTopicOp{
		Name:   name,
		Config: config,
	}
	return s.propose(ctx, OpUpdateTopic, op)
}

// CreatePartition creates a new partition.
func (s *Store) CreatePartition(ctx context.Context, partition *PartitionInfo) error {
	op := CreatePartitionOp{
		Partition: *partition,
	}
	return s.propose(ctx, OpCreatePartition, op)
}

// BatchCreatePartitions creates multiple partitions atomically in a single Raft proposal.
// This is more efficient than calling CreatePartition multiple times and prevents
// leadership instability under rapid operations.
func (s *Store) BatchCreatePartitions(ctx context.Context, partitions []*PartitionInfo) error {
	// Convert to []PartitionInfo (not pointers)
	parts := make([]PartitionInfo, len(partitions))
	for i, p := range partitions {
		parts[i] = *p
	}

	op := BatchCreatePartitionsOp{
		Partitions: parts,
	}
	return s.propose(ctx, OpBatchCreatePartitions, op)
}

// UpdatePartition updates a partition.
func (s *Store) UpdatePartition(ctx context.Context, partition *PartitionInfo) error {
	op := UpdatePartitionOp{
		Partition: *partition,
	}
	return s.propose(ctx, OpUpdatePartition, op)
}

// UpdateLeader updates the leader for a partition.
func (s *Store) UpdateLeader(ctx context.Context, topic string, partition int, leader, leaderEpoch uint64) error {
	op := UpdateLeaderOp{
		Topic:       topic,
		Partition:   partition,
		Leader:      leader,
		LeaderEpoch: leaderEpoch,
	}
	return s.propose(ctx, OpUpdateLeader, op)
}

// UpdateISR updates the ISR for a partition.
func (s *Store) UpdateISR(ctx context.Context, topic string, partition int, isr []uint64) error {
	op := UpdateISROp{
		Topic:     topic,
		Partition: partition,
		ISR:       isr,
	}
	return s.propose(ctx, OpUpdateISR, op)
}

// UpdateReplicaList updates the replica list for a partition.
func (s *Store) UpdateReplicaList(ctx context.Context, topic string, partition int, replicas []uint64) error {
	op := UpdateReplicaListOp{
		Topic:     topic,
		Partition: partition,
		Replicas:  replicas,
	}
	return s.propose(ctx, OpUpdateReplicaList, op)
}

// GetBroker returns a broker by ID.
func (s *Store) GetBroker(id uint64) (*BrokerInfo, bool) {
	return s.fsm.GetBroker(id)
}

// GetTopic returns a topic by name.
func (s *Store) GetTopic(name string) (*TopicInfo, bool) {
	return s.fsm.GetTopic(name)
}

// GetPartition returns a partition by topic and partition number.
func (s *Store) GetPartition(topic string, partition int) (*PartitionInfo, bool) {
	return s.fsm.GetPartition(topic, partition)
}

// ListBrokers returns all brokers.
func (s *Store) ListBrokers() []*BrokerInfo {
	return s.fsm.ListBrokers()
}

// ListTopics returns all topics.
func (s *Store) ListTopics() []*TopicInfo {
	return s.fsm.ListTopics()
}

// ListPartitions returns all partitions for a topic.
func (s *Store) ListPartitions(topic string) []*PartitionInfo {
	return s.fsm.ListPartitions(topic)
}

// GetState returns a clone of the current metadata state.
func (s *Store) GetState() *ClusterMetadata {
	return s.fsm.GetState()
}

// IsLeader returns true if the local node is the leader.
func (s *Store) IsLeader() bool {
	return s.consensus.IsLeader()
}

// Leader returns the ID of the current leader.
func (s *Store) Leader() uint64 {
	return s.consensus.Leader()
}

// AllocatePartitions assigns partitions to brokers based on the allocation strategy.
// This is a helper method that creates partition assignments.
func (s *Store) AllocatePartitions(topic string, numPartitions, replicationFactor int) ([]*PartitionInfo, error) {
	// Get available brokers
	brokers := s.ListBrokers()
	if len(brokers) < replicationFactor {
		return nil, fmt.Errorf("not enough brokers: have %d, need %d", len(brokers), replicationFactor)
	}

	// Simple round-robin allocation
	partitions := make([]*PartitionInfo, numPartitions)
	for i := 0; i < numPartitions; i++ {
		replicas := make([]uint64, replicationFactor)
		for j := 0; j < replicationFactor; j++ {
			brokerIdx := (i + j) % len(brokers)
			replicas[j] = brokers[brokerIdx].ID
		}

		partitions[i] = &PartitionInfo{
			Topic:     topic,
			Partition: i,
			Leader:    replicas[0], // First replica is leader
			Replicas:  replicas,
			ISR:       replicas, // Initially all replicas are in ISR
		}
	}

	return partitions, nil
}
