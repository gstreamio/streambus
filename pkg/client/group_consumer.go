package client

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gstreamio/streambus/pkg/consumer/group"
	"github.com/gstreamio/streambus/pkg/protocol"
)

// consumerProtocolType is the protocol type StreamBus consumer groups use,
// matching Kafka's so the coordinator can tell consumer groups apart from
// other kinds of group.
const consumerProtocolType = "consumer"

// Backoff bounds for retrying SyncGroup while a rebalance is in progress.
const (
	syncRetryInterval    = 50 * time.Millisecond
	maxSyncRetryInterval = 1 * time.Second
)

// GroupConsumer is a consumer that coordinates with other consumers in a group.
//
// Subscribe joins the group through the broker-side coordinator: it sends
// JoinGroup, computes the assignment if it is elected leader, exchanges it
// through SyncGroup, and then heartbeats to keep its membership alive. Poll
// fetches from the partitions the coordinator assigned, and CommitSync stores
// positions back in the group so another member resumes where this one left
// off.
//
// A GroupConsumer is safe for use from one goroutine at a time; Poll and
// CommitSync must not be called concurrently with each other.
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

	// positions is the next offset to fetch for each assigned partition,
	// seeded from committed offsets when partitions are assigned.
	positions map[string]map[int32]int64

	// protocolName is the assignment protocol the coordinator selected.
	protocolName string

	// rejoinNeeded is set when the coordinator reports a rebalance, so the
	// next Poll rejoins instead of fetching against a stale assignment.
	rejoinNeeded bool

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
		positions:         make(map[string]map[int32]int64),
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

// Poll fetches messages from the partitions assigned to this member.
//
// If the coordinator has signalled a rebalance since the last call, Poll
// rejoins the group first and returns messages for the new assignment. The
// returned map contains an entry for every assigned partition, empty where a
// partition had nothing new.
func (gc *GroupConsumer) Poll(ctx context.Context) (map[string]map[int32][]protocol.Message, error) {
	if atomic.LoadInt32(&gc.closed) == 1 {
		return nil, ErrConsumerClosed
	}

	gc.mu.RLock()
	state := gc.state
	rejoin := gc.rejoinNeeded
	gc.mu.RUnlock()

	if state == StateUnjoined {
		return nil, fmt.Errorf("consumer is not subscribed")
	}

	// A rebalance invalidates the current assignment: rejoin before fetching
	// so this member does not keep reading partitions it no longer owns.
	if rejoin || state != StateStable {
		if err := gc.rejoin(ctx); err != nil {
			return nil, err
		}
	}

	gc.mu.RLock()
	assignment := copyAssignment(gc.assignment)
	positions := make(map[string]map[int32]int64, len(gc.positions))
	for topic, byPartition := range gc.positions {
		positions[topic] = make(map[int32]int64, len(byPartition))
		for partition, offset := range byPartition {
			positions[topic][partition] = offset
		}
	}
	gc.mu.RUnlock()

	result := make(map[string]map[int32][]protocol.Message, len(assignment))
	var fetched int64

	for topic, partitions := range assignment {
		result[topic] = make(map[int32][]protocol.Message, len(partitions))

		for _, partition := range partitions {
			messages, next, err := gc.fetchPartition(ctx, topic, partition, positions[topic][partition])
			if err != nil {
				return nil, fmt.Errorf("fetching %s-%d: %w", topic, partition, err)
			}

			result[topic][partition] = messages
			fetched += int64(len(messages))

			gc.mu.Lock()
			if gc.positions[topic] == nil {
				gc.positions[topic] = make(map[int32]int64)
			}
			gc.positions[topic][partition] = next
			gc.mu.Unlock()
		}
	}

	atomic.AddInt64(&gc.messagesRead, fetched)
	return result, nil
}

// fetchPartition reads one partition from offset, returning the messages and
// the offset to resume from.
func (gc *GroupConsumer) fetchPartition(
	ctx context.Context,
	topic string,
	partition int32,
	offset int64,
) ([]protocol.Message, int64, error) {
	resp, err := gc.client.Fetch(ctx, &FetchRequest{
		Topic:     topic,
		Partition: partition,
		Offset:    offset,
		MaxBytes:  int32(gc.client.config.ConsumerConfig.MaxFetchBytes),
	})
	if err != nil {
		return nil, offset, err
	}

	next := offset
	for _, msg := range resp.Messages {
		if msg.Offset >= next {
			next = msg.Offset + 1
		}
	}

	return resp.Messages, next, nil
}

