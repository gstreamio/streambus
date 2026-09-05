package client

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestGroupConsumer builds a group consumer against a running broker.
func newTestGroupConsumer(t *testing.T, c *Client, groupID string, topics ...string) *GroupConsumer {
	t.Helper()

	config := DefaultGroupConsumerConfig()
	config.GroupID = groupID
	config.Topics = topics
	config.HeartbeatIntervalMs = 200

	gc, err := NewGroupConsumer(c, config)
	require.NoError(t, err)
	t.Cleanup(func() { _ = gc.Close() })

	return gc
}

func TestGroupConsumer_Create(t *testing.T) {
	client := &Client{config: DefaultConfig()}

	config := DefaultGroupConsumerConfig()
	config.GroupID = "test-group"
	config.Topics = []string{"test-topic"}

	gc, err := NewGroupConsumer(client, config)
	require.NoError(t, err)
	assert.Equal(t, "test-group", gc.groupID)
	assert.Equal(t, StateUnjoined, gc.state)

	// Test missing group ID
	_, err = NewGroupConsumer(client, DefaultGroupConsumerConfig())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "group_id")

	// Test missing topics
	config2 := DefaultGroupConsumerConfig()
	config2.GroupID = "test-group"
	_, err = NewGroupConsumer(client, config2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "topics")
}

func TestGroupConsumer_SubscribeJoinsAndIsAssignedPartitions(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 3)

	gc := newTestGroupConsumer(t, client, "analytics", "orders")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, gc.Subscribe(ctx))

	assert.Equal(t, StateStable, gc.state)
	assert.NotEmpty(t, gc.memberID, "coordinator should have assigned a member ID")

	// A single member owns every partition of its subscribed topic.
	assignment := gc.Assignment()
	require.Contains(t, assignment, "orders")
	assert.ElementsMatch(t, []int32{0, 1, 2}, assignment["orders"])
}

func TestGroupConsumer_SubscribeUnknownTopicFails(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)

	gc := newTestGroupConsumer(t, client, "analytics", "never-created")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Assigning around a topic the broker does not have would silently drop
	// everything published to it later, so the join must fail instead.
	err := gc.Subscribe(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "never-created")
	assert.Equal(t, StateUnjoined, gc.state)
}

func TestGroupConsumer_TwoMembersSplitPartitions(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 4)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	first := newTestGroupConsumer(t, client, "analytics", "orders")
	require.NoError(t, first.Subscribe(ctx))

	// The second join starts a new generation and the first member is the
	// group leader, so its rejoin is what produces the new assignment. Like
	// any Kafka-style consumer, every member must take part in the rebalance,
	// so the two calls have to overlap.
	second := newTestGroupConsumer(t, client, "analytics", "orders")

	secondJoined := make(chan error, 1)
	go func() { secondJoined <- second.Subscribe(ctx) }()

	// Give the second member time to reach JoinGroup and bump the generation
	// before the leader rejoins.
	time.Sleep(200 * time.Millisecond)

	first.markRejoinNeeded()
	require.NoError(t, first.rejoin(ctx))

	select {
	case err := <-secondJoined:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("second member never completed its join")
	}

	firstPartitions := first.Assignment()["orders"]
	secondPartitions := second.Assignment()["orders"]

	assert.Len(t, firstPartitions, 2, "four partitions across two members")
	assert.Len(t, secondPartitions, 2, "four partitions across two members")

	// No partition may be assigned to both members.
	owned := make(map[int32]int)
	for _, p := range firstPartitions {
		owned[p]++
	}
	for _, p := range secondPartitions {
		owned[p]++
	}
	for partition, count := range owned {
		assert.Equal(t, 1, count, "partition %d assigned %d times", partition, count)
	}
	assert.Len(t, owned, 4, "every partition should be assigned exactly once")
}

func TestGroupConsumer_PollReadsProducedMessages(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	producer := NewProducer(client)
	for i := 0; i < 3; i++ {
		require.NoError(t, producer.SendToPartition(ctx, "orders", 0, nil, []byte{byte('a' + i)}))
	}
	require.NoError(t, producer.FlushAll(ctx))

	gc := newTestGroupConsumer(t, client, "analytics", "orders")
	require.NoError(t, gc.Subscribe(ctx))

	messages, err := gc.Poll(ctx)
	require.NoError(t, err)

	require.Contains(t, messages, "orders")
	got := messages["orders"][0]
	require.Len(t, got, 3, "should read every produced message")
	for i, msg := range got {
		assert.Equal(t, []byte{byte('a' + i)}, msg.Value)
		assert.Equal(t, int64(i), msg.Offset)
	}

	// The next poll resumes after what it already read.
	position, ok := gc.Position("orders", 0)
	require.True(t, ok)
	assert.Equal(t, int64(3), position)

	second, err := gc.Poll(ctx)
	require.NoError(t, err)
	assert.Empty(t, second["orders"][0], "already-read messages must not be redelivered")
}

func TestGroupConsumer_CommitSyncPersistsOffsets(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 2)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	gc := newTestGroupConsumer(t, client, "analytics", "orders")
	require.NoError(t, gc.Subscribe(ctx))

	offsets := map[string]map[int32]int64{
		"orders": {0: 100, 1: 250},
	}
	require.NoError(t, gc.CommitSync(ctx, offsets))

	assert.Equal(t, int64(2), gc.Stats().OffsetsCommitted)

	committed, err := gc.Committed(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(100), committed["orders"][0])
	assert.Equal(t, int64(250), committed["orders"][1])
}

