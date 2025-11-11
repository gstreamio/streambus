package bench

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/shawntherrien/streambus/pkg/client"
	"github.com/shawntherrien/streambus/pkg/storage"
)

// BenchmarkE2E_ProducerThroughput measures end-to-end producer throughput
// with different message sizes and batch configurations
func BenchmarkE2E_ProducerThroughput(b *testing.B) {
	testCases := []struct {
		name       string
		msgSize    int
		batchSize  int
		numThreads int
	}{
		{"SmallMsg_SingleThread", 100, 1, 1},
		{"SmallMsg_Batched", 100, 100, 1},
		{"MediumMsg_SingleThread", 1024, 1, 1},
		{"MediumMsg_Batched", 1024, 100, 1},
		{"LargeMsg_SingleThread", 10240, 1, 1},
		{"LargeMsg_Batched", 10240, 100, 1},
		{"SmallMsg_Concurrent", 100, 100, 10},
		{"MediumMsg_Concurrent", 1024, 100, 10},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			benchmarkProducerThroughput(b, tc.msgSize, tc.batchSize, tc.numThreads)
		})
	}
}

func benchmarkProducerThroughput(b *testing.B, msgSize, batchSize, numThreads int) {
	// Skip if broker not available (unit test mode)
	if testing.Short() {
		b.Skip("Skipping integration benchmark in short mode")
	}

	cfg := &client.Config{
		Brokers: []string{"localhost:9092"},
		Timeout: 10 * time.Second,
	}

	c, err := client.NewClient(cfg)
	if err != nil {
		b.Skipf("Cannot connect to broker: %v", err)
		return
	}
	defer c.Close()

	topic := "bench-throughput"
	value := make([]byte, msgSize)
	for i := range value {
		value[i] = byte(i % 256)
	}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	var wg sync.WaitGroup
	errCh := make(chan error, numThreads)

	messagesPerThread := b.N / numThreads
	if messagesPerThread == 0 {
		messagesPerThread = 1
	}

	for t := 0; t < numThreads; t++ {
		wg.Add(1)
		go func(threadID int) {
			defer wg.Done()

			producer, err := c.NewProducer()
			if err != nil {
				errCh <- err
				return
			}

			for i := 0; i < messagesPerThread; i++ {
				key := fmt.Sprintf("key-%d-%d", threadID, i)
				if err := producer.Send(ctx, topic, []byte(key), value); err != nil {
					errCh <- err
					return
				}
			}
		}(t)
	}

	wg.Wait()
	close(errCh)

	if err := <-errCh; err != nil {
		b.Fatalf("Producer error: %v", err)
	}

	b.StopTimer()

	// Calculate throughput
	totalBytes := int64(b.N) * int64(msgSize)
	throughputMBps := float64(totalBytes) / b.Elapsed().Seconds() / (1024 * 1024)
	messagesPerSec := float64(b.N) / b.Elapsed().Seconds()

	b.ReportMetric(throughputMBps, "MB/s")
	b.ReportMetric(messagesPerSec, "msgs/s")
}

// BenchmarkE2E_ConsumerThroughput measures end-to-end consumer throughput
func BenchmarkE2E_ConsumerThroughput(b *testing.B) {
	testCases := []struct {
		name       string
		msgSize    int
		numMsgs    int
		batchSize  int
		numThreads int
	}{
		{"SmallMsg_SingleThread", 100, 10000, 100, 1},
		{"MediumMsg_SingleThread", 1024, 10000, 100, 1},
		{"LargeMsg_SingleThread", 10240, 1000, 100, 1},
		{"SmallMsg_Concurrent", 100, 10000, 100, 5},
		{"MediumMsg_Concurrent", 1024, 10000, 100, 5},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			benchmarkConsumerThroughput(b, tc.msgSize, tc.numMsgs, tc.batchSize, tc.numThreads)
		})
	}
}

