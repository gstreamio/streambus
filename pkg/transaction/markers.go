package transaction

import (
	"fmt"
	"sort"
	"sync"
)

// MarkerWriter writes transaction control records to partitions.
//
// EndTxn calls this for every partition a transaction touched. An
// implementation must not return until the marker is durable on that
// partition: the coordinator reports a transaction as committed only once
// every marker has been written, so a marker that is merely buffered would
// turn "committed" back into the empty promise this interface exists to
// prevent.
type MarkerWriter interface {
	// WriteMarker appends a transaction marker to a partition and returns
	// once the write is durable.
	WriteMarker(topic string, partition int32, marker *TransactionMarker) error
}

// OffsetCommitter materializes offsets that were committed inside a
// transaction.
//
// Offsets sent through TxnOffsetCommit are held by the coordinator until the
// transaction outcome is known: committing publishes them to the consumer
// group, aborting discards them. That is what makes a read-process-write loop
// atomic across the produced records and the consumed positions.
type OffsetCommitter interface {
	// CommitOffsets publishes a transaction's offsets to a consumer group.
	CommitOffsets(groupID string, offsets map[string]map[int32]OffsetMetadata) error
}

// SetMarkerWriter installs the writer used to persist transaction markers.
//
// A coordinator with no marker writer cannot honour a commit, and EndTxn
// fails with ErrorTransactionCoordinatorNotAvailable rather than reporting a
// commit it did not perform.
func (tc *TransactionCoordinator) SetMarkerWriter(writer MarkerWriter) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.markerWriter = writer
}

// SetOffsetCommitter installs the committer used to publish transactional
// offsets. A coordinator without one rejects TxnOffsetCommit, rather than
// accepting offsets it could never publish.
func (tc *TransactionCoordinator) SetOffsetCommitter(committer OffsetCommitter) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.offsetCommitter = committer
}

// writeMarkers writes a commit or abort marker to every partition in the
// transaction, in a deterministic order.
//
// It stops at the first failure and reports which partition failed. The
// transaction is left in its prepare state so a retry can finish it: reporting
// success after a partial write would tell the producer its records are
// committed on partitions that never received a marker.
//
// Calling it again for the same transaction - as EndTxn does on a retry, and
// the expiry sweep does for a stuck prepare - is safe. It always starts from
// the first partition in sorted order, so an already-marked partition gets a
// second, benign marker record: consumers already filter markers out of a
// fetch response by their headers, and the partition-side bookkeeping
// (Partition.EndTransaction / AbortTransaction) is itself idempotent, since
// it keys off whether the partition still has that producer epoch's
// transaction open rather than off the call count. A partition that never
// got its first marker still has the transaction open, so a retry records it
// exactly as the first attempt would have.
func (tc *TransactionCoordinator) writeMarkers(txn *TransactionMetadata, commit bool) error {
	if len(txn.Partitions) == 0 {
		return nil
	}

	if tc.markerWriter == nil {
		return fmt.Errorf("no marker writer configured: cannot durably end transaction %s", txn.TransactionID)
	}

	marker := &TransactionMarker{
		ProducerID:    txn.ProducerID,
		ProducerEpoch: txn.ProducerEpoch,
		Commit:        commit,
		Timestamp:     tc.now().UnixNano(),
	}

	for _, partition := range sortedPartitions(txn.Partitions) {
		if err := tc.markerWriter.WriteMarker(partition.Topic, partition.Partition, marker); err != nil {
			return fmt.Errorf("writing transaction marker to %s-%d: %w",
				partition.Topic, partition.Partition, err)
		}
	}

	return nil
}

// sortedPartitions orders partitions by topic then partition ID, so markers
// are always written in the same order and a failure is reproducible.
func sortedPartitions(partitions []PartitionMetadata) []PartitionMetadata {
	ordered := make([]PartitionMetadata, len(partitions))
	copy(ordered, partitions)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Topic != ordered[j].Topic {
			return ordered[i].Topic < ordered[j].Topic
		}
		return ordered[i].Partition < ordered[j].Partition
	})
	return ordered
}

// MemoryMarkerWriter records transaction markers in memory.
//
// It is intended for tests and single-process development, and mirrors
// NewMemoryTransactionLog. It provides no durability across restarts, so a
// production coordinator should be given a writer backed by real partition
// logs.
type MemoryMarkerWriter struct {
	mu      sync.Mutex
	markers []RecordedMarker
}

// RecordedMarker is one marker written to a partition.
type RecordedMarker struct {
	Topic     string
	Partition int32
	Marker    TransactionMarker
}

// NewMemoryMarkerWriter creates an in-memory marker writer.
func NewMemoryMarkerWriter() *MemoryMarkerWriter {
	return &MemoryMarkerWriter{}
}

// WriteMarker records a marker.
func (w *MemoryMarkerWriter) WriteMarker(topic string, partition int32, marker *TransactionMarker) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.markers = append(w.markers, RecordedMarker{
		Topic:     topic,
		Partition: partition,
		Marker:    *marker,
	})
	return nil
}

// Markers returns a copy of every marker written so far.
func (w *MemoryMarkerWriter) Markers() []RecordedMarker {
	w.mu.Lock()
	defer w.mu.Unlock()

	out := make([]RecordedMarker, len(w.markers))
	copy(out, w.markers)
	return out
}

// MemoryOffsetCommitter records published transactional offsets in memory,
// for tests and single-process development.
type MemoryOffsetCommitter struct {
	mu      sync.Mutex
	commits []RecordedOffsetCommit
}

// RecordedOffsetCommit is one published set of transactional offsets.
type RecordedOffsetCommit struct {
	GroupID string
	Offsets map[string]map[int32]OffsetMetadata
}

// NewMemoryOffsetCommitter creates an in-memory offset committer.
func NewMemoryOffsetCommitter() *MemoryOffsetCommitter {
	return &MemoryOffsetCommitter{}
}

// CommitOffsets records a published offset set.
func (c *MemoryOffsetCommitter) CommitOffsets(groupID string, offsets map[string]map[int32]OffsetMetadata) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	copied := make(map[string]map[int32]OffsetMetadata, len(offsets))
	for topic, byPartition := range offsets {
		partitions := make(map[int32]OffsetMetadata, len(byPartition))
		for partition, offset := range byPartition {
			partitions[partition] = offset
		}
		copied[topic] = partitions
	}

	c.commits = append(c.commits, RecordedOffsetCommit{GroupID: groupID, Offsets: copied})
	return nil
}

// Commits returns a copy of every published offset set.
func (c *MemoryOffsetCommitter) Commits() []RecordedOffsetCommit {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]RecordedOffsetCommit, len(c.commits))
	copy(out, c.commits)
	return out
}
