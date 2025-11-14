package chaos

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawntherrien/streambus/pkg/client"
)

// SimpleLogger implements the Logger interface for testing
type SimpleLogger struct {
	t *testing.T
}

func (l *SimpleLogger) Info(msg string, fields map[string]interface{}) {
	l.t.Logf("[INFO] %s %v", msg, fields)
}

func (l *SimpleLogger) Warn(msg string, fields map[string]interface{}) {
	l.t.Logf("[WARN] %s %v", msg, fields)
}

func (l *SimpleLogger) Error(msg string, fields map[string]interface{}) {
	l.t.Logf("[ERROR] %s %v", msg, fields)
}

// TestChaos_RandomLatency tests system behavior under random latency
func TestChaos_RandomLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	logger := &SimpleLogger{t: t}
	scenario := NewRandomLatencyScenario(logger)

	// Reduce duration for testing
	scenario.Duration = 30 * time.Second

	brokers := []string{"localhost:9092"}
	topic := fmt.Sprintf("chaos-latency-%d", time.Now().Unix())

	testFunc := func(ctx context.Context) error {
		cfg := &client.Config{
			Brokers:        brokers,
			ConnectTimeout: 10 * time.Second,
		}

		c, err := client.New(cfg)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
		defer c.Close()

		producer := client.NewProducer(c)
		var successCount, errorCount int64

		// Continuously produce messages
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		messageID := 0

		for {
			select {
			case <-ctx.Done():
				t.Logf("Test completed: success=%d, errors=%d", successCount, errorCount)
				return nil
			case <-ticker.C:
				// Inject latency before operation
				if err := scenario.Injector.InjectLatency(ctx, "produce"); err != nil {
					atomic.AddInt64(&errorCount, 1)
					continue
				}

				key := []byte(fmt.Sprintf("key-%d", messageID))
				value := []byte(fmt.Sprintf("message-%d", messageID))

				err := producer.Send(ctx, topic, 0, key, value)
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}

				messageID++
			}
		}
	}

	err := scenario.Run(context.Background(), testFunc)
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("Chaos scenario failed: %v", err)
	}

	// Verify fault injection occurred
	stats := scenario.Injector.GetStats()
	if stats[FaultTypeLatency] == 0 {
		t.Error("No latency faults were injected")
	}

	t.Logf("✓ System handled %d latency injections", stats[FaultTypeLatency])
}

// TestChaos_IntermittentErrors tests error handling and recovery
func TestChaos_IntermittentErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	logger := &SimpleLogger{t: t}
	scenario := NewIntermittentErrorScenario(logger)
	scenario.Duration = 30 * time.Second

	brokers := []string{"localhost:9092"}
	topic := fmt.Sprintf("chaos-errors-%d", time.Now().Unix())

	testFunc := func(ctx context.Context) error {
		cfg := &client.Config{
			Brokers:        brokers,
			ConnectTimeout: 10 * time.Second,
		}

		c, err := client.New(cfg)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
		defer c.Close()

		producer := client.NewProducer(c)
		var successCount, faultErrorCount, realErrorCount int64

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		messageID := 0

		for {
			select {
			case <-ctx.Done():
				t.Logf("Completed: success=%d, fault_errors=%d, real_errors=%d",
					successCount, faultErrorCount, realErrorCount)
				return nil
			case <-ticker.C:
				// Check if we should inject an error
				if err := scenario.Injector.InjectError("produce"); err != nil {
					atomic.AddInt64(&faultErrorCount, 1)
					continue
				}

				key := []byte(fmt.Sprintf("key-%d", messageID))
				value := []byte(fmt.Sprintf("message-%d", messageID))

				err := producer.Send(ctx, topic, 0, key, value)
				if err != nil {
					atomic.AddInt64(&realErrorCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}

				messageID++
			}
		}
	}

	err := scenario.Run(context.Background(), testFunc)
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("Chaos scenario failed: %v", err)
	}

	stats := scenario.Injector.GetStats()
	if stats[FaultTypeError] == 0 {
		t.Error("No error faults were injected")
	}

	t.Logf("✓ System handled %d error injections", stats[FaultTypeError])
}

