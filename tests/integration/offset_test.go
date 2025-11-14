package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/client"
	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOffsetManagement verifies that offsets are correctly incremented and tracked
func TestOffsetManagement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := client.DefaultConfig()
	config.Brokers = []string{"localhost:9092"}

	c, err := client.New(config)
	require.NoError(t, err, "Failed to create client")
	defer c.Close()

	topic := fmt.Sprintf("test-offset-mgmt-%d", time.Now().Unix())

	// Create topic first
	err = c.CreateTopic(topic, 1, 1)
	require.NoError(t, err, "Failed to create topic")

	// Wait for topic to be ready
	time.Sleep(1 * time.Second)

	// Produce messages with known content
	producer := client.NewProducer(c)
	defer producer.Close()

	messageCount := 10
	expectedMessages := make(map[int64]string)

	for i := 0; i < messageCount; i++ {
		key := fmt.Sprintf("key-%d", i)
		value := fmt.Sprintf("value-%d", i)

		err := producer.Send(topic, []byte(key), []byte(value))
		require.NoError(t, err, "Failed to send message %d", i)

		expectedMessages[int64(i)] = value
	}

	// Flush to ensure all messages are written
	err = producer.Flush(topic)
	require.NoError(t, err, "Failed to flush producer")

	// Give broker time to persist messages
	time.Sleep(2 * time.Second)

	// Test 1: Consume from beginning and verify offsets are sequential
	t.Run("SequentialOffsets", func(t *testing.T) {
		consumer := client.NewConsumer(c, topic, 0)
		defer consumer.Close()

		// Start from beginning
		err := consumer.Seek(0)
		require.NoError(t, err)

		messages, err := consumer.FetchN(messageCount)
		require.NoError(t, err)
		require.Len(t, messages, messageCount)

		// Verify offsets are 0, 1, 2, ..., n-1
		for i, msg := range messages {
			assert.Equal(t, int64(i), msg.Offset,
				"Message %d should have offset %d but has %d", i, i, msg.Offset)

			// Also verify content matches
			expectedValue := fmt.Sprintf("value-%d", i)
			assert.Equal(t, expectedValue, string(msg.Value),
				"Message at offset %d has wrong value", msg.Offset)
		}
	})

	// Test 2: Seek to specific offset and verify
	t.Run("SeekToOffset", func(t *testing.T) {
		consumer := client.NewConsumer(c, topic, 0)
		defer consumer.Close()

		// Seek to offset 5
		seekOffset := int64(5)
		err := consumer.Seek(seekOffset)
		require.NoError(t, err)

		// Fetch next 3 messages
		messages, err := consumer.FetchN(3)
		require.NoError(t, err)
		require.Len(t, messages, 3)

		// Should get messages with offsets 5, 6, 7
		for i, msg := range messages {
			expectedOffset := seekOffset + int64(i)
			assert.Equal(t, expectedOffset, msg.Offset,
				"After seeking to %d, message %d should have offset %d but has %d",
				seekOffset, i, expectedOffset, msg.Offset)
		}
	})

	// Test 3: Verify offset persistence across consumer restarts
	t.Run("OffsetPersistence", func(t *testing.T) {
		// First consumer reads 3 messages
		consumer1 := client.NewConsumer(c, topic, 0)
		messages1, err := consumer1.FetchN(3)
		require.NoError(t, err)
		require.Len(t, messages1, 3)

		lastOffset := messages1[len(messages1)-1].Offset
		consumer1.Close()

		// New consumer should be able to continue from next offset
		consumer2 := client.NewConsumer(c, topic, 0)
		defer consumer2.Close()

		err = consumer2.Seek(lastOffset + 1)
		require.NoError(t, err)

		messages2, err := consumer2.FetchN(1)
		require.NoError(t, err)
		require.Len(t, messages2, 1)

		// Should get the next message
		assert.Equal(t, lastOffset+1, messages2[0].Offset)
	})

	// Test 4: Produce more messages and verify offsets continue correctly
	t.Run("OffsetContinuation", func(t *testing.T) {
		// Get current high water mark
		consumer := client.NewConsumer(c, topic, 0)
		defer consumer.Close()

		// Seek to end to find the highest offset
		err := consumer.Seek(-1) // Latest
		require.NoError(t, err)

		// Produce 5 more messages
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("new-key-%d", i)
			value := fmt.Sprintf("new-value-%d", i)
			err := producer.Send(topic, []byte(key), []byte(value))
			require.NoError(t, err)
		}

		err = producer.Flush(topic)
		require.NoError(t, err)

		time.Sleep(1 * time.Second)

		// Consume the new messages
		err = consumer.Seek(int64(messageCount))
		require.NoError(t, err)

		newMessages, err := consumer.FetchN(5)
		require.NoError(t, err)
		require.Len(t, newMessages, 5)

		// Verify offsets continue from where they left off
		for i, msg := range newMessages {
			expectedOffset := int64(messageCount + i)
			assert.Equal(t, expectedOffset, msg.Offset,
				"New message %d should have offset %d but has %d",
				i, expectedOffset, msg.Offset)
		}
	})
}

