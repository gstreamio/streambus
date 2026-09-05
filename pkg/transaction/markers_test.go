package transaction

import (
	"errors"
	"testing"
	"time"
)

// failingMarkerWriter fails on a named partition, to exercise partial-write
// handling.
type failingMarkerWriter struct {
	failTopic     string
	failPartition int32
	written       []RecordedMarker
}

func (w *failingMarkerWriter) WriteMarker(topic string, partition int32, marker *TransactionMarker) error {
	if topic == w.failTopic && partition == w.failPartition {
		return errors.New("disk full")
	}
	w.written = append(w.written, RecordedMarker{Topic: topic, Partition: partition, Marker: *marker})
	return nil
}

// failingOffsetCommitter always fails.
type failingOffsetCommitter struct{}

func (c *failingOffsetCommitter) CommitOffsets(string, map[string]map[int32]OffsetMetadata) error {
	return errors.New("group coordinator unavailable")
}

// newTestCoordinator builds a coordinator with in-memory dependencies.
func newTestCoordinator(t *testing.T) (*TransactionCoordinator, *MemoryMarkerWriter, *MemoryOffsetCommitter) {
	t.Helper()

	markers := NewMemoryMarkerWriter()
	committer := NewMemoryOffsetCommitter()

	tc := NewTransactionCoordinator(NewMemoryTransactionLog(), DefaultCoordinatorConfig(), testLogger())
	tc.SetMarkerWriter(markers)
	tc.SetOffsetCommitter(committer)
	t.Cleanup(tc.Stop)

	return tc, markers, committer
}

