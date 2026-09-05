package bench

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/client"
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

	c, err := newBenchClient([]string{"localhost:9092"}, 10*time.Second)
	if err != nil {
		b.Skipf("Cannot connect to broker: %v", err)
		return
	}
	defer c.Close()

	producer := client.NewProducer(c)

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

	c, err := newBenchClient([]string{"localhost:9092"}, 10*time.Second)
	if err != nil {
		b.Skipf("Cannot connect to broker: %v", err)
		return
	}
	defer c.Close()

	topic := "bench-consume-latency"

	// Pre-populate messages
	producer := client.NewProducer(c)

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
	gcConfig := client.DefaultGroupConsumerConfig()
	gcConfig.GroupID = "bench-latency-group"
	gcConfig.Topics = []string{topic}

	consumer, err := client.NewGroupConsumer(c, gcConfig)
	if err != nil {
		b.Fatalf("Failed to create consumer: %v", err)
	}
	defer consumer.Close()

	if err := consumer.Subscribe(ctx); err != nil {
		b.Fatalf("Failed to subscribe: %v", err)
	}

	latencies := make([]time.Duration, 0, b.N)

	b.ResetTimer()

	consumed := 0
	for consumed < b.N {
		start := time.Now()
		result, err := consumer.Poll(ctx)
		if err != nil {
			b.Fatalf("Poll failed: %v", err)
		}

		latency := time.Since(start)
		n := countMessages(result)
		if n > 0 {
			// Average latency per message in this poll
			avgLatency := latency / time.Duration(n)
			for i := 0; i < n; i++ {
				latencies = append(latencies, avgLatency)
			}
			consumed += n
		} else {
			// GroupConsumer.Poll returns immediately regardless of whether
			// anything was available, so back off before retrying instead of
			// spinning the CPU waiting for the next message.
			time.Sleep(pollBackoff)
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

	c, err := newBenchClient([]string{"localhost:9092"}, 10*time.Second)
	if err != nil {
		b.Skipf("Cannot connect to broker: %v", err)
		return
	}
	defer c.Close()

	producer := client.NewProducer(c)

	topic := "bench-roundtrip"
	ctx := context.Background()

	gcConfig := client.DefaultGroupConsumerConfig()
	gcConfig.GroupID = "bench-roundtrip-group"
	gcConfig.Topics = []string{topic}

	consumer, err := client.NewGroupConsumer(c, gcConfig)
	if err != nil {
		b.Fatalf("Failed to create consumer: %v", err)
	}
	defer consumer.Close()

	if err := consumer.Subscribe(ctx); err != nil {
		b.Fatalf("Failed to subscribe: %v", err)
	}

	value := []byte("roundtrip-test-message")

	latencies := make([]time.Duration, 0, b.N)

	b.ResetTimer()

	const roundTripDeadline = 5 * time.Second

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)

		// Produce
		start := time.Now()
		if err := producer.Send(ctx, topic, []byte(key), value); err != nil {
			b.Fatalf("Send failed: %v", err)
		}

		// Consume: GroupConsumer.Poll does one fetch pass and returns
		// immediately rather than blocking for up to a given timeout, so wait
		// for the message to show up by polling in a loop with our own
		// deadline instead.
		received := 0
		deadline := start.Add(roundTripDeadline)
		for time.Now().Before(deadline) {
			result, err := consumer.Poll(ctx)
			if err != nil {
				b.Fatalf("Poll failed: %v", err)
			}
			received = countMessages(result)
			if received > 0 {
				break
			}
			time.Sleep(pollBackoff)
		}

		if received > 0 {
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
