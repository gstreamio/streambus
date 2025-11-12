package client

import (
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/server"
)

// setupTestServer creates a test server for integration tests
func setupTestServer(t *testing.T) (*server.Server, string) {
	// Create temp directory for test
	tmpDir := t.TempDir()

	config := server.DefaultConfig()
	config.Address = ":0" // Use random port

	handler := server.NewHandlerWithDataDir(tmpDir)
	srv, err := server.New(config, handler)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	err = srv.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	addr := srv.Listener().Addr().String()
	return srv, addr
}

func TestClient_New(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("Expected client to be created")
	}

	if client.config != config {
		t.Error("Expected config to be set")
	}

	if client.pool == nil {
		t.Error("Expected pool to be created")
	}
}

func TestClient_NewWithInvalidConfig(t *testing.T) {
	config := &Config{
		Brokers: []string{}, // No brokers
	}

	_, err := New(config)
	if err == nil {
		t.Fatal("Expected error for invalid config")
	}
}

func TestClient_HealthCheck(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer srv.Stop()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	err = client.HealthCheck(addr)
	if err != nil {
		t.Errorf("Health check failed: %v", err)
	}
}

func TestClient_CreateTopic(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer srv.Stop()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	err = client.CreateTopic("test-topic", 1, 1)
	if err != nil {
		t.Errorf("CreateTopic failed: %v", err)
	}
}

func TestClient_DeleteTopic(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer srv.Stop()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create topic first
	err = client.CreateTopic("test-topic", 1, 1)
	if err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	err = client.DeleteTopic("test-topic")
	if err != nil {
		t.Errorf("DeleteTopic failed: %v", err)
	}
}

func TestClient_ListTopics(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer srv.Stop()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	topics, err := client.ListTopics()
	if err != nil {
		t.Errorf("ListTopics failed: %v", err)
	}

	// Should return empty list or actual topics
	_ = topics
}

func TestClient_Close(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	err = client.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Second close should not error
	err = client.Close()
	if err != nil {
		t.Errorf("Second close failed: %v", err)
	}

	// Operations after close should fail
	err = client.HealthCheck(config.Brokers[0])
	if err != ErrClientClosed {
		t.Errorf("Expected ErrClientClosed, got %v", err)
	}
}

func TestProducer_New(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	producer := NewProducer(client)
	if producer == nil {
		t.Fatal("Expected producer to be created")
	}
	defer producer.Close()

	if producer.client != client {
		t.Error("Expected client to be set")
	}
}

func TestProducer_Send(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer srv.Stop()

	config := DefaultConfig()
	config.Brokers = []string{addr}
	config.ProducerConfig.BatchSize = 0 // Disable batching for this test

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create topic first
	err = client.CreateTopic("test-topic", 1, 1)
	if err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	producer := NewProducerWithConfig(client, config.ProducerConfig)
	defer producer.Close()

	err = producer.Send("test-topic", []byte("key1"), []byte("value1"))
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}

	stats := producer.Stats()
	if stats.MessagesSent != 1 {
		t.Errorf("Expected 1 message sent, got %d", stats.MessagesSent)
	}
}

func TestProducer_SendToPartition(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer srv.Stop()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create topic first
	err = client.CreateTopic("test-topic", 1, 1)
	if err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	producer := NewProducer(client)
	defer producer.Close()

	err = producer.SendToPartition("test-topic", 0, []byte("key1"), []byte("value1"))
	if err != nil {
		t.Errorf("SendToPartition failed: %v", err)
	}
}

func TestProducer_SendMessages(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer srv.Stop()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create topic first
	err = client.CreateTopic("test-topic", 1, 1)
	if err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	producer := NewProducer(client)
	defer producer.Close()

	messages := []protocol.Message{
		{Key: []byte("key1"), Value: []byte("value1")},
		{Key: []byte("key2"), Value: []byte("value2")},
		{Key: []byte("key3"), Value: []byte("value3")},
	}

	err = producer.SendMessages("test-topic", messages)
	if err != nil {
		t.Errorf("SendMessages failed: %v", err)
	}
}