// Position returns the next offset this consumer will fetch for a partition.
func (gc *GroupConsumer) Position(topic string, partition int32) (int64, bool) {
	gc.mu.RLock()
	defer gc.mu.RUnlock()

	byPartition, ok := gc.positions[topic]
	if !ok {
		return 0, false
	}
	offset, ok := byPartition[partition]
	return offset, ok
}

// CommitSync commits offsets synchronously to the group coordinator.
//
// Passing nil commits this consumer's current fetch positions, which is what
// an at-least-once loop wants after processing a batch from Poll. The call
// returns only once the coordinator has accepted every offset.
func (gc *GroupConsumer) CommitSync(ctx context.Context, offsets map[string]map[int32]int64) error {
	if atomic.LoadInt32(&gc.closed) == 1 {
		return ErrConsumerClosed
	}

	gc.mu.RLock()
	memberID := gc.memberID
	generationID := gc.generationID
	state := gc.state
	if offsets == nil {
		offsets = make(map[string]map[int32]int64, len(gc.positions))
		for topic, byPartition := range gc.positions {
			partitions := make(map[int32]int64, len(byPartition))
			for partition, offset := range byPartition {
				partitions[partition] = offset
			}
			offsets[topic] = partitions
		}
	}
	gc.mu.RUnlock()

	if state == StateUnjoined || memberID == "" {
		return fmt.Errorf("consumer is not subscribed")
	}
	if len(offsets) == 0 {
		return nil
	}

	req := &protocol.OffsetCommitRequest{
		GroupID:      gc.groupID,
		GenerationID: generationID,
		MemberID:     memberID,
		Topics:       offsetCommitTopics(offsets),
	}

	resp, err := gc.client.CommitOffsets(ctx, req)
	if err != nil {
		return fmt.Errorf("committing offsets: %w", err)
	}

	if code := resp.FirstError(); code != protocol.ErrNone {
		// A generation error means this member was rebalanced out mid-batch.
		// Surfacing it as a rejoin request is what lets the caller retry
		// against the assignment it actually owns.
		if code == protocol.ErrIllegalGeneration || code == protocol.ErrRebalanceInProgress {
			gc.markRejoinNeeded()
		}
		return fmt.Errorf("committing offsets: %s", coordinationErrorText(code))
	}

	var committed int64
	for _, partitions := range offsets {
		committed += int64(len(partitions))
	}
	atomic.AddInt64(&gc.offsetsCommitted, committed)

	return nil
}

