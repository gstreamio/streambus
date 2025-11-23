package client

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
)

func TestNewPartitionConsumer(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	topic := "test-topic"
	partitions := []uint32{0, 1, 2}

	pc := NewPartitionConsumer(client, topic, partitions)
	if pc == nil {
		t.Fatal("Expected non-nil partition consumer")
	}

	if pc.topic != topic {
		t.Errorf("Expected topic %s, got %s", topic, pc.topic)
	}

	if len(pc.partitions) != len(partitions) {
		t.Errorf("Expected %d partitions, got %d", len(partitions), len(pc.partitions))
	}

	if len(pc.partitionStates) != len(partitions) {
		t.Errorf("Expected %d partition states, got %d", len(partitions), len(pc.partitionStates))
	}

	// Verify partition states are initialized
	for _, pid := range partitions {
		state, exists := pc.partitionStates[pid]
		if !exists {
			t.Errorf("Expected partition %d state to be initialized", pid)
		}
		if state.partitionID != pid {
			t.Errorf("Expected partition ID %d, got %d", pid, state.partitionID)
		}
		if state.offset != client.config.ConsumerConfig.StartOffset {
			t.Errorf("Expected offset %d, got %d", client.config.ConsumerConfig.StartOffset, state.offset)
		}
	}
}

func TestNewPartitionConsumerWithConfig(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	topic := "test-topic"
	partitions := []uint32{0}
	consumerConfig := ConsumerConfig{
		StartOffset:   100,
		MaxFetchBytes: 2048,
		MaxWaitTime:   5 * time.Second,
	}

	pc := NewPartitionConsumerWithConfig(client, topic, partitions, consumerConfig)
	if pc == nil {
		t.Fatal("Expected non-nil partition consumer")
	}

	if pc.config.StartOffset != consumerConfig.StartOffset {
		t.Errorf("Expected start offset %d, got %d", consumerConfig.StartOffset, pc.config.StartOffset)
	}

	// Verify partition state uses custom config
	state := pc.partitionStates[0]
	if state.offset != consumerConfig.StartOffset {
		t.Errorf("Expected offset %d, got %d", consumerConfig.StartOffset, state.offset)
	}
}

func TestPartitionConsumer_FetchFromPartition(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create topic first
	topic := "test-topic-partition"
	if err := client.CreateTopic(topic, 1, 1); err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	// Produce some messages
	producer := NewProducer(client)
	for i := 0; i < 5; i++ {
		if err := producer.SendToPartition(topic, 0, []byte("key"), []byte("test message")); err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}
	}

	time.Sleep(100 * time.Millisecond)

	// Create partition consumer
	pc := NewPartitionConsumer(client, topic, []uint32{0})
	defer pc.Close()

	// Test fetching from partition
	messages, err := pc.FetchFromPartition(0)
	if err != nil {
		t.Errorf("Failed to fetch from partition: %v", err)
	}

	// Note: Messages might be empty in test environment, but fetch should succeed
	t.Logf("Fetched %d messages", len(messages))

	// Verify metrics - fetch count should be updated even if no messages
	metrics := pc.Metrics()
	if metrics.FetchCount == 0 {
		t.Error("Expected fetch count to be updated")
	}
}

func TestPartitionConsumer_FetchFromPartition_InvalidPartition(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0})
	defer pc.Close()

	// Try to fetch from unassigned partition
	_, err = pc.FetchFromPartition(99)
	if err == nil {
		t.Error("Expected error for invalid partition")
	}
}

func TestPartitionConsumer_FetchFromPartition_Closed(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0})
	pc.Close()

	// Try to fetch after close
	_, err = pc.FetchFromPartition(0)
	if err != ErrConsumerClosed {
		t.Errorf("Expected ErrConsumerClosed, got %v", err)
	}
}

func TestPartitionConsumer_FetchAll(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create topic with multiple partitions
	topic := "test-topic-multi"
	if err := client.CreateTopic(topic, 3, 1); err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	// Produce to multiple partitions
	producer := NewProducer(client)
	for pid := uint32(0); pid < 3; pid++ {
		if err := producer.SendToPartition(topic, pid, []byte("key"), []byte("test message")); err != nil {
			t.Logf("Failed to send to partition %d: %v", pid, err)
		}
	}

	time.Sleep(100 * time.Millisecond)

	// Create partition consumer for all partitions
	pc := NewPartitionConsumer(client, topic, []uint32{0, 1, 2})
	defer pc.Close()

	// Test FetchAll
	results, err := pc.FetchAll()
	if err != nil {
		t.Errorf("Failed to fetch all: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected results from at least one partition")
	}

	// Verify results structure
	for pid, messages := range results {
		if pid > 2 {
			t.Errorf("Unexpected partition ID %d", pid)
		}
		t.Logf("Partition %d: %d messages", pid, len(messages))
	}
}

