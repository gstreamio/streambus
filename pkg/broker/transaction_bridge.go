package broker

import (
	"fmt"
	"time"

	"github.com/gstreamio/streambus/pkg/consumer/group"
	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/server"
	"github.com/gstreamio/streambus/pkg/storage"
	"github.com/gstreamio/streambus/pkg/transaction"
)

// Header keys identifying a transaction control record in a partition log.
//
// A marker is stored as a normal log record carrying these headers, so it
// occupies an offset in the partition and is recovered with the rest of the
// log. Consumers reading with read-committed semantics use them to tell a
// control record apart from user data.
//
// These alias the definitions in pkg/protocol rather than restating them:
// the fetch path (pkg/server) has to recognize the same headers to hide
// markers from consumers, and protocol is the one package both sides
// already depend on.
const (
	ControlHeaderKey          = protocol.ControlHeaderKey
	ControlTypeTxnMarker      = protocol.ControlTypeTxnMarker
	TxnCommitHeaderKey        = protocol.TxnCommitHeaderKey
	TxnProducerIDHeaderKey    = protocol.TxnProducerIDHeaderKey
	TxnProducerEpochHeaderKey = protocol.TxnProducerEpochHeaderKey
)

// logMarkerWriter writes transaction markers into real partition logs.
//
// It satisfies transaction.MarkerWriter, and flushes after each append so the
// coordinator's promise - that a committed transaction's markers are durable
// before it reports success - is one the storage layer has actually kept.
type logMarkerWriter struct {
	topicManager *server.TopicManager
}

// newLogMarkerWriter creates a marker writer over a topic manager.
func newLogMarkerWriter(topicManager *server.TopicManager) *logMarkerWriter {
	return &logMarkerWriter{topicManager: topicManager}
}

// WriteMarker appends a transaction marker to a partition and flushes it.
func (w *logMarkerWriter) WriteMarker(topic string, partitionID int32, marker *transaction.TransactionMarker) error {
	if w.topicManager == nil {
		return fmt.Errorf("storage is not initialized")
	}
	if partitionID < 0 {
		return fmt.Errorf("invalid partition %d for topic %s", partitionID, topic)
	}

	//nolint:gosec // partitionID is checked non-negative above
	partition, err := w.topicManager.GetPartition(topic, uint32(partitionID))
	if err != nil {
		return fmt.Errorf("locating partition for marker: %w", err)
	}

	timestamp := time.Unix(0, marker.Timestamp)
	if marker.Timestamp == 0 {
		timestamp = time.Now()
	}

	record := storage.Message{
		Value:     nil,
		Timestamp: timestamp,
		Headers: protocol.TransactionMarkerHeaders(
			int64(marker.ProducerID), int16(marker.ProducerEpoch), marker.Commit),
	}

	log := partition.Log()
	offsets, err := log.Append(&storage.MessageBatch{
		Messages:      []storage.Message{record},
		Timestamp:     timestamp,
		ProducerID:    int64(marker.ProducerID),
		ProducerEpoch: int16(marker.ProducerEpoch),
	})
	if err != nil {
		return fmt.Errorf("appending marker: %w", err)
	}

	// Flush before returning: the coordinator treats a successful write as a
	// durability guarantee, so a buffered marker would make "committed" mean
	// less than it claims.
	if err := log.Flush(); err != nil {
		return fmt.Errorf("flushing marker: %w", err)
	}

	// The transaction is resolved now - committed or aborted, it no longer
	// blocks read-committed fetches. Clearing this after the marker is
	// durable (not before) means a fetch can never observe the barrier
	// lifted for a transaction whose marker isn't actually on the log yet.
	//
	// An abort additionally hands the marker's own offset to
	// AbortTransaction, which remembers [start, this offset) as the range a
	// read-committed fetch must keep hiding even after the barrier lifts - a
	// commit has nothing to hide, so EndTransaction alone is enough for it.
	if marker.Commit {
		partition.EndTransaction(int64(marker.ProducerID), int16(marker.ProducerEpoch))
	} else {
		partition.AbortTransaction(int64(marker.ProducerID), int16(marker.ProducerEpoch), int64(offsets[0]))
	}

	return nil
}

// groupOffsetCommitter publishes transactional offsets to the consumer group
// coordinator once a transaction commits. It satisfies
// transaction.OffsetCommitter.
type groupOffsetCommitter struct {
	coordinator *group.GroupCoordinator
}

// newGroupOffsetCommitter creates an offset committer over a group coordinator.
func newGroupOffsetCommitter(coordinator *group.GroupCoordinator) *groupOffsetCommitter {
	return &groupOffsetCommitter{coordinator: coordinator}
}

// CommitOffsets publishes a transaction's offsets to the group.
//
// The commit is made outside any generation (GenerationID -1) because the
// producer, not a group member, is committing on the group's behalf: the
// consumer that read the records may have been rebalanced away by now, and
// the transaction's outcome must not depend on that.
func (c *groupOffsetCommitter) CommitOffsets(groupID string, offsets map[string]map[int32]transaction.OffsetMetadata) error {
	if c.coordinator == nil {
		return fmt.Errorf("consumer group coordinator is not available")
	}
	if groupID == "" {
		return fmt.Errorf("no consumer group named for transactional offsets")
	}

	converted := make(map[string]map[int32]group.OffsetCommitData, len(offsets))
	for topic, byPartition := range offsets {
		partitions := make(map[int32]group.OffsetCommitData, len(byPartition))
		for partition, offset := range byPartition {
			partitions[partition] = group.OffsetCommitData{
				Offset:   offset.Offset,
				Metadata: offset.Metadata,
			}
		}
		converted[topic] = partitions
	}

	resp, err := c.coordinator.HandleOffsetCommit(&group.OffsetCommitRequest{
		GroupID:      groupID,
		GenerationID: -1,
		Offsets:      converted,
	})
	if err != nil {
		return fmt.Errorf("committing transactional offsets: %w", err)
	}

	for topic, byPartition := range resp.Errors {
		for partition, code := range byPartition {
			if code != group.ErrorCodeNone {
				return fmt.Errorf("committing transactional offset for %s-%d: group error code %d",
					topic, partition, code)
			}
		}
	}

	return nil
}
