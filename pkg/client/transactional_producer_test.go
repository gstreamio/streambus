package client

import (
	"context"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestTransactionalProducer builds a transactional producer against a
// running broker.
func newTestTransactionalProducer(t *testing.T, c *Client, txnID string) *TransactionalProducer {
	t.Helper()

	config := DefaultTransactionalProducerConfig()
	config.TransactionID = txnID
	config.RequestTimeout = 5 * time.Second

	tp, err := NewTransactionalProducer(c, config)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tp.Close() })

	return tp
}

// readPartition reads every message currently in a partition under
// read_uncommitted isolation (the FetchRequest zero value).
func readPartition(t *testing.T, c *Client, topic string, partition int32) []protocol.Message {
	t.Helper()
	return readPartitionIsolation(t, c, topic, partition, protocol.IsolationReadUncommitted)
}

// readPartitionIsolation reads every message currently in a partition under
// the given isolation level.
func readPartitionIsolation(t *testing.T, c *Client, topic string, partition int32, level protocol.IsolationLevel) []protocol.Message {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Fetch(ctx, &FetchRequest{
		Topic:          topic,
		Partition:      partition,
		Offset:         0,
		MaxBytes:       1024 * 1024,
		IsolationLevel: level,
	})
	require.NoError(t, err)
	return resp.Messages
}

func TestTransactionalProducer_Create(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)

	tp := newTestTransactionalProducer(t, client, "txn-1")

	assert.Equal(t, ProducerStateReady, tp.state)
	assert.NotZero(t, tp.producerID, "coordinator should have assigned a producer ID")
}

func TestTransactionalProducer_CreateValidation(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)

	_, err := NewTransactionalProducer(client, DefaultTransactionalProducerConfig())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction_id")
}

func TestTransactionalProducer_ReinitializationFencesOlderEpoch(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)

	first := newTestTransactionalProducer(t, client, "txn-1")
	firstEpoch := first.producerEpoch

	// Reclaiming the same transactional ID must bump the epoch so the older
	// instance can be fenced.
	second := newTestTransactionalProducer(t, client, "txn-1")

	assert.Equal(t, first.producerID, second.producerID,
		"the same transactional ID keeps its producer ID")
	assert.Greater(t, second.producerEpoch, firstEpoch,
		"reinitializing must bump the epoch")
}

func TestTransactionalProducer_BeginTransaction(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)

	tp := newTestTransactionalProducer(t, client, "txn-1")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, tp.BeginTransaction(ctx))
	assert.Equal(t, ProducerStateInTransaction, tp.state)

	// A second begin without ending the first must be rejected.
	err := tp.BeginTransaction(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already in progress")
}

func TestTransactionalProducer_CommitWritesMessagesAndMarkers(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 2)

	tp := newTestTransactionalProducer(t, client, "txn-1")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, tp.BeginTransaction(ctx))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("first")}))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("second")}))
	require.NoError(t, tp.Send(ctx, "orders", 1, protocol.Message{Value: []byte("third")}))

	// Nothing is written before the commit.
	assert.Empty(t, readPartition(t, client, "orders", 0),
		"uncommitted messages must not be on the partition")

	require.NoError(t, tp.CommitTransaction(ctx))

	assert.Equal(t, ProducerStateReady, tp.state)
	assert.Equal(t, int64(1), tp.Stats().TransactionsCommitted)

	// Both partitions carry their records plus the commit marker.
	partition0 := readPartition(t, client, "orders", 0)
	require.GreaterOrEqual(t, len(partition0), 2)
	assert.Equal(t, []byte("first"), partition0[0].Value)
	assert.Equal(t, []byte("second"), partition0[1].Value)

	partition1 := readPartition(t, client, "orders", 1)
	require.GreaterOrEqual(t, len(partition1), 1)
	assert.Equal(t, []byte("third"), partition1[0].Value)

	// A commit marker must exist for every participating partition.
	markers := broker.Markers.Markers()
	require.Len(t, markers, 2, "one marker per participating partition")
	for _, marker := range markers {
		assert.True(t, marker.Marker.Commit, "%s-%d should have a commit marker",
			marker.Topic, marker.Partition)
		assert.Equal(t, int64(tp.producerID), int64(marker.Marker.ProducerID))
	}
}

func TestTransactionalProducer_AbortDiscardsMessages(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	tp := newTestTransactionalProducer(t, client, "txn-1")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, tp.BeginTransaction(ctx))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("never")}))

	require.NoError(t, tp.AbortTransaction(ctx))

	assert.Equal(t, ProducerStateReady, tp.state)
	assert.Equal(t, int64(1), tp.Stats().TransactionsAborted)

	// Aborted messages must never reach the partition.
	for _, msg := range readPartition(t, client, "orders", 0) {
		assert.NotEqual(t, []byte("never"), msg.Value,
			"an aborted transaction's messages must not be written")
	}

	// The coordinator still wrote an abort marker.
	markers := broker.Markers.Markers()
	require.Len(t, markers, 1)
	assert.False(t, markers[0].Marker.Commit, "expected an abort marker")
}

