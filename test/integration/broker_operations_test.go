package integration

import (
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/broker"
	"github.com/gstreamio/streambus/pkg/client"
	"github.com/gstreamio/streambus/pkg/consensus"
)

// TestBrokerOperations tests complete broker lifecycle and operations
// Covers pkg/broker, pkg/server, pkg/storage initialization and operation paths
func TestBrokerOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping broker operations integration test in short mode")
	}

	config := &broker.Config{
		BrokerID:    1,
		Host:        "localhost",
		Port:        19500,
		GRPCPort:    19501,
		HTTPPort:    18500,
		DataDir:     t.TempDir() + "/data",
		RaftDataDir: t.TempDir() + "/raft",
		LogLevel:    "info",
		RaftPeers: []consensus.Peer{
			{ID: 1, Addr: "localhost:17500"},
		},
	}

	// Test 1: Broker Creation and Startup
	t.Run("Startup", func(t *testing.T) {
		b, err := broker.New(config)
		if err != nil {
			t.Fatalf("Failed to create broker: %v", err)
		}

		if err := b.Start(); err != nil {
			t.Fatalf("Failed to start broker: %v", err)
		}
		defer func() { _ = b.Stop() }()

		// Wait for broker to be ready
		time.Sleep(3 * time.Second)

		if !b.IsReady() {
			t.Error("Broker should be ready after startup")
		}

		t.Log("Broker started successfully")
	})

	// Test 2: Client Connection and Topic Creation
	t.Run("ClientOperations", func(t *testing.T) {
		b, err := broker.New(config)
		if err != nil {
			t.Fatalf("Failed to create broker: %v", err)
		}

		if err := b.Start(); err != nil {
			t.Fatalf("Failed to start broker: %v", err)
		}
		defer func() { _ = b.Stop() }()

		time.Sleep(3 * time.Second)

		// Connect client
		clientConfig := client.DefaultConfig()
		clientConfig.Brokers = []string{"localhost:19500"}

		c, err := client.New(clientConfig)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer func() { _ = c.Close() }()

		// Create topic
		topicName := "test-topic"
		if err := c.CreateTopic(topicName, 1, 1); err != nil {
			t.Fatalf("Failed to create topic: %v", err)
		}

		time.Sleep(1 * time.Second)

		// Produce message
		producer := client.NewProducer(c)
		defer func() { _ = producer.Close() }()

		testData := []byte("integration test message")
		if err := producer.Send(topicName, []byte("key1"), testData); err != nil {
			t.Fatalf("Failed to produce message: %v", err)
		}

		// Consume message
		consumer := client.NewConsumer(c, topicName, 0)
		defer func() { _ = consumer.Close() }()

		_ = consumer.SeekToBeginning()
		messages, err := consumer.Fetch()
		if err != nil {
			t.Fatalf("Failed to consume messages: %v", err)
		}

		if len(messages) == 0 {
			t.Fatal("Expected at least 1 message")
		}

		if string(messages[0].Value) != string(testData) {
			t.Errorf("Message value = %s, want %s", messages[0].Value, testData)
		}

		t.Log("Client operations successful")
	})

	// Test 3: Graceful Shutdown
	t.Run("GracefulShutdown", func(t *testing.T) {
		b, err := broker.New(config)
		if err != nil {
			t.Fatalf("Failed to create broker: %v", err)
		}

		if err := b.Start(); err != nil {
			t.Fatalf("Failed to start broker: %v", err)
		}

		time.Sleep(3 * time.Second)

		// Connect client
		clientConfig := client.DefaultConfig()
		clientConfig.Brokers = []string{"localhost:19500"}

		c, err := client.New(clientConfig)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer func() { _ = c.Close() }()

		// Create topic
		if err := c.CreateTopic("shutdown-test", 1, 1); err != nil {
			t.Fatalf("Failed to create topic: %v", err)
		}

		// Stop broker gracefully
		if err := b.Stop(); err != nil {
			t.Errorf("Graceful shutdown failed: %v", err)
		}

		t.Log("Graceful shutdown successful")
	})

	// Test 4: Data Persistence Across Restarts
	t.Run("Persistence", func(t *testing.T) {
		// First broker instance
		b1, err := broker.New(config)
		if err != nil {
			t.Fatalf("Failed to create broker: %v", err)
		}

		if err := b1.Start(); err != nil {
			t.Fatalf("Failed to start broker: %v", err)
		}

		time.Sleep(3 * time.Second)

		// Create topic and produce data
		clientConfig := client.DefaultConfig()
		clientConfig.Brokers = []string{"localhost:19500"}

		c1, err := client.New(clientConfig)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		topicName := "persist-topic"
		if err := c1.CreateTopic(topicName, 1, 1); err != nil {
			t.Fatalf("Failed to create topic: %v", err)
		}

		producer := client.NewProducer(c1)
		testMsg := []byte("persistent message")
		if err := producer.Send(topicName, []byte("key"), testMsg); err != nil {
			t.Fatalf("Failed to produce message: %v", err)
		}

		_ = producer.Close()
		_ = c1.Close()
		_ = b1.Stop()

		// Wait for cleanup
		time.Sleep(2 * time.Second)

		// Second broker instance - restart
		b2, err := broker.New(config)
		if err != nil {
			t.Fatalf("Failed to create broker for restart: %v", err)
		}

		if err := b2.Start(); err != nil {
			t.Fatalf("Failed to restart broker: %v", err)
		}
		defer func() { _ = b2.Stop() }()

		time.Sleep(3 * time.Second)

		// Reconnect and verify data persisted
		c2, err := client.New(clientConfig)
		if err != nil {
			t.Fatalf("Failed to create client after restart: %v", err)
		}
		defer func() { _ = c2.Close() }()

		consumer := client.NewConsumer(c2, topicName, 0)
		defer func() { _ = consumer.Close() }()

		_ = consumer.SeekToBeginning()
		messages, err := consumer.Fetch()
		if err != nil {
			t.Fatalf("Failed to consume messages after restart: %v", err)
		}

		if len(messages) == 0 {
			t.Fatal("Expected persisted message after restart")
		}

		if string(messages[0].Value) != string(testMsg) {
			t.Errorf("Persisted message = %s, want %s", messages[0].Value, testMsg)
		}

		t.Log("Data persistence verified")
	})

	// Test 5: Multiple Topics and Partitions
	t.Run("MultipleTopics", func(t *testing.T) {
		b, err := broker.New(config)
		if err != nil {
			t.Fatalf("Failed to create broker: %v", err)
		}

		if err := b.Start(); err != nil {
			t.Fatalf("Failed to start broker: %v", err)
		}
		defer func() { _ = b.Stop() }()

		time.Sleep(3 * time.Second)

		clientConfig := client.DefaultConfig()
		clientConfig.Brokers = []string{"localhost:19500"}

		c, err := client.New(clientConfig)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer func() { _ = c.Close() }()

		// Create multiple topics
		topics := []string{"topic-a", "topic-b", "topic-c"}
		for _, topic := range topics {
			if err := c.CreateTopic(topic, 3, 1); err != nil {
				t.Fatalf("Failed to create topic %s: %v", topic, err)
			}
		}

		time.Sleep(1 * time.Second)

		// Produce to each topic
		producer := client.NewProducer(c)
		defer func() { _ = producer.Close() }()

		for _, topic := range topics {
			msg := []byte("message for " + topic)
			if err := producer.Send(topic, []byte("key"), msg); err != nil {
				t.Errorf("Failed to produce to %s: %v", topic, err)
			}
		}

		// Verify messages in each topic
		for _, topic := range topics {
			consumer := client.NewConsumer(c, topic, 0)
			_ = consumer.SeekToBeginning()
			messages, err := consumer.Fetch()
			_ = consumer.Close()

			if err != nil {
				t.Errorf("Failed to consume from %s: %v", topic, err)
				continue
			}

			if len(messages) == 0 {
				t.Errorf("No messages in topic %s", topic)
			}
		}

		t.Log("Multiple topics test successful")
	})

	// Test 6: Health Check System
	t.Run("HealthChecks", func(t *testing.T) {
		b, err := broker.New(config)
		if err != nil {
			t.Fatalf("Failed to create broker: %v", err)
		}

		if err := b.Start(); err != nil {
			t.Fatalf("Failed to start broker: %v", err)
		}
		defer func() { _ = b.Stop() }()

		time.Sleep(3 * time.Second)

		// Verify liveness
		if !b.IsAlive() {
			t.Error("Broker should be alive")
		}

		// Verify readiness
		if !b.IsReady() {
			t.Error("Broker should be ready")
		}

		// Verify leader status
		if !b.IsLeader() {
			t.Error("Single-node broker should be leader")
		}

		t.Log("Health checks passed")
	})
}