// TestOffsetOutOfRange tests behavior when requesting invalid offsets
func TestOffsetOutOfRange(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := client.DefaultConfig()
	config.Brokers = []string{"localhost:9092"}

	c, err := client.New(config)
	require.NoError(t, err)
	defer c.Close()

	topic := fmt.Sprintf("test-offset-range-%d", time.Now().Unix())

	// Create topic
	err = c.CreateTopic(topic, 1, 1)
	require.NoError(t, err)
	time.Sleep(1 * time.Second)

	// Produce 5 messages
	producer := client.NewProducer(c)
	defer producer.Close()

	for i := 0; i < 5; i++ {
		err := producer.Send(topic, []byte(fmt.Sprintf("key-%d", i)), []byte(fmt.Sprintf("value-%d", i)))
		require.NoError(t, err)
	}
	producer.Flush(topic)
	time.Sleep(1 * time.Second)

	consumer := client.NewConsumer(c, topic, 0)
	defer consumer.Close()

	// Test 1: Seek beyond available messages
	t.Run("SeekBeyondEnd", func(t *testing.T) {
		err := consumer.Seek(1000)
		require.NoError(t, err) // Seek itself might succeed

		messages, err := consumer.FetchN(1)
		// Should either return empty or error
		if err == nil {
			assert.Len(t, messages, 0, "Should not return messages for offset beyond end")
		}
	})

	// Test 2: Negative offset (other than -1 for latest)
	t.Run("NegativeOffset", func(t *testing.T) {
		err := consumer.Seek(-5)
		assert.Error(t, err, "Should error on negative offset")
	})
}

// TestConcurrentOffsetAccess tests that concurrent consumers handle offsets correctly
func TestConcurrentOffsetAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := client.DefaultConfig()
	config.Brokers = []string{"localhost:9092"}

	c, err := client.New(config)
	require.NoError(t, err)
	defer c.Close()

	topic := fmt.Sprintf("test-concurrent-offset-%d", time.Now().Unix())

	// Create topic
	err = c.CreateTopic(topic, 1, 1)
	require.NoError(t, err)
	time.Sleep(1 * time.Second)

	// Produce 100 messages
	producer := client.NewProducer(c)
	defer producer.Close()

	messageCount := 100
	for i := 0; i < messageCount; i++ {
		err := producer.Send(topic, []byte(fmt.Sprintf("key-%d", i)), []byte(fmt.Sprintf("value-%d", i)))
		require.NoError(t, err)
	}
	producer.Flush(topic)
	time.Sleep(2 * time.Second)

	// Create multiple consumers reading different ranges
	type consumerTest struct {
		startOffset int64
		count       int
		messages    []protocol.Message
		err         error
	}

	tests := []consumerTest{
		{startOffset: 0, count: 20},   // Consumer 1: offsets 0-19
		{startOffset: 20, count: 20},  // Consumer 2: offsets 20-39
		{startOffset: 40, count: 20},  // Consumer 3: offsets 40-59
		{startOffset: 60, count: 20},  // Consumer 4: offsets 60-79
		{startOffset: 80, count: 20},  // Consumer 5: offsets 80-99
	}

	// Run consumers concurrently
	done := make(chan int, len(tests))
	for i := range tests {
		go func(idx int) {
			consumer := client.NewConsumer(c, topic, 0)
			defer consumer.Close()

			err := consumer.Seek(tests[idx].startOffset)
			if err != nil {
				tests[idx].err = err
				done <- idx
				return
			}

			messages, err := consumer.FetchN(tests[idx].count)
			tests[idx].messages = messages
			tests[idx].err = err
			done <- idx
		}(i)
	}

	// Wait for all consumers
	for i := 0; i < len(tests); i++ {
		<-done
	}

	// Verify results
	allOffsets := make(map[int64]bool)
	for i, test := range tests {
		require.NoError(t, test.err, "Consumer %d failed", i)
		require.Len(t, test.messages, test.count, "Consumer %d got wrong message count", i)

		// Verify offsets are correct and unique
		for j, msg := range test.messages {
			expectedOffset := test.startOffset + int64(j)
			assert.Equal(t, expectedOffset, msg.Offset,
				"Consumer %d message %d has wrong offset", i, j)

			// Check for duplicates
			assert.False(t, allOffsets[msg.Offset],
				"Duplicate offset %d found", msg.Offset)
			allOffsets[msg.Offset] = true
		}
	}

	// Verify we got all offsets 0-99
	assert.Len(t, allOffsets, messageCount, "Should have exactly %d unique offsets", messageCount)
}

// BenchmarkOffsetTracking benchmarks offset tracking performance
func BenchmarkOffsetTracking(b *testing.B) {
	config := client.DefaultConfig()
	config.Brokers = []string{"localhost:9092"}

	c, err := client.New(config)
	if err != nil {
		b.Skip("Cannot connect to broker")
	}
	defer c.Close()

	topic := fmt.Sprintf("bench-offset-%d", time.Now().Unix())

	// Create topic
	_ = c.CreateTopic(topic, 1, 1)
	time.Sleep(1 * time.Second)

	// Produce test messages
	producer := client.NewProducer(c)
	for i := 0; i < 1000; i++ {
		_ = producer.Send(topic, []byte(fmt.Sprintf("key-%d", i)), []byte(fmt.Sprintf("value-%d", i)))
	}
	_ = producer.Flush(topic)
	producer.Close()
	time.Sleep(1 * time.Second)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		consumer := client.NewConsumer(c, topic, 0)

		// Seek to random offset
		offset := int64(i % 1000)
		_ = consumer.Seek(offset)

		// Fetch one message
		_, _ = consumer.FetchN(1)

		consumer.Close()
	}
}