package server

import (
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/storage"
)

// appendTransactionalMessage writes one record stamped with a producer
// identity, mirroring what a real transactional produce would persist
// through handleProduce, and returns the offset it landed at.
func appendTransactionalMessage(t *testing.T, partition *Partition, producerID int64, producerEpoch int16) int64 {
	t.Helper()

	offsets, err := partition.Log().Append(&storage.MessageBatch{
		Messages:      []storage.Message{{Value: []byte("v")}},
		Timestamp:     time.Now(),
		ProducerID:    producerID,
		ProducerEpoch: producerEpoch,
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	return int64(offsets[0])
}

// appendPlainMessage writes a single ordinary (non-transactional) record to
// a partition's log, purely to give tests something to measure a high water
// mark against.
func appendPlainMessage(t *testing.T, partition *Partition) {
	t.Helper()

	_, err := partition.Log().Append(&storage.MessageBatch{
		Messages:  []storage.Message{{Value: []byte("v")}},
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
}

// appendPlainMessages writes n ordinary records, so a test can pick
// transaction start offsets that are actually within the log: in reality
// BeginTransaction is only ever called with an offset a batch was just
// assigned, which is by definition <= the high water mark once appended.
func appendPlainMessages(t *testing.T, partition *Partition, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		appendPlainMessage(t, partition)
	}
}

// newTestPartition creates a topic with a single partition for exercising
// Partition's open-transaction bookkeeping directly, without going through
// the wire protocol.
func newTestPartition(t *testing.T) (*TopicManager, *Partition) {
	t.Helper()

	tm := NewTopicManager(t.TempDir())
	t.Cleanup(func() { _ = tm.Close() })

	if err := tm.CreateTopic("txn-topic", 1); err != nil {
		t.Fatalf("CreateTopic failed: %v", err)
	}

	partition, err := tm.GetPartition("txn-topic", 0)
	if err != nil {
		t.Fatalf("GetPartition failed: %v", err)
	}

	return tm, partition
}

func TestPartition_LastStableOffset_NoOpenTransactions(t *testing.T) {
	_, partition := newTestPartition(t)

	appendPlainMessage(t, partition)

	lso := partition.LastStableOffset()
	hwm := int64(partition.Log().HighWaterMark())
	if lso != hwm {
		t.Errorf("LastStableOffset() = %d, want high water mark %d with no open transactions", lso, hwm)
	}
}

func TestPartition_BeginTransaction_TracksFirstOffsetOnly(t *testing.T) {
	_, partition := newTestPartition(t)
	appendPlainMessages(t, partition, 10)

	partition.BeginTransaction(1000, 0, 5)
	// A later call for the same producer epoch must not move the barrier:
	// only the first record of a transaction defines where it started.
	partition.BeginTransaction(1000, 0, 9)

	if got := partition.LastStableOffset(); got != 5 {
		t.Errorf("LastStableOffset() = %d, want 5 (first BeginTransaction call wins)", got)
	}
}

func TestPartition_BeginTransaction_IgnoresProducerIDZero(t *testing.T) {
	_, partition := newTestPartition(t)

	appendPlainMessage(t, partition)

	// producerID 0 is the sentinel for "not transactional"; it must never
	// become a barrier, or every ordinary produce would block read-committed
	// fetches forever.
	partition.BeginTransaction(0, 0, 0)

	hwm := int64(partition.Log().HighWaterMark())
	if got := partition.LastStableOffset(); got != hwm {
		t.Errorf("LastStableOffset() = %d, want high water mark %d; producerID 0 must be ignored", got, hwm)
	}
}

func TestPartition_EndTransaction_ClearsTrackedOffset(t *testing.T) {
	_, partition := newTestPartition(t)
	appendPlainMessages(t, partition, 5)

	partition.BeginTransaction(1000, 0, 3)
	if got := partition.LastStableOffset(); got != 3 {
		t.Fatalf("LastStableOffset() = %d, want 3 before EndTransaction", got)
	}

	partition.EndTransaction(1000, 0)

	hwm := int64(partition.Log().HighWaterMark())
	if got := partition.LastStableOffset(); got != hwm {
		t.Errorf("LastStableOffset() = %d, want high water mark %d after EndTransaction", got, hwm)
	}
}

func TestPartition_EndTransaction_NoOpWhenNothingTracked(t *testing.T) {
	_, partition := newTestPartition(t)

	// No transaction was ever begun for this producer epoch (or any other):
	// clearing it must be a harmless no-op, matching the marker path for a
	// partition a transaction never actually wrote to.
	partition.EndTransaction(4242, 0)

	hwm := int64(partition.Log().HighWaterMark())
	if got := partition.LastStableOffset(); got != hwm {
		t.Errorf("LastStableOffset() = %d, want high water mark %d", got, hwm)
	}
}

func TestPartition_LastStableOffset_MultipleOpenTransactions_ReturnsEarliest(t *testing.T) {
	_, partition := newTestPartition(t)
	appendPlainMessages(t, partition, 10)

	partition.BeginTransaction(1000, 0, 5)
	partition.BeginTransaction(2000, 0, 2)
	partition.BeginTransaction(3000, 0, 8)

	if got := partition.LastStableOffset(); got != 2 {
		t.Errorf("LastStableOffset() = %d, want 2 (earliest of 5, 2, 8)", got)
	}

	// Resolving the earliest one should advance the barrier to the next
	// earliest still-open transaction, not all the way to the high water
	// mark.
	partition.EndTransaction(2000, 0)
	if got := partition.LastStableOffset(); got != 5 {
		t.Errorf("LastStableOffset() = %d, want 5 after resolving the earliest transaction", got)
	}
}

func TestPartition_EndTransaction_DifferentEpochLeavesEntryTracked(t *testing.T) {
	_, partition := newTestPartition(t)
	appendPlainMessages(t, partition, 10)

	// A stale marker for an old epoch (e.g. a retried EndTxn after the
	// producer was already fenced and reclaimed a new epoch) must not clear
	// the current epoch's open transaction.
	partition.BeginTransaction(1000, 1, 5)
	partition.EndTransaction(1000, 0)

	if got := partition.LastStableOffset(); got != 5 {
		t.Errorf("LastStableOffset() = %d, want 5; clearing the wrong epoch must not affect the tracked one", got)
	}
}

func TestPartition_AbortTransaction_HidesOnlyThatProducersRecordsInRange(t *testing.T) {
	_, partition := newTestPartition(t)

	start := appendTransactionalMessage(t, partition, 1000, 0) // producer A's only record
	partition.BeginTransaction(1000, 0, start)

	markerOffset := appendTransactionalMessage(t, partition, 1000, 0) // A's abort marker occupies an offset too
	partition.AbortTransaction(1000, 0, markerOffset)

	if !partition.IsAborted(1000, 0, start) {
		t.Errorf("IsAborted(1000, 0, %d) = false, want true (inside the aborted range)", start)
	}
	if partition.IsAborted(1000, 0, markerOffset) {
		t.Errorf("IsAborted(1000, 0, %d) = true, want false; the range's end (the marker's own offset) is exclusive", markerOffset)
	}
	// A different producer's record at the same offset must never be hidden
	// by another producer's aborted range - only that producer's own records
	// belong to its transaction.
	if partition.IsAborted(2000, 0, start) {
		t.Errorf("IsAborted(2000, 0, %d) = true, want false; a different producer must not be affected", start)
	}
}

func TestPartition_AbortTransaction_NoOpWhenNothingWasOpen(t *testing.T) {
	_, partition := newTestPartition(t)

	// Aborting a producer epoch with no BeginTransaction call (e.g. a marker
	// for a transaction that produced no records on this partition) must not
	// fabricate a range to hide.
	partition.AbortTransaction(9999, 0, 100)

	if partition.IsAborted(9999, 0, 0) {
		t.Error("IsAborted = true after aborting a producer epoch that was never open")
	}
}

func TestPartition_AbortTransaction_ResolvesLastStableOffsetLikeEndTransaction(t *testing.T) {
	_, partition := newTestPartition(t)
	appendPlainMessages(t, partition, 5)

	partition.BeginTransaction(1000, 0, 3)
	if got := partition.LastStableOffset(); got != 3 {
		t.Fatalf("LastStableOffset() = %d, want 3 before AbortTransaction", got)
	}

	partition.AbortTransaction(1000, 0, 5)

	hwm := int64(partition.Log().HighWaterMark())
	if got := partition.LastStableOffset(); got != hwm {
		t.Errorf("LastStableOffset() = %d, want high water mark %d after AbortTransaction", got, hwm)
	}
}

// TestPartition_IsAborted_ProducerIDZeroNeverMatches guards the
// non-transactional sentinel: even if an aborted range's zero-valued fields
// happened to line up, an ordinary record must never be reported aborted.
func TestPartition_IsAborted_ProducerIDZeroNeverMatches(t *testing.T) {
	_, partition := newTestPartition(t)

	partition.BeginTransaction(0, 0, 0) // no-op: producer ID 0 is never tracked
	partition.AbortTransaction(0, 0, 1)

	if partition.IsAborted(0, 0, 0) {
		t.Error("IsAborted(0, 0, 0) = true, want false; producer ID 0 must never be treated as aborted")
	}
}

// TestPartition_AbortTransaction_PrunedOnceRetentionPassesTheRange verifies
// the bound on abortedTxns' memory: once the log's start offset has advanced
// past an aborted range's end, retention has already deleted every record
// that range could hide, so the range itself must be forgotten rather than
// accumulating for the life of the partition.
func TestPartition_AbortTransaction_PrunedOnceRetentionPassesTheRange(t *testing.T) {
	_, partition := newTestPartition(t)

	start := appendTransactionalMessage(t, partition, 1000, 0)
	partition.BeginTransaction(1000, 0, start)
	markerOffset := appendTransactionalMessage(t, partition, 1000, 0)
	partition.AbortTransaction(1000, 0, markerOffset)

	if !partition.IsAborted(1000, 0, start) {
		t.Fatalf("IsAborted(1000, 0, %d) = false, want true before retention advances", start)
	}

	// Simulate retention deleting everything up to and including the
	// aborted range: the range's end is exclusive (the marker's offset), so
	// deleting up to markerOffset+1 has removed every record it covered.
	if err := partition.Log().Delete(storage.Offset(markerOffset + 1)); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// LastStableOffset is what actually prunes (see its doc comment); call
	// it to trigger the prune rather than relying on AbortTransaction, since
	// no further abort happens here.
	partition.LastStableOffset()

	if partition.IsAborted(1000, 0, start) {
		t.Errorf("IsAborted(1000, 0, %d) = true, want false; the range should have been pruned once retention passed it", start)
	}
}
