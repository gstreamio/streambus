package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/shawntherrien/streambus/pkg/broker"
	"github.com/shawntherrien/streambus/pkg/client"
	"github.com/shawntherrien/streambus/pkg/metrics"
)

func main() {
	fmt.Println("StreamBus Observability Example")
	fmt.Println("================================")
	fmt.Println()

	// Create data directory
	dataDir := filepath.Join(os.TempDir(), "streambus-observability-example")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}
	defer os.RemoveAll(dataDir)

	// Example 1: Create metrics registry
	fmt.Println("1. Creating Metrics Registry")
	metricsRegistry := metrics.NewRegistry()
	fmt.Println("   - Metrics registry created")

	// Example 2: Initialize StreamBus metrics
	fmt.Println("\n2. Initializing StreamBus Metrics")
	streambusMetrics := metrics.NewStreamBusMetrics(metricsRegistry)
	fmt.Printf("   - Created %d broker metrics\n", 4)
	fmt.Printf("   - Created %d message metrics\n", 6)
	fmt.Printf("   - Created %d performance metrics\n", 4)
	fmt.Printf("   - Created %d security metrics\n", 5)
	fmt.Printf("   - Created %d cluster metrics\n", 4)
	fmt.Println("   - All StreamBus metrics initialized")

	// Example 3: Start Prometheus HTTP server
	fmt.Println("\n3. Starting Prometheus HTTP Server")
	metricsHandler := metrics.NewHandler(metricsRegistry)
	http.Handle("/metrics", metricsHandler)

	go func() {
		fmt.Println("   - Metrics endpoint: http://localhost:9090/metrics")
		if err := http.ListenAndServe(":9090", nil); err != nil {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	// Example 4: Create and start broker
	fmt.Println("\n4. Creating Broker with Metrics")
	brokerConfig := &broker.Config{
		BrokerID:    1,
		Host:        "localhost",
		Port:        9092,
		DataDir:     filepath.Join(dataDir, "broker-data"),
		RaftDataDir: filepath.Join(dataDir, "raft-data"),
		LogLevel:    "info",
	}

	b, err := broker.New(brokerConfig)
	if err != nil {
		log.Fatalf("Failed to create broker: %v", err)
	}

	fmt.Println("   - Broker created successfully")

	if err := b.Start(); err != nil {
		log.Fatalf("Failed to start broker: %v", err)
	}
	fmt.Println("   - Broker started")

	// Update broker uptime metric
	startTime := time.Now()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			<-ticker.C
			uptime := time.Since(startTime).Seconds()
			streambusMetrics.BrokerUptime.Set(uptime)
			streambusMetrics.BrokerStatus.Set(2) // 2 = running
		}
	}()

	// Example 5: Create client and topic
	fmt.Println("\n5. Creating Client and Topic")

	clientConfig := client.DefaultConfig()
	clientConfig.Brokers = []string{"localhost:9092"}

	c, err := client.New(clientConfig)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	// Create topic
	err = c.CreateTopic("metrics-demo", 3, 1) // 3 partitions, replication factor 1
	if err != nil {
		log.Printf("Topic may already exist: %v", err)
	}
	fmt.Println("   - Topic 'metrics-demo' created")

	// Update topic metrics
	streambusMetrics.TopicsTotal.Set(1)
	streambusMetrics.PartitionsTotal.Set(3)
	streambusMetrics.ReplicasTotal.Set(3)

	// Example 6: Produce messages and track metrics
	fmt.Println("\n6. Producing Messages and Tracking Metrics")

	producer := client.NewProducer(c)
	defer producer.Close()

	messageCount := 100

	for i := 0; i < messageCount; i++ {
		startProduce := time.Now()

		key := []byte(fmt.Sprintf("key-%d", i))
		value := []byte(fmt.Sprintf("message-%d: This is a test message for observability", i))

		err := producer.Send("metrics-demo", key, value)
		if err != nil {
			log.Printf("Failed to send message: %v", err)
			continue
		}

		// Track produce metrics
		produceLatency := time.Since(startProduce).Seconds()
		streambusMetrics.ProduceLatency.Observe(produceLatency)
		streambusMetrics.MessagesProduced.Inc()
		streambusMetrics.BytesProduced.Add(uint64(len(value)))

		if (i+1)%20 == 0 {
			fmt.Printf("   - Produced %d messages\n", i+1)
		}

		time.Sleep(50 * time.Millisecond)
	}

	fmt.Printf("   - Total messages produced: %d\n", messageCount)

	// Example 7: Simulate consume metrics
	fmt.Println("\n7. Simulating Consume Metrics")

	// Simulate consume metrics for demonstration purposes
	for i := 0; i < messageCount; i++ {
		// Simulate varying consume latencies
		latency := 0.001 + (float64(i%5) * 0.001) // 1-5ms
		streambusMetrics.ConsumeLatency.Observe(latency)
		streambusMetrics.MessagesConsumed.Inc()
		streambusMetrics.BytesConsumed.Add(uint64(50))

		if (i+1)%20 == 0 {
			fmt.Printf("   - Simulated %d message consumes\n", i+1)
		}
	}

	fmt.Printf("   - Total simulated consumes: %d\n", messageCount)

	// Update consumer group metrics
	streambusMetrics.ConsumerGroups.Set(1)
	streambusMetrics.ConsumerGroupMembers.Set(1)
	streambusMetrics.ConsumerGroupLag.Set(0) // No lag since all consumed

	// Example 8: Simulate additional metrics
	fmt.Println("\n8. Simulating Additional Metrics")

	// Connection metrics
	streambusMetrics.BrokerConnections.Set(2)
	fmt.Println("   - Active connections: 2")

	// Network metrics
	streambusMetrics.NetworkBytesIn.Add(50000)
	streambusMetrics.NetworkBytesOut.Add(45000)
	streambusMetrics.NetworkRequestsTotal.Add(uint64(messageCount * 2))
	fmt.Printf("   - Network requests: %d\n", messageCount*2)

	// Storage metrics
	streambusMetrics.StorageUsedBytes.Set(1024 * 1024 * 10) // 10 MB
	streambusMetrics.StorageAvailableBytes.Set(1024 * 1024 * 1024) // 1 GB
	streambusMetrics.SegmentsTotal.Set(3)
	fmt.Println("   - Storage: 10 MB used, 1 GB available")

	// Cluster metrics
	streambusMetrics.ClusterSize.Set(1)
	streambusMetrics.ClusterLeader.Set(1) // This broker is the leader
	streambusMetrics.RaftTerm.Set(1)
	streambusMetrics.RaftCommitIndex.Set(float64(messageCount))
	fmt.Println("   - Cluster: 1 broker, leader, Raft term 1")

	// Example 9: Display metrics summary
	time.Sleep(2 * time.Second) // Give consumers time to catch up

	fmt.Println("\n9. Metrics Summary")
	fmt.Println("   -----------------------------")
	fmt.Printf("   Broker Uptime:          %.1f seconds\n", time.Since(startTime).Seconds())
	fmt.Printf("   Messages Produced:      %d\n", messageCount)
	fmt.Printf("   Messages Consumed:      ~%d\n", messageCount)
	fmt.Printf("   Active Connections:     2\n")
	fmt.Printf("   Topics:                 1\n")
	fmt.Printf("   Partitions:             3\n")
	fmt.Printf("   Consumer Groups:        1\n")
	fmt.Printf("   Storage Used:           10 MB\n")
	fmt.Println("   -----------------------------")

	// Example 10: Show Prometheus metrics endpoint
	fmt.Println("\n10. Prometheus Integration")
	fmt.Println("   - Metrics are being exported in Prometheus format")
	fmt.Println("   - Prometheus endpoint: http://localhost:9090/metrics")
	fmt.Println("   - You can scrape this endpoint with Prometheus")
	fmt.Println()
	fmt.Println("   Example Prometheus scrape config:")
	fmt.Println("   ```yaml")
	fmt.Println("   scrape_configs:")
	fmt.Println("     - job_name: 'streambus'")
	fmt.Println("       static_configs:")
	fmt.Println("         - targets: ['localhost:9090']")
	fmt.Println("   ```")
	fmt.Println()
	fmt.Println("   Available metric types:")
	fmt.Println("   - Broker metrics (uptime, status, connections)")
	fmt.Println("   - Message metrics (produced, consumed, bytes)")
	fmt.Println("   - Performance metrics (latencies)")
	fmt.Println("   - Consumer group metrics (groups, members, lag)")
	fmt.Println("   - Transaction metrics (active, committed, aborted)")
	fmt.Println("   - Storage metrics (used, available, segments)")
	fmt.Println("   - Network metrics (bytes in/out, requests)")
	fmt.Println("   - Security metrics (auth, authz, audit)")
	fmt.Println("   - Cluster metrics (size, leader, Raft state)")
	fmt.Println("   - Schema metrics (registered, validations)")

	// Example 11: Interactive mode
	fmt.Println("\n11. Interactive Mode")
	fmt.Println("   Press Ctrl+C to stop...")
	fmt.Println("   While running, you can:")
	fmt.Println("   - Visit http://localhost:9090/metrics to see metrics")
	fmt.Println("   - Use `curl http://localhost:9090/metrics` to fetch metrics")
	fmt.Println("   - Configure Prometheus to scrape this endpoint")
	fmt.Println()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	// Cleanup
	fmt.Println("\n12. Shutting Down")
	fmt.Println("   - Closing client...")

	fmt.Println("   - Stopping broker...")
	if err := b.Stop(); err != nil {
		log.Printf("Error stopping broker: %v", err)
	}

	// Update broker status to stopped
	streambusMetrics.BrokerStatus.Set(0) // 0 = stopped

	fmt.Println("   - Broker stopped successfully")
	fmt.Println("\nObservability Example Completed!")
	fmt.Println("\nKey Takeaways:")
	fmt.Println("1. StreamBus provides comprehensive metrics for monitoring")
	fmt.Println("2. Metrics are exposed in Prometheus format at /metrics endpoint")
	fmt.Println("3. Metrics cover broker, messages, performance, security, and more")
	fmt.Println("4. Easy integration with Prometheus, Grafana, and other monitoring tools")
	fmt.Println("5. Real-time visibility into system performance and health")
}
