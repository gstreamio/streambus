package client

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/consumer/group"
	"github.com/gstreamio/streambus/pkg/logging"
	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/server"
	"github.com/gstreamio/streambus/pkg/storage"
	"github.com/gstreamio/streambus/pkg/transaction"
)

// testBroker is a real StreamBus broker running in-process for client tests:
// the wire-protocol handler, a consumer group coordinator and a transaction
// coordinator, over a temporary data directory.
type testBroker struct {
	Addr             string
	GroupCoordinator *group.GroupCoordinator
	TxnCoordinator   *transaction.TransactionCoordinator
	Markers          *transaction.MemoryMarkerWriter
	TopicManager     *server.TopicManager
}

// startTestBroker starts a StreamBus broker on an ephemeral port and stops it
// when the test ends.
func startTestBroker(t *testing.T) *testBroker {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to reserve a port: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("Failed to release reserved port: %v", err)
	}

	logger := logging.New(&logging.Config{Level: logging.LevelError, Component: "test"})

	topicManager := server.NewTopicManager(t.TempDir())
	t.Cleanup(func() { _ = topicManager.Close() })

	groupCoordinator := group.NewGroupCoordinator(
		group.NewMemoryOffsetStorage(), group.DefaultCoordinatorConfig())
	t.Cleanup(func() { _ = groupCoordinator.Stop() })

	// Markers go to the real partition logs, exactly as pkg/broker wires them
	// in production: a memory writer here would never call
	// Partition.EndTransaction, so read-committed fetches would keep seeing a
	// committed transaction's barrier and the harness would test nothing.
	markers := transaction.NewMemoryMarkerWriter()
	txnCoordinator := transaction.NewTransactionCoordinator(
		transaction.NewMemoryTransactionLog(), transaction.DefaultCoordinatorConfig(), logger)
	txnCoordinator.SetMarkerWriter(&recordingLogMarkerWriter{
		topicManager: topicManager,
		recorder:     markers,
	})
	txnCoordinator.SetOffsetCommitter(&groupOffsetBridge{coordinator: groupCoordinator})
	t.Cleanup(txnCoordinator.Stop)

	var handler server.RequestHandler = server.NewHandlerWithTopicManager(topicManager)
	handler = server.NewCoordinationHandler(handler, groupCoordinator, newSingleBrokerLocator(addr))
	handler = server.NewTransactionHandler(handler, txnCoordinator)

	config := server.DefaultConfig()
	config.Address = addr

	srv, err := server.New(config, handler)
	if err != nil {
		t.Fatalf("Failed to create StreamBus server: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start StreamBus server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	return &testBroker{
		Addr:             addr,
		GroupCoordinator: groupCoordinator,
		TxnCoordinator:   txnCoordinator,
		Markers:          markers,
		TopicManager:     topicManager,
	}
}

// singleBrokerLocator answers every FindCoordinator request with the one
// broker this harness runs, mirroring a real single-node cluster where every
// key is necessarily coordinated by that same broker.
type singleBrokerLocator struct {
	host string
	port int32
}

// newSingleBrokerLocator builds a locator over a "host:port" address.
func newSingleBrokerLocator(addr string) *singleBrokerLocator {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		// addr comes from a net.Listener this file just created, so a
		// malformed address means the harness itself is broken.
		panic("testbroker: invalid broker address " + addr + ": " + err.Error())
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		panic("testbroker: invalid broker port in " + addr + ": " + err.Error())
	}
	return &singleBrokerLocator{host: host, port: int32(port)} //nolint:gosec // port comes from a real listener, within int32
}

func (l *singleBrokerLocator) FindCoordinator(_ protocol.CoordinatorKeyType, _ string) (int32, string, int32, protocol.ErrorCode) {
	return 0, l.host, l.port, protocol.ErrNone
}

// recordingLogMarkerWriter writes transaction markers to real partition logs
// and also records them, so a test can assert on what was written without
// giving up the production behaviour the log writer provides - notably
// resolving the partition's last stable offset.
type recordingLogMarkerWriter struct {
	topicManager *server.TopicManager
	recorder     *transaction.MemoryMarkerWriter
}

func (w *recordingLogMarkerWriter) WriteMarker(topic string, partitionID int32, marker *transaction.TransactionMarker) error {
	partition, err := w.topicManager.GetPartition(topic, uint32(partitionID))
	if err != nil {
		return err
	}

	timestamp := time.Unix(0, marker.Timestamp)
	if marker.Timestamp == 0 {
		timestamp = time.Now()
	}

	log := partition.Log()
	if _, err := log.Append(&storage.MessageBatch{
		Messages: []storage.Message{{
			Timestamp: timestamp,
			Headers:   protocol.TransactionMarkerHeaders(int64(marker.ProducerID), int16(marker.ProducerEpoch), marker.Commit),
		}},
		Timestamp:     timestamp,
		ProducerID:    int64(marker.ProducerID),
		ProducerEpoch: int16(marker.ProducerEpoch),
	}); err != nil {
		return err
	}
	if err := log.Flush(); err != nil {
		return err
	}

	partition.EndTransaction(int64(marker.ProducerID), int16(marker.ProducerEpoch))

	return w.recorder.WriteMarker(topic, partitionID, marker)
}

// groupOffsetBridge publishes transactional offsets into a group coordinator,
// mirroring what the broker wires up in production.
type groupOffsetBridge struct {
	coordinator *group.GroupCoordinator
}

func (b *groupOffsetBridge) CommitOffsets(groupID string, offsets map[string]map[int32]transaction.OffsetMetadata) error {
	converted := make(map[string]map[int32]group.OffsetCommitData, len(offsets))
	for topic, byPartition := range offsets {
		partitions := make(map[int32]group.OffsetCommitData, len(byPartition))
		for partition, offset := range byPartition {
			partitions[partition] = group.OffsetCommitData{Offset: offset.Offset, Metadata: offset.Metadata}
		}
		converted[topic] = partitions
	}

	_, err := b.coordinator.HandleOffsetCommit(&group.OffsetCommitRequest{
		GroupID:      groupID,
		GenerationID: -1,
		Offsets:      converted,
	})
	return err
}

// newTestClient returns a client pointed at the broker.
func newTestClient(t *testing.T, broker *testBroker) *Client {
	t.Helper()

	config := DefaultConfig()
	config.Brokers = []string{broker.Addr}
	config.RequestTimeout = 5 * time.Second

	c, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	return c
}

// createTestTopic creates a topic with the given partition count.
func createTestTopic(t *testing.T, c *Client, topic string, partitions uint32) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.CreateTopic(ctx, topic, partitions, 1); err != nil {
		t.Fatalf("Failed to create topic %s: %v", topic, err)
	}
}
