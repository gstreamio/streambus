package server

import (
	"bytes"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/storage"
)

// reopenTopicManager simulates a broker restart: it closes tm (as Close
// would be called during shutdown) and returns a fresh TopicManager pointed
// at the same data directory, which replays each partition's log the same
// way loadExistingTopics does on a real process start.
func reopenTopicManager(t *testing.T, tm *TopicManager, dataDir string) *TopicManager {
	t.Helper()
	if err := tm.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	tm2 := NewTopicManager(dataDir)
	t.Cleanup(func() { _ = tm2.Close() })
	return tm2
}

// appendMarker writes a transaction-marker record for (producerID,
// producerEpoch, commit) and returns the offset it landed at, mirroring what
// pkg/broker's logMarkerWriter persists.
func appendMarker(t *testing.T, partition *Partition, producerID int64, producerEpoch int16, commit bool) int64 {
	t.Helper()

	offsets, err := partition.Log().Append(&storage.MessageBatch{
		Messages: []storage.Message{{
			Headers:   protocol.TransactionMarkerHeaders(producerID, producerEpoch, commit),
			Timestamp: time.Now(),
		}},
		Timestamp:     time.Now(),
		ProducerID:    producerID,
		ProducerEpoch: producerEpoch,
	})
	if err != nil {
		t.Fatalf("appending marker failed: %v", err)
	}
	return int64(offsets[0])
}

// TestRebuildTransactionState_AbortedTransactionStaysHiddenAcrossRestart is
// the regression test for the restart correctness gap the team lead called
// out: without replaying the log on recovery, abortedTxns starts empty after
// a restart and a previously-hidden aborted transaction's records would
// become visible again under read-committed. That must not happen.
func TestRebuildTransactionState_AbortedTransactionStaysHiddenAcrossRestart(t *testing.T) {
	dataDir := t.TempDir()
	tm := NewTopicManager(dataDir)
	if err := tm.CreateTopic("orders", 1); err != nil {
		t.Fatalf("CreateTopic failed: %v", err)
	}
	partition, err := tm.GetPartition("orders", 0)
	if err != nil {
		t.Fatalf("GetPartition failed: %v", err)
	}

	start := appendTransactionalMessage(t, partition, 1000, 0)
	partition.BeginTransaction(1000, 0, start)
	markerOffset := appendMarker(t, partition, 1000, 0, false)
	partition.AbortTransaction(1000, 0, markerOffset)

	if !partition.IsAborted(1000, 0, start) {
		t.Fatalf("IsAborted(1000, 0, %d) = false before restart, want true", start)
	}

	tm2 := reopenTopicManager(t, tm, dataDir)
	partition2, err := tm2.GetPartition("orders", 0)
	if err != nil {
		t.Fatalf("GetPartition after restart failed: %v", err)
	}

	if !partition2.IsAborted(1000, 0, start) {
		t.Errorf("IsAborted(1000, 0, %d) = false after restart, want true; the abort must survive recovery", start)
	}

	hwm := int64(partition2.Log().HighWaterMark())
	if got := partition2.LastStableOffset(); got != hwm {
		t.Errorf("LastStableOffset() = %d after restart, want high water mark %d; the resolved transaction must not block reads", got, hwm)
	}
}

// TestRebuildTransactionState_StillOpenTransactionStaysOpenAcrossRestart
// covers the companion gap: a transaction that never received a marker
// before the restart is still genuinely open, and recovery must keep
// blocking read-committed fetches at its start offset - not silently drop
// it just because openTxns lives only in memory.
func TestRebuildTransactionState_StillOpenTransactionStaysOpenAcrossRestart(t *testing.T) {
	dataDir := t.TempDir()
	tm := NewTopicManager(dataDir)
	if err := tm.CreateTopic("orders", 1); err != nil {
		t.Fatalf("CreateTopic failed: %v", err)
	}
	partition, err := tm.GetPartition("orders", 0)
	if err != nil {
		t.Fatalf("GetPartition failed: %v", err)
	}

	start := appendTransactionalMessage(t, partition, 1000, 0)
	partition.BeginTransaction(1000, 0, start)
	appendPlainMessage(t, partition) // an ordinary record after the open transaction

	tm2 := reopenTopicManager(t, tm, dataDir)
	partition2, err := tm2.GetPartition("orders", 0)
	if err != nil {
		t.Fatalf("GetPartition after restart failed: %v", err)
	}

	if got := partition2.LastStableOffset(); got != start {
		t.Errorf("LastStableOffset() = %d after restart, want %d; the still-open transaction must keep blocking reads", got, start)
	}
}

// TestRebuildTransactionState_CommittedTransactionNotTreatedAsAborted
// verifies recovery tells commit and abort apart: a committed transaction's
// records must remain visible after a restart, not be mistaken for an
// aborted one just because a marker record is present.
func TestRebuildTransactionState_CommittedTransactionNotTreatedAsAborted(t *testing.T) {
	dataDir := t.TempDir()
	tm := NewTopicManager(dataDir)
	if err := tm.CreateTopic("orders", 1); err != nil {
		t.Fatalf("CreateTopic failed: %v", err)
	}
	partition, err := tm.GetPartition("orders", 0)
	if err != nil {
		t.Fatalf("GetPartition failed: %v", err)
	}

	start := appendTransactionalMessage(t, partition, 1000, 0)
	partition.BeginTransaction(1000, 0, start)
	markerOffset := appendMarker(t, partition, 1000, 0, true)
	partition.EndTransaction(1000, 0)

	tm2 := reopenTopicManager(t, tm, dataDir)
	partition2, err := tm2.GetPartition("orders", 0)
	if err != nil {
		t.Fatalf("GetPartition after restart failed: %v", err)
	}

	if partition2.IsAborted(1000, 0, start) {
		t.Errorf("IsAborted(1000, 0, %d) = true after restart, want false; the transaction committed", start)
	}
	hwm := int64(partition2.Log().HighWaterMark())
	if got := partition2.LastStableOffset(); got != hwm {
		t.Errorf("LastStableOffset() = %d after restart, want high water mark %d", got, hwm)
	}
	if markerOffset >= hwm {
		t.Fatalf("test setup: marker offset %d not below high water mark %d", markerOffset, hwm)
	}
}