// Committed returns the offsets the group has committed for this consumer's
// subscribed topics. A partition with no committed offset is absent from the
// result rather than reported as zero.
func (gc *GroupConsumer) Committed(ctx context.Context) (map[string]map[int32]int64, error) {
	if atomic.LoadInt32(&gc.closed) == 1 {
		return nil, ErrConsumerClosed
	}

	resp, err := gc.client.FetchOffsets(ctx, &protocol.OffsetFetchRequest{
		GroupID: gc.groupID,
	})
	if err != nil {
		return nil, fmt.Errorf("fetching committed offsets: %w", err)
	}

	committed := make(map[string]map[int32]int64)
	for _, topic := range resp.Topics {
		for _, partition := range topic.Partitions {
			if partition.ErrorCode != protocol.ErrNone {
				return nil, fmt.Errorf("fetching committed offset for %s-%d: %s",
					topic.Topic, partition.Partition, coordinationErrorText(partition.ErrorCode))
			}
			if partition.Offset == protocol.OffsetNoCommittedValue {
				continue
			}
			if committed[topic.Topic] == nil {
				committed[topic.Topic] = make(map[int32]int64)
			}
			committed[topic.Topic][partition.Partition] = partition.Offset
		}
	}

	return committed, nil
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

// joinGroup runs a full join/sync round with the coordinator and installs the
// resulting assignment.
func (gc *GroupConsumer) joinGroup(ctx context.Context) error {
	gc.mu.RLock()
	memberID := gc.memberID
	gc.mu.RUnlock()

	joinResp, err := gc.client.JoinGroup(ctx, &protocol.JoinGroupRequest{
		GroupID:            gc.groupID,
		MemberID:           memberID,
		ClientID:           gc.config.ClientID,
		ProtocolType:       consumerProtocolType,
		SessionTimeoutMs:   gc.config.SessionTimeoutMs,
		RebalanceTimeoutMs: gc.config.RebalanceTimeoutMs,
		Protocols: []protocol.GroupProtocol{
			{
				Name:     gc.config.AssignmentStrategy,
				Metadata: gc.encodeSubscription(),
			},
		},
	})
	if err != nil {
		return err
	}
	if joinResp.ErrorCode != protocol.ErrNone {
		return fmt.Errorf("joining group %s: %s", gc.groupID, coordinationErrorText(joinResp.ErrorCode))
	}

	gc.mu.Lock()
	gc.memberID = joinResp.MemberID
	gc.generationID = joinResp.GenerationID
	gc.protocolName = joinResp.ProtocolName
	gc.state = StateRebalancing
	gc.mu.Unlock()

	// Only the elected leader computes the assignment; every other member
	// sends an empty SyncGroup and receives its share back.
	var assignments []protocol.SyncGroupAssignment
	if joinResp.IsLeader() {
		assignments, err = gc.computeAssignments(ctx, joinResp)
		if err != nil {
			return fmt.Errorf("computing group assignment: %w", err)
		}
	}

	syncResp, err := gc.syncWithRetry(ctx, joinResp, assignments)
	if err != nil {
		return err
	}

	assignment, err := decodeAssignment(syncResp.Assignment)
	if err != nil {
		return fmt.Errorf("decoding assignment: %w", err)
	}

	return gc.applyAssignment(ctx, assignment)
}

// syncWithRetry sends SyncGroup, retrying while the coordinator reports that
// the rebalance is still in progress.
//
// A follower reaches SyncGroup before the leader has published the
// assignment, and the coordinator answers ErrRebalanceInProgress until it
// does. Retrying here is what turns that into a normal join rather than a
// failure the caller has to interpret. The retry is bounded by the rebalance
// timeout, so a leader that never syncs surfaces as an error instead of
// hanging forever.
func (gc *GroupConsumer) syncWithRetry(
	ctx context.Context,
	joinResp *protocol.JoinGroupResponse,
	assignments []protocol.SyncGroupAssignment,
) (*protocol.SyncGroupResponse, error) {
	deadline := time.Now().Add(time.Duration(gc.config.RebalanceTimeoutMs) * time.Millisecond)
	backoff := syncRetryInterval

	for {
		resp, err := gc.client.SyncGroup(ctx, &protocol.SyncGroupRequest{
			GroupID:      gc.groupID,
			GenerationID: joinResp.GenerationID,
			MemberID:     joinResp.MemberID,
			Assignments:  assignments,
		})
		if err != nil {
			return nil, err
		}
		if resp.ErrorCode == protocol.ErrNone {
			return resp, nil
		}
		if resp.ErrorCode != protocol.ErrRebalanceInProgress {
			return nil, fmt.Errorf("syncing group %s: %s", gc.groupID, coordinationErrorText(resp.ErrorCode))
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf(
				"syncing group %s: rebalance did not complete within the rebalance timeout", gc.groupID)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}

		if backoff < maxSyncRetryInterval {
			backoff *= 2
		}
	}
}

// computeAssignments runs the group's assignment strategy over the members the
// coordinator reported, producing one encoded assignment per member.
func (gc *GroupConsumer) computeAssignments(
	ctx context.Context,
	joinResp *protocol.JoinGroupResponse,
) ([]protocol.SyncGroupAssignment, error) {
	subscriptions := make([]group.MemberSubscription, 0, len(joinResp.Members))
	allTopics := make(map[string]bool)

	for _, member := range joinResp.Members {
		sub, err := protocol.DecodeSubscription(member.Metadata)
		if err != nil {
			return nil, fmt.Errorf("decoding subscription for member %s: %w", member.MemberID, err)
		}
		subscriptions = append(subscriptions, group.MemberSubscription{
			MemberID: member.MemberID,
			Topics:   sub.Topics,
		})
		for _, topic := range sub.Topics {
			allTopics[topic] = true
		}
	}

	topics := make([]string, 0, len(allTopics))
	for topic := range allTopics {
		topics = append(topics, topic)
	}
	sort.Strings(topics)

	counts, err := gc.client.TopicPartitionCounts(ctx, topics)
	if err != nil {
		return nil, fmt.Errorf("listing partitions to assign: %w", err)
	}

	// A subscribed topic the broker does not know about is a real problem
	// for the group: assigning around it would silently drop everything
	// published to it once it appears.
	partitions := make([]group.TopicPartition, 0)
	for _, topic := range topics {
		count, ok := counts[topic]
		if !ok {
			return nil, fmt.Errorf("topic %s does not exist on the broker", topic)
		}
		for i := uint32(0); i < count; i++ {
			//nolint:gosec // partition counts are far below int32 range
			partitions = append(partitions, group.TopicPartition{Topic: topic, Partition: int32(i)})
		}
	}

	assignor := group.GetAssignor(joinResp.ProtocolName)
	assigned := assignor.Assign(subscriptions, partitions)

	// Every member must appear in the response, including one that was
	// assigned nothing: the coordinator only hands back assignments it was
	// given, so an omitted member would wait for an assignment forever.
	result := make([]protocol.SyncGroupAssignment, 0, len(joinResp.Members))
	for _, member := range joinResp.Members {
		byTopic := make(map[string][]int32)
		for _, tp := range assigned[member.MemberID] {
			byTopic[tp.Topic] = append(byTopic[tp.Topic], tp.Partition)
		}
		for topic := range byTopic {
			sort.Slice(byTopic[topic], func(i, j int) bool { return byTopic[topic][i] < byTopic[topic][j] })
		}

		result = append(result, protocol.SyncGroupAssignment{
			MemberID:   member.MemberID,
			Assignment: protocol.EncodeMemberAssignment(&protocol.MemberAssignment{Partitions: byTopic}),
		})
	}

	return result, nil
}

// applyAssignment installs a new assignment, seeding fetch positions from the
// group's committed offsets and notifying the rebalance listener.
func (gc *GroupConsumer) applyAssignment(ctx context.Context, assignment map[string][]int32) error {
	gc.mu.RLock()
	previous := copyAssignment(gc.assignment)
	listener := gc.rebalanceListener
	gc.mu.RUnlock()

	if len(previous) > 0 && listener != nil {
		listener.OnPartitionsRevoked(previous)
	}

	// Resume from the group's committed offsets so a partition moving between
	// members continues where the previous owner stopped rather than
	// replaying from the beginning.
	committed, err := gc.Committed(ctx)
	if err != nil {
		return err
	}

	positions := make(map[string]map[int32]int64, len(assignment))
	for topic, partitions := range assignment {
		positions[topic] = make(map[int32]int64, len(partitions))
		for _, partition := range partitions {
			positions[topic][partition] = committed[topic][partition]
		}
	}

	gc.mu.Lock()
	gc.assignment = assignment
	gc.positions = positions
	gc.state = StateStable
	gc.rejoinNeeded = false
	gc.mu.Unlock()

	atomic.AddInt64(&gc.rebalanceCount, 1)

	if listener != nil {
		listener.OnPartitionsAssigned(copyAssignment(assignment))
	}

	return nil
}

// rejoin re-runs the join/sync round after a rebalance.
func (gc *GroupConsumer) rejoin(ctx context.Context) error {
	gc.mu.Lock()
	gc.state = StateRebalancing
	gc.rejoinNeeded = false
	gc.mu.Unlock()

	if err := gc.joinGroup(ctx); err != nil {
		gc.markRejoinNeeded()
		return fmt.Errorf("rejoining group %s: %w", gc.groupID, err)
	}
	return nil
}

// markRejoinNeeded records that the assignment is stale and the next Poll must
// rejoin before fetching.
func (gc *GroupConsumer) markRejoinNeeded() {
	gc.mu.Lock()
	gc.rejoinNeeded = true
	if gc.state == StateStable {
		gc.state = StateRebalancing
	}
	gc.mu.Unlock()
}

func (gc *GroupConsumer) leaveGroup(ctx context.Context) error {
	gc.mu.RLock()
	memberID := gc.memberID
	gc.mu.RUnlock()

	if memberID == "" {
		return nil // Not joined
	}

	resp, err := gc.client.LeaveGroup(ctx, &protocol.LeaveGroupRequest{
		GroupID:  gc.groupID,
		MemberID: memberID,
	})
	if err != nil {
		return err
	}
	if resp.ErrorCode != protocol.ErrNone {
		return fmt.Errorf("leaving group %s: %s", gc.groupID, coordinationErrorText(resp.ErrorCode))
	}

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
			gc.heartbeatOnce()
		}
	}
}