func benchmarkConsumerThroughput(b *testing.B, msgSize, numMsgs, batchSize, numThreads int) {
	if testing.Short() {
		b.Skip("Skipping integration benchmark in short mode")
	}

	cfg := &client.Config{
		Brokers: []string{"localhost:9092"},
		Timeout: 10 * time.Second,
	}

	c, err := client.NewClient(cfg)
	if err != nil {
		b.Skipf("Cannot connect to broker: %v", err)
		return
	}
	defer c.Close()

	topic := "bench-consumer-throughput"

	// Pre-populate messages
	producer, err := c.NewProducer()
	if err != nil {
		b.Fatalf("Failed to create producer: %v", err)
	}

	value := make([]byte, msgSize)
	for i := range value {
		value[i] = byte(i % 256)
	}

	ctx := context.Background()
	for i := 0; i < numMsgs; i++ {
		key := fmt.Sprintf("key-%d", i)
		if err := producer.Send(ctx, topic, []byte(key), value); err != nil {
			b.Fatalf("Failed to produce message: %v", err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	var wg sync.WaitGroup
	errCh := make(chan error, numThreads)
	totalConsumed := int64(0)
	var consumedMu sync.Mutex

	messagesPerThread := numMsgs / numThreads
	if messagesPerThread == 0 {
		messagesPerThread = numMsgs
	}

	for t := 0; t < numThreads; t++ {
		wg.Add(1)
		go func(threadID int) {
			defer wg.Done()

			consumer, err := c.NewConsumer(fmt.Sprintf("bench-consumer-group-%d", threadID))
			if err != nil {
				errCh <- err
				return
			}
			defer consumer.Close()

			if err := consumer.Subscribe(topic); err != nil {
				errCh <- err
				return
			}

			consumed := 0
			for consumed < messagesPerThread {
				msgs, err := consumer.Poll(ctx, 1*time.Second)
				if err != nil {
					errCh <- err
					return
				}
				consumed += len(msgs)
			}

			consumedMu.Lock()
			totalConsumed += int64(consumed)
			consumedMu.Unlock()
		}(t)
	}

	wg.Wait()
	close(errCh)

	if err := <-errCh; err != nil {
		b.Fatalf("Consumer error: %v", err)
	}

	b.StopTimer()

	// Calculate throughput
	totalBytes := totalConsumed * int64(msgSize)
	throughputMBps := float64(totalBytes) / b.Elapsed().Seconds() / (1024 * 1024)
	messagesPerSec := float64(totalConsumed) / b.Elapsed().Seconds()

	b.ReportMetric(throughputMBps, "MB/s")
	b.ReportMetric(messagesPerSec, "msgs/s")
}

// BenchmarkStorage_AppendThroughput measures storage layer append throughput
func BenchmarkStorage_AppendThroughput(b *testing.B) {
	testCases := []struct {
		name      string
		msgSize   int
		batchSize int
	}{
		{"SmallMsg_SingleBatch", 100, 1},
		{"SmallMsg_LargeBatch", 100, 1000},
		{"MediumMsg_SingleBatch", 1024, 1},
		{"MediumMsg_LargeBatch", 1024, 1000},
		{"LargeMsg_SingleBatch", 10240, 1},
		{"LargeMsg_MediumBatch", 10240, 100},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			benchmarkStorageAppend(b, tc.msgSize, tc.batchSize)
		})
	}
}

func benchmarkStorageAppend(b *testing.B, msgSize, batchSize int) {
	dir := b.TempDir()

	config := storage.Config{
		DataDir: dir,
		WAL: storage.WALConfig{
			SegmentSize:   1024 * 1024 * 100, // 100MB
			FsyncPolicy:   storage.FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: storage.MemTableConfig{
			MaxSize:      1024 * 1024 * 100, // 100MB
			NumImmutable: 2,
		},
	}

	log, err := storage.NewLog(dir, config)
	if err != nil {
		b.Fatalf("Failed to create log: %v", err)
	}
	defer log.Close()

	value := make([]byte, msgSize)
	for i := range value {
		value[i] = byte(i % 256)
	}

	messages := make([]storage.Message, batchSize)
	for i := range messages {
		messages[i] = storage.Message{
			Key:   []byte(fmt.Sprintf("key-%d", i)),
			Value: value,
		}
	}

	batch := &storage.MessageBatch{
		Messages: messages,
	}

	b.ResetTimer()
	b.ReportAllocs()

	numBatches := b.N / batchSize
	if numBatches == 0 {
		numBatches = 1
	}

	for i := 0; i < numBatches; i++ {
		if _, err := log.Append(batch); err != nil {
			b.Fatalf("Append failed: %v", err)
		}
	}

	b.StopTimer()

	// Calculate metrics
	totalMessages := numBatches * batchSize
	totalBytes := int64(totalMessages) * int64(msgSize)
	throughputMBps := float64(totalBytes) / b.Elapsed().Seconds() / (1024 * 1024)
	messagesPerSec := float64(totalMessages) / b.Elapsed().Seconds()

	b.ReportMetric(throughputMBps, "MB/s")
	b.ReportMetric(messagesPerSec, "msgs/s")
}