// TestRebuildTransactionState_EmptyLogIsANoOp guards the fast path taken by
// every partition with nothing retained yet - creating a brand-new topic
// must not fail or panic just because there is no log history to replay.
func TestRebuildTransactionState_EmptyLogIsANoOp(t *testing.T) {
	dataDir := t.TempDir()
	tm := NewTopicManager(dataDir)
	if err := tm.CreateTopic("orders", 1); err != nil {
		t.Fatalf("CreateTopic failed: %v", err)
	}
	partition, err := tm.GetPartition("orders", 0)
	if err != nil {
		t.Fatalf("GetPartition failed: %v", err)
	}

	if got := partition.LastStableOffset(); got != 0 {
		t.Errorf("LastStableOffset() = %d for an empty partition, want 0", got)
	}
}

// boundedReadSpyLog wraps a real storage.Log and asserts rebuildTransactionState
// reads it through bounded Read windows rather than materializing the whole
// retained range at once - the specific mistake (a single ReadRange over
// [StartOffset, HighWaterMark)) that turns replaying a heavily-retained
// partition's transaction state into an OOM at broker startup. Embedding
// storage.Log promotes every other method unchanged; only Read and ReadRange
// are overridden.
type boundedReadSpyLog struct {
	storage.Log
	t         *testing.T
	maxWindow int
	readCalls int
}

func (s *boundedReadSpyLog) Read(offset storage.Offset, maxBytes int) ([]*storage.Message, error) {
	s.readCalls++
	if maxBytes > s.maxWindow {
		s.t.Errorf("Read maxBytes = %d, want <= %d; rebuildTransactionState must bound its replay window", maxBytes, s.maxWindow)
	}
	return s.Log.Read(offset, maxBytes)
}

func (s *boundedReadSpyLog) ReadRange(start, end storage.Offset) ([]*storage.Message, error) {
	s.t.Fatalf("ReadRange(%d, %d) called; rebuildTransactionState must replay in bounded windows via Read, never materialize the whole retained range at once", start, end)
	return nil, nil
}

// TestRebuildTransactionState_ReplaysInBoundedWindows is the regression test
// for the OOM this rebuild used to risk at startup: a single ReadRange over
// a partition's entire retained log materializes every key and value in it,
// even though replay only needs each record's offset, headers and producer
// identity. It pins two properties together deliberately, because the
// tempting fix for one breaks the other: peak memory must be bounded by the
// window (never by how much the partition retains), and an abort recorded
// near the very beginning of the log must still be found - a "replay only
// the last N offsets" fix would pass the first property and silently fail
// the second by un-hiding exactly that abort.
func TestRebuildTransactionState_ReplaysInBoundedWindows(t *testing.T) {
	dataDir := t.TempDir()
	tm := NewTopicManager(dataDir)
	if err := tm.CreateTopic("orders", 1); err != nil {
		t.Fatalf("CreateTopic failed: %v", err)
	}
	partition, err := tm.GetPartition("orders", 0)
	if err != nil {
		t.Fatalf("GetPartition failed: %v", err)
	}

	// The transaction to hide sits at the very start of the log.
	start := appendTransactionalMessage(t, partition, 1000, 0)
	partition.BeginTransaction(1000, 0, start)
	markerOffset := appendMarker(t, partition, 1000, 0, false)
	partition.AbortTransaction(1000, 0, markerOffset)

	// Pad the log with enough large records afterward that its total size
	// is several times the replay window - large enough that a single
	// ReadRange materializing all of it is clearly distinguishable from any
	// one bounded Read window, and that replay must cross several window
	// boundaries to reach the end of the log at all.
	const padRecords = 20
	padValue := bytes.Repeat([]byte("x"), rebuildReplayWindowBytes/4)
	for i := 0; i < padRecords; i++ {
		if _, err := partition.Log().Append(&storage.MessageBatch{
			Messages:  []storage.Message{{Value: padValue}},
			Timestamp: time.Now(),
		}); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	spy := &boundedReadSpyLog{Log: partition.Log(), t: t, maxWindow: rebuildReplayWindowBytes}
	replayPartition := &Partition{id: partition.ID(), log: spy}

	if err := replayPartition.rebuildTransactionState(); err != nil {
		t.Fatalf("rebuildTransactionState failed: %v", err)
	}

	if spy.readCalls < 2 {
		t.Errorf("Read called %d times, want at least 2; this much padding should force replay across a window boundary", spy.readCalls)
	}
	if !replayPartition.IsAborted(1000, 0, start) {
		t.Errorf("IsAborted(1000, 0, %d) = false, want true; an abort near the start of the log must survive a windowed replay", start)
	}
}