// TestTransactionalProducer_ReadCommittedVisibilityFollowsCommit covers the
// isolation property this whole design rests on, through the real
// TransactionalProducer/CommitTransaction path: a read_committed fetch must
// not see a transaction's records before it commits, and must see them once
// it has.
//
// This does not, by itself, prove the records were ever gated on anything -
// Send buffers messages in memory only, and flushMessages (which actually
// writes them) runs immediately before EndTxn inside the same
// CommitTransaction call, so "before" here is trivially empty regardless of
// whether the write was tagged correctly. See
// TestProducer_UnresolvedTransactionalWriteHiddenFromReadCommitted below for
// the test that actually pins down the tagging mechanism this property
// depends on.
func TestTransactionalProducer_ReadCommittedVisibilityFollowsCommit(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	tp := newTestTransactionalProducer(t, client, "txn-read-committed")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, tp.BeginTransaction(ctx))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("staged")}))

	assert.Empty(t, readPartitionIsolation(t, client, "orders", 0, protocol.IsolationReadCommitted),
		"a read_committed fetch must not see records staged before commit")

	require.NoError(t, tp.CommitTransaction(ctx))

	committed := readPartitionIsolation(t, client, "orders", 0, protocol.IsolationReadCommitted)
	require.Len(t, committed, 1)
	assert.Equal(t, []byte("staged"), committed[0].Value)
}

// TestTransactionalProducer_AbortHiddenFromReadCommittedFetch extends
// TestTransactionalProducer_AbortDiscardsMessages with an explicit
// read_committed fetch, rather than relying on the (also true, but weaker)
// read_uncommitted check that test already makes.
func TestTransactionalProducer_AbortHiddenFromReadCommittedFetch(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	tp := newTestTransactionalProducer(t, client, "txn-abort-read-committed")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, tp.BeginTransaction(ctx))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("never")}))
	require.NoError(t, tp.AbortTransaction(ctx))

	for _, msg := range readPartitionIsolation(t, client, "orders", 0, protocol.IsolationReadCommitted) {
		assert.NotEqual(t, []byte("never"), msg.Value,
			"an aborted transaction's messages must not be visible under read_committed either")
	}
	assert.NotEmpty(t, broker.Markers.Markers(), "the abort marker should still have been written")
}

// TestProducer_UnresolvedTransactionalWriteHiddenFromReadCommitted is the
// test that actually pins down the mechanism
// TestTransactionalProducer_ReadCommittedVisibilityFollowsCommit cannot
// reach: a write tagged with a producer id/epoch that never gets an EndTxn
// at all - standing in for the real crash window between flushMessages
// succeeding and EndTxn resolving that CommitTransaction cannot avoid (see
// its own internal structure, and pkg/replication/link's commitBatch doc
// comment, which names this exact risk) - must stay hidden from
// read_committed indefinitely, while remaining fully visible under
// read_uncommitted (the write genuinely reached the log; it is only gated,
// not withheld).
//
// This writes through newTransactionalInternalProducer directly rather than
// through a whole TransactionalProducer, because Send/CommitTransaction give
// no way to stop between the write and the marker - that gap is exactly
// what this test needs to hold open.
func TestProducer_UnresolvedTransactionalWriteHiddenFromReadCommitted(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	producer := newTransactionalInternalProducer(client, 9001, 1)
	defer func() { _ = producer.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, producer.SendMessagesToPartition(ctx, "orders", 0,
		[]protocol.Message{{Value: []byte("in-flight")}}))
	require.NoError(t, producer.FlushAll(ctx))

	assert.Len(t, readPartitionIsolation(t, client, "orders", 0, protocol.IsolationReadUncommitted), 1,
		"the write genuinely reached the log")
	assert.Empty(t, readPartitionIsolation(t, client, "orders", 0, protocol.IsolationReadCommitted),
		"a write tagged as transactional but never resolved by EndTxn must stay hidden from read_committed")
}

// TestProducer_PlainWriteUnaffectedByTagging is the control for the fix
// above: NewProducer/NewProducerWithConfig must keep sending producer id 0 -
// the broker's sentinel for "not transactional" - so ordinary, non-
// transactional traffic is visible under every isolation level exactly as
// before this change.
func TestProducer_PlainWriteUnaffectedByTagging(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	producer := NewProducer(client)
	defer func() { _ = producer.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, producer.Send(ctx, "orders", nil, []byte("plain")))
	require.NoError(t, producer.FlushAll(ctx)) // the default client config batches; force it out now

	assert.Len(t, readPartitionIsolation(t, client, "orders", 0, protocol.IsolationReadCommitted), 1,
		"a plain, non-transactional write must remain visible under read_committed immediately")
}

