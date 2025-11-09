package client

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

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

	// Subscribe
	err = gc.Subscribe(ctx)
	require.NoError(t, err)

	// Verify state changed
	assert.Equal(t, StateStable, gc.state)
	assert.NotEmpty(t, gc.memberID)

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

	// Subscribe
	err = gc.Subscribe(ctx)
	require.NoError(t, err)

	// Get assignment
	assignment := gc.Assignment()
	assert.NotEmpty(t, assignment)
	assert.Contains(t, assignment, "test-topic")

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

	// Subscribe
	err = gc.Subscribe(ctx)
	require.NoError(t, err)

	// Commit offsets
	offsets := map[string]map[int32]int64{
		"test-topic": {
			0: 100,
		},
	}

	err = gc.CommitSync(ctx, offsets)
	require.NoError(t, err)

	// Verify stats
	stats := gc.Stats()
	assert.Equal(t, int64(1), stats.OffsetsCommitted)

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

	// Subscribe
	err = gc.Subscribe(ctx)
	require.NoError(t, err)

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

	// Subscribe
	err = gc.Subscribe(ctx)
	require.NoError(t, err)

	// Get stats
	stats := gc.Stats()
	assert.Equal(t, "test-group", stats.GroupID)
	assert.NotEmpty(t, stats.MemberID)
	assert.Equal(t, StateStable, stats.State)
	assert.Equal(t, int64(1), stats.RebalanceCount) // One rebalance on join

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

	// Subscribe (will trigger rebalance)
	err = gc.Subscribe(ctx)
	require.NoError(t, err)

	// Verify listener was called
	assert.NotNil(t, assignedPartitions)
	assert.Contains(t, assignedPartitions, "test-topic")

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

	// Subscribe
	err = gc.Subscribe(ctx)
	require.NoError(t, err)

	// Wait a bit to allow heartbeats to run
	time.Sleep(300 * time.Millisecond)

	// Close should stop heartbeats cleanly
	err = gc.Close()
	require.NoError(t, err)
}
