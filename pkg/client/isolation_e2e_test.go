package client

import (
	"context"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fetchAll drains a partition from offset 0 at the given isolation level,
// following NextOffset, and returns the message values it saw. It stops when a
// fetch stops making progress, so a consumer that fails to advance past a
// filtered record shows up as a bounded failure rather than an infinite loop.
func fetchAll(t *testing.T, c *Client, topic string, partition int32, level protocol.IsolationLevel) []string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var values []string
	offset := int64(0)

	for i := 0; i < 50; i++ {
		resp, err := c.Fetch(ctx, &FetchRequest{
			Topic:          topic,
			Partition:      partition,
			Offset:         offset,
			MaxBytes:       1024 * 1024,
			IsolationLevel: level,
		})
		require.NoError(t, err)

		for _, msg := range resp.Messages {
			values = append(values, string(msg.Value))
		}

		next := resp.NextOffset
		if next < 0 {
			// Old-server fallback: last message + 1.
			if len(resp.Messages) == 0 {
				return values
			}
			next = resp.Messages[len(resp.Messages)-1].Offset + 1
		}
		if next <= offset {
			return values
		}
		offset = next
	}

	t.Fatal("fetch loop did not terminate: the consumer never stopped advancing")
	return values
}

func TestIsolation_CommittedTransactionIsVisibleAndMarkerIsHidden(t *testing.T) {
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

	// A committed transaction's records are visible, and the marker the
	// coordinator wrote to resolve it must never reach a consumer.
	for _, level := range []protocol.IsolationLevel{
		protocol.IsolationReadUncommitted,
		protocol.IsolationReadCommitted,
	} {
		got := fetchAll(t, client, "orders", 0, level)
		assert.Equal(t, []string{"a", "b"}, got, "isolation level %v", level)
	}
}

func TestIsolation_ConsumerAdvancesPastAMarkerOnlyWindow(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	tp := newTestTransactionalProducer(t, client, "txn-1")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Two transactions: the marker of the first sits between the two
	// batches, so a naive consumer reading a window containing only that
	// marker would stop making progress and never see the second batch.
	require.NoError(t, tp.BeginTransaction(ctx))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("first")}))
	require.NoError(t, tp.CommitTransaction(ctx))

	require.NoError(t, tp.BeginTransaction(ctx))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("second")}))
	require.NoError(t, tp.CommitTransaction(ctx))

	got := fetchAll(t, client, "orders", 0, protocol.IsolationReadCommitted)

	assert.Equal(t, []string{"first", "second"}, got,
		"a marker between batches must not stall the consumer")
}

func TestIsolation_AbortedTransactionWritesNoRecords(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	tp := newTestTransactionalProducer(t, client, "txn-1")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, tp.BeginTransaction(ctx))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("kept")}))
	require.NoError(t, tp.CommitTransaction(ctx))

	require.NoError(t, tp.BeginTransaction(ctx))
	require.NoError(t, tp.Send(ctx, "orders", 0, protocol.Message{Value: []byte("dropped")}))
	require.NoError(t, tp.AbortTransaction(ctx))

	// TransactionalProducer buffers a transaction's records until commit, so
	// an aborted transaction never puts anything on the partition in the
	// first place - a read_uncommitted consumer cannot see it either.
	for _, level := range []protocol.IsolationLevel{
		protocol.IsolationReadUncommitted,
		protocol.IsolationReadCommitted,
	} {
		got := fetchAll(t, client, "orders", 0, level)
		assert.NotContains(t, got, "dropped", "isolation level %v", level)
		assert.Contains(t, got, "kept", "isolation level %v", level)
	}
}

func TestIsolation_ReadCommittedStopsAtAnOpenTransaction(t *testing.T) {
	broker := startTestBroker(t)
	client := newTestClient(t, broker)
	createTestTopic(t, client, "orders", 1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// A plain record first, so there is something committed to read.
	producer := NewProducer(client)
	require.NoError(t, producer.SendToPartition(ctx, "orders", 0, nil, []byte("settled")))
	require.NoError(t, producer.FlushAll(ctx))

	// Now open a transaction and write to the partition directly, without
	// ending it. This is the shape a streaming transactional producer has;
	// our own TransactionalProducer buffers instead, so this goes through the
	// wire protocol by hand.
	initResp, err := client.InitProducerID(ctx, &protocol.InitProducerIDRequest{
		TransactionID:        "txn-open",
		TransactionTimeoutMs: 60000,
	})
	require.NoError(t, err)
	require.Equal(t, protocol.ErrNone, initResp.ErrorCode)

	addResp, err := client.AddPartitionsToTxn(ctx, &protocol.AddPartitionsToTxnRequest{
		TransactionID: "txn-open",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Partitions:    []protocol.TxnPartition{{Topic: "orders", Partition: 0}},
	})
	require.NoError(t, err)
	require.Equal(t, protocol.ErrNone, addResp.FirstError())

	_, err = client.sendRequestWithRetry(ctx, broker.Addr, &protocol.Request{
		Header: protocol.RequestHeader{Type: protocol.RequestTypeProduce, Version: protocol.ProtocolVersion},
		Payload: &protocol.ProduceRequest{
			Topic:         "orders",
			PartitionID:   0,
			Messages:      []protocol.Message{{Value: []byte("in-flight")}},
			Acks:          protocol.AcksOne,
			ProducerID:    initResp.ProducerID,
			ProducerEpoch: initResp.ProducerEpoch,
		},
	})
	require.NoError(t, err)

	// read_uncommitted sees the in-flight record; read_committed must not.
	uncommitted := fetchAll(t, client, "orders", 0, protocol.IsolationReadUncommitted)
	assert.Contains(t, uncommitted, "in-flight",
		"read_uncommitted should see a record from an open transaction")

	committed := fetchAll(t, client, "orders", 0, protocol.IsolationReadCommitted)
	assert.Contains(t, committed, "settled")
	assert.NotContains(t, committed, "in-flight",
		"read_committed must stop at the last stable offset")

	// Once the transaction commits, the barrier lifts.
	endResp, err := client.EndTxn(ctx, &protocol.EndTxnRequest{
		TransactionID: "txn-open",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Commit:        true,
	})
	require.NoError(t, err)
	require.Equal(t, protocol.ErrNone, endResp.ErrorCode)

	after := fetchAll(t, client, "orders", 0, protocol.IsolationReadCommitted)
	assert.Contains(t, after, "in-flight",
		"a committed transaction's records must become visible")
}
