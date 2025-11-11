package bench

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawntherrien/streambus/pkg/client"
)

// BenchmarkConcurrent_MultiProducer tests concurrent producers
func BenchmarkConcurrent_MultiProducer(b *testing.B) {
	testCases := []struct {
		name        string
		numProducers int
		msgSize     int
	}{
		{"10Producers_SmallMsg", 10, 100},
		{"50Producers_SmallMsg", 50, 100},
		{"100Producers_SmallMsg", 100, 100},
		{"10Producers_MediumMsg", 10, 1024},
		{"50Producers_MediumMsg", 50, 1024},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			benchmarkConcurrentProducers(b, tc.numProducers, tc.msgSize)
		})
	}
}

func benchmarkConcurrentProducers(b *testing.B, numProducers, msgSize int) {
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

	topic := "bench-concurrent-producers"
	value := make([]byte, msgSize)
	for i := range value {
		value[i] = byte(i % 256)
	}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	var wg sync.WaitGroup
	errCh := make(chan error, numProducers)
	var totalSent int64

	messagesPerProducer := b.N / numProducers
	if messagesPerProducer == 0 {
		messagesPerProducer = 1
	}

	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()

			producer, err := c.NewProducer()
			if err != nil {
				errCh <- err
				return
			}

			sent := 0
			for i := 0; i < messagesPerProducer; i++ {
				key := fmt.Sprintf("producer-%d-msg-%d", producerID, i)
				if err := producer.Send(ctx, topic, []byte(key), value); err != nil {
					errCh <- err
					return
				}
				sent++
			}

			atomic.AddInt64(&totalSent, int64(sent))
		}(p)
	}

	wg.Wait()
	close(errCh)

	if err := <-errCh; err != nil {
		b.Fatalf("Producer error: %v", err)
	}

	b.StopTimer()

	totalBytes := totalSent * int64(msgSize)
	throughputMBps := float64(totalBytes) / b.Elapsed().Seconds() / (1024 * 1024)
	messagesPerSec := float64(totalSent) / b.Elapsed().Seconds()

	b.ReportMetric(throughputMBps, "MB/s")
	b.ReportMetric(messagesPerSec, "msgs/s")
	b.ReportMetric(float64(totalSent), "total_msgs")
}

// BenchmarkConcurrent_MultiConsumer tests concurrent consumers in the same group
func BenchmarkConcurrent_MultiConsumer(b *testing.B) {
	testCases := []struct {
		name         string
		numConsumers int
		msgSize      int
		numMessages  int
	}{
		{"5Consumers_SmallMsg", 5, 100, 10000},
		{"10Consumers_SmallMsg", 10, 100, 10000},
		{"5Consumers_MediumMsg", 5, 1024, 10000},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			benchmarkConcurrentConsumers(b, tc.numConsumers, tc.msgSize, tc.numMessages)
		})
	}
}

