package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/shawntherrien/streambus/pkg/client"
)

// TestE2E_ProducerConsumerLifecycle tests the complete producer-consumer flow
func TestE2E_ProducerConsumerLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Test configuration
	brokers := []string{"localhost:9092"}
	topic := fmt.Sprintf("test-lifecycle-%d", time.Now().Unix())
	numMessages := 100

	// Create client
	cfg := &client.Config{
		Brokers:        brokers,
		ConnectTimeout: 10 * time.Second,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
	}

	c, err := client.New(cfg)
	if err != nil {
		t.Skipf("Cannot connect to broker: %v", err)
		return
	}
	defer c.Close()

	ctx := context.Background()

	// Step 1: Produce messages
	t.Log("Step 1: Producing messages")
	producer := client.NewProducer(c)

	for i := 0; i < numMessages; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		value := []byte(fmt.Sprintf("message-%d", i))

		err := producer.Send(ctx, topic, 0, key, value)
		if err != nil {
			t.Fatalf("Failed to send message %d: %v", i, err)
		}
	}

	t.Logf("Produced %d messages", numMessages)

	// Step 2: Consume messages
	t.Log("Step 2: Consuming messages")
	consumer := client.NewConsumer(c, topic, 0)

	consumedCount := 0
	timeout := time.After(30 * time.Second)

	for consumedCount < numMessages {
		select {
		case <-timeout:
			t.Fatalf("Timeout: only consumed %d/%d messages", consumedCount, numMessages)
		default:
			messages, err := consumer.Fetch(ctx, 0, 1024*1024)
			if err != nil {
				t.Fatalf("Failed to fetch messages: %v", err)
			}

			if len(messages) > 0 {
				consumedCount += len(messages)
				t.Logf("Consumed batch of %d messages (total: %d/%d)", len(messages), consumedCount, numMessages)
			} else {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}

	// Verify all messages consumed
	if consumedCount != numMessages {
		t.Errorf("Expected %d messages, consumed %d", numMessages, consumedCount)
	}

	t.Logf("✓ Successfully produced and consumed %d messages", numMessages)
}

// TestE2E_MultiPartition tests producing and consuming across multiple partitions
func TestE2E_MultiPartition(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	brokers := []string{"localhost:9092"}
	topic := fmt.Sprintf("test-multipart-%d", time.Now().Unix())
	numPartitions := 3
	messagesPerPartition := 50

	cfg := &client.Config{
		Brokers:        brokers,
		ConnectTimeout: 10 * time.Second,
	}

	c, err := client.New(cfg)
	if err != nil {
		t.Skipf("Cannot connect to broker: %v", err)
		return
	}
	defer c.Close()

	ctx := context.Background()

	// Create topic with multiple partitions (would need admin API)
	// For now, assume topic exists or is auto-created

	// Produce to each partition
	producer := client.NewProducer(c)

	for partID := 0; partID < numPartitions; partID++ {
		for i := 0; i < messagesPerPartition; i++ {
			key := []byte(fmt.Sprintf("part%d-key-%d", partID, i))
			value := []byte(fmt.Sprintf("part%d-msg-%d", partID, i))

			err := producer.Send(ctx, topic, uint32(partID), key, value)
			if err != nil {
				t.Fatalf("Failed to send to partition %d: %v", partID, err)
			}
		}
		t.Logf("Produced %d messages to partition %d", messagesPerPartition, partID)
	}

	// Consume from each partition
	for partID := 0; partID < numPartitions; partID++ {
		consumer := client.NewConsumer(c, topic, uint32(partID))

		consumed := 0
		timeout := time.After(15 * time.Second)

		for consumed < messagesPerPartition {
			select {
			case <-timeout:
				t.Fatalf("Timeout consuming from partition %d: got %d/%d", partID, consumed, messagesPerPartition)
			default:
				messages, err := consumer.Fetch(ctx, 0, 1024*1024)
				if err != nil {
					t.Fatalf("Fetch failed for partition %d: %v", partID, err)
				}

				consumed += len(messages)
				if len(messages) == 0 {
					time.Sleep(100 * time.Millisecond)
				}
			}
		}

		t.Logf("✓ Consumed %d messages from partition %d", consumed, partID)
	}
}

// TestE2E_LargeMessages tests handling of large message payloads
func TestE2E_LargeMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	brokers := []string{"localhost:9092"}
	topic := fmt.Sprintf("test-large-%d", time.Now().Unix())

	cfg := &client.Config{
		Brokers:        brokers,
		ConnectTimeout: 10 * time.Second,
	}

	c, err := client.New(cfg)
	if err != nil {
		t.Skipf("Cannot connect to broker: %v", err)
		return
	}
	defer c.Close()

	ctx := context.Background()

	// Test various large message sizes
	sizes := []int{
		1 * 1024,      // 1KB
		10 * 1024,     // 10KB
		100 * 1024,    // 100KB
		1024 * 1024,   // 1MB
	}

	producer := client.NewProducer(c)

	for _, size := range sizes {
		t.Logf("Testing message size: %d bytes", size)

		value := make([]byte, size)
		for i := range value {
			value[i] = byte(i % 256)
		}

		key := []byte(fmt.Sprintf("large-%d", size))

		// Produce
		err := producer.Send(ctx, topic, 0, key, value)
		if err != nil {
			t.Fatalf("Failed to send %d byte message: %v", size, err)
		}

		// Consume
		consumer := client.NewConsumer(c, topic, 0)
		messages, err := consumer.Fetch(ctx, 0, 10*1024*1024)
		if err != nil {
			t.Fatalf("Failed to fetch %d byte message: %v", size, err)
		}

		if len(messages) == 0 {
			t.Fatalf("No messages fetched for size %d", size)
		}

		// Verify size
		if len(messages[0].Value) != size {
			t.Errorf("Expected message size %d, got %d", size, len(messages[0].Value))
		}

		// Verify content
		for i := range messages[0].Value {
			if messages[0].Value[i] != byte(i%256) {
				t.Errorf("Content mismatch at offset %d", i)
				break
			}
		}

		t.Logf("✓ Successfully sent and received %d byte message", size)
	}
}