// beginTransaction initialises a producer and registers partitions.
func beginTransaction(t *testing.T, tc *TransactionCoordinator, txnID TransactionID, partitions ...PartitionMetadata) (ProducerID, ProducerEpoch) {
	t.Helper()

	initResp, err := tc.InitProducerID(&InitProducerIDRequest{
		TransactionID:      txnID,
		TransactionTimeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("InitProducerID failed: %v", err)
	}
	if initResp.ErrorCode != ErrorNone {
		t.Fatalf("InitProducerID returned %v", initResp.ErrorCode)
	}

	if len(partitions) > 0 {
		addResp, err := tc.AddPartitionsToTxn(&AddPartitionsToTxnRequest{
			TransactionID: txnID,
			ProducerID:    initResp.ProducerID,
			ProducerEpoch: initResp.ProducerEpoch,
			Partitions:    partitions,
		})
		if err != nil {
			t.Fatalf("AddPartitionsToTxn failed: %v", err)
		}
		for topic, byPartition := range addResp.Errors {
			for partition, code := range byPartition {
				if code != ErrorNone {
					t.Fatalf("AddPartitionsToTxn(%s-%d) returned %v", topic, partition, code)
				}
			}
		}
	}

	return initResp.ProducerID, initResp.ProducerEpoch
}

func TestEndTxn_WritesCommitMarkerToEveryPartition(t *testing.T) {
	tc, markers, _ := newTestCoordinator(t)

	producerID, epoch := beginTransaction(t, tc, "txn-1",
		PartitionMetadata{Topic: "orders", Partition: 0},
		PartitionMetadata{Topic: "orders", Partition: 1},
		PartitionMetadata{Topic: "events", Partition: 0},
	)

	resp, err := tc.EndTxn(&EndTxnRequest{
		TransactionID: "txn-1", ProducerID: producerID, ProducerEpoch: epoch, Commit: true,
	})
	if err != nil {
		t.Fatalf("EndTxn failed: %v", err)
	}
	if resp.ErrorCode != ErrorNone {
		t.Fatalf("EndTxn returned %v, want ErrorNone", resp.ErrorCode)
	}

	written := markers.Markers()
	if len(written) != 3 {
		t.Fatalf("wrote %d markers, want 3", len(written))
	}
	for _, m := range written {
		if !m.Marker.Commit {
			t.Errorf("%s-%d got an abort marker, want commit", m.Topic, m.Partition)
		}
		if m.Marker.ProducerID != producerID || m.Marker.ProducerEpoch != epoch {
			t.Errorf("%s-%d marker has producer %d/%d, want %d/%d",
				m.Topic, m.Partition, m.Marker.ProducerID, m.Marker.ProducerEpoch, producerID, epoch)
		}
		if m.Marker.Timestamp == 0 {
			t.Errorf("%s-%d marker has no timestamp", m.Topic, m.Partition)
		}
	}

	state, err := tc.GetTransactionState("txn-1")
	if err != nil {
		t.Fatalf("GetTransactionState failed: %v", err)
	}
	if state != StateCompleteCommit {
		t.Errorf("state = %v, want StateCompleteCommit", state)
	}
}

func TestEndTxn_WritesAbortMarkers(t *testing.T) {
	tc, markers, _ := newTestCoordinator(t)

	producerID, epoch := beginTransaction(t, tc, "txn-1",
		PartitionMetadata{Topic: "orders", Partition: 0})

	resp, err := tc.EndTxn(&EndTxnRequest{
		TransactionID: "txn-1", ProducerID: producerID, ProducerEpoch: epoch, Commit: false,
	})
	if err != nil {
		t.Fatalf("EndTxn failed: %v", err)
	}
	if resp.ErrorCode != ErrorNone {
		t.Fatalf("EndTxn returned %v, want ErrorNone", resp.ErrorCode)
	}

	written := markers.Markers()
	if len(written) != 1 {
		t.Fatalf("wrote %d markers, want 1", len(written))
	}
	if written[0].Marker.Commit {
		t.Error("got a commit marker for an aborted transaction")
	}
}

func TestEndTxn_MarkerWriteFailureDoesNotReportCommit(t *testing.T) {
	writer := &failingMarkerWriter{failTopic: "events", failPartition: 0}

	tc := NewTransactionCoordinator(NewMemoryTransactionLog(), DefaultCoordinatorConfig(), testLogger())
	tc.SetMarkerWriter(writer)
	defer tc.Stop()

	producerID, epoch := beginTransaction(t, tc, "txn-1",
		PartitionMetadata{Topic: "orders", Partition: 0},
		PartitionMetadata{Topic: "events", Partition: 0},
	)

	resp, err := tc.EndTxn(&EndTxnRequest{
		TransactionID: "txn-1", ProducerID: producerID, ProducerEpoch: epoch, Commit: true,
	})
	if err != nil {
		t.Fatalf("EndTxn failed: %v", err)
	}
	if resp.ErrorCode == ErrorNone {
		t.Fatal("EndTxn reported success despite a marker write failing")
	}

	// The transaction must stay in its prepare state so a retry can finish it.
	state, err := tc.GetTransactionState("txn-1")
	if err != nil {
		t.Fatalf("GetTransactionState failed: %v", err)
	}
	if state != StatePrepareCommit {
		t.Errorf("state = %v, want StatePrepareCommit after a partial marker write", state)
	}
}

func TestEndTxn_WithoutMarkerWriterFails(t *testing.T) {
	tc := NewTransactionCoordinator(NewMemoryTransactionLog(), DefaultCoordinatorConfig(), testLogger())
	defer tc.Stop()

	producerID, epoch := beginTransaction(t, tc, "txn-1",
		PartitionMetadata{Topic: "orders", Partition: 0})

	resp, err := tc.EndTxn(&EndTxnRequest{
		TransactionID: "txn-1", ProducerID: producerID, ProducerEpoch: epoch, Commit: true,
	})
	if err != nil {
		t.Fatalf("EndTxn failed: %v", err)
	}
	if resp.ErrorCode != ErrorTransactionCoordinatorNotAvailable {
		t.Errorf("EndTxn returned %v, want ErrorTransactionCoordinatorNotAvailable", resp.ErrorCode)
	}
}

func TestEndTxn_UnstartedTransactionIsRejected(t *testing.T) {
	tc, markers, _ := newTestCoordinator(t)

	// InitProducerID alone does not begin a transaction; AddPartitionsToTxn
	// does. Ending one that was never begun must be rejected rather than
	// reported as a successful commit.
	producerID, epoch := beginTransaction(t, tc, "txn-1")

	resp, err := tc.EndTxn(&EndTxnRequest{
		TransactionID: "txn-1", ProducerID: producerID, ProducerEpoch: epoch, Commit: true,
	})
	if err != nil {
		t.Fatalf("EndTxn failed: %v", err)
	}
	if resp.ErrorCode != ErrorInvalidTransactionState {
		t.Errorf("EndTxn returned %v, want ErrorInvalidTransactionState", resp.ErrorCode)
	}
	if len(markers.Markers()) != 0 {
		t.Error("markers were written for a transaction that was never begun")
	}
}

func TestTxnOffsetCommit_PublishedOnlyOnCommit(t *testing.T) {
	tc, _, committer := newTestCoordinator(t)

	producerID, epoch := beginTransaction(t, tc, "txn-1",
		PartitionMetadata{Topic: "orders", Partition: 0})

	if _, err := tc.AddOffsetsToTxn(&AddOffsetsToTxnRequest{
		TransactionID: "txn-1", ProducerID: producerID, ProducerEpoch: epoch, GroupID: "analytics",
	}); err != nil {
		t.Fatalf("AddOffsetsToTxn failed: %v", err)
	}

	resp, err := tc.TxnOffsetCommit(&TxnOffsetCommitRequest{
		TransactionID: "txn-1",
		GroupID:       "analytics",
		ProducerID:    producerID,
		ProducerEpoch: epoch,
		Offsets: map[string]map[int32]OffsetMetadata{
			"source": {0: {Offset: 42, Metadata: "cp"}},
		},
	})
	if err != nil {
		t.Fatalf("TxnOffsetCommit failed: %v", err)
	}
	if code := resp.Errors["source"][0]; code != ErrorNone {
		t.Fatalf("TxnOffsetCommit returned %v", code)
	}

	// Offsets must not be visible before the transaction resolves.
	if len(committer.Commits()) != 0 {
		t.Fatal("offsets were published before the transaction committed")
	}

	if _, err := tc.EndTxn(&EndTxnRequest{
		TransactionID: "txn-1", ProducerID: producerID, ProducerEpoch: epoch, Commit: true,
	}); err != nil {
		t.Fatalf("EndTxn failed: %v", err)
	}

	commits := committer.Commits()
	if len(commits) != 1 {
		t.Fatalf("published %d offset sets, want 1", len(commits))
	}
	if commits[0].GroupID != "analytics" {
		t.Errorf("published to group %q, want analytics", commits[0].GroupID)
	}
	if got := commits[0].Offsets["source"][0].Offset; got != 42 {
		t.Errorf("published offset %d, want 42", got)
	}
}

func TestTxnOffsetCommit_DiscardedOnAbort(t *testing.T) {
	tc, _, committer := newTestCoordinator(t)

	producerID, epoch := beginTransaction(t, tc, "txn-1",
		PartitionMetadata{Topic: "orders", Partition: 0})

	if _, err := tc.AddOffsetsToTxn(&AddOffsetsToTxnRequest{
		TransactionID: "txn-1", ProducerID: producerID, ProducerEpoch: epoch, GroupID: "analytics",
	}); err != nil {
		t.Fatalf("AddOffsetsToTxn failed: %v", err)
	}
	if _, err := tc.TxnOffsetCommit(&TxnOffsetCommitRequest{
		TransactionID: "txn-1",
		GroupID:       "analytics",
		ProducerID:    producerID,
		ProducerEpoch: epoch,
		Offsets:       map[string]map[int32]OffsetMetadata{"source": {0: {Offset: 42}}},
	}); err != nil {
		t.Fatalf("TxnOffsetCommit failed: %v", err)
	}

	if _, err := tc.EndTxn(&EndTxnRequest{
		TransactionID: "txn-1", ProducerID: producerID, ProducerEpoch: epoch, Commit: false,
	}); err != nil {
		t.Fatalf("EndTxn failed: %v", err)
	}

	if commits := committer.Commits(); len(commits) != 0 {
		t.Errorf("aborting published %d offset sets, want 0", len(commits))
	}
}

func TestTxnOffsetCommit_RequiresAddOffsetsToTxn(t *testing.T) {
	tc, _, _ := newTestCoordinator(t)

	producerID, epoch := beginTransaction(t, tc, "txn-1",
		PartitionMetadata{Topic: "orders", Partition: 0})

	// No AddOffsetsToTxn: the transaction never declared a group.
	resp, err := tc.TxnOffsetCommit(&TxnOffsetCommitRequest{
		TransactionID: "txn-1",
		GroupID:       "analytics",
		ProducerID:    producerID,
		ProducerEpoch: epoch,
		Offsets:       map[string]map[int32]OffsetMetadata{"source": {0: {Offset: 42}}},
	})
	if err != nil {
		t.Fatalf("TxnOffsetCommit failed: %v", err)
	}
	if code := resp.Errors["source"][0]; code != ErrorInvalidTransactionState {
		t.Errorf("returned %v, want ErrorInvalidTransactionState", code)
	}
}

func TestTxnOffsetCommit_RejectsDifferentGroup(t *testing.T) {
	tc, _, _ := newTestCoordinator(t)

	producerID, epoch := beginTransaction(t, tc, "txn-1",
		PartitionMetadata{Topic: "orders", Partition: 0})

	if _, err := tc.AddOffsetsToTxn(&AddOffsetsToTxnRequest{
		TransactionID: "txn-1", ProducerID: producerID, ProducerEpoch: epoch, GroupID: "analytics",
	}); err != nil {
		t.Fatalf("AddOffsetsToTxn failed: %v", err)
	}

	resp, err := tc.TxnOffsetCommit(&TxnOffsetCommitRequest{
		TransactionID: "txn-1",
		GroupID:       "somebody-elses-group",
		ProducerID:    producerID,
		ProducerEpoch: epoch,
		Offsets:       map[string]map[int32]OffsetMetadata{"source": {0: {Offset: 42}}},
	})
	if err != nil {
		t.Fatalf("TxnOffsetCommit failed: %v", err)
	}
	if code := resp.Errors["source"][0]; code != ErrorInvalidTransactionState {
		t.Errorf("returned %v, want ErrorInvalidTransactionState", code)
	}
}

func TestEndTxn_OffsetPublishFailureDoesNotReportCommit(t *testing.T) {
	tc := NewTransactionCoordinator(NewMemoryTransactionLog(), DefaultCoordinatorConfig(), testLogger())
	tc.SetMarkerWriter(NewMemoryMarkerWriter())
	tc.SetOffsetCommitter(&failingOffsetCommitter{})
	defer tc.Stop()

	producerID, epoch := beginTransaction(t, tc, "txn-1",
		PartitionMetadata{Topic: "orders", Partition: 0})

	if _, err := tc.AddOffsetsToTxn(&AddOffsetsToTxnRequest{
		TransactionID: "txn-1", ProducerID: producerID, ProducerEpoch: epoch, GroupID: "analytics",
	}); err != nil {
		t.Fatalf("AddOffsetsToTxn failed: %v", err)
	}
	if _, err := tc.TxnOffsetCommit(&TxnOffsetCommitRequest{
		TransactionID: "txn-1",
		GroupID:       "analytics",
		ProducerID:    producerID,
		ProducerEpoch: epoch,
		Offsets:       map[string]map[int32]OffsetMetadata{"source": {0: {Offset: 42}}},
	}); err != nil {
		t.Fatalf("TxnOffsetCommit failed: %v", err)
	}

	resp, err := tc.EndTxn(&EndTxnRequest{
		TransactionID: "txn-1", ProducerID: producerID, ProducerEpoch: epoch, Commit: true,
	})
	if err != nil {
		t.Fatalf("EndTxn failed: %v", err)
	}
	if resp.ErrorCode == ErrorNone {
		t.Error("EndTxn reported success despite failing to publish offsets")
	}
}

func TestSortedPartitions(t *testing.T) {
	ordered := sortedPartitions([]PartitionMetadata{
		{Topic: "orders", Partition: 2},
		{Topic: "events", Partition: 1},
		{Topic: "orders", Partition: 0},
		{Topic: "events", Partition: 0},
	})

	want := []PartitionMetadata{
		{Topic: "events", Partition: 0},
		{Topic: "events", Partition: 1},
		{Topic: "orders", Partition: 0},
		{Topic: "orders", Partition: 2},
	}
	for i := range want {
		if ordered[i] != want[i] {
			t.Fatalf("sortedPartitions()[%d] = %+v, want %+v", i, ordered[i], want[i])
		}
	}
}

func TestExpiredTransaction_WritesAbortMarkers(t *testing.T) {
	markers := NewMemoryMarkerWriter()

	config := DefaultCoordinatorConfig()
	config.DefaultTransactionTimeout = 50 * time.Millisecond
	config.MaxTransactionTimeout = time.Second
	config.ExpirationCheckInterval = 20 * time.Millisecond

	tc := NewTransactionCoordinator(NewMemoryTransactionLog(), config, testLogger())
	tc.SetMarkerWriter(markers)
	tc.SetOffsetCommitter(NewMemoryOffsetCommitter())
	defer tc.Stop()

	initResp, err := tc.InitProducerID(&InitProducerIDRequest{
		TransactionID:      "txn-expiring",
		TransactionTimeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("InitProducerID failed: %v", err)
	}

	if _, err := tc.AddPartitionsToTxn(&AddPartitionsToTxnRequest{
		TransactionID: "txn-expiring",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Partitions:    []PartitionMetadata{{Topic: "orders", Partition: 0}},
	}); err != nil {
		t.Fatalf("AddPartitionsToTxn failed: %v", err)
	}

	// A transaction abandoned by its producer must still be resolved on the
	// partition: without an abort marker the partition never learns the
	// transaction ended, which pins its last stable offset forever and
	// stalls every read-committed consumer on it.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(markers.Markers()) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	written := markers.Markers()
	if len(written) != 1 {
		t.Fatalf("expired transaction wrote %d markers, want 1", len(written))
	}
	if written[0].Marker.Commit {
		t.Error("expired transaction wrote a commit marker, want abort")
	}
	if written[0].Topic != "orders" || written[0].Partition != 0 {
		t.Errorf("marker went to %s-%d, want orders-0", written[0].Topic, written[0].Partition)
	}

	state, err := tc.GetTransactionState("txn-expiring")
	if err != nil {
		t.Fatalf("GetTransactionState failed: %v", err)
	}
	if state != StateCompleteAbort {
		t.Errorf("state = %v, want StateCompleteAbort", state)
	}
}

func TestExpiredTransaction_MarkerFailureLeavesItRetryable(t *testing.T) {
	writer := &failingMarkerWriter{failTopic: "orders", failPartition: 0}

	config := DefaultCoordinatorConfig()
	config.DefaultTransactionTimeout = 50 * time.Millisecond
	config.MaxTransactionTimeout = time.Second
	config.ExpirationCheckInterval = 20 * time.Millisecond

	tc := NewTransactionCoordinator(NewMemoryTransactionLog(), config, testLogger())
	tc.SetMarkerWriter(writer)
	defer tc.Stop()

	initResp, err := tc.InitProducerID(&InitProducerIDRequest{
		TransactionID:      "txn-stuck",
		TransactionTimeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("InitProducerID failed: %v", err)
	}
	if _, err := tc.AddPartitionsToTxn(&AddPartitionsToTxnRequest{
		TransactionID: "txn-stuck",
		ProducerID:    initResp.ProducerID,
		ProducerEpoch: initResp.ProducerEpoch,
		Partitions:    []PartitionMetadata{{Topic: "orders", Partition: 0}},
	}); err != nil {
		t.Fatalf("AddPartitionsToTxn failed: %v", err)
	}

	// Give the expiry sweep several chances to run and fail.
	time.Sleep(300 * time.Millisecond)

	// It must not be reported as aborted when no marker was written: that
	// would abandon the partition with nothing left to drive a retry.
	state, err := tc.GetTransactionState("txn-stuck")
	if err != nil {
		t.Fatalf("GetTransactionState failed: %v", err)
	}
	if state == StateCompleteAbort {
		t.Error("transaction reported CompleteAbort despite its abort marker failing to write")
	}
}