func TestPartitionConsumer_FetchAll_Closed(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0, 1})
	pc.Close()

	// Try to fetch all after close
	_, err = pc.FetchAll()
	if err != ErrConsumerClosed {
		t.Errorf("Expected ErrConsumerClosed, got %v", err)
	}
}

func TestPartitionConsumer_FetchRoundRobin(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	topic := "test-topic-rr"
	if err := client.CreateTopic(topic, 2, 1); err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	// Produce messages
	producer := NewProducer(client)
	for i := 0; i < 10; i++ {
		pid := uint32(i % 2)
		if err := producer.SendToPartition(topic, pid, []byte("key"), []byte("test message")); err != nil {
			t.Logf("Failed to send message: %v", err)
		}
	}

	time.Sleep(100 * time.Millisecond)

	// Create partition consumer
	pc := NewPartitionConsumer(client, topic, []uint32{0, 1})
	defer pc.Close()

	// Test FetchRoundRobin
	messages, err := pc.FetchRoundRobin()
	if err != nil {
		t.Errorf("Failed to fetch round robin: %v", err)
	}

	t.Logf("Fetched %d messages in round-robin", len(messages))
}

func TestPartitionConsumer_FetchRoundRobin_NoPartitions(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create consumer with no partitions
	pc := NewPartitionConsumer(client, "test-topic", []uint32{})
	defer pc.Close()

	// Try to fetch with no partitions
	_, err = pc.FetchRoundRobin()
	if err == nil {
		t.Error("Expected error when fetching with no partitions")
	}
}

func TestPartitionConsumer_FetchRoundRobin_Closed(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0})
	pc.Close()

	// Try to fetch after close
	_, err = pc.FetchRoundRobin()
	if err != ErrConsumerClosed {
		t.Errorf("Expected ErrConsumerClosed, got %v", err)
	}
}

func TestPartitionConsumer_SeekPartition(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0, 1})
	defer pc.Close()

	// Test seeking to specific offset
	err = pc.SeekPartition(0, 100)
	if err != nil {
		t.Errorf("Failed to seek partition: %v", err)
	}

	// Verify offset was updated
	offsets := pc.GetOffsets()
	if offsets[0] != 100 {
		t.Errorf("Expected offset 100, got %d", offsets[0])
	}

	// Test seeking to end (-1)
	state := pc.partitionStates[1]
	state.highWater = 500
	err = pc.SeekPartition(1, -1)
	if err != nil {
		t.Errorf("Failed to seek to end: %v", err)
	}

	offsets = pc.GetOffsets()
	if offsets[1] != 500 {
		t.Errorf("Expected offset 500 (high water), got %d", offsets[1])
	}
}

func TestPartitionConsumer_SeekPartition_InvalidPartition(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0})
	defer pc.Close()

	// Try to seek invalid partition
	err = pc.SeekPartition(99, 100)
	if err == nil {
		t.Error("Expected error for invalid partition")
	}
}

func TestPartitionConsumer_SeekPartition_InvalidOffset(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0})
	defer pc.Close()

	// Try to seek to invalid offset
	err = pc.SeekPartition(0, -2)
	if err != ErrInvalidOffset {
		t.Errorf("Expected ErrInvalidOffset, got %v", err)
	}
}

func TestPartitionConsumer_SeekPartition_Closed(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0})
	pc.Close()

	// Try to seek after close
	err = pc.SeekPartition(0, 100)
	if err != ErrConsumerClosed {
		t.Errorf("Expected ErrConsumerClosed, got %v", err)
	}
}

func TestPartitionConsumer_SeekAll(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0, 1, 2})
	defer pc.Close()

	// Test seeking all partitions
	err = pc.SeekAll(200)
	if err != nil {
		t.Errorf("Failed to seek all: %v", err)
	}

	// Verify all offsets were updated
	offsets := pc.GetOffsets()
	for pid, offset := range offsets {
		if offset != 200 {
			t.Errorf("Partition %d: expected offset 200, got %d", pid, offset)
		}
	}
}

func TestPartitionConsumer_SeekAll_InvalidOffset(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0})
	defer pc.Close()

	// Try to seek all with invalid offset
	err = pc.SeekAll(-10)
	if err != ErrInvalidOffset {
		t.Errorf("Expected ErrInvalidOffset, got %v", err)
	}
}

func TestPartitionConsumer_SeekAll_Closed(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0})
	pc.Close()

	// Try to seek all after close
	err = pc.SeekAll(100)
	if err != ErrConsumerClosed {
		t.Errorf("Expected ErrConsumerClosed, got %v", err)
	}
}