func TestProducer_Batching(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer srv.Stop()

	config := DefaultConfig()
	config.Brokers = []string{addr}
	config.ProducerConfig.BatchSize = 3
	config.ProducerConfig.BatchTimeout = 100 * time.Millisecond

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create topic first
	err = client.CreateTopic("test-topic", 1, 1)
	if err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	producer := NewProducerWithConfig(client, config.ProducerConfig)
	defer producer.Close()

	// Send messages that should batch
	for i := 0; i < 3; i++ {
		err = producer.Send("test-topic", []byte("key"), []byte("value"))
		if err != nil {
			t.Errorf("Send failed: %v", err)
		}
	}

	// Wait for batch to flush
	time.Sleep(200 * time.Millisecond)

	stats := producer.Stats()
	if stats.MessagesSent != 3 {
		t.Errorf("Expected 3 messages sent, got %d", stats.MessagesSent)
	}
}

func TestProducer_Flush(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer srv.Stop()

	config := DefaultConfig()
	config.Brokers = []string{addr}
	config.ProducerConfig.BatchSize = 10
	config.ProducerConfig.BatchTimeout = 10 * time.Second

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create topic first
	err = client.CreateTopic("test-topic", 1, 1)
	if err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	producer := NewProducerWithConfig(client, config.ProducerConfig)
	defer producer.Close()

	// Send a few messages (less than batch size)
	err = producer.Send("test-topic", []byte("key1"), []byte("value1"))
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}

	err = producer.Send("test-topic", []byte("key2"), []byte("value2"))
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}

	// Flush immediately
	err = producer.Flush("test-topic")
	if err != nil {
		t.Errorf("Flush failed: %v", err)
	}

	stats := producer.Stats()
	if stats.MessagesSent != 2 {
		t.Errorf("Expected 2 messages sent, got %d", stats.MessagesSent)
	}
}

func TestProducer_Close(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	producer := NewProducer(client)

	err = producer.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Operations after close should fail
	err = producer.Send("test-topic", []byte("key"), []byte("value"))
	if err != ErrProducerClosed {
		t.Errorf("Expected ErrProducerClosed, got %v", err)
	}
}

func TestConsumer_New(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	consumer := NewConsumer(client, "test-topic", 0)
	if consumer == nil {
		t.Fatal("Expected consumer to be created")
	}
	defer consumer.Close()

	if consumer.client != client {
		t.Error("Expected client to be set")
	}

	if consumer.topic != "test-topic" {
		t.Error("Expected topic to be set")
	}

	if consumer.partitionID != 0 {
		t.Error("Expected partition to be set")
	}
}

func TestConsumer_Fetch(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer srv.Stop()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create topic first
	err = client.CreateTopic("test-topic", 1, 1)
	if err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	// Produce some messages
	producer := NewProducer(client)
	producer.Send("test-topic", []byte("key1"), []byte("value1"))
	producer.Send("test-topic", []byte("key2"), []byte("value2"))
	producer.Close()

	// Now consume
	consumer := NewConsumer(client, "test-topic", 0)
	defer consumer.Close()

	// Seek to beginning to read from offset 0
	err = consumer.SeekToBeginning()
	if err != nil {
		t.Fatalf("Failed to seek to beginning: %v", err)
	}

	// Fetch messages
	messages, err := consumer.Fetch()
	if err != nil {
		t.Errorf("Fetch failed: %v", err)
	}

	// Should return messages
	if messages == nil {
		t.Error("Expected non-nil message list")
	}

	if len(messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(messages))
	}
}

func TestConsumer_Seek(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	consumer := NewConsumer(client, "test-topic", 0)
	defer consumer.Close()

	err = consumer.Seek(100)
	if err != nil {
		t.Errorf("Seek failed: %v", err)
	}

	if consumer.CurrentOffset() != 100 {
		t.Errorf("Expected offset 100, got %d", consumer.CurrentOffset())
	}
}

func TestConsumer_SeekToBeginning(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	consumer := NewConsumer(client, "test-topic", 0)
	defer consumer.Close()

	// Set offset to something non-zero
	consumer.Seek(100)

	err = consumer.SeekToBeginning()
	if err != nil {
		t.Errorf("SeekToBeginning failed: %v", err)
	}

	if consumer.CurrentOffset() != 0 {
		t.Errorf("Expected offset 0, got %d", consumer.CurrentOffset())
	}
}