// heartbeatOnce sends one heartbeat and reacts to what the coordinator says.
//
// A transport failure is left alone: the session timeout is what decides
// whether this member is still alive, and treating a single failed heartbeat
// as an eviction would rebalance the group over a blip.
func (gc *GroupConsumer) heartbeatOnce() {
	gc.mu.RLock()
	memberID := gc.memberID
	generationID := gc.generationID
	state := gc.state
	gc.mu.RUnlock()

	if state != StateStable || memberID == "" {
		return
	}

	ctx, cancel := context.WithTimeout(gc.heartbeatCtx,
		time.Duration(gc.config.SessionTimeoutMs)*time.Millisecond)
	defer cancel()

	resp, err := gc.client.Heartbeat(ctx, &protocol.HeartbeatRequest{
		GroupID:      gc.groupID,
		GenerationID: generationID,
		MemberID:     memberID,
	})
	if err != nil {
		return
	}

	switch resp.ErrorCode {
	case protocol.ErrNone:
		return
	case protocol.ErrRebalanceInProgress, protocol.ErrIllegalGeneration:
		// The group moved on: rejoin on the next Poll.
		gc.markRejoinNeeded()
	case protocol.ErrUnknownMemberID:
		// This member was evicted; drop its identity so the next join is
		// treated as a fresh one rather than being rejected again.
		gc.mu.Lock()
		gc.memberID = ""
		gc.rejoinNeeded = true
		gc.state = StateRebalancing
		gc.mu.Unlock()
	default:
		gc.markRejoinNeeded()
	}
}

