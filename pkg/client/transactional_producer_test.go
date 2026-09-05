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

// readPartition reads every message currently in a partition.
func readPartition(t *testing.T, c *Client, topic string, partition int32) []protocol.Message {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Fetch(ctx, &FetchRequest{
		Topic:     topic,
		Partition: partition,
		Offset:    0,
		MaxBytes:  1024 * 1024,
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