func TestPartitionConsumer_GetOffsets(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0, 1, 2})
	defer pc.Close()

	// Set different offsets
	if err := pc.SeekPartition(0, 10); err != nil {
		t.Fatalf("Failed to seek partition 0: %v", err)
	}
	if err := pc.SeekPartition(1, 20); err != nil {
		t.Fatalf("Failed to seek partition 1: %v", err)
	}
	if err := pc.SeekPartition(2, 30); err != nil {
		t.Fatalf("Failed to seek partition 2: %v", err)
	}

	// Get offsets
	offsets := pc.GetOffsets()

	if offsets[0] != 10 {
		t.Errorf("Expected offset 10 for partition 0, got %d", offsets[0])
	}
	if offsets[1] != 20 {
		t.Errorf("Expected offset 20 for partition 1, got %d", offsets[1])
	}
	if offsets[2] != 30 {
		t.Errorf("Expected offset 30 for partition 2, got %d", offsets[2])
	}
}

func TestPartitionConsumer_GetPartitionInfo(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0})
	defer pc.Close()

	// Set partition state
	state := pc.partitionStates[0]
	state.mu.Lock()
	state.offset = 100
	state.highWater = 200
	state.lastFetch = time.Now()
	state.mu.Unlock()

	// Get partition info
	info, err := pc.GetPartitionInfo(0)
	if err != nil {
		t.Errorf("Failed to get partition info: %v", err)
	}

	if info.PartitionID != 0 {
		t.Errorf("Expected partition ID 0, got %d", info.PartitionID)
	}
	if info.Offset != 100 {
		t.Errorf("Expected offset 100, got %d", info.Offset)
	}
	if info.HighWater != 200 {
		t.Errorf("Expected high water 200, got %d", info.HighWater)
	}
	if info.Lag != 100 {
		t.Errorf("Expected lag 100, got %d", info.Lag)
	}
}

func TestPartitionConsumer_GetPartitionInfo_InvalidPartition(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0})
	defer pc.Close()

	// Try to get info for invalid partition
	_, err = pc.GetPartitionInfo(99)
	if err == nil {
		t.Error("Expected error for invalid partition")
	}
}

func TestPartitionConsumer_PollPartitions(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	topic := "test-topic-poll"
	if err := client.CreateTopic(topic, 1, 1); err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	// Produce messages
	producer := NewProducer(client)
	for i := 0; i < 3; i++ {
		if err := producer.SendToPartition(topic, 0, []byte("key"), []byte("test message")); err != nil {
			t.Logf("Failed to send message: %v", err)
		}
	}

	time.Sleep(100 * time.Millisecond)

	pc := NewPartitionConsumer(client, topic, []uint32{0})
	defer pc.Close()

	// Test polling with context
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var mu sync.Mutex
	messageCount := 0
	handler := func(pid uint32, messages []protocol.Message) error {
		mu.Lock()
		messageCount += len(messages)
		mu.Unlock()
		return nil
	}

	err = pc.PollPartitions(ctx, 100*time.Millisecond, handler)
	if err != context.DeadlineExceeded && err != context.Canceled {
		t.Errorf("Expected context deadline or canceled, got %v", err)
	}

	mu.Lock()
	t.Logf("Polled %d messages", messageCount)
	mu.Unlock()
}

func TestPartitionConsumer_PollPartitions_Closed(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0})
	pc.Close()

	ctx := context.Background()
	handler := func(pid uint32, messages []protocol.Message) error {
		return nil
	}

	// Try to poll after close
	err = pc.PollPartitions(ctx, 100*time.Millisecond, handler)
	if err != ErrConsumerClosed {
		t.Errorf("Expected ErrConsumerClosed, got %v", err)
	}
}

func TestPartitionConsumer_Close(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0, 1})

	// Close should not error
	err = pc.Close()
	if err != nil {
		t.Errorf("Unexpected error during close: %v", err)
	}

	// Second close should be idempotent
	err = pc.Close()
	if err != nil {
		t.Errorf("Expected no error on second close, got %v", err)
	}

	// Verify consumer is closed
	if pc.partitionStates != nil {
		t.Error("Expected partition states to be nil after close")
	}
}

func TestPartitionConsumer_Metrics(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	pc := NewPartitionConsumer(client, "test-topic", []uint32{0})
	defer pc.Close()

	// Initial metrics should be zero
	metrics := pc.Metrics()
	if metrics.MessagesRead != 0 {
		t.Errorf("Expected 0 messages read, got %d", metrics.MessagesRead)
	}
	if metrics.BytesRead != 0 {
		t.Errorf("Expected 0 bytes read, got %d", metrics.BytesRead)
	}
	if metrics.FetchCount != 0 {
		t.Errorf("Expected 0 fetch count, got %d", metrics.FetchCount)
	}
}