// encodeSubscription serializes this consumer's topic subscription for
// JoinGroup.
func (gc *GroupConsumer) encodeSubscription() []byte {
	return protocol.EncodeSubscription(&protocol.Subscription{Topics: gc.topics})
}

// decodeAssignment parses an assignment blob from SyncGroup. An empty blob
// means the member was assigned no partitions.
func decodeAssignment(data []byte) (map[string][]int32, error) {
	if len(data) == 0 {
		return make(map[string][]int32), nil
	}

	decoded, err := protocol.DecodeMemberAssignment(data)
	if err != nil {
		return nil, err
	}

	assignment := make(map[string][]int32, len(decoded.Partitions))
	for topic, partitions := range decoded.Partitions {
		assignment[topic] = append([]int32{}, partitions...)
	}
	return assignment, nil
}

// copyAssignment returns a deep copy of an assignment map.
func copyAssignment(assignment map[string][]int32) map[string][]int32 {
	out := make(map[string][]int32, len(assignment))
	for topic, partitions := range assignment {
		out[topic] = append([]int32{}, partitions...)
	}
	return out
}

// offsetCommitTopics converts an offset map into wire form, sorted so a
// commit produces the same request for the same offsets.
func offsetCommitTopics(offsets map[string]map[int32]int64) []protocol.OffsetCommitTopic {
	topicNames := make([]string, 0, len(offsets))
	for topic := range offsets {
		topicNames = append(topicNames, topic)
	}
	sort.Strings(topicNames)

	topics := make([]protocol.OffsetCommitTopic, 0, len(topicNames))
	for _, topic := range topicNames {
		byPartition := offsets[topic]
		ids := make([]int32, 0, len(byPartition))
		for partition := range byPartition {
			ids = append(ids, partition)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

		partitions := make([]protocol.OffsetCommitPartition, 0, len(ids))
		for _, id := range ids {
			partitions = append(partitions, protocol.OffsetCommitPartition{
				Partition: id,
				Offset:    byPartition[id],
			})
		}
		topics = append(topics, protocol.OffsetCommitTopic{Topic: topic, Partitions: partitions})
	}

	return topics
}

// DefaultRebalanceListener is a no-op rebalance listener
type DefaultRebalanceListener struct{}

func (l *DefaultRebalanceListener) OnPartitionsRevoked(partitions map[string][]int32) {
	// No-op
}

func (l *DefaultRebalanceListener) OnPartitionsAssigned(partitions map[string][]int32) {
	// No-op
}
