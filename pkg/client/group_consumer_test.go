package client

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupConsumer_Create(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultGroupConsumerConfig()
	config.GroupID = "test-group"
	config.Topics = []string{"test-topic"}

	gc, err := NewGroupConsumer(client, config)
	require.NoError(t, err)
	assert.NotNil(t, gc)
	assert.Equal(t, "test-group", gc.groupID)
	assert.Equal(t, []string{"test-topic"}, gc.topics)
	assert.Equal(t, StateUnjoined, gc.state)
}

func TestGroupConsumer_CreateValidation(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	// Test missing group ID
	config1 := DefaultGroupConsumerConfig()
	config1.Topics = []string{"test-topic"}
	_, err := NewGroupConsumer(client, config1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "group_id")

	// Test missing topics
	config2 := DefaultGroupConsumerConfig()
	config2.GroupID = "test-group"
	_, err = NewGroupConsumer(client, config2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "topics")
}

func TestGroupConsumer_Subscribe(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultGroupConsumerConfig()
	config.GroupID = "test-group"
	config.Topics = []string{"test-topic"}

	gc, err := NewGroupConsumer(client, config)
	require.NoError(t, err)

	ctx := context.Background()

	// Subscribe fails: group coordination against a broker-side coordinator
	// isn't implemented, so this must not silently pretend to succeed.
	err = gc.Subscribe(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrGroupCoordinationNotImplemented))

	// State rolls back to unjoined rather than faking a stable join.
	assert.Equal(t, StateUnjoined, gc.state)
	assert.Empty(t, gc.memberID)

	// Clean up
	gc.Close()
}

func TestGroupConsumer_Assignment(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultGroupConsumerConfig()
	config.GroupID = "test-group"
	config.Topics = []string{"test-topic"}

	gc, err := NewGroupConsumer(client, config)
	require.NoError(t, err)

	ctx := context.Background()

	// Subscribe fails before any assignment can happen.
	err = gc.Subscribe(ctx)
	require.Error(t, err)

	// No assignment is made since the group was never actually joined.
	assignment := gc.Assignment()
	assert.Empty(t, assignment)

	// Clean up
	gc.Close()
}

func TestGroupConsumer_CommitSync(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultGroupConsumerConfig()
	config.GroupID = "test-group"
	config.Topics = []string{"test-topic"}

	gc, err := NewGroupConsumer(client, config)
	require.NoError(t, err)

	ctx := context.Background()

	// Commit offsets: must fail rather than silently reporting success,
	// since there's no coordinator to persist the commit with.
	offsets := map[string]map[int32]int64{
		"test-topic": {
			0: 100,
		},
	}

	err = gc.CommitSync(ctx, offsets)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrGroupCoordinationNotImplemented))

	// Stats must not claim a commit that never happened.
	stats := gc.Stats()
	assert.Equal(t, int64(0), stats.OffsetsCommitted)

	// Clean up
	gc.Close()
}

func TestGroupConsumer_Close(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultGroupConsumerConfig()
	config.GroupID = "test-group"
	config.Topics = []string{"test-topic"}

	gc, err := NewGroupConsumer(client, config)
	require.NoError(t, err)

	ctx := context.Background()

	// Subscribe fails, but Close must still work cleanly afterward.
	err = gc.Subscribe(ctx)
	require.Error(t, err)

	// Close
	err = gc.Close()
	require.NoError(t, err)

	// Verify closed
	assert.Equal(t, int32(1), atomic.LoadInt32(&gc.closed))

	// Second close should return error
	err = gc.Close()
	assert.Equal(t, ErrConsumerClosed, err)
}

func TestGroupConsumer_Stats(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultGroupConsumerConfig()
	config.GroupID = "test-group"
	config.Topics = []string{"test-topic"}

	gc, err := NewGroupConsumer(client, config)
	require.NoError(t, err)

	ctx := context.Background()

	// Subscribe fails, so stats must reflect an unjoined consumer, not a
	// faked stable membership.
	err = gc.Subscribe(ctx)
	require.Error(t, err)

	// Get stats
	stats := gc.Stats()
	assert.Equal(t, "test-group", stats.GroupID)
	assert.Empty(t, stats.MemberID)
	assert.Equal(t, StateUnjoined, stats.State)
	assert.Equal(t, int64(0), stats.RebalanceCount)

	// Clean up
	gc.Close()
}

func TestGroupConsumer_RebalanceListener(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultGroupConsumerConfig()
	config.GroupID = "test-group"
	config.Topics = []string{"test-topic"}

	gc, err := NewGroupConsumer(client, config)
	require.NoError(t, err)

	// Track rebalance events
	var assignedPartitions map[string][]int32
	listener := &TestRebalanceListener{
		onAssigned: func(partitions map[string][]int32) {
			assignedPartitions = partitions
		},
	}

	gc.SetRebalanceListener(listener)

	ctx := context.Background()

	// Subscribe fails before any rebalance/assignment can occur.
	err = gc.Subscribe(ctx)
	require.Error(t, err)

	// Listener must not fire for an assignment that never happened.
	assert.Nil(t, assignedPartitions)

	// Clean up
	gc.Close()
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

func TestGroupConsumer_HeartbeatSender(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	config := DefaultGroupConsumerConfig()
	config.GroupID = "test-group"
	config.Topics = []string{"test-topic"}
	config.HeartbeatIntervalMs = 100 // Fast heartbeats for testing

	gc, err := NewGroupConsumer(client, config)
	require.NoError(t, err)

	ctx := context.Background()

	// Subscribe fails, so the heartbeat sender goroutine is never started.
	err = gc.Subscribe(ctx)
	require.Error(t, err)

	// Close must still return cleanly (no goroutine leak / deadlock waiting
	// on a heartbeat sender that never launched).
	err = gc.Close()
	require.NoError(t, err)
}