func TestTransactionalProducer_MultipleTransactions(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	tp := newTestTransactionalProducer(t, client, "txn-1")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Commit, then abort, then commit again on the same producer.
	require.NoError(t, tp.BeginTransaction(ctx))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("kept-1")}))
	require.NoError(t, tp.CommitTransaction(ctx))

	require.NoError(t, tp.BeginTransaction(ctx))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("dropped")}))
	require.NoError(t, tp.AbortTransaction(ctx))

	require.NoError(t, tp.BeginTransaction(ctx))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("kept-2")}))
	require.NoError(t, tp.CommitTransaction(ctx))

	stats := tp.Stats()
	assert.Equal(t, int64(2), stats.TransactionsCommitted)
	assert.Equal(t, int64(1), stats.TransactionsAborted)

	var values []string
	for _, msg := range readPartition(t, client, "orders", 0) {
		if len(msg.Value) > 0 {
			values = append(values, string(msg.Value))
		}
	}
	assert.Contains(t, values, "kept-1")
	assert.Contains(t, values, "kept-2")
	assert.NotContains(t, values, "dropped")
}

func TestTransactionalProducer_SendWithoutTransaction(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	tp := newTestTransactionalProducer(t, client, "txn-1")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("x")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no transaction in progress")

	err = tp.CommitTransaction(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no transaction in progress")
}

func TestTransactionalProducer_SendOffsetsCommitAtomically(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	tp := newTestTransactionalProducer(t, client, "txn-1")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, tp.BeginTransaction(ctx))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("out")}))
	require.NoError(t, tp.SendOffsetsToTransaction(ctx, "analytics",
		map[string]map[int32]int64{"source": {0: 42}}))

	// The offsets must not be visible to the group before the commit.
	before, err := client.FetchOffsets(ctx, &protocol.OffsetFetchRequest{GroupID: "analytics"})
	require.NoError(t, err)
	assert.Empty(t, before.Topics, "offsets became visible before the transaction committed")

	require.NoError(t, tp.CommitTransaction(ctx))

	after, err := client.FetchOffsets(ctx, &protocol.OffsetFetchRequest{
		GroupID: "analytics",
		Topics:  []protocol.OffsetFetchTopic{{Topic: "source", Partitions: []int32{0}}},
	})
	require.NoError(t, err)
	require.Len(t, after.Topics, 1)
	require.Len(t, after.Topics[0].Partitions, 1)
	assert.Equal(t, int64(42), after.Topics[0].Partitions[0].Offset)
}

func TestTransactionalProducer_SendOffsetsDiscardedOnAbort(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	tp := newTestTransactionalProducer(t, client, "txn-1")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, tp.BeginTransaction(ctx))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("out")}))
	require.NoError(t, tp.SendOffsetsToTransaction(ctx, "analytics",
		map[string]map[int32]int64{"source": {0: 42}}))

	require.NoError(t, tp.AbortTransaction(ctx))

	after, err := client.FetchOffsets(ctx, &protocol.OffsetFetchRequest{GroupID: "analytics"})
	require.NoError(t, err)
	assert.Empty(t, after.Topics, "an aborted transaction's offsets must not be published")
}

func TestTransactionalProducer_SendOffsetsWithoutTransaction(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)

	tp := newTestTransactionalProducer(t, client, "txn-1")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := tp.SendOffsetsToTransaction(ctx, "analytics",
		map[string]map[int32]int64{"source": {0: 42}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no transaction in progress")
}

func TestTransactionalProducer_SendOffsetsRejectsSecondGroup(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	tp := newTestTransactionalProducer(t, client, "txn-1")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, tp.BeginTransaction(ctx))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("out")}))
	require.NoError(t, tp.SendOffsetsToTransaction(ctx, "analytics",
		map[string]map[int32]int64{"source": {0: 42}}))

	err := tp.SendOffsetsToTransaction(ctx, "other-group",
		map[string]map[int32]int64{"source": {0: 43}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "analytics")
}

func TestTransactionalProducer_Close(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	config := DefaultTransactionalProducerConfig()
	config.TransactionID = "txn-1"
	config.RequestTimeout = 5 * time.Second

	tp, err := NewTransactionalProducer(client, config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, tp.BeginTransaction(ctx))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("never")}))

	// Closing mid-transaction aborts it rather than leaving it open until it
	// times out.
	require.NoError(t, tp.Close())
	assert.Equal(t, ProducerStateClosed, tp.state)
	assert.Equal(t, int64(1), tp.Stats().TransactionsAborted)

	markers := broker.Markers.Markers()
	require.Len(t, markers, 1)
	assert.False(t, markers[0].Marker.Commit, "closing should abort, not commit")

	// Second close reports the producer is already closed.
	assert.Equal(t, ErrProducerClosed, tp.Close())
}

func TestTransactionalProducer_Stats(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	tp := newTestTransactionalProducer(t, client, "txn-1")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, tp.BeginTransaction(ctx))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("a")}))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("b")}))
	require.NoError(t, tp.CommitTransaction(ctx))

	stats := tp.Stats()
	assert.Equal(t, tp.producerID, stats.ProducerID)
	assert.Equal(t, ProducerStateReady, stats.State)
	assert.Equal(t, int64(1), stats.TransactionsCommitted)
	assert.Equal(t, int64(0), stats.TransactionsAborted)
	assert.Equal(t, int64(2), stats.MessagesProduced)
}
