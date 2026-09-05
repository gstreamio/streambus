package server

import (
	"testing"

	"github.com/gstreamio/streambus/pkg/logging"
	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/transaction"
)

// newTransactionTestHandler builds a handler over a live transaction
// coordinator with in-memory markers and offsets.
func newTransactionTestHandler(t *testing.T) (*TransactionHandler, *transaction.MemoryMarkerWriter, *passthroughHandler) {
	t.Helper()

	markers := transaction.NewMemoryMarkerWriter()

	coordinator := transaction.NewTransactionCoordinator(
		transaction.NewMemoryTransactionLog(),
		transaction.DefaultCoordinatorConfig(),
		logging.New(&logging.Config{Level: logging.LevelError, Component: "test"}),
	)
	coordinator.SetMarkerWriter(markers)
	coordinator.SetOffsetCommitter(transaction.NewMemoryOffsetCommitter())
	t.Cleanup(coordinator.Stop)

	base := &passthroughHandler{}
	return NewTransactionHandler(base, coordinator), markers, base
}

// initProducer claims a producer identity through the handler.
func initProducer(t *testing.T, handler *TransactionHandler, txnID string) *protocol.InitProducerIDResponse {
	t.Helper()

	resp := handler.Handle(request(protocol.RequestTypeInitProducerID, &protocol.InitProducerIDRequest{
		TransactionID:        txnID,
		TransactionTimeoutMs: 30000,
	}))

	payload, ok := resp.Payload.(*protocol.InitProducerIDResponse)
	if !ok {
		t.Fatalf("InitProducerID payload = %T", resp.Payload)
	}
	if payload.ErrorCode != protocol.ErrNone {
		t.Fatalf("InitProducerID error = %v", payload.ErrorCode)
	}
	return payload
}

func TestTransactionHandler_FullCommitFlow(t *testing.T) {
	handler, markers, _ := newTransactionTestHandler(t)

	producer := initProducer(t, handler, "txn-1")
	if producer.ProducerID == 0 {
		t.Fatal("coordinator assigned no producer ID")
	}

	addResp := handler.Handle(request(protocol.RequestTypeAddPartitionsToTxn, &protocol.AddPartitionsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    producer.ProducerID,
		ProducerEpoch: producer.ProducerEpoch,
		Partitions: []protocol.TxnPartition{
			{Topic: "orders", Partition: 0},
			{Topic: "events", Partition: 1},
		},
	}))
	add, ok := addResp.Payload.(*protocol.AddPartitionsToTxnResponse)
	if !ok {
		t.Fatalf("AddPartitionsToTxn payload = %T", addResp.Payload)
	}
	if code := add.FirstError(); code != protocol.ErrNone {
		t.Fatalf("AddPartitionsToTxn error = %v", code)
	}
	if len(add.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(add.Results))
	}

	endResp := handler.Handle(request(protocol.RequestTypeEndTxn, &protocol.EndTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    producer.ProducerID,
		ProducerEpoch: producer.ProducerEpoch,
		Commit:        true,
	}))
	end, ok := endResp.Payload.(*protocol.EndTxnResponse)
	if !ok {
		t.Fatalf("EndTxn payload = %T", endResp.Payload)
	}
	if end.ErrorCode != protocol.ErrNone {
		t.Fatalf("EndTxn error = %v", end.ErrorCode)
	}

	written := markers.Markers()
	if len(written) != 2 {
		t.Fatalf("wrote %d markers, want one per participating partition", len(written))
	}
	for _, m := range written {
		if !m.Marker.Commit {
			t.Errorf("%s-%d got an abort marker, want commit", m.Topic, m.Partition)
		}
	}
}

func TestTransactionHandler_AbortWritesAbortMarkers(t *testing.T) {
	handler, markers, _ := newTransactionTestHandler(t)

	producer := initProducer(t, handler, "txn-1")

	handler.Handle(request(protocol.RequestTypeAddPartitionsToTxn, &protocol.AddPartitionsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    producer.ProducerID,
		ProducerEpoch: producer.ProducerEpoch,
		Partitions:    []protocol.TxnPartition{{Topic: "orders", Partition: 0}},
	}))

	handler.Handle(request(protocol.RequestTypeEndTxn, &protocol.EndTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    producer.ProducerID,
		ProducerEpoch: producer.ProducerEpoch,
		Commit:        false,
	}))

	written := markers.Markers()
	if len(written) != 1 {
		t.Fatalf("wrote %d markers, want 1", len(written))
	}
	if written[0].Marker.Commit {
		t.Error("expected an abort marker")
	}
}