// TestE2E_HighThroughput tests system under high message volume
func TestE2E_HighThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	brokers := []string{"localhost:9092"}
	topic := fmt.Sprintf("test-throughput-%d", time.Now().Unix())
	numMessages := 10000

	cfg := &client.Config{
		Brokers:        brokers,
		ConnectTimeout: 10 * time.Second,
	}

	c, err := client.New(cfg)
	if err != nil {
		t.Skipf("Cannot connect to broker: %v", err)
		return
	}
	defer c.Close()

	ctx := context.Background()

	// Produce messages as fast as possible
	producer := client.NewProducer(c)

	start := time.Now()

	for i := 0; i < numMessages; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		value := []byte(fmt.Sprintf("message-%d-data", i))

		err := producer.Send(ctx, topic, 0, key, value)
		if err != nil {
			t.Fatalf("Failed to send message %d: %v", i, err)
		}

		if (i+1)%1000 == 0 {
			t.Logf("Produced %d messages", i+1)
		}
	}

	produceTime := time.Since(start)
	produceRate := float64(numMessages) / produceTime.Seconds()

	t.Logf("Produced %d messages in %v (%.0f msgs/sec)", numMessages, produceTime, produceRate)

	// Consume all messages
	consumer := client.NewConsumer(c, topic, 0)

	consumed := 0
	consumeStart := time.Now()
	timeout := time.After(60 * time.Second)

	for consumed < numMessages {
		select {
		case <-timeout:
			t.Fatalf("Timeout: consumed %d/%d messages", consumed, numMessages)
		default:
			messages, err := consumer.Fetch(ctx, 0, 10*1024*1024)
			if err != nil {
				t.Fatalf("Fetch failed: %v", err)
			}

			consumed += len(messages)

			if consumed%1000 == 0 && consumed > 0 {
				t.Logf("Consumed %d messages", consumed)
			}

			if len(messages) == 0 {
				time.Sleep(10 * time.Millisecond)
			}
		}
	}

	consumeTime := time.Since(consumeStart)
	consumeRate := float64(consumed) / consumeTime.Seconds()

	t.Logf("Consumed %d messages in %v (%.0f msgs/sec)", consumed, consumeTime, consumeRate)

	if consumed != numMessages {
		t.Errorf("Message count mismatch: produced %d, consumed %d", numMessages, consumed)
	}

	t.Logf("✓ High throughput test passed")
	t.Logf("  Produce: %.0f msgs/sec", produceRate)
	t.Logf("  Consume: %.0f msgs/sec", consumeRate)
}

// TestE2E_OrderingGuarantee tests message ordering within a partition
func TestE2E_OrderingGuarantee(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	brokers := []string{"localhost:9092"}
	topic := fmt.Sprintf("test-ordering-%d", time.Now().Unix())
	numMessages := 100

	cfg := &client.Config{
		Brokers:        brokers,
		ConnectTimeout: 10 * time.Second,
	}

	c, err := client.New(cfg)
	if err != nil {
		t.Skipf("Cannot connect to broker: %v", err)
		return
	}
	defer c.Close()

	ctx := context.Background()

	// Produce messages with sequence numbers
	producer := client.NewProducer(c)

	for i := 0; i < numMessages; i++ {
		key := []byte(fmt.Sprintf("seq-%05d", i))
		value := []byte(fmt.Sprintf("message-%d", i))

		err := producer.Send(ctx, topic, 0, key, value)
		if err != nil {
			t.Fatalf("Failed to send message %d: %v", i, err)
		}
	}

	t.Logf("Produced %d sequenced messages", numMessages)

	// Consume and verify order
	consumer := client.NewConsumer(c, topic, 0)

	consumed := 0
	expectedSeq := 0
	timeout := time.After(30 * time.Second)

	for consumed < numMessages {
		select {
		case <-timeout:
			t.Fatalf("Timeout: consumed %d/%d messages", consumed, numMessages)
		default:
			messages, err := consumer.Fetch(ctx, 0, 1024*1024)
			if err != nil {
				t.Fatalf("Fetch failed: %v", err)
			}

			for _, msg := range messages {
				// Verify sequence number
				expected := fmt.Sprintf("seq-%05d", expectedSeq)
				if string(msg.Key) != expected {
					t.Errorf("Ordering violation: expected %s, got %s", expected, string(msg.Key))
				}
				expectedSeq++
			}

			consumed += len(messages)

			if len(messages) == 0 {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}

	t.Logf("✓ All %d messages received in correct order", consumed)
}
