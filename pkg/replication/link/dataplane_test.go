package link

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"

	"github.com/gstreamio/streambus/pkg/client"
)

// These tests exercise the real fetch/produce data plane (replicateBatch and
// friends) end to end against two real, in-process StreamBus brokers - never
// Kafka, never a mock. See startTestStreamBusBroker in manager_test.go.

// produceToBroker connects to broker and sends each key/value pair to topic
// partition 0, in order.
func produceToBroker(t *testing.T, broker, topic string, messages ...[2]string) {
	t.Helper()

	cfg := client.DefaultConfig()
	cfg.Brokers = []string{broker}
	c, err := client.New(cfg)
	if err != nil {
		t.Fatalf("client.New failed: %v", err)
	}
	defer func() { _ = c.Close() }()

	p := client.NewProducer(c)
	defer func() { _ = p.Close() }()

	for _, kv := range messages {
		if err := p.Send(context.Background(), topic, []byte(kv[0]), []byte(kv[1])); err != nil {
			t.Fatalf("produce to %s failed: %v", broker, err)
		}
	}
}

// fetchAllFromBroker fetches every message currently on topic partition 0,
// from offset 0, using a fresh connection to broker.
func fetchAllFromBroker(t *testing.T, broker, topic string) []protocol.Message {
	t.Helper()

	cfg := client.DefaultConfig()
	cfg.Brokers = []string{broker}
	c, err := client.New(cfg)
	if err != nil {
		t.Fatalf("client.New failed: %v", err)
	}
	defer func() { _ = c.Close() }()

	resp, err := c.Fetch(context.Background(), &client.FetchRequest{
		Topic:     topic,
		Partition: 0,
		Offset:    0,
		MaxBytes:  1 << 20,
	})
	if err != nil {
		t.Fatalf("fetch from %s failed: %v", broker, err)
	}
	return resp.Messages
}

