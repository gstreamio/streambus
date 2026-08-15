package client

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gstreamio/streambus/pkg/consumer/group"
	"github.com/gstreamio/streambus/pkg/protocol"
)

// GroupConsumer is a consumer that coordinates with other consumers in a group.
//
// NOT YET IMPLEMENTED: join/sync/heartbeat/offset-commit coordination against a
// broker-side group coordinator does not exist yet. Subscribe returns
// ErrGroupCoordinationNotImplemented rather than silently faking a successful
// join, as earlier versions did. Use Consumer or PartitionConsumer for
// single-partition consumption in the meantime.
type GroupConsumer struct {
	client *Client
	config GroupConsumerConfig

	// Group membership
	groupID      string
	memberID     string
	generationID int32

	// Topics and assignment
	topics     []string
	assignment map[string][]int32 // topic -> partitions

	// State
	mu     sync.RWMutex
	state  ConsumerState
	closed int32

	// Coordination
	heartbeatCtx    context.Context
	heartbeatCancel context.CancelFunc
	heartbeatWg     sync.WaitGroup

	// Rebalance listener
	rebalanceListener RebalanceListener

	// Metrics
	rebalanceCount   int64
	messagesRead     int64
	offsetsCommitted int64
}

// ConsumerState represents the state of a group consumer
type ConsumerState int

const (
	StateUnjoined ConsumerState = iota
	StateJoining
	StateRebalancing
	StateStable
)

// GroupConsumerConfig holds group consumer configuration
type GroupConsumerConfig struct {
	// Consumer group ID
	GroupID string

	// Topics to subscribe to
	Topics []string

	// Session timeout (how long coordinator waits before removing member)
	SessionTimeoutMs int32

	// Rebalance timeout (max time for rebalance)
	RebalanceTimeoutMs int32

	// Heartbeat interval (how often to send heartbeats)
	HeartbeatIntervalMs int32

	// Assignment strategy
	AssignmentStrategy string // "range", "roundrobin", "sticky"

	// Auto commit offsets
	AutoCommit bool

	// Auto commit interval
	AutoCommitIntervalMs int32

	// Client ID for identification
	ClientID string
}

// DefaultGroupConsumerConfig returns default configuration
func DefaultGroupConsumerConfig() GroupConsumerConfig {
	return GroupConsumerConfig{
		SessionTimeoutMs:     30000, // 30 seconds
		RebalanceTimeoutMs:   60000, // 60 seconds
		HeartbeatIntervalMs:  3000,  // 3 seconds
		AssignmentStrategy:   "range",
		AutoCommit:           true,
		AutoCommitIntervalMs: 5000, // 5 seconds
		ClientID:             "streambus-consumer",
	}
}

// RebalanceListener is called during rebalancing events
type RebalanceListener interface {
	// OnPartitionsRevoked is called before partitions are revoked
	OnPartitionsRevoked(partitions map[string][]int32)

	// OnPartitionsAssigned is called after new partitions are assigned
	OnPartitionsAssigned(partitions map[string][]int32)
}

// NewGroupConsumer creates a new group consumer
func NewGroupConsumer(client *Client, config GroupConsumerConfig) (*GroupConsumer, error) {
	if config.GroupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}
	if len(config.Topics) == 0 {
		return nil, fmt.Errorf("topics are required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	gc := &GroupConsumer{
		client:            client,
		config:            config,
		groupID:           config.GroupID,
		topics:            config.Topics,
		assignment:        make(map[string][]int32),
		state:             StateUnjoined,
		heartbeatCtx:      ctx,
		heartbeatCancel:   cancel,
		rebalanceListener: &DefaultRebalanceListener{},
	}

	return gc, nil
}

// SetRebalanceListener sets a custom rebalance listener
func (gc *GroupConsumer) SetRebalanceListener(listener RebalanceListener) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.rebalanceListener = listener
}

// Subscribe subscribes to the group and starts consuming
func (gc *GroupConsumer) Subscribe(ctx context.Context) error {
	gc.mu.Lock()
	if gc.state != StateUnjoined {
		gc.mu.Unlock()
		return fmt.Errorf("consumer already subscribed")
	}
	gc.state = StateJoining
	gc.mu.Unlock()

	// Join the group
	if err := gc.joinGroup(ctx); err != nil {
		gc.mu.Lock()
		gc.state = StateUnjoined
		gc.mu.Unlock()
		return fmt.Errorf("failed to join group: %w", err)
	}

	// Start heartbeat sender
	gc.heartbeatWg.Add(1)
	go gc.heartbeatSender()

	return nil
}

// Poll polls for messages from assigned partitions
func (gc *GroupConsumer) Poll(ctx context.Context) (map[string]map[int32][]protocol.Message, error) {
	if atomic.LoadInt32(&gc.closed) == 1 {
		return nil, ErrConsumerClosed
	}

	gc.mu.RLock()
	if gc.state != StateStable {
		gc.mu.RUnlock()
		return nil, fmt.Errorf("consumer not in stable state")
	}

	// Get current assignment
	assignment := make(map[string][]int32)
	for topic, partitions := range gc.assignment {
		assignment[topic] = append([]int32{}, partitions...)
	}
	gc.mu.RUnlock()

	// Fetch from all assigned partitions
	result := make(map[string]map[int32][]protocol.Message)

	for topic, partitions := range assignment {
		result[topic] = make(map[int32][]protocol.Message)

		for _, partition := range partitions {
			// TODO: Fetch from partition
			// For now, return empty
			result[topic][partition] = []protocol.Message{}
		}
	}

	atomic.AddInt64(&gc.messagesRead, 1)
	return result, nil
}