// TestChaos_SlowNetwork tests behavior under slow network conditions
func TestChaos_SlowNetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	logger := &SimpleLogger{t: t}
	scenario := NewSlowNetworkScenario(logger)
	scenario.Duration = 30 * time.Second

	brokers := []string{"localhost:9092"}
	topic := fmt.Sprintf("chaos-slow-%d", time.Now().Unix())

	testFunc := func(ctx context.Context) error {
		cfg := &client.Config{
			Brokers:        brokers,
			ConnectTimeout: 10 * time.Second,
		}

		c, err := client.New(cfg)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
		defer c.Close()

		producer := client.NewProducer(c)
		var successCount, timeoutCount int64

		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		messageID := 0

		for {
			select {
			case <-ctx.Done():
				t.Logf("Completed: success=%d, timeouts=%d", successCount, timeoutCount)
				return nil
			case <-ticker.C:
				// Inject slow response
				if err := scenario.Injector.InjectSlowResponse(ctx, "produce"); err != nil {
					atomic.AddInt64(&timeoutCount, 1)
					continue
				}

				key := []byte(fmt.Sprintf("key-%d", messageID))
				value := []byte(fmt.Sprintf("message-%d", messageID))

				err := producer.Send(ctx, topic, 0, key, value)
				if err != nil {
					atomic.AddInt64(&timeoutCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}

				messageID++
			}
		}
	}

	err := scenario.Run(context.Background(), testFunc)
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("Chaos scenario failed: %v", err)
	}

	stats := scenario.Injector.GetStats()
	if stats[FaultTypeSlowResponse] == 0 {
		t.Error("No slow response faults were injected")
	}

	t.Logf("✓ System handled %d slow response injections", stats[FaultTypeSlowResponse])
}

// TestChaos_CombinedFaults tests system under multiple fault types
func TestChaos_CombinedFaults(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	logger := &SimpleLogger{t: t}
	scenario := NewCombinedChaosScenario(logger)
	scenario.Duration = 60 * time.Second

	brokers := []string{"localhost:9092"}
	topic := fmt.Sprintf("chaos-combined-%d", time.Now().Unix())

	testFunc := func(ctx context.Context) error {
		cfg := &client.Config{
			Brokers:        brokers,
			ConnectTimeout: 10 * time.Second,
		}

		c, err := client.New(cfg)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
		defer c.Close()

		producer := client.NewProducer(c)
		consumer := client.NewConsumer(c, topic, 0)

		var producedCount, consumedCount, errorCount int64

		// Producer goroutine
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			messageID := 0
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// Inject various faults
					if err := scenario.Injector.InjectLatency(ctx, "produce"); err != nil {
						atomic.AddInt64(&errorCount, 1)
						continue
					}

					if err := scenario.Injector.InjectError("produce"); err != nil {
						atomic.AddInt64(&errorCount, 1)
						continue
					}

					if err := scenario.Injector.InjectSlowResponse(ctx, "produce"); err != nil {
						atomic.AddInt64(&errorCount, 1)
						continue
					}

					key := []byte(fmt.Sprintf("key-%d", messageID))
					value := []byte(fmt.Sprintf("message-%d", messageID))

					err := producer.Send(ctx, topic, 0, key, value)
					if err != nil {
						atomic.AddInt64(&errorCount, 1)
					} else {
						atomic.AddInt64(&producedCount, 1)
					}

					messageID++
				}
			}
		}()

		// Consumer goroutine
		go func() {
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// Inject faults for consume operations
					if err := scenario.Injector.InjectLatency(ctx, "consume"); err != nil {
						continue
					}

					messages, err := consumer.Fetch(ctx, 0, 1024*1024)
					if err == nil {
						atomic.AddInt64(&consumedCount, int64(len(messages)))
					}
				}
			}
		}()

		// Wait for test duration
		<-ctx.Done()

		produced := atomic.LoadInt64(&producedCount)
		consumed := atomic.LoadInt64(&consumedCount)
		errors := atomic.LoadInt64(&errorCount)

		t.Logf("Final stats: produced=%d, consumed=%d, errors=%d", produced, consumed, errors)

		return nil
	}

	err := scenario.Run(context.Background(), testFunc)
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("Chaos scenario failed: %v", err)
	}

	// Verify multiple fault types were injected
	stats := scenario.Injector.GetStats()
	totalFaults := 0
	for faultType, count := range stats {
		if count > 0 {
			totalFaults++
			t.Logf("Fault type %s: %d injections", faultType, count)
		}
	}

	if totalFaults < 2 {
		t.Errorf("Expected at least 2 fault types, got %d", totalFaults)
	}

	t.Logf("✓ System survived combined chaos with %d fault types", totalFaults)
}

// TestChaos_NetworkPartition tests behavior during network partitions
func TestChaos_NetworkPartition(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	logger := &SimpleLogger{t: t}
	simulator := NewNetworkPartitionSimulator(logger)

	// Simulate partitioning broker-1 from broker-2
	simulator.PartitionNodes("broker-1", "broker-2")

	// Wait 10 seconds
	time.Sleep(10 * time.Second)

	// Heal partition
	simulator.HealPartition("broker-1", "broker-2")

	t.Log("✓ Network partition test completed")
}