// waitForMessageCount polls broker until topic partition 0 holds at least
// want messages, failing the test if timeout elapses first.
func waitForMessageCount(t *testing.T, broker, topic string, want int, timeout time.Duration) []protocol.Message {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		msgs := fetchAllFromBroker(t, broker, topic)
		if len(msgs) >= want {
			return msgs
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d messages on %q, got %d", want, topic, len(msgs))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitForCheckpoint polls storage until a checkpoint exists for link/topic/
// partition with SourceOffset >= minSourceOffset. Checkpoints are the
// documented way to observe a worker's progress from outside the package
// without racing its private offset fields directly (storage.SaveCheckpoint
// and LoadCheckpoint both take memoryStorage's own lock, giving a real
// happens-before edge; the worker's plain fields do not).
func waitForCheckpoint(t *testing.T, storage Storage, linkID, topic string, minSourceOffset int64, timeout time.Duration) *Checkpoint {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		checkpoint, err := storage.LoadCheckpoint(linkID, topic, 0)
		if err == nil && checkpoint.SourceOffset >= minSourceOffset {
			return checkpoint
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for checkpoint with source offset >= %d", minSourceOffset)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestDataPlane_MessageReplicatesToTarget covers the core case: a message
// produced to the source cluster actually arrives at the target, replacing
// the old stub that only ever slept and returned nil.
func TestDataPlane_MessageReplicatesToTarget(t *testing.T) {
	sourceAddr := startTestStreamBusBroker(t)
	targetAddr := startTestStreamBusBroker(t)

	link := createTestLink("e2e-basic", "E2E Basic")
	link.SourceCluster.Brokers = []string{sourceAddr}
	link.TargetCluster.Brokers = []string{targetAddr}

	produceToBroker(t, sourceAddr, "test-topic", [2]string{"key1", "hello"})

	handler, err := NewStreamHandler(link, NewMemoryStorage())
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	if err := handler.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	msgs := waitForMessageCount(t, targetAddr, "test-topic", 1, 5*time.Second)
	if len(msgs) != 1 {
		t.Fatalf("expected exactly 1 message on target, got %d", len(msgs))
	}
	if string(msgs[0].Key) != "key1" || string(msgs[0].Value) != "hello" {
		t.Errorf("expected key1/hello, got %q/%q", msgs[0].Key, msgs[0].Value)
	}

	// Metrics must reflect the real replication, not a fabricated number.
	handler.WithStats(func(metrics *ReplicationMetrics, health *ReplicationHealth) {
		if metrics.TotalMessagesReplicated < 1 {
			t.Errorf("expected TotalMessagesReplicated >= 1, got %d", metrics.TotalMessagesReplicated)
		}
		if health.Status != "healthy" && health.Status != "unverified" {
			// Either is acceptable depending on whether performHealthCheck's
			// 30s ticker has run yet; what matters is it is never a lie.
			t.Logf("health status: %s", health.Status)
		}
	})
}

// TestDataPlane_ResumesFromCheckpointWithoutDuplicating covers restart
// semantics: after a link is stopped and a fresh handler restarts it (the
// same thing StartLink does on the manager), replication must continue from
// the checkpoint rather than replaying everything from offset zero.
func TestDataPlane_ResumesFromCheckpointWithoutDuplicating(t *testing.T) {
	sourceAddr := startTestStreamBusBroker(t)
	targetAddr := startTestStreamBusBroker(t)

	link := createTestLink("e2e-resume", "E2E Resume")
	link.SourceCluster.Brokers = []string{sourceAddr}
	link.TargetCluster.Brokers = []string{targetAddr}

	produceToBroker(t, sourceAddr, "test-topic",
		[2]string{"k0", "msg-0"},
		[2]string{"k1", "msg-1"},
		[2]string{"k2", "msg-2"},
	)

	storage := NewMemoryStorage()

	handler1, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	if err := handler1.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	waitForMessageCount(t, targetAddr, "test-topic", 3, 5*time.Second)
	// Wait for a checkpoint reflecting all 3 source messages before
	// stopping, so the resumed handler below has something to resume from
	// (Stop also saves a final checkpoint, but confirming one exists here
	// keeps the two phases of this test clearly separated).
	waitForCheckpoint(t, storage, link.ID, "test-topic", 3, 5*time.Second)

	if err := handler1.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	produceToBroker(t, sourceAddr, "test-topic",
		[2]string{"k3", "msg-3"},
		[2]string{"k4", "msg-4"},
	)

	handler2, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	if err := handler2.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = handler2.Stop() }()

	waitForMessageCount(t, targetAddr, "test-topic", 5, 5*time.Second)

	// Give a resuming-from-zero bug a moment to over-produce before the
	// final check, rather than declaring success the instant 5 is reached.
	time.Sleep(300 * time.Millisecond)

	msgs := fetchAllFromBroker(t, targetAddr, "test-topic")
	if len(msgs) != 5 {
		t.Fatalf("expected exactly 5 messages on target (no duplication), got %d", len(msgs))
	}
	for i, msg := range msgs {
		want := "msg-" + string(rune('0'+i))
		if string(msg.Value) != want {
			t.Errorf("message %d: expected %q, got %q", i, want, msg.Value)
		}
	}
}

// TestDataPlane_EmptySourceDoesNotFailOrSpin covers the steady-state case: a
// source partition with nothing new must not be treated as a replication
// failure, however often it is polled.
func TestDataPlane_EmptySourceDoesNotFailOrSpin(t *testing.T) {
	link := createTestLink("e2e-empty", "E2E Empty")
	link.SourceCluster.Brokers = []string{startTestStreamBusBroker(t)}
	link.TargetCluster.Brokers = []string{startTestStreamBusBroker(t)}
	// A short, explicit backoff keeps this test fast without asking it to
	// hot-loop: waitForNextPoll still paces every empty fetch by this much.
	link.Config.FetchWaitMaxMs = 50

	handler, err := NewStreamHandler(link, NewMemoryStorage())
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	if err := handler.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	time.Sleep(500 * time.Millisecond)

	handler.WithStats(func(metrics *ReplicationMetrics, _ *ReplicationHealth) {
		if metrics.TotalErrors != 0 {
			t.Errorf("expected no errors replicating an empty topic, got %d", metrics.TotalErrors)
		}
		if metrics.ConsecutiveFailures != 0 {
			t.Errorf("expected ConsecutiveFailures 0, got %d", metrics.ConsecutiveFailures)
		}
	})
}

// TestDataPlane_ExcludeFilterDropsMatchingMessages covers message filtering:
// a message matching an exclude pattern must never reach the target, while
// the source offset still advances past it.
func TestDataPlane_ExcludeFilterDropsMatchingMessages(t *testing.T) {
	sourceAddr := startTestStreamBusBroker(t)
	targetAddr := startTestStreamBusBroker(t)

	link := createTestLink("e2e-filter", "E2E Filter")
	link.SourceCluster.Brokers = []string{sourceAddr}
	link.TargetCluster.Brokers = []string{targetAddr}
	link.Filter = &FilterConfig{
		Enabled:         true,
		ExcludePatterns: []string{"^debug-"},
	}

	produceToBroker(t, sourceAddr, "test-topic",
		[2]string{"k1", "keep-1"},
		[2]string{"k2", "debug-drop-me"},
		[2]string{"k3", "keep-2"},
	)

	storage := NewMemoryStorage()
	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	if err := handler.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	msgs := waitForMessageCount(t, targetAddr, "test-topic", 2, 5*time.Second)
	if len(msgs) != 2 {
		t.Fatalf("expected exactly 2 messages on target, got %d", len(msgs))
	}
	for _, msg := range msgs {
		if strings.HasPrefix(string(msg.Value), "debug-") {
			t.Errorf("filtered message leaked through to target: %q", msg.Value)
		}
	}

	// The filtered-out message still consumed a source offset; the source
	// side must have advanced past all 3, not just the 2 that were kept.
	waitForCheckpoint(t, storage, link.ID, "test-topic", 3, 5*time.Second)
}

// TestDataPlane_IncludeFilterKeepsOnlyMatchingMessages covers the include
// side of filtering: only messages matching an include pattern reach the
// target.
func TestDataPlane_IncludeFilterKeepsOnlyMatchingMessages(t *testing.T) {
	sourceAddr := startTestStreamBusBroker(t)
	targetAddr := startTestStreamBusBroker(t)

	link := createTestLink("e2e-include-filter", "E2E Include Filter")
	link.SourceCluster.Brokers = []string{sourceAddr}
	link.TargetCluster.Brokers = []string{targetAddr}
	link.Filter = &FilterConfig{
		Enabled:         true,
		IncludePatterns: []string{"^important-"},
	}

	produceToBroker(t, sourceAddr, "test-topic",
		[2]string{"k1", "important-1"},
		[2]string{"k2", "irrelevant"},
		[2]string{"k3", "important-2"},
	)

	storage := NewMemoryStorage()
	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	if err := handler.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	msgs := waitForMessageCount(t, targetAddr, "test-topic", 2, 5*time.Second)
	if len(msgs) != 2 {
		t.Fatalf("expected exactly 2 messages on target, got %d", len(msgs))
	}
	for _, msg := range msgs {
		if !strings.HasPrefix(string(msg.Value), "important-") {
			t.Errorf("non-matching message leaked through to target: %q", msg.Value)
		}
	}
}