func TestConsumer_Close(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	consumer := NewConsumer(client, "test-topic", 0)

	err = consumer.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Operations after close should fail
	_, err = consumer.Fetch()
	if err != ErrConsumerClosed {
		t.Errorf("Expected ErrConsumerClosed, got %v", err)
	}
}

func TestConnectionPool_GetAndPut(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer srv.Stop()

	config := DefaultConfig()
	config.Brokers = []string{addr}
	config.MaxConnectionsPerBroker = 2

	pool := NewConnectionPool(config)
	defer pool.Close()

	// Get first connection
	conn1, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}

	if conn1 == nil {
		t.Fatal("Expected connection to be returned")
	}

	// Get second connection
	conn2, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("Failed to get second connection: %v", err)
	}

	if conn2 == nil {
		t.Fatal("Expected second connection to be returned")
	}

	// Should be different connections
	if conn1 == conn2 {
		t.Error("Expected different connections")
	}

	// Return connections
	pool.Put(conn1)
	pool.Put(conn2)

	// Get connection again - should reuse
	conn3, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("Failed to get third connection: %v", err)
	}

	// Should be one of the previous connections
	if conn3 != conn1 && conn3 != conn2 {
		t.Error("Expected connection to be reused")
	}
}

func TestConnectionPool_Stats(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer srv.Stop()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	pool := NewConnectionPool(config)
	defer pool.Close()

	// Initial stats
	stats := pool.Stats()
	if stats.TotalConnections != 0 {
		t.Errorf("Expected 0 connections, got %d", stats.TotalConnections)
	}

	// Get a connection
	conn, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}

	stats = pool.Stats()
	if stats.TotalConnections != 1 {
		t.Errorf("Expected 1 connection, got %d", stats.TotalConnections)
	}

	if stats.ActiveConnections != 1 {
		t.Errorf("Expected 1 active connection, got %d", stats.ActiveConnections)
	}

	// Return connection
	pool.Put(conn)

	stats = pool.Stats()
	if stats.IdleConnections != 1 {
		t.Errorf("Expected 1 idle connection, got %d", stats.IdleConnections)
	}
}

func TestConnectionPool_Close(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer srv.Stop()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	pool := NewConnectionPool(config)

	// Get a connection
	conn, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}
	pool.Put(conn)

	// Close pool
	err = pool.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Getting connection after close should fail
	_, err = pool.Get(addr)
	if err != ErrConnectionPoolClosed {
		t.Errorf("Expected ErrConnectionPoolClosed, got %v", err)
	}
}

// Benchmarks

func BenchmarkProducer_Send(b *testing.B) {
	srv, addr := setupTestServer(&testing.T{})
	defer srv.Stop()

	config := DefaultConfig()
	config.Brokers = []string{addr}
	config.ProducerConfig.BatchSize = 0 // Disable batching

	client, _ := New(config)
	defer client.Close()

	// Create topic first
	client.CreateTopic("test-topic", 1, 1)

	producer := NewProducerWithConfig(client, config.ProducerConfig)
	defer producer.Close()

	key := []byte("benchmark-key")
	value := []byte("benchmark-value-with-some-data")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := producer.Send("test-topic", key, value)
		if err != nil {
			b.Fatalf("Send failed: %v", err)
		}
	}
}

func BenchmarkConsumer_Fetch(b *testing.B) {
	srv, addr := setupTestServer(&testing.T{})
	defer srv.Stop()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, _ := New(config)
	defer client.Close()

	// Create topic and produce some messages
	client.CreateTopic("test-topic", 1, 1)
	producer := NewProducer(client)
	for i := 0; i < 100; i++ {
		producer.Send("test-topic", []byte("key"), []byte("value"))
	}
	producer.Close()

	consumer := NewConsumer(client, "test-topic", 0)
	defer consumer.Close()
	consumer.SeekToBeginning()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := consumer.Fetch()
		if err != nil {
			b.Fatalf("Fetch failed: %v", err)
		}
	}
}