func benchmarkConcurrentConsumers(b *testing.B, numConsumers, msgSize, numMessages int) {
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

	topic := "bench-concurrent-consumers"

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
	for i := 0; i < numMessages; i++ {
		key := fmt.Sprintf("key-%d", i)
		if err := producer.Send(ctx, topic, []byte(key), value); err != nil {
			b.Fatalf("Failed to produce message: %v", err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	var wg sync.WaitGroup
	errCh := make(chan error, numConsumers)
	var totalConsumed int64

	groupName := "bench-concurrent-consumer-group"

	for c := 0; c < numConsumers; c++ {
		wg.Add(1)
		go func(consumerID int) {
			defer wg.Done()

			consumer, err := c.NewConsumer(groupName)
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
			targetPerConsumer := numMessages / numConsumers

			for consumed < targetPerConsumer {
				msgs, err := consumer.Poll(ctx, 1*time.Second)
				if err != nil {
					errCh <- err
					return
				}
				consumed += len(msgs)
			}

			atomic.AddInt64(&totalConsumed, int64(consumed))
		}(c)
	}

	wg.Wait()
	close(errCh)

	if err := <-errCh; err != nil {
		b.Fatalf("Consumer error: %v", err)
	}

	b.StopTimer()

	totalBytes := totalConsumed * int64(msgSize)
	throughputMBps := float64(totalBytes) / b.Elapsed().Seconds() / (1024 * 1024)
	messagesPerSec := float64(totalConsumed) / b.Elapsed().Seconds()

	b.ReportMetric(throughputMBps, "MB/s")
	b.ReportMetric(messagesPerSec, "msgs/s")
	b.ReportMetric(float64(totalConsumed), "total_msgs")
}

// BenchmarkConcurrent_MixedWorkload tests mixed producer/consumer workload
func BenchmarkConcurrent_MixedWorkload(b *testing.B) {
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

	topic := "bench-mixed-workload"
	msgSize := 1024
	value := make([]byte, msgSize)
	for i := range value {
		value[i] = byte(i % 256)
	}

	ctx := context.Background()
	numProducers := 10
	numConsumers := 10

	b.ResetTimer()
	b.ReportAllocs()

	var wg sync.WaitGroup
	errCh := make(chan error, numProducers+numConsumers)
	var totalProduced, totalConsumed int64

	messagesPerProducer := b.N / numProducers
	if messagesPerProducer == 0 {
		messagesPerProducer = 1
	}

	// Start producers
	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()

			producer, err := c.NewProducer()
			if err != nil {
				errCh <- err
				return
			}

			for i := 0; i < messagesPerProducer; i++ {
				key := fmt.Sprintf("producer-%d-msg-%d", producerID, i)
				if err := producer.Send(ctx, topic, []byte(key), value); err != nil {
					errCh <- err
					return
				}
				atomic.AddInt64(&totalProduced, 1)
			}
		}(p)
	}

	// Start consumers
	for c := 0; c < numConsumers; c++ {
		wg.Add(1)
		go func(consumerID int) {
			defer wg.Done()

			consumer, err := c.NewConsumer(fmt.Sprintf("bench-mixed-group-%d", consumerID))
			if err != nil {
				errCh <- err
				return
			}
			defer consumer.Close()

			if err := consumer.Subscribe(topic); err != nil {
				errCh <- err
				return
			}

			targetMsgs := messagesPerProducer
			consumed := 0

			for consumed < targetMsgs {
				msgs, err := consumer.Poll(ctx, 1*time.Second)
				if err != nil {
					errCh <- err
					return
				}
				consumed += len(msgs)
				atomic.AddInt64(&totalConsumed, int64(len(msgs)))
			}
		}(c)
	}

	wg.Wait()
	close(errCh)

	if err := <-errCh; err != nil {
		b.Fatalf("Workload error: %v", err)
	}

	b.StopTimer()

	b.ReportMetric(float64(totalProduced), "produced")
	b.ReportMetric(float64(totalConsumed), "consumed")
	b.ReportMetric(float64(totalProduced)/b.Elapsed().Seconds(), "produce_rate")
	b.ReportMetric(float64(totalConsumed)/b.Elapsed().Seconds(), "consume_rate")
}

// BenchmarkConcurrent_ProducerContention tests contention on a single topic
func BenchmarkConcurrent_ProducerContention(b *testing.B) {
	testCases := []struct {
		name        string
		numProducers int
	}{
		{"LowContention_5", 5},
		{"MediumContention_20", 20},
		{"HighContention_50", 50},
		{"ExtremeContention_100", 100},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			benchmarkProducerContention(b, tc.numProducers)
		})
	}
}

func benchmarkProducerContention(b *testing.B, numProducers int) {
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

	topic := "bench-contention"
	value := []byte("contention-test-message")
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	var wg sync.WaitGroup
	errCh := make(chan error, numProducers)
	var successCount int64

	messagesPerProducer := b.N / numProducers
	if messagesPerProducer == 0 {
		messagesPerProducer = 1
	}

	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()

			producer, err := c.NewProducer()
			if err != nil {
				errCh <- err
				return
			}

			for i := 0; i < messagesPerProducer; i++ {
				key := fmt.Sprintf("key-%d", i)
				if err := producer.Send(ctx, topic, []byte(key), value); err != nil {
					errCh <- err
					return
				}
				atomic.AddInt64(&successCount, 1)
			}
		}(p)
	}

	wg.Wait()
	close(errCh)

	if err := <-errCh; err != nil {
		b.Fatalf("Producer error: %v", err)
	}

	b.StopTimer()

	throughput := float64(successCount) / b.Elapsed().Seconds()
	b.ReportMetric(throughput, "msgs/s")
	b.ReportMetric(float64(successCount), "total_msgs")
}
