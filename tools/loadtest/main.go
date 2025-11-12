package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gstreamio/streambus/pkg/client"
	"github.com/gstreamio/streambus/pkg/logging"
)

type LoadTestConfig struct {
	Brokers          []string
	Topic            string
	NumProducers     int
	NumConsumers     int
	MessageSize      int
	MessagesPerSec   int
	Duration         time.Duration
	BatchSize        int
	Compression      string
	ReportInterval   time.Duration
	ConsumerGroupID  string
	UseTransactions  bool
}

type LoadTestStats struct {
	MessagesSent     int64
	MessagesReceived int64
	BytesSent        int64
	BytesReceived    int64
	Errors           int64
	TotalLatency     int64
	LatencyCount     int64
}

func main() {
	config := parseFlags()

	logger := logging.New(&logging.Config{
		Level: logging.LevelInfo,
	})

	logger.Info("Starting load test", logging.Fields{
		"brokers":      config.Brokers,
		"topic":        config.Topic,
		"producers":    config.NumProducers,
		"consumers":    config.NumConsumers,
		"message_size": config.MessageSize,
		"target_rate":  config.MessagesPerSec,
		"duration":     config.Duration,
	})

	// Create stats
	stats := &LoadTestStats{}

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start stats reporter
	go reportStats(ctx, stats, config, logger)

	// Start load test
	var wg sync.WaitGroup

	// Start producers
	for i := 0; i < config.NumProducers; i++ {
		wg.Add(1)
		go runProducer(ctx, &wg, i, config, stats, logger)
	}

	// Start consumers
	for i := 0; i < config.NumConsumers; i++ {
		wg.Add(1)
		go runConsumer(ctx, &wg, i, config, stats, logger)
	}

	// Wait for duration or signal
	select {
	case <-time.After(config.Duration):
		logger.Info("Load test duration completed")
		cancel()
	case <-sigChan:
		logger.Info("Received interrupt signal, stopping load test")
		cancel()
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Print final stats
	printFinalStats(stats, config, logger)
}

func parseFlags() *LoadTestConfig {
	config := &LoadTestConfig{}

	var brokerList string
	flag.StringVar(&brokerList, "brokers", "localhost:9092", "Comma-separated list of broker addresses")
	flag.StringVar(&config.Topic, "topic", "loadtest", "Topic to use for load test")
	flag.IntVar(&config.NumProducers, "producers", 1, "Number of producer threads")
	flag.IntVar(&config.NumConsumers, "consumers", 1, "Number of consumer threads")
	flag.IntVar(&config.MessageSize, "message-size", 1024, "Size of each message in bytes")
	flag.IntVar(&config.MessagesPerSec, "rate", 1000, "Target messages per second (across all producers)")
	flag.DurationVar(&config.Duration, "duration", 60*time.Second, "Duration of load test")
	flag.IntVar(&config.BatchSize, "batch-size", 100, "Number of messages per batch")
	flag.StringVar(&config.Compression, "compression", "none", "Compression: none, gzip, snappy, lz4")
	flag.DurationVar(&config.ReportInterval, "report-interval", 10*time.Second, "Stats reporting interval")
	flag.StringVar(&config.ConsumerGroupID, "consumer-group", "loadtest-group", "Consumer group ID")
	flag.BoolVar(&config.UseTransactions, "transactions", false, "Use transactional producers")
	flag.Parse()

	// Parse broker list
	config.Brokers = []string{brokerList}
	if brokerList != "" {
		// Split by comma if provided
		config.Brokers = []string{brokerList} // Simplified for now
	}

	return config
}

func runProducer(ctx context.Context, wg *sync.WaitGroup, id int, config *LoadTestConfig, stats *LoadTestStats, logger *logging.Logger) {
	defer wg.Done()

	// Create client
	clientCfg := &client.Config{
		Brokers:        config.Brokers,
		ConnectTimeout: 30 * time.Second,
	}

	c, err := client.New(clientCfg)
	if err != nil {
		logger.Error("Failed to create client", err, logging.Fields{
			"producer_id": id,
		})
		atomic.AddInt64(&stats.Errors, 1)
		return
	}
	defer c.Close()

	producer := client.NewProducer(c)
	if producer == nil {
		logger.Error("Failed to create producer", fmt.Errorf("producer is nil"), logging.Fields{
			"producer_id": id,
		})
		atomic.AddInt64(&stats.Errors, 1)
		return
	}

	// Calculate rate per producer
	targetRatePerProducer := config.MessagesPerSec / config.NumProducers
	if targetRatePerProducer == 0 {
		targetRatePerProducer = 1
	}

	// Rate limiting
	ticker := time.NewTicker(time.Second / time.Duration(targetRatePerProducer))
	defer ticker.Stop()

	// Generate message value
	value := make([]byte, config.MessageSize)
	for i := range value {
		value[i] = byte(rand.Intn(256))
	}

	messageCount := 0

	logger.Info("Producer started", logging.Fields{
		"producer_id": id,
		"target_rate": targetRatePerProducer,
	})

	for {
		select {
		case <-ctx.Done():
			logger.Info("Producer stopping", logging.Fields{
				"producer_id":     id,
				"messages_sent":   messageCount,
			})
			return
		case <-ticker.C:
			key := []byte(fmt.Sprintf("producer-%d-msg-%d", id, messageCount))

			start := time.Now()
			err := producer.Send(config.Topic, key, value)
			latency := time.Since(start)

			if err != nil {
				atomic.AddInt64(&stats.Errors, 1)
				logger.Error("Send failed", err, logging.Fields{
					"producer_id": id,
				})
				continue
			}

			atomic.AddInt64(&stats.MessagesSent, 1)
			atomic.AddInt64(&stats.BytesSent, int64(len(key)+len(value)))
			atomic.AddInt64(&stats.TotalLatency, latency.Microseconds())
			atomic.AddInt64(&stats.LatencyCount, 1)
			messageCount++
		}
	}
}

func runConsumer(ctx context.Context, wg *sync.WaitGroup, id int, config *LoadTestConfig, stats *LoadTestStats, logger *logging.Logger) {
	defer wg.Done()

	// Create client
	clientCfg := &client.Config{
		Brokers:        config.Brokers,
		ConnectTimeout: 30 * time.Second,
	}

	c, err := client.New(clientCfg)
	if err != nil {
		logger.Error("Failed to create client", err, logging.Fields{
			"consumer_id": id,
		})
		atomic.AddInt64(&stats.Errors, 1)
		return
	}
	defer c.Close()

	groupID := fmt.Sprintf("%s-%d", config.ConsumerGroupID, id)
	consumer := client.NewConsumer(c, config.Topic, 0)
	if consumer == nil {
		logger.Error("Failed to create consumer", fmt.Errorf("consumer is nil"), logging.Fields{
			"consumer_id": id,
		})
		atomic.AddInt64(&stats.Errors, 1)
		return
	}
	defer consumer.Close()

	_ = groupID // Suppress unused variable warning

	logger.Info("Consumer started", logging.Fields{
		"consumer_id": id,
		"group_id":    groupID,
	})

	messageCount := 0

	for {
		select {
		case <-ctx.Done():
			logger.Info("Consumer stopping", logging.Fields{
				"consumer_id":       id,
				"messages_received": messageCount,
			})
			return
		default:
			msgs, err := consumer.Fetch()
			if err != nil {
				if ctx.Err() != nil {
					// Context cancelled, normal shutdown
					return
				}
				atomic.AddInt64(&stats.Errors, 1)
				logger.Error("Fetch failed", err, logging.Fields{
					"consumer_id": id,
				})
				time.Sleep(100 * time.Millisecond)
				continue
			}

			if len(msgs) > 0 {
				for _, msg := range msgs {
					atomic.AddInt64(&stats.MessagesReceived, 1)
					atomic.AddInt64(&stats.BytesReceived, int64(len(msg.Key)+len(msg.Value)))
					messageCount++
				}
			} else {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

func reportStats(ctx context.Context, stats *LoadTestStats, config *LoadTestConfig, logger *logging.Logger) {
	ticker := time.NewTicker(config.ReportInterval)
	defer ticker.Stop()

	var lastSent, lastReceived, lastBytesSent, lastBytesReceived int64
	lastTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			elapsed := now.Sub(lastTime).Seconds()

			sent := atomic.LoadInt64(&stats.MessagesSent)
			received := atomic.LoadInt64(&stats.MessagesReceived)
			bytesSent := atomic.LoadInt64(&stats.BytesSent)
			bytesReceived := atomic.LoadInt64(&stats.BytesReceived)
			errors := atomic.LoadInt64(&stats.Errors)
			totalLatency := atomic.LoadInt64(&stats.TotalLatency)
			latencyCount := atomic.LoadInt64(&stats.LatencyCount)

			// Calculate rates
			sendRate := float64(sent-lastSent) / elapsed
			receiveRate := float64(received-lastReceived) / elapsed
			sendThroughput := float64(bytesSent-lastBytesSent) / elapsed / (1024 * 1024) // MB/s
			receiveThroughput := float64(bytesReceived-lastBytesReceived) / elapsed / (1024 * 1024)

			// Calculate average latency
			var avgLatency float64
			if latencyCount > 0 {
				avgLatency = float64(totalLatency) / float64(latencyCount) / 1000.0 // Convert to ms
			}

			logger.Info("Load test stats", logging.Fields{
				"sent":                sent,
				"received":            received,
				"send_rate":           fmt.Sprintf("%.0f msgs/s", sendRate),
				"receive_rate":        fmt.Sprintf("%.0f msgs/s", receiveRate),
				"send_throughput":     fmt.Sprintf("%.2f MB/s", sendThroughput),
				"receive_throughput":  fmt.Sprintf("%.2f MB/s", receiveThroughput),
				"avg_latency":         fmt.Sprintf("%.2f ms", avgLatency),
				"errors":              errors,
				"lag":                 sent - received,
			})

			lastSent = sent
			lastReceived = received
			lastBytesSent = bytesSent
			lastBytesReceived = bytesReceived
			lastTime = now
		}
	}
}

func printFinalStats(stats *LoadTestStats, config *LoadTestConfig, logger *logging.Logger) {
	sent := atomic.LoadInt64(&stats.MessagesSent)
	received := atomic.LoadInt64(&stats.MessagesReceived)
	bytesSent := atomic.LoadInt64(&stats.BytesSent)
	bytesReceived := atomic.LoadInt64(&stats.BytesReceived)
	errors := atomic.LoadInt64(&stats.Errors)
	totalLatency := atomic.LoadInt64(&stats.TotalLatency)
	latencyCount := atomic.LoadInt64(&stats.LatencyCount)

	// Calculate overall rates
	sendRate := float64(sent) / config.Duration.Seconds()
	receiveRate := float64(received) / config.Duration.Seconds()
	sendThroughput := float64(bytesSent) / config.Duration.Seconds() / (1024 * 1024)
	receiveThroughput := float64(bytesReceived) / config.Duration.Seconds() / (1024 * 1024)

	var avgLatency float64
	if latencyCount > 0 {
		avgLatency = float64(totalLatency) / float64(latencyCount) / 1000.0
	}

	fmt.Println("\n========================================")
	fmt.Println("Load Test Complete")
	fmt.Println("========================================")
	fmt.Printf("Duration:              %v\n", config.Duration)
	fmt.Printf("Messages Sent:         %d\n", sent)
	fmt.Printf("Messages Received:     %d\n", received)
	fmt.Printf("Message Loss:          %d (%.2f%%)\n", sent-received, float64(sent-received)/float64(sent)*100)
	fmt.Printf("Errors:                %d\n", errors)
	fmt.Println("----------------------------------------")
	fmt.Printf("Send Rate:             %.0f msgs/s\n", sendRate)
	fmt.Printf("Receive Rate:          %.0f msgs/s\n", receiveRate)
	fmt.Printf("Send Throughput:       %.2f MB/s\n", sendThroughput)
	fmt.Printf("Receive Throughput:    %.2f MB/s\n", receiveThroughput)
	fmt.Printf("Average Latency:       %.2f ms\n", avgLatency)
	fmt.Println("========================================")

	logger.Info("Load test completed", logging.Fields{
		"total_sent":       sent,
		"total_received":   received,
		"send_rate":        fmt.Sprintf("%.0f msgs/s", sendRate),
		"send_throughput":  fmt.Sprintf("%.2f MB/s", sendThroughput),
		"avg_latency_ms":   fmt.Sprintf("%.2f", avgLatency),
		"errors":           errors,
	})
}
