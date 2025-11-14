package bench

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/shawntherrien/streambus/pkg/client"
)

// BenchmarkE2E_ProduceLatency measures end-to-end produce latency distribution
func BenchmarkE2E_ProduceLatency(b *testing.B) {
	testCases := []struct {
		name    string
		msgSize int
	}{
		{"SmallMsg_100B", 100},
		{"MediumMsg_1KB", 1024},
		{"LargeMsg_10KB", 10240},
		{"XLargeMsg_100KB", 102400},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			benchmarkProduceLatency(b, tc.msgSize)
		})
	}
}

func benchmarkProduceLatency(b *testing.B, msgSize int) {
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

	producer, err := c.NewProducer()
	if err != nil {
		b.Fatalf("Failed to create producer: %v", err)
	}

	topic := "bench-latency"
	value := make([]byte, msgSize)
	for i := range value {
		value[i] = byte(i % 256)
	}

	ctx := context.Background()

	// Warm up
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("warmup-%d", i)
		producer.Send(ctx, topic, []byte(key), value)
	}

	// Collect latency samples
	latencies := make([]time.Duration, 0, b.N)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		start := time.Now()
		if err := producer.Send(ctx, topic, []byte(key), value); err != nil {
			b.Fatalf("Send failed: %v", err)
		}
		latency := time.Since(start)
		latencies = append(latencies, latency)
	}

	b.StopTimer()

	// Calculate percentiles
	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]
	p999 := latencies[len(latencies)*999/1000]

	var sum time.Duration
	for _, lat := range latencies {
		sum += lat
	}
	avg := sum / time.Duration(len(latencies))

	b.ReportMetric(float64(avg.Microseconds()), "avg_µs")
	b.ReportMetric(float64(p50.Microseconds()), "p50_µs")
	b.ReportMetric(float64(p95.Microseconds()), "p95_µs")
	b.ReportMetric(float64(p99.Microseconds()), "p99_µs")
	b.ReportMetric(float64(p999.Microseconds()), "p999_µs")
}

// BenchmarkE2E_ConsumeLatency measures end-to-end consume latency
func BenchmarkE2E_ConsumeLatency(b *testing.B) {
	testCases := []struct {
		name    string
		msgSize int
	}{
		{"SmallMsg_100B", 100},
		{"MediumMsg_1KB", 1024},
		{"LargeMsg_10KB", 10240},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			benchmarkConsumeLatency(b, tc.msgSize)
		})
	}
}

func benchmarkConsumeLatency(b *testing.B, msgSize int) {
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

	topic := "bench-consume-latency"

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
	numMessages := b.N
	if numMessages < 1000 {
		numMessages = 1000
	}

	for i := 0; i < numMessages; i++ {
		key := fmt.Sprintf("key-%d", i)
		if err := producer.Send(ctx, topic, []byte(key), value); err != nil {
			b.Fatalf("Failed to produce message: %v", err)
		}
	}

	// Create consumer
	consumer, err := c.NewConsumer("bench-latency-group")
	if err != nil {
		b.Fatalf("Failed to create consumer: %v", err)
	}
	defer consumer.Close()

	if err := consumer.Subscribe(topic); err != nil {
		b.Fatalf("Failed to subscribe: %v", err)
	}

	latencies := make([]time.Duration, 0, b.N)

	b.ResetTimer()

	consumed := 0
	for consumed < b.N {
		start := time.Now()
		msgs, err := consumer.Poll(ctx, 100*time.Millisecond)
		if err != nil {
			b.Fatalf("Poll failed: %v", err)
		}

		latency := time.Since(start)
		if len(msgs) > 0 {
			// Average latency per message in this poll
			avgLatency := latency / time.Duration(len(msgs))
			for range msgs {
				latencies = append(latencies, avgLatency)
			}
			consumed += len(msgs)
		}
	}

	b.StopTimer()

	// Calculate percentiles
	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	if len(latencies) == 0 {
		b.Fatal("No latency samples collected")
	}

	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]

	var sum time.Duration
	for _, lat := range latencies {
		sum += lat
	}
	avg := sum / time.Duration(len(latencies))

	b.ReportMetric(float64(avg.Microseconds()), "avg_µs")
	b.ReportMetric(float64(p50.Microseconds()), "p50_µs")
	b.ReportMetric(float64(p95.Microseconds()), "p95_µs")
	b.ReportMetric(float64(p99.Microseconds()), "p99_µs")
}

// BenchmarkE2E_RoundTripLatency measures full round-trip latency (produce + consume)
func BenchmarkE2E_RoundTripLatency(b *testing.B) {
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

	producer, err := c.NewProducer()
	if err != nil {
		b.Fatalf("Failed to create producer: %v", err)
	}

	consumer, err := c.NewConsumer("bench-roundtrip-group")
	if err != nil {
		b.Fatalf("Failed to create consumer: %v", err)
	}
	defer consumer.Close()

	topic := "bench-roundtrip"
	if err := consumer.Subscribe(topic); err != nil {
		b.Fatalf("Failed to subscribe: %v", err)
	}

	ctx := context.Background()
	value := []byte("roundtrip-test-message")

	latencies := make([]time.Duration, 0, b.N)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)

		// Produce
		start := time.Now()
		if err := producer.Send(ctx, topic, []byte(key), value); err != nil {
			b.Fatalf("Send failed: %v", err)
		}

		// Consume
		msgs, err := consumer.Poll(ctx, 5*time.Second)
		if err != nil {
			b.Fatalf("Poll failed: %v", err)
		}

		if len(msgs) > 0 {
			latency := time.Since(start)
			latencies = append(latencies, latency)
		}
	}

	b.StopTimer()

	// Calculate statistics
	if len(latencies) == 0 {
		b.Fatal("No round-trip samples collected")
	}

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]

	var sum time.Duration
	for _, lat := range latencies {
		sum += lat
	}
	avg := sum / time.Duration(len(latencies))

	b.ReportMetric(float64(avg.Milliseconds()), "avg_ms")
	b.ReportMetric(float64(p50.Milliseconds()), "p50_ms")
	b.ReportMetric(float64(p95.Milliseconds()), "p95_ms")
	b.ReportMetric(float64(p99.Milliseconds()), "p99_ms")
}