func TestTransactionHandler_OffsetsInTransaction(t *testing.T) {
	handler, _, _ := newTransactionTestHandler(t)

	producer := initProducer(t, handler, "txn-1")

	handler.Handle(request(protocol.RequestTypeAddPartitionsToTxn, &protocol.AddPartitionsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    producer.ProducerID,
		ProducerEpoch: producer.ProducerEpoch,
		Partitions:    []protocol.TxnPartition{{Topic: "orders", Partition: 0}},
	}))

	addOffsetsResp := handler.Handle(request(protocol.RequestTypeAddOffsetsToTxn, &protocol.AddOffsetsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    producer.ProducerID,
		ProducerEpoch: producer.ProducerEpoch,
		GroupID:       "analytics",
	}))
	addOffsets, ok := addOffsetsResp.Payload.(*protocol.AddOffsetsToTxnResponse)
	if !ok {
		t.Fatalf("AddOffsetsToTxn payload = %T", addOffsetsResp.Payload)
	}
	if addOffsets.ErrorCode != protocol.ErrNone {
		t.Fatalf("AddOffsetsToTxn error = %v", addOffsets.ErrorCode)
	}

	commitResp := handler.Handle(request(protocol.RequestTypeTxnOffsetCommit, &protocol.TxnOffsetCommitRequest{
		TransactionID: "txn-1",
		GroupID:       "analytics",
		ProducerID:    producer.ProducerID,
		ProducerEpoch: producer.ProducerEpoch,
		Topics: []protocol.OffsetCommitTopic{
			{Topic: "source", Partitions: []protocol.OffsetCommitPartition{{Partition: 0, Offset: 42}}},
		},
	}))
	commit, ok := commitResp.Payload.(*protocol.TxnOffsetCommitResponse)
	if !ok {
		t.Fatalf("TxnOffsetCommit payload = %T", commitResp.Payload)
	}
	if code := commit.FirstError(); code != protocol.ErrNone {
		t.Fatalf("TxnOffsetCommit error = %v", code)
	}
}

func TestTransactionHandler_FencedProducer(t *testing.T) {
	handler, _, _ := newTransactionTestHandler(t)

	first := initProducer(t, handler, "txn-1")

	// Reclaiming the transactional ID bumps the epoch, fencing the old one.
	second := initProducer(t, handler, "txn-1")
	if second.ProducerEpoch <= first.ProducerEpoch {
		t.Fatalf("epoch did not advance: %d then %d", first.ProducerEpoch, second.ProducerEpoch)
	}

	resp := handler.Handle(request(protocol.RequestTypeAddPartitionsToTxn, &protocol.AddPartitionsToTxnRequest{
		TransactionID: "txn-1",
		ProducerID:    first.ProducerID,
		ProducerEpoch: first.ProducerEpoch,
		Partitions:    []protocol.TxnPartition{{Topic: "orders", Partition: 0}},
	}))

	// The fenced producer must not be allowed to join the transaction.
	if resp.Header.Status == protocol.StatusOK {
		if add, ok := resp.Payload.(*protocol.AddPartitionsToTxnResponse); ok {
			if code := add.FirstError(); code == protocol.ErrNone {
				t.Error("a fenced producer was allowed to add partitions")
			}
		}
	}
}

func TestTransactionHandler_PassesThroughOtherRequests(t *testing.T) {
	handler, _, base := newTransactionTestHandler(t)

	handler.Handle(request(protocol.RequestTypeListTopics, &protocol.ListTopicsRequest{}))

	if !base.called {
		t.Error("a non-transaction request should reach the base handler")
	}
}

func TestTransactionHandler_NoCoordinator(t *testing.T) {
	base := &passthroughHandler{}
	handler := NewTransactionHandler(base, nil)

	resp := handler.Handle(request(protocol.RequestTypeEndTxn, &protocol.EndTxnRequest{TransactionID: "txn-1"}))

	if resp.Header.Status != protocol.StatusError {
		t.Fatalf("status = %v, want Error", resp.Header.Status)
	}
	if resp.Header.ErrorCode != protocol.ErrTransactionCoordinatorNotAvailable {
		t.Errorf("error code = %v, want ErrTransactionCoordinatorNotAvailable", resp.Header.ErrorCode)
	}
	if base.called {
		t.Error("a transaction request must not fall through to the base handler")
	}
}

func TestTransactionHandler_WrongPayloadType(t *testing.T) {
	handler, _, _ := newTransactionTestHandler(t)

	resp := handler.Handle(request(protocol.RequestTypeEndTxn, &protocol.HeartbeatRequest{}))

	if resp.Header.ErrorCode != protocol.ErrInvalidRequest {
		t.Errorf("error code = %v, want ErrInvalidRequest", resp.Header.ErrorCode)
	}
}

func TestTxnErrorToProtocol(t *testing.T) {
	tests := []struct {
		name string
		code transaction.ErrorCode
		want protocol.ErrorCode
	}{
		{"none", transaction.ErrorNone, protocol.ErrNone},
		{"producer fenced", transaction.ErrorProducerFenced, protocol.ErrProducerFenced},
		{"invalid state", transaction.ErrorInvalidTransactionState, protocol.ErrInvalidTransactionState},
		{"coordinator unavailable", transaction.ErrorTransactionCoordinatorNotAvailable, protocol.ErrTransactionCoordinatorNotAvailable},
		// An unmapped non-zero code must never look like success.
		{"unmapped", transaction.ErrorCode(9999), protocol.ErrInvalidTransactionState},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := txnErrorToProtocol(tt.code); got != tt.want {
				t.Errorf("txnErrorToProtocol(%v) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}
