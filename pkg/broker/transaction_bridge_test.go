package broker

import (
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/consumer/group"
	"github.com/gstreamio/streambus/pkg/server"
	"github.com/gstreamio/streambus/pkg/transaction"
)

func TestLogMarkerWriter_WritesDurableMarker(t *testing.T) {
	topicManager := server.NewTopicManager(t.TempDir())
	t.Cleanup(func() { _ = topicManager.Close() })

	if err := topicManager.CreateTopic("orders", 1); err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	writer := newLogMarkerWriter(topicManager)
	marker := &transaction.TransactionMarker{
		ProducerID:    1234,
		ProducerEpoch: 2,
		Commit:        true,
		Timestamp:     time.Now().UnixNano(),
	}

	if err := writer.WriteMarker("orders", 0, marker); err != nil {
		t.Fatalf("WriteMarker failed: %v", err)
	}

	messages, err := topicManager.ReadMessages("orders", 0, 0, 10)
	if err != nil {
		t.Fatalf("ReadMessages failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("read %d records, want 1", len(messages))
	}

	headers := messages[0].Headers
	if got := string(headers[ControlHeaderKey]); got != ControlTypeTxnMarker {
		t.Errorf("control header = %q, want %q", got, ControlTypeTxnMarker)
	}
	if got := string(headers[TxnCommitHeaderKey]); got != "true" {
		t.Errorf("commit header = %q, want true", got)
	}
	if got := string(headers[TxnProducerIDHeaderKey]); got != "1234" {
		t.Errorf("producer ID header = %q, want 1234", got)
	}
	if got := string(headers[TxnProducerEpochHeaderKey]); got != "2" {
		t.Errorf("producer epoch header = %q, want 2", got)
	}
}

func TestLogMarkerWriter_AbortMarker(t *testing.T) {
	topicManager := server.NewTopicManager(t.TempDir())
	t.Cleanup(func() { _ = topicManager.Close() })

	if err := topicManager.CreateTopic("orders", 1); err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	writer := newLogMarkerWriter(topicManager)
	if err := writer.WriteMarker("orders", 0, &transaction.TransactionMarker{
		ProducerID: 1, Commit: false, Timestamp: time.Now().UnixNano(),
	}); err != nil {
		t.Fatalf("WriteMarker failed: %v", err)
	}

	messages, err := topicManager.ReadMessages("orders", 0, 0, 10)
	if err != nil {
		t.Fatalf("ReadMessages failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("read %d records, want 1", len(messages))
	}
	if got := string(messages[0].Headers[TxnCommitHeaderKey]); got != "false" {
		t.Errorf("commit header = %q, want false", got)
	}
}

func TestLogMarkerWriter_UnknownPartitionFails(t *testing.T) {
	topicManager := server.NewTopicManager(t.TempDir())
	t.Cleanup(func() { _ = topicManager.Close() })

	writer := newLogMarkerWriter(topicManager)

	// A partition that does not exist must fail rather than silently drop the
	// marker, which would let a transaction report a commit it did not make.
	if err := writer.WriteMarker("never-created", 0, &transaction.TransactionMarker{
		ProducerID: 1, Commit: true,
	}); err == nil {
		t.Error("expected an error writing to an unknown partition")
	}
}

func TestLogMarkerWriter_NegativePartitionFails(t *testing.T) {
	topicManager := server.NewTopicManager(t.TempDir())
	t.Cleanup(func() { _ = topicManager.Close() })

	writer := newLogMarkerWriter(topicManager)

	if err := writer.WriteMarker("orders", -1, &transaction.TransactionMarker{ProducerID: 1}); err == nil {
		t.Error("expected an error for a negative partition")
	}
}

func TestLogMarkerWriter_NoStorageFails(t *testing.T) {
	writer := newLogMarkerWriter(nil)

	if err := writer.WriteMarker("orders", 0, &transaction.TransactionMarker{ProducerID: 1}); err == nil {
		t.Error("expected an error when storage is not initialized")
	}
}

func TestGroupOffsetCommitter_PublishesOffsets(t *testing.T) {
	coordinator := group.NewGroupCoordinator(group.NewMemoryOffsetStorage(), group.DefaultCoordinatorConfig())
	t.Cleanup(func() { _ = coordinator.Stop() })

	committer := newGroupOffsetCommitter(coordinator)

	if err := committer.CommitOffsets("analytics", map[string]map[int32]transaction.OffsetMetadata{
		"source": {0: {Offset: 42, Metadata: "cp"}},
	}); err != nil {
		t.Fatalf("CommitOffsets failed: %v", err)
	}

	resp, err := coordinator.HandleOffsetFetch(&group.OffsetFetchRequest{
		GroupID: "analytics",
		Topics:  map[string][]int32{"source": {0}},
	})
	if err != nil {
		t.Fatalf("HandleOffsetFetch failed: %v", err)
	}
	if got := resp.Offsets["source"][0].Offset; got != 42 {
		t.Errorf("committed offset = %d, want 42", got)
	}
}

func TestGroupOffsetCommitter_RequiresGroupID(t *testing.T) {
	coordinator := group.NewGroupCoordinator(group.NewMemoryOffsetStorage(), group.DefaultCoordinatorConfig())
	t.Cleanup(func() { _ = coordinator.Stop() })

	committer := newGroupOffsetCommitter(coordinator)

	if err := committer.CommitOffsets("", map[string]map[int32]transaction.OffsetMetadata{
		"source": {0: {Offset: 42}},
	}); err == nil {
		t.Error("expected an error committing offsets with no group named")
	}
}

func TestGroupOffsetCommitter_NoCoordinatorFails(t *testing.T) {
	committer := newGroupOffsetCommitter(nil)

	if err := committer.CommitOffsets("analytics", map[string]map[int32]transaction.OffsetMetadata{
		"source": {0: {Offset: 42}},
	}); err == nil {
		t.Error("expected an error when no group coordinator is available")
	}
}
