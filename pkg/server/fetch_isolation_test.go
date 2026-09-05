package server

import (
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/storage"
)

// produceOne sends a single-message produce request through the handler and
// returns the fetch response for a subsequent fetch, mirroring how a real
// client would drive it end-to-end through the wire types.
func produceOne(t *testing.T, h *Handler, topic string, producerID int64, producerEpoch int16) {
	t.Helper()

	resp := h.Handle(&protocol.Request{
		Header: protocol.RequestHeader{Type: protocol.RequestTypeProduce, Version: protocol.ProtocolVersion},
		Payload: &protocol.ProduceRequest{
			Topic:         topic,
			PartitionID:   0,
			Messages:      []protocol.Message{{Value: []byte("v")}},
			ProducerID:    producerID,
			ProducerEpoch: producerEpoch,
		},
	})
	if resp.Header.Status != protocol.StatusOK {
		t.Fatalf("produce failed: %+v", resp.Payload)
	}
}

// appendControlRecord writes a bare transaction-marker-shaped control record
// directly to a partition's log, the same way pkg/broker's logMarkerWriter
// does. Handler tests use this instead of driving a real transaction
// coordinator, since hiding control records from consumers must hold
// regardless of how the marker got there.
func appendControlRecord(t *testing.T, h *Handler, topic string) {
	t.Helper()

	if !h.TopicManager().TopicExists(topic) {
		if err := h.TopicManager().CreateTopic(topic, 1); err != nil {
			t.Fatalf("CreateTopic failed: %v", err)
		}
	}

	partition, err := h.TopicManager().GetPartition(topic, 0)
	if err != nil {
		t.Fatalf("GetPartition failed: %v", err)
	}

	_, err = partition.Log().Append(&storage.MessageBatch{
		Messages: []storage.Message{{
			Headers: map[string][]byte{
				protocol.ControlHeaderKey: []byte(protocol.ControlTypeTxnMarker),
			},
		}},
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("appending control record failed: %v", err)
	}
}

func fetch(t *testing.T, h *Handler, topic string, offset int64, isolation protocol.IsolationLevel) *protocol.FetchResponse {
	t.Helper()

	resp := h.Handle(&protocol.Request{
		Header: protocol.RequestHeader{Type: protocol.RequestTypeFetch, Version: protocol.ProtocolVersion},
		Payload: &protocol.FetchRequest{
			Topic:          topic,
			PartitionID:    0,
			Offset:         offset,
			MaxBytes:       1024 * 1024,
			IsolationLevel: isolation,
		},
	})
	if resp.Header.Status != protocol.StatusOK {
		t.Fatalf("fetch failed: %+v", resp.Payload)
	}
	return resp.Payload.(*protocol.FetchResponse)
}

func TestHandler_handleFetch_HidesControlRecordsFromConsumers(t *testing.T) {
	h := NewHandlerWithDataDir(t.TempDir())
	defer h.Close()

	const topic = "orders"
	produceOne(t, h, topic, 0, 0)    // offset 0: user data
	appendControlRecord(t, h, topic) // offset 1: marker
	produceOne(t, h, topic, 0, 0)    // offset 2: user data

	resp := fetch(t, h, topic, 0, protocol.IsolationReadUncommitted)

	if len(resp.Messages) != 2 {
		t.Fatalf("Messages = %d, want 2 (control record at offset 1 must be hidden)", len(resp.Messages))
	}
	if resp.Messages[0].Offset != 0 || resp.Messages[1].Offset != 2 {
		t.Errorf("unexpected offsets returned: %d, %d", resp.Messages[0].Offset, resp.Messages[1].Offset)
	}
	if resp.NextOffset != 3 {
		t.Errorf("NextOffset = %d, want 3", resp.NextOffset)
	}
}

// TestHandler_handleFetch_AdvancesPastAFilteredMarker is the regression test
// for the subtlest part of this feature: a fetch window that contains only a
// filtered control record must still tell the client to move past it, or a
// consumer would re-fetch the same apparently-empty window forever.
func TestHandler_handleFetch_AdvancesPastAFilteredMarker(t *testing.T) {
	h := NewHandlerWithDataDir(t.TempDir())
	defer h.Close()

	const topic = "orders"
	appendControlRecord(t, h, topic) // the only record at offset 0

	resp := fetch(t, h, topic, 0, protocol.IsolationReadUncommitted)

	if len(resp.Messages) != 0 {
		t.Fatalf("Messages = %d, want 0 (only record is a filtered marker)", len(resp.Messages))
	}
	if resp.NextOffset != 1 {
		t.Fatalf("NextOffset = %d, want 1; a consumer fetching at 0 again would loop forever", resp.NextOffset)
	}
	if resp.HighWaterMark != 1 {
		t.Fatalf("HighWaterMark = %d, want 1", resp.HighWaterMark)
	}

	// A second fetch from the advanced offset must report "caught up"
	// (NextOffset == the offset asked for), not error or loop.
	caughtUp := fetch(t, h, topic, resp.NextOffset, protocol.IsolationReadUncommitted)
	if len(caughtUp.Messages) != 0 || caughtUp.NextOffset != 1 {
		t.Errorf("second fetch = %+v, want empty and caught up at offset 1", caughtUp)
	}
}

func TestHandler_handleFetch_ReadCommittedClampsToLastStableOffset(t *testing.T) {
	h := NewHandlerWithDataDir(t.TempDir())
	defer h.Close()

	const topic = "orders"
	produceOne(t, h, topic, 1000, 0) // offset 0, opens a transaction
	produceOne(t, h, topic, 0, 0)    // offset 1, ordinary record

	// read_committed must not see past the still-open transaction's start.
	blocked := fetch(t, h, topic, 0, protocol.IsolationReadCommitted)
	if len(blocked.Messages) != 0 {
		t.Fatalf("Messages = %d, want 0 while the transaction at offset 0 is open", len(blocked.Messages))
	}
	if blocked.NextOffset != 0 {
		t.Errorf("NextOffset = %d, want 0 (unchanged: nothing became visible)", blocked.NextOffset)
	}
	if blocked.LastStableOffset != 0 {
		t.Errorf("LastStableOffset = %d, want 0", blocked.LastStableOffset)
	}
	if blocked.HighWaterMark != 2 {
		t.Errorf("HighWaterMark = %d, want 2", blocked.HighWaterMark)
	}

	// read_uncommitted is unaffected by the open transaction.
	unblocked := fetch(t, h, topic, 0, protocol.IsolationReadUncommitted)
	if len(unblocked.Messages) != 2 {
		t.Fatalf("Messages = %d, want 2 under read_uncommitted", len(unblocked.Messages))
	}

	// Resolve the transaction the same way logMarkerWriter does once a
	// marker is durably written, then read_committed should see everything.
	partition, err := h.TopicManager().GetPartition(topic, 0)
	if err != nil {
		t.Fatalf("GetPartition failed: %v", err)
	}
	partition.EndTransaction(1000, 0)

	resolved := fetch(t, h, topic, 0, protocol.IsolationReadCommitted)
	if len(resolved.Messages) != 2 {
		t.Fatalf("Messages = %d, want 2 once the transaction is resolved", len(resolved.Messages))
	}
	if resolved.NextOffset != 2 {
		t.Errorf("NextOffset = %d, want 2", resolved.NextOffset)
	}
	if resolved.LastStableOffset != resolved.HighWaterMark {
		t.Errorf("LastStableOffset = %d, want HighWaterMark %d once no transaction is open",
			resolved.LastStableOffset, resolved.HighWaterMark)
	}
}

func TestHandler_handleFetch_ReadCommittedAdvancesPastResolvedTransactionOnly(t *testing.T) {
	h := NewHandlerWithDataDir(t.TempDir())
	defer h.Close()

	const topic = "orders"
	produceOne(t, h, topic, 1000, 0) // offset 0: producer A opens a transaction
	produceOne(t, h, topic, 2000, 0) // offset 1: producer B opens a transaction
	produceOne(t, h, topic, 0, 0)    // offset 2: ordinary record

	partition, err := h.TopicManager().GetPartition(topic, 0)
	if err != nil {
		t.Fatalf("GetPartition failed: %v", err)
	}

	// Both transactions open: nothing is visible yet.
	blocked := fetch(t, h, topic, 0, protocol.IsolationReadCommitted)
	if len(blocked.Messages) != 0 || blocked.LastStableOffset != 0 {
		t.Fatalf("expected everything blocked with two open transactions, got %+v", blocked)
	}

	// Resolving producer A (the earlier one) must only advance the barrier
	// to producer B's start offset, not all the way to the high water mark.
	partition.EndTransaction(1000, 0)
	partial := fetch(t, h, topic, 0, protocol.IsolationReadCommitted)
	if len(partial.Messages) != 1 || partial.Messages[0].Offset != 0 {
		t.Fatalf("expected only offset 0 visible after resolving producer A, got %+v", partial.Messages)
	}
	if partial.NextOffset != 1 {
		t.Errorf("NextOffset = %d, want 1 (still blocked by producer B)", partial.NextOffset)
	}

	// Resolving producer B unblocks the rest.
	partition.EndTransaction(2000, 0)
	full := fetch(t, h, topic, 1, protocol.IsolationReadCommitted)
	if len(full.Messages) != 2 {
		t.Fatalf("Messages = %d, want 2 once both transactions are resolved", len(full.Messages))
	}
}

// appendMarkerRecord writes a transaction-marker record through the same
// wire-visible path a real fetch reads from, and returns the offset it
// landed at.
func appendMarkerRecord(t *testing.T, h *Handler, topic string, producerID int64, producerEpoch int16, commit bool) int64 {
	t.Helper()

	partition, err := h.TopicManager().GetPartition(topic, 0)
	if err != nil {
		t.Fatalf("GetPartition failed: %v", err)
	}

	offsets, err := partition.Log().Append(&storage.MessageBatch{
		Messages: []storage.Message{{
			Headers: protocol.TransactionMarkerHeaders(producerID, producerEpoch, commit),
		}},
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("appending marker failed: %v", err)
	}
	return int64(offsets[0])
}

// TestHandler_handleFetch_ReadCommittedHidesOnlyTheAbortedProducersRecords
// is the end-to-end case for this feature: once a transaction's own records
// carry its producer identity, an aborted transaction's records must be
// hidden even after its marker lifts the LastStableOffset barrier - while
// another producer's records interleaved in the very same offset range stay
// visible once that producer's own transaction resolves. Without per-record
// producer identity there would be no way to tell these apart.
func TestHandler_handleFetch_ReadCommittedHidesOnlyTheAbortedProducersRecords(t *testing.T) {
	h := NewHandlerWithDataDir(t.TempDir())
	defer h.Close()

	const topic = "orders"
	produceOne(t, h, topic, 1000, 0) // offset 0: producer A (aborts)
	produceOne(t, h, topic, 2000, 0) // offset 1: producer B (commits), interleaved with A
	produceOne(t, h, topic, 1000, 0) // offset 2: producer A again, same transaction

	partition, err := h.TopicManager().GetPartition(topic, 0)
	if err != nil {
		t.Fatalf("GetPartition failed: %v", err)
	}

	abortMarkerOffset := appendMarkerRecord(t, h, topic, 1000, 0, false)
	partition.AbortTransaction(1000, 0, abortMarkerOffset)

	commitMarkerOffset := appendMarkerRecord(t, h, topic, 2000, 0, true)
	partition.EndTransaction(2000, 0)

	resp := fetch(t, h, topic, 0, protocol.IsolationReadCommitted)
	if len(resp.Messages) != 1 || resp.Messages[0].Offset != 1 {
		t.Fatalf("Messages = %+v, want only offset 1 (producer B's committed record)", resp.Messages)
	}

	hwm := commitMarkerOffset + 1
	if resp.HighWaterMark != hwm {
		t.Fatalf("HighWaterMark = %d, want %d", resp.HighWaterMark, hwm)
	}
	if resp.LastStableOffset != resp.HighWaterMark {
		t.Errorf("LastStableOffset = %d, want %d; both transactions are resolved", resp.LastStableOffset, resp.HighWaterMark)
	}
	if resp.NextOffset != resp.HighWaterMark {
		t.Errorf("NextOffset = %d, want %d", resp.NextOffset, resp.HighWaterMark)
	}

	// read_uncommitted is unaffected by the abort: all three data records are
	// visible, only the two control records are hidden.
	unblocked := fetch(t, h, topic, 0, protocol.IsolationReadUncommitted)
	if len(unblocked.Messages) != 3 {
		t.Fatalf("Messages = %d, want 3 under read_uncommitted", len(unblocked.Messages))
	}
}
