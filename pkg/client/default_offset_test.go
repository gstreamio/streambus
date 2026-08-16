package client

import (
	"context"
	"testing"
	"time"
)

// TestConsumer_DefaultStartOffsetResolvesToLatest verifies that a Consumer
// created with default config (StartOffset: -1, "latest") and never
// explicitly Seek'd resolves that sentinel to the real end-of-log offset the
// first time it fetches, rather than sending the literal -1 to the broker
// and getting ErrOffsetOutOfRange. It should behave like "caught up, nothing
// new yet" for pre-existing messages, and pick up messages produced after
// that first fetch.
func TestConsumer_DefaultStartOffsetResolvesToLatest(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	ctx := context.Background()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	topic := "default-offset-topic"
	if err := client.CreateTopic(ctx, topic, 1, 1); err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	// Produce messages BEFORE the consumer's first fetch.
	producer := NewProducer(client)
	for i := 0; i < 3; i++ {
		if err := producer.Send(ctx, topic, []byte("key"), []byte("pre-existing")); err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}
	}
	time.Sleep(100 * time.Millisecond)

	// Consumer with default config - StartOffset is -1 ("latest"), no Seek call.
	consumer := NewConsumer(client, topic, 0)
	defer consumer.Close()

	if consumer.CurrentOffset() != -1 {
		t.Fatalf("Expected unresolved offset -1 before first fetch, got %d", consumer.CurrentOffset())
	}

	messages, err := consumer.FetchN(ctx, 10)
	if err != nil {
		t.Fatalf("Expected first fetch with default offset to succeed, got error: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("Expected 0 pre-existing messages, got %d", len(messages))
	}
	if consumer.CurrentOffset() != 3 {
		t.Errorf("Expected offset to resolve to end-of-log (3), got %d", consumer.CurrentOffset())
	}

	// Produce a new message after the first fetch resolved "latest".
	if err := producer.Send(ctx, topic, []byte("key"), []byte("new-message")); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	messages, err = consumer.FetchN(ctx, 10)
	if err != nil {
		t.Fatalf("Second fetch failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("Expected 1 new message, got %d", len(messages))
	}
	if string(messages[0].Value) != "new-message" {
		t.Errorf("Expected 'new-message', got %s", string(messages[0].Value))
	}
}

// TestPartitionConsumer_DefaultStartOffsetResolvesToLatest is the
// PartitionConsumer analogue of TestConsumer_DefaultStartOffsetResolvesToLatest.
func TestPartitionConsumer_DefaultStartOffsetResolvesToLatest(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	ctx := context.Background()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	topic := "default-offset-partition-topic"
	if err := client.CreateTopic(ctx, topic, 1, 1); err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	producer := NewProducer(client)
	for i := 0; i < 3; i++ {
		if err := producer.SendToPartition(ctx, topic, 0, []byte("key"), []byte("pre-existing")); err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}
	}
	time.Sleep(100 * time.Millisecond)

	// PartitionConsumer with default config - StartOffset is -1 ("latest"),
	// no SeekPartition/SeekAll call.
	pc := NewPartitionConsumer(client, topic, []uint32{0})
	defer pc.Close()

	if offsets := pc.GetOffsets(); offsets[0] != -1 {
		t.Fatalf("Expected unresolved offset -1 before first fetch, got %d", offsets[0])
	}

	messages, err := pc.FetchFromPartition(ctx, 0)
	if err != nil {
		t.Fatalf("Expected first fetch with default offset to succeed, got error: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("Expected 0 pre-existing messages, got %d", len(messages))
	}
	if offsets := pc.GetOffsets(); offsets[0] != 3 {
		t.Errorf("Expected offset to resolve to end-of-log (3), got %d", offsets[0])
	}

	if err := producer.SendToPartition(ctx, topic, 0, []byte("key"), []byte("new-message")); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	messages, err = pc.FetchFromPartition(ctx, 0)
	if err != nil {
		t.Fatalf("Second fetch failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("Expected 1 new message, got %d", len(messages))
	}
	if string(messages[0].Value) != "new-message" {
		t.Errorf("Expected 'new-message', got %s", string(messages[0].Value))
	}
}
