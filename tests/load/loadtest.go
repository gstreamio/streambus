package load

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shawntherrien/streambus/pkg/client"
)

// Config holds load test configuration
type Config struct {
	Brokers      []string
	Duration     time.Duration
	WarmupPeriod time.Duration
	RampUpPeriod time.Duration

	Producers     int
	Consumers     int
	MessageSize   int
	Rate          int // messages per second (0 = unlimited)
	BatchSize     int
	Compression   string
	Acks          string

	Topic             string
	Partitions        int
	ReplicationFactor int

	Percentiles []float64
}

// Results holds test results
type Results struct {
	Duration time.Duration

	MessagesProduced uint64
	MessagesFailed   uint64
	MessagesConsumed uint64

	BytesProduced uint64
	BytesConsumed uint64

	LatencyMin   time.Duration
	LatencyMax   time.Duration
	LatencyP50   time.Duration
	LatencyP90   time.Duration
	LatencyP95   time.Duration
	LatencyP99   time.Duration
	LatencyP999  time.Duration

	ProducerErrors uint64
	ConsumerErrors uint64

	Throughput         float64 // messages per second
	ProducerThroughput float64
	ConsumerThroughput float64
	Bandwidth          float64 // MB per second
}

// LoadTest represents a load test instance
type LoadTest struct {
	config  *Config
	results *Results

	producers []*client.Producer
	consumers []*client.Consumer

	latencies []time.Duration
	mu        sync.Mutex

	stopCh chan struct{}
}

// NewLoadTest creates a new load test
func NewLoadTest(config *Config) *LoadTest {
	if config.Percentiles == nil {
		config.Percentiles = []float64{50, 90, 95, 99, 99.9}
	}

	return &LoadTest{
		config:  config,
		results: &Results{},
		stopCh:  make(chan struct{}),
	}
}

// Run executes the load test
func (lt *LoadTest) Run(ctx context.Context) (*Results, error) {
	fmt.Println("=== StreamBus Load Test ===")
	fmt.Printf("Brokers: %v\n", lt.config.Brokers)
	fmt.Printf("Duration: %s\n", lt.config.Duration)
	fmt.Printf("Producers: %d, Consumers: %d\n", lt.config.Producers, lt.config.Consumers)
	fmt.Printf("Message Size: %d bytes\n", lt.config.MessageSize)
	fmt.Printf("Target Rate: %d msgs/sec\n", lt.config.Rate)
	fmt.Println()

	// Setup
	if err := lt.setup(ctx); err != nil {
		return nil, fmt.Errorf("setup failed: %w", err)
	}
	defer lt.cleanup()

	// Warmup
	if lt.config.WarmupPeriod > 0 {
		fmt.Printf("Warming up for %s...\n", lt.config.WarmupPeriod)
		time.Sleep(lt.config.WarmupPeriod)
	}

	// Start test
	startTime := time.Now()

	var wg sync.WaitGroup
	testCtx, cancel := context.WithTimeout(ctx, lt.config.Duration)
	defer cancel()

	// Start producers
	for i := 0; i < lt.config.Producers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			lt.runProducer(testCtx, id)
		}(i)
	}

	// Start consumers
	for i := 0; i < lt.config.Consumers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			lt.runConsumer(testCtx, id)
		}(i)
	}

	// Wait for test to complete
	wg.Wait()

	lt.results.Duration = time.Since(startTime)

	// Calculate final metrics
	lt.calculateMetrics()

	return lt.results, nil
}

func (lt *LoadTest) setup(ctx context.Context) error {
	// TODO: Create topic if it doesn't exist
	// TODO: Initialize producers and consumers
	return nil
}

func (lt *LoadTest) cleanup() {
	// TODO: Close all producers and consumers
}

func (lt *LoadTest) runProducer(ctx context.Context, id int) {
	message := make([]byte, lt.config.MessageSize)
	ticker := time.NewTicker(time.Second / time.Duration(lt.config.Rate/lt.config.Producers))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			start := time.Now()

			// TODO: Send message via producer
			// For now, simulate
			time.Sleep(1 * time.Millisecond)

			latency := time.Since(start)

			atomic.AddUint64(&lt.results.MessagesProduced, 1)
			atomic.AddUint64(&lt.results.BytesProduced, uint64(len(message)))

			lt.recordLatency(latency)
		}
	}
}

func (lt *LoadTest) runConsumer(ctx context.Context, id int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// TODO: Consume message
			// For now, simulate
			time.Sleep(1 * time.Millisecond)

			atomic.AddUint64(&lt.results.MessagesConsumed, 1)
			atomic.AddUint64(&lt.results.BytesConsumed, uint64(lt.config.MessageSize))
		}
	}
}

func (lt *LoadTest) recordLatency(latency time.Duration) {
	lt.mu.Lock()
	lt.latencies = append(lt.latencies, latency)
	lt.mu.Unlock()
}

func (lt *LoadTest) calculateMetrics() {
	duration := lt.results.Duration.Seconds()

	lt.results.ProducerThroughput = float64(lt.results.MessagesProduced) / duration
	lt.results.ConsumerThroughput = float64(lt.results.MessagesConsumed) / duration
	lt.results.Throughput = (lt.results.ProducerThroughput + lt.results.ConsumerThroughput) / 2

	lt.results.Bandwidth = float64(lt.results.BytesProduced) / duration / 1024 / 1024 // MB/s

	// Calculate latency percentiles
	if len(lt.latencies) > 0 {
		// TODO: Calculate percentiles properly
		lt.results.LatencyP50 = lt.latencies[len(lt.latencies)/2]
		lt.results.LatencyP99 = lt.latencies[len(lt.latencies)*99/100]
	}
}

// Print prints the test results
func (r *Results) Print() {
	fmt.Println("\n=== Load Test Results ===")
	fmt.Printf("Duration: %s\n", r.Duration)
	fmt.Println()

	fmt.Println("Messages:")
	fmt.Printf("  Produced: %d (%.0f msgs/sec)\n", r.MessagesProduced, r.ProducerThroughput)
	fmt.Printf("  Consumed: %d (%.0f msgs/sec)\n", r.MessagesConsumed, r.ConsumerThroughput)
	fmt.Printf("  Failed: %d\n", r.MessagesFailed)
	fmt.Println()

	fmt.Println("Throughput:")
	fmt.Printf("  Average: %.0f msgs/sec\n", r.Throughput)
	fmt.Printf("  Bandwidth: %.2f MB/sec\n", r.Bandwidth)
	fmt.Println()

	fmt.Println("Latency:")
	fmt.Printf("  p50: %s\n", r.LatencyP50)
	fmt.Printf("  p90: %s\n", r.LatencyP90)
	fmt.Printf("  p95: %s\n", r.LatencyP95)
	fmt.Printf("  p99: %s\n", r.LatencyP99)
	fmt.Printf("  p99.9: %s\n", r.LatencyP999)
	fmt.Println()

	fmt.Println("Errors:")
	fmt.Printf("  Producer: %d\n", r.ProducerErrors)
	fmt.Printf("  Consumer: %d\n", r.ConsumerErrors)
	fmt.Println("========================")
}