func TestGroupConsumer_CommitSyncDefaultsToCurrentPositions(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	producer := NewProducer(client)
	for i := 0; i < 5; i++ {
		require.NoError(t, producer.SendToPartition(ctx, "orders", 0, nil, []byte{byte(i)}))
	}
	require.NoError(t, producer.FlushAll(ctx))

	gc := newTestGroupConsumer(t, client, "analytics", "orders")
	require.NoError(t, gc.Subscribe(ctx))

	_, err := gc.Poll(ctx)
	require.NoError(t, err)

	// A nil offset map commits what was actually consumed.
	require.NoError(t, gc.CommitSync(ctx, nil))

	committed, err := gc.Committed(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(5), committed["orders"][0])
}

func TestGroupConsumer_ResumesFromCommittedOffset(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	producer := NewProducer(client)
	for i := 0; i < 4; i++ {
		require.NoError(t, producer.SendToPartition(ctx, "orders", 0, nil, []byte{byte(i)}))
	}
	require.NoError(t, producer.FlushAll(ctx))

	first := newTestGroupConsumer(t, client, "analytics", "orders")
	require.NoError(t, first.Subscribe(ctx))
	require.NoError(t, first.CommitSync(ctx, map[string]map[int32]int64{"orders": {0: 2}}))
	require.NoError(t, first.Close())

	// A fresh member of the same group picks up where the last one committed
	// rather than replaying from the beginning.
	second := newTestGroupConsumer(t, client, "analytics", "orders")
	require.NoError(t, second.Subscribe(ctx))

	position, ok := second.Position("orders", 0)
	require.True(t, ok)
	assert.Equal(t, int64(2), position)

	messages, err := second.Poll(ctx)
	require.NoError(t, err)
	assert.Len(t, messages["orders"][0], 2, "should only see the uncommitted tail")
}

func TestGroupConsumer_CommitSyncWithoutSubscribeFails(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)

	gc := newTestGroupConsumer(t, client, "analytics", "orders")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := gc.CommitSync(ctx, map[string]map[int32]int64{"orders": {0: 1}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not subscribed")
	assert.Equal(t, int64(0), gc.Stats().OffsetsCommitted)
}

func TestGroupConsumer_RebalanceListenerFires(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 2)

	gc := newTestGroupConsumer(t, client, "analytics", "orders")

	var assigned map[string][]int32
	gc.SetRebalanceListener(&TestRebalanceListener{
		onAssigned: func(partitions map[string][]int32) { assigned = partitions },
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, gc.Subscribe(ctx))

	require.NotNil(t, assigned, "listener should fire on assignment")
	assert.ElementsMatch(t, []int32{0, 1}, assigned["orders"])
}

func TestGroupConsumer_LeavingGroupReleasesPartitions(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 2)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	gc := newTestGroupConsumer(t, client, "analytics", "orders")
	require.NoError(t, gc.Subscribe(ctx))
	memberID := gc.memberID

	require.NoError(t, gc.Close())

	// The coordinator must no longer list the departed member.
	group := broker.GroupCoordinator.GetGroup("analytics")
	require.NotNil(t, group)
	assert.NotContains(t, group.Members, memberID)
}

func TestGroupConsumer_Close(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config := DefaultGroupConsumerConfig()
	config.GroupID = "analytics"
	config.Topics = []string{"orders"}

	gc, err := NewGroupConsumer(client, config)
	require.NoError(t, err)
	require.NoError(t, gc.Subscribe(ctx))

	require.NoError(t, gc.Close())
	assert.Equal(t, int32(1), atomic.LoadInt32(&gc.closed))

	// Second close should return error
	assert.Equal(t, ErrConsumerClosed, gc.Close())
}

func TestGroupConsumer_Stats(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	gc := newTestGroupConsumer(t, client, "analytics", "orders")
	require.NoError(t, gc.Subscribe(ctx))

	stats := gc.Stats()
	assert.Equal(t, "analytics", stats.GroupID)
	assert.NotEmpty(t, stats.MemberID)
	assert.Equal(t, StateStable, stats.State)
	assert.Equal(t, int64(1), stats.RebalanceCount)
}

func TestGroupConsumer_HeartbeatKeepsMembershipAlive(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	config := DefaultGroupConsumerConfig()
	config.GroupID = "analytics"
	config.Topics = []string{"orders"}
	// The coordinator's minimum session timeout is 6s; heartbeat far more
	// often than that so the member survives only if heartbeats are actually
	// reaching the coordinator.
	config.SessionTimeoutMs = 6000
	config.HeartbeatIntervalMs = 100

	gc, err := NewGroupConsumer(client, config)
	require.NoError(t, err)
	t.Cleanup(func() { _ = gc.Close() })

	require.NoError(t, gc.Subscribe(ctx))
	memberID := gc.memberID

	// Sleep past the session timeout: without heartbeats the coordinator's
	// expiry sweep would have evicted this member by now.
	time.Sleep(8 * time.Second)

	group := broker.GroupCoordinator.GetGroup("analytics")
	require.NotNil(t, group)
	assert.Contains(t, group.Members, memberID,
		"member expired despite the heartbeat sender running")
}

// TestRebalanceListener is a test implementation of RebalanceListener
type TestRebalanceListener struct {
	onRevoked  func(partitions map[string][]int32)
	onAssigned func(partitions map[string][]int32)
}

func (l *TestRebalanceListener) OnPartitionsRevoked(partitions map[string][]int32) {
	if l.onRevoked != nil {
		l.onRevoked(partitions)
	}
}

func (l *TestRebalanceListener) OnPartitionsAssigned(partitions map[string][]int32) {
	if l.onAssigned != nil {
		l.onAssigned(partitions)
	}
}