// CommitSync commits offsets synchronously.
//
// NOT YET IMPLEMENTED: returns ErrGroupCoordinationNotImplemented. There is no
// broker-side group coordinator to commit offsets to yet, so this used to
// silently report success while only incrementing a metric — offsets were
// never actually persisted anywhere. Failing clearly here is safer than
// callers believing their commit succeeded.
func (gc *GroupConsumer) CommitSync(ctx context.Context, offsets map[string]map[int32]int64) error {
	if atomic.LoadInt32(&gc.closed) == 1 {
		return ErrConsumerClosed
	}
	_ = offsets

	return ErrGroupCoordinationNotImplemented
}

// Close closes the consumer and leaves the group
func (gc *GroupConsumer) Close() error {
	if !atomic.CompareAndSwapInt32(&gc.closed, 0, 1) {
		return ErrConsumerClosed
	}

	// Leave the group
	if err := gc.leaveGroup(context.Background()); err != nil {
		// Log but don't fail
		fmt.Printf("Error leaving group: %v\n", err)
	}

	// Stop heartbeat
	gc.heartbeatCancel()
	gc.heartbeatWg.Wait()

	return nil
}

// Assignment returns the current partition assignment
func (gc *GroupConsumer) Assignment() map[string][]int32 {
	gc.mu.RLock()
	defer gc.mu.RUnlock()

	result := make(map[string][]int32)
	for topic, partitions := range gc.assignment {
		result[topic] = append([]int32{}, partitions...)
	}
	return result
}

// Stats returns consumer statistics
func (gc *GroupConsumer) Stats() GroupConsumerStats {
	return GroupConsumerStats{
		GroupID:          gc.groupID,
		MemberID:         gc.memberID,
		State:            gc.state,
		RebalanceCount:   atomic.LoadInt64(&gc.rebalanceCount),
		MessagesRead:     atomic.LoadInt64(&gc.messagesRead),
		OffsetsCommitted: atomic.LoadInt64(&gc.offsetsCommitted),
	}
}

// GroupConsumerStats holds statistics for a group consumer
type GroupConsumerStats struct {
	GroupID          string
	MemberID         string
	State            ConsumerState
	RebalanceCount   int64
	MessagesRead     int64
	OffsetsCommitted int64
}

// Internal methods

func (gc *GroupConsumer) joinGroup(ctx context.Context) error {
	// Build join request
	joinReq := &group.JoinGroupRequest{
		GroupID:            gc.groupID,
		SessionTimeoutMs:   gc.config.SessionTimeoutMs,
		RebalanceTimeoutMs: gc.config.RebalanceTimeoutMs,
		MemberID:           gc.memberID, // Empty on first join
		ClientID:           gc.config.ClientID,
		ProtocolType:       "consumer",
		Protocols: []group.ProtocolMetadata{
			{
				Name:     gc.config.AssignmentStrategy,
				Metadata: gc.encodeSubscription(),
			},
		},
	}
	_ = joinReq

	// Sending the join request to a broker-side group coordinator is not
	// implemented yet. Fail clearly instead of simulating a successful join
	// and a single-member sync/assignment, which previously made GroupConsumer
	// look functional while never actually delivering messages (see Poll).
	return ErrGroupCoordinationNotImplemented
}

func (gc *GroupConsumer) leaveGroup(ctx context.Context) error {
	gc.mu.RLock()
	memberID := gc.memberID
	gc.mu.RUnlock()

	if memberID == "" {
		return nil // Not joined
	}

	// Build leave request
	leaveReq := &group.LeaveGroupRequest{
		GroupID:  gc.groupID,
		MemberID: memberID,
	}

	// TODO: Send leave request to coordinator
	_ = leaveReq

	return nil
}

func (gc *GroupConsumer) heartbeatSender() {
	defer gc.heartbeatWg.Done()

	ticker := time.NewTicker(time.Duration(gc.config.HeartbeatIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-gc.heartbeatCtx.Done():
			return
		case <-ticker.C:
			_ = gc.sendHeartbeat()
		}
	}
}

func (gc *GroupConsumer) sendHeartbeat() error {
	gc.mu.RLock()
	memberID := gc.memberID
	generationID := gc.generationID
	state := gc.state
	gc.mu.RUnlock()

	if state != StateStable {
		return nil // Only send heartbeats when stable
	}

	// Build heartbeat request
	hbReq := &group.HeartbeatRequest{
		GroupID:      gc.groupID,
		GenerationID: generationID,
		MemberID:     memberID,
	}

	// TODO: Send heartbeat to coordinator
	_ = hbReq

	return nil
}

func (gc *GroupConsumer) encodeSubscription() []byte {
	// TODO: Properly encode subscription metadata
	// For now, return simple encoding
	subscription := ""
	for i, topic := range gc.topics {
		if i > 0 {
			subscription += ","
		}
		subscription += topic
	}
	return []byte(subscription)
}

// DefaultRebalanceListener is a no-op rebalance listener
type DefaultRebalanceListener struct{}

func (l *DefaultRebalanceListener) OnPartitionsRevoked(partitions map[string][]int32) {
	// No-op
}

func (l *DefaultRebalanceListener) OnPartitionsAssigned(partitions map[string][]int32) {
	// No-op
}
