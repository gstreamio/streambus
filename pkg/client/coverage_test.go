package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
)

// TestClient_Stats tests the Stats method (0% coverage)
func TestClient_Stats(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	stats := client.Stats()

	if stats.Uptime == 0 {
		t.Error("Expected uptime > 0")
	}

	// Stats should have pool stats
	if stats.PoolStats.TotalConnections < 0 {
		t.Error("Expected valid pool stats")
	}
}

// TestConfig_Validate tests the Validate method with various scenarios (0% coverage)
func TestConfig_Validate_NoBrokers(t *testing.T) {
	config := &Config{
		Brokers: []string{},
	}

	err := config.Validate()
	if err != ErrNoBrokers {
		t.Errorf("Expected ErrNoBrokers, got %v", err)
	}
}

func TestConfig_Validate_InvalidConnectTimeout(t *testing.T) {
	config := &Config{
		Brokers:        []string{"localhost:9092"},
		ConnectTimeout: 0,
	}

	err := config.Validate()
	if err != ErrInvalidTimeout {
		t.Errorf("Expected ErrInvalidTimeout, got %v", err)
	}
}

func TestConfig_Validate_InvalidMaxConnections(t *testing.T) {
	config := &Config{
		Brokers:                 []string{"localhost:9092"},
		ConnectTimeout:          10 * time.Second,
		MaxConnectionsPerBroker: 0,
	}

	err := config.Validate()
	if err != ErrInvalidMaxConnections {
		t.Errorf("Expected ErrInvalidMaxConnections, got %v", err)
	}
}

func TestConfig_Validate_InvalidRetries(t *testing.T) {
	config := &Config{
		Brokers:                 []string{"localhost:9092"},
		ConnectTimeout:          10 * time.Second,
		MaxConnectionsPerBroker: 5,
		MaxRetries:              -1,
	}

	err := config.Validate()
	if err != ErrInvalidRetries {
		t.Errorf("Expected ErrInvalidRetries, got %v", err)
	}
}

func TestConfig_Validate_WithSecurity(t *testing.T) {
	config := &Config{
		Brokers:                 []string{"localhost:9092"},
		ConnectTimeout:          10 * time.Second,
		MaxConnectionsPerBroker: 5,
		MaxRetries:              3,
		Security: &SecurityConfig{
			TLS: &TLSConfig{
				Enabled:  true,
				CertFile: "cert.pem",
				KeyFile:  "", // Missing key file
			},
		},
	}

	err := config.Validate()
	if err != ErrInvalidTLSConfig {
		t.Errorf("Expected ErrInvalidTLSConfig, got %v", err)
	}
}

func TestConfig_Validate_ValidConfig(t *testing.T) {
	config := DefaultConfig()
	err := config.Validate()
	if err != nil {
		t.Errorf("Valid config should not error: %v", err)
	}
}

// TestSecurityConfig_Validate tests SecurityConfig validation (0% coverage)
func TestSecurityConfig_Validate_TLS_CertWithoutKey(t *testing.T) {
	sc := &SecurityConfig{
		TLS: &TLSConfig{
			Enabled:  true,
			CertFile: "cert.pem",
			KeyFile:  "",
		},
	}

	err := sc.Validate()
	if err != ErrInvalidTLSConfig {
		t.Errorf("Expected ErrInvalidTLSConfig, got %v", err)
	}
}

func TestSecurityConfig_Validate_TLS_KeyWithoutCert(t *testing.T) {
	sc := &SecurityConfig{
		TLS: &TLSConfig{
			Enabled:  true,
			CertFile: "",
			KeyFile:  "key.pem",
		},
	}

	err := sc.Validate()
	if err != ErrInvalidTLSConfig {
		t.Errorf("Expected ErrInvalidTLSConfig, got %v", err)
	}
}

func TestSecurityConfig_Validate_SASL_NoUsername(t *testing.T) {
	sc := &SecurityConfig{
		SASL: &SASLConfig{
			Enabled:  true,
			Username: "",
			Password: "password",
		},
	}

	err := sc.Validate()
	if err != ErrInvalidSASLConfig {
		t.Errorf("Expected ErrInvalidSASLConfig, got %v", err)
	}
}

func TestSecurityConfig_Validate_SASL_NoPassword(t *testing.T) {
	sc := &SecurityConfig{
		SASL: &SASLConfig{
			Enabled:  true,
			Username: "user",
			Password: "",
		},
	}

	err := sc.Validate()
	if err != ErrInvalidSASLConfig {
		t.Errorf("Expected ErrInvalidSASLConfig, got %v", err)
	}
}

func TestSecurityConfig_Validate_SASL_DefaultMechanism(t *testing.T) {
	sc := &SecurityConfig{
		SASL: &SASLConfig{
			Enabled:   true,
			Username:  "user",
			Password:  "password",
			Mechanism: "",
		},
	}

	err := sc.Validate()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Should set default mechanism
	if sc.SASL.Mechanism != "SCRAM-SHA-256" {
		t.Errorf("Expected default mechanism SCRAM-SHA-256, got %s", sc.SASL.Mechanism)
	}
}

// TestConsumer_Topic tests the Topic method (0% coverage)
func TestConsumer_Topic(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	consumer := NewConsumer(client, "test-topic", 0)
	defer consumer.Close()

	topic := consumer.Topic()
	if topic != "test-topic" {
		t.Errorf("Expected topic 'test-topic', got %s", topic)
	}
}

// TestConsumer_Partition tests the Partition method (0% coverage)
func TestConsumer_Partition(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	consumer := NewConsumer(client, "test-topic", 5)
	defer consumer.Close()

	partition := consumer.Partition()
	if partition != 5 {
		t.Errorf("Expected partition 5, got %d", partition)
	}
}

// TestConsumer_Poll tests the Poll method (0% coverage)
func TestConsumer_Poll(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create topic and produce messages
	err = client.CreateTopic("test-topic", 1, 1)
	if err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	producer := NewProducer(client)
	_ = producer.Send("test-topic", []byte("key1"), []byte("value1"))
	_ = producer.Send("test-topic", []byte("key2"), []byte("value2"))
	producer.Close()

	consumer := NewConsumer(client, "test-topic", 0)
	defer consumer.Close()

	_ = consumer.SeekToBeginning()

	// Test Poll with a handler that stops after a few messages
	messageCount := 0
	done := make(chan bool, 1)

	go func() {
		err := consumer.Poll(10*time.Millisecond, func(messages []protocol.Message) error {
			messageCount += len(messages)
			if messageCount >= 2 {
				// Close consumer to stop polling
				consumer.Close()
			}
			return nil
		})
		// Error expected when consumer is closed
		if err != ErrConsumerClosed {
			t.Logf("Poll returned: %v", err)
		}
		done <- true
	}()

	// Wait for polling to complete or timeout
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		consumer.Close()
		t.Fatal("Poll timed out")
	}

	if messageCount != 2 {
		t.Errorf("Expected to receive 2 messages, got %d", messageCount)
	}
}

func TestConsumer_Poll_HandlerError(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create topic and produce messages
	err = client.CreateTopic("test-topic", 1, 1)
	if err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	producer := NewProducer(client)
	_ = producer.Send("test-topic", []byte("key1"), []byte("value1"))
	producer.Close()

	consumer := NewConsumer(client, "test-topic", 0)
	defer consumer.Close()

	_ = consumer.SeekToBeginning()

	// Test Poll with a handler that returns an error
	err = consumer.Poll(10*time.Millisecond, func(messages []protocol.Message) error {
		return ErrConsumerClosed
	})

	if err == nil || err.Error() != "handler error: consumer is closed" {
		t.Errorf("Expected handler error, got %v", err)
	}
}

func TestConsumer_Poll_Closed(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	consumer := NewConsumer(client, "test-topic", 0)
	consumer.Close()

	// Poll on closed consumer should fail immediately
	err = consumer.Poll(10*time.Millisecond, func(messages []protocol.Message) error {
		return nil
	})

	if err != ErrConsumerClosed {
		t.Errorf("Expected ErrConsumerClosed, got %v", err)
	}
}

// TestGroupConsumer_OnPartitionsRevoked tests the default rebalance listener (0% coverage)
func TestGroupConsumer_OnPartitionsRevoked(t *testing.T) {
	listener := &DefaultRebalanceListener{}

	// Should not panic
	listener.OnPartitionsRevoked(map[string][]int32{
		"topic1": {0, 1, 2},
	})
}

// TestGroupConsumer_OnPartitionsAssigned tests the default rebalance listener (0% coverage)
func TestGroupConsumer_OnPartitionsAssigned(t *testing.T) {
	listener := &DefaultRebalanceListener{}

	// Should not panic
	listener.OnPartitionsAssigned(map[string][]int32{
		"topic1": {0, 1, 2},
	})
}

// TestGroupConsumer_Poll tests the Poll method (0% coverage)
func TestGroupConsumer_Poll(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	gcConfig := DefaultGroupConsumerConfig()
	gcConfig.GroupID = "test-group"
	gcConfig.Topics = []string{"test-topic"}

	gc, err := NewGroupConsumer(client, gcConfig)
	if err != nil {
		t.Fatalf("Failed to create group consumer: %v", err)
	}
	defer gc.Close()

	// Subscribe to start the consumer
	err = gc.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Poll should succeed
	ctx := context.Background()
	messages, err := gc.Poll(ctx)
	if err != nil {
		t.Errorf("Poll failed: %v", err)
	}

	if messages == nil {
		t.Error("Expected non-nil messages")
	}
}

func TestGroupConsumer_Poll_Closed(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	gcConfig := DefaultGroupConsumerConfig()
	gcConfig.GroupID = "test-group"
	gcConfig.Topics = []string{"test-topic"}

	gc, err := NewGroupConsumer(client, gcConfig)
	if err != nil {
		t.Fatalf("Failed to create group consumer: %v", err)
	}

	gc.Close()

	// Poll on closed consumer should fail
	ctx := context.Background()
	_, err = gc.Poll(ctx)
	if err != ErrConsumerClosed {
		t.Errorf("Expected ErrConsumerClosed, got %v", err)
	}
}

func TestGroupConsumer_Poll_NotStable(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	gcConfig := DefaultGroupConsumerConfig()
	gcConfig.GroupID = "test-group"
	gcConfig.Topics = []string{"test-topic"}

	gc, err := NewGroupConsumer(client, gcConfig)
	if err != nil {
		t.Fatalf("Failed to create group consumer: %v", err)
	}
	defer gc.Close()

	// Poll without subscribing (not in stable state)
	ctx := context.Background()
	_, err = gc.Poll(ctx)
	if err == nil {
		t.Error("Expected error when polling in non-stable state")
	}
}

// TestClient_sendRequestWithRetry tests retry logic (46.7% coverage)
func TestClient_sendRequestWithRetry_Success(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}
	config.MaxRetries = 3
	config.RetryBackoff = 10 * time.Millisecond

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type:    protocol.RequestTypeHealthCheck,
			Version: protocol.ProtocolVersion,
			Flags:   protocol.FlagNone,
		},
		Payload: &protocol.HealthCheckRequest{},
	}

	resp, err := client.sendRequestWithRetry(addr, req)
	if err != nil {
		t.Errorf("sendRequestWithRetry failed: %v", err)
	}

	if resp == nil {
		t.Error("Expected non-nil response")
	}
}

func TestClient_sendRequestWithRetry_ClientClosed(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Close client
	client.Close()

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type:    protocol.RequestTypeHealthCheck,
			Version: protocol.ProtocolVersion,
			Flags:   protocol.FlagNone,
		},
		Payload: &protocol.HealthCheckRequest{},
	}

	_, err = client.sendRequestWithRetry(config.Brokers[0], req)
	if err == nil {
		t.Error("Expected error when client is closed")
	}
}

func TestClient_sendRequestWithRetry_Backoff(t *testing.T) {
	config := DefaultConfig()
	config.Brokers = []string{"invalid-broker:9999"}
	config.MaxRetries = 2
	config.RetryBackoff = 10 * time.Millisecond
	config.RetryMaxDelay = 50 * time.Millisecond
	config.ConnectTimeout = 50 * time.Millisecond

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type:    protocol.RequestTypeHealthCheck,
			Version: protocol.ProtocolVersion,
			Flags:   protocol.FlagNone,
		},
		Payload: &protocol.HealthCheckRequest{},
	}

	start := time.Now()
	_, err = client.sendRequestWithRetry(config.Brokers[0], req)
	duration := time.Since(start)

	// Should have retried and failed
	if err == nil {
		t.Error("Expected error for invalid broker")
	}

	// Should have taken some time due to retries
	if duration < 20*time.Millisecond {
		t.Errorf("Expected retry delay, but completed too quickly: %v", duration)
	}
}

// TestClient_Fetch tests Fetch with error cases (57.9% coverage)
func TestClient_Fetch_InvalidTopic(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	req := &FetchRequest{
		Topic:     "",
		Partition: 0,
		Offset:    0,
		MaxBytes:  1024,
	}

	_, err = client.Fetch(context.Background(), req)
	if err != ErrInvalidTopic {
		t.Errorf("Expected ErrInvalidTopic, got %v", err)
	}
}

func TestClient_Fetch_InvalidPartition(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	req := &FetchRequest{
		Topic:     "test-topic",
		Partition: -1,
		Offset:    0,
		MaxBytes:  1024,
	}

	_, err = client.Fetch(context.Background(), req)
	if err != ErrInvalidPartition {
		t.Errorf("Expected ErrInvalidPartition, got %v", err)
	}
}

func TestClient_Fetch_InvalidOffset(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	req := &FetchRequest{
		Topic:     "test-topic",
		Partition: 0,
		Offset:    -1,
		MaxBytes:  1024,
	}

	_, err = client.Fetch(context.Background(), req)
	if err != ErrInvalidOffset {
		t.Errorf("Expected ErrInvalidOffset, got %v", err)
	}
}

func TestClient_Fetch_ClientClosed(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	client.Close()

	req := &FetchRequest{
		Topic:     "test-topic",
		Partition: 0,
		Offset:    0,
		MaxBytes:  1024,
	}

	_, err = client.Fetch(context.Background(), req)
	if err != ErrClientClosed {
		t.Errorf("Expected ErrClientClosed, got %v", err)
	}
}

// TestConnectionPool_createConnection tests connection creation with TLS (56.5% coverage)
func TestConnectionPool_createConnection_WithTLS(t *testing.T) {
	config := DefaultConfig()
	config.Brokers = []string{"invalid-host:9999"}
	config.ConnectTimeout = 100 * time.Millisecond
	config.Security = &SecurityConfig{
		TLS: &TLSConfig{
			Enabled:            true,
			InsecureSkipVerify: true,
		},
	}

	pool := NewConnectionPool(config)
	defer pool.Close()

	// Attempt to create connection (will fail since no TLS server, but tests the code path)
	_, err := pool.createConnection("invalid-host:9999")
	if err == nil {
		t.Error("Expected error connecting to non-existent TLS server")
	}
}

func TestConnectionPool_createConnection_WithoutTLS(t *testing.T) {
	config := DefaultConfig()
	config.Brokers = []string{"invalid-host:9999"}
	config.ConnectTimeout = 100 * time.Millisecond

	pool := NewConnectionPool(config)
	defer pool.Close()

	// Should fail to connect
	_, err := pool.createConnection("invalid-host:9999")
	if err == nil {
		t.Error("Expected error connecting to invalid host")
	}
}

// TestConnectionPool_buildTLSConfig tests TLS config building (57.9% coverage)
func TestConnectionPool_buildTLSConfig_Basic(t *testing.T) {
	config := DefaultConfig()
	config.Security = &SecurityConfig{
		TLS: &TLSConfig{
			Enabled:            true,
			InsecureSkipVerify: true,
			ServerName:         "test-server",
		},
	}

	pool := NewConnectionPool(config)
	defer pool.Close()

	tlsConfig, err := pool.buildTLSConfig()
	if err != nil {
		t.Errorf("buildTLSConfig failed: %v", err)
	}

	if !tlsConfig.InsecureSkipVerify {
		t.Error("Expected InsecureSkipVerify to be true")
	}

	if tlsConfig.ServerName != "test-server" {
		t.Errorf("Expected ServerName 'test-server', got %s", tlsConfig.ServerName)
	}
}

func TestConnectionPool_buildTLSConfig_WithCA(t *testing.T) {
	tmpDir := t.TempDir()
	caFile := filepath.Join(tmpDir, "ca.pem")

	// Create a dummy CA file (invalid but tests the code path)
	caCert := `-----BEGIN CERTIFICATE-----
MIICljCCAX4CCQDKh3l5K5fVCTANBgkqhkiG9w0BAQsFADANMQswCQYDVQQGEwJV
UzAeFw0yNDAxMDEwMDAwMDBaFw0yNTAxMDEwMDAwMDBaMA0xCzAJBgNVBAYTAlVT
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA1234567890
-----END CERTIFICATE-----`

	if err := os.WriteFile(caFile, []byte(caCert), 0600); err != nil {
		t.Fatalf("Failed to write CA file: %v", err)
	}

	config := DefaultConfig()
	config.Security = &SecurityConfig{
		TLS: &TLSConfig{
			Enabled: true,
			CAFile:  caFile,
		},
	}

	pool := NewConnectionPool(config)
	defer pool.Close()

	// Should fail because the CA cert is invalid
	_, err := pool.buildTLSConfig()
	if err == nil {
		t.Error("Expected error for invalid CA certificate")
	}
}

func TestConnectionPool_buildTLSConfig_WithClientCert(t *testing.T) {
	tmpDir := t.TempDir()

	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")

	// Write invalid cert/key files to test error path
	if err := os.WriteFile(certFile, []byte("invalid cert"), 0600); err != nil {
		t.Fatalf("Failed to write cert file: %v", err)
	}
	if err := os.WriteFile(keyFile, []byte("invalid key"), 0600); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}

	config := DefaultConfig()
	config.Security = &SecurityConfig{
		TLS: &TLSConfig{
			Enabled:  true,
			CertFile: certFile,
			KeyFile:  keyFile,
		},
	}

	pool := NewConnectionPool(config)
	defer pool.Close()

	// Should fail with invalid certificate
	_, err := pool.buildTLSConfig()
	if err == nil {
		t.Error("Expected error for invalid certificate")
	}
}

func TestConnectionPool_buildTLSConfig_Cached(t *testing.T) {
	config := DefaultConfig()
	config.Security = &SecurityConfig{
		TLS: &TLSConfig{
			Enabled:            true,
			InsecureSkipVerify: true,
		},
	}

	pool := NewConnectionPool(config)
	defer pool.Close()

	tlsConfig1, err := pool.buildTLSConfig()
	if err != nil {
		t.Errorf("buildTLSConfig failed: %v", err)
	}

	// Call again - should return cached config
	tlsConfig2, err := pool.buildTLSConfig()
	if err != nil {
		t.Errorf("buildTLSConfig failed: %v", err)
	}

	// Should be the same instance (cached)
	if tlsConfig1 != tlsConfig2 {
		t.Error("Expected cached TLS config to be returned")
	}
}

// TestGroupConsumer_Subscribe tests Subscribe error cases (57.1% coverage)
func TestGroupConsumer_Subscribe_AlreadySubscribed(t *testing.T) {
	config := DefaultConfig()
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	gcConfig := DefaultGroupConsumerConfig()
	gcConfig.GroupID = "test-group"
	gcConfig.Topics = []string{"test-topic"}

	gc, err := NewGroupConsumer(client, gcConfig)
	if err != nil {
		t.Fatalf("Failed to create group consumer: %v", err)
	}
	defer gc.Close()

	// Subscribe first time
	err = gc.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("First subscribe failed: %v", err)
	}

	// Subscribe second time - should fail
	err = gc.Subscribe(context.Background())
	if err == nil {
		t.Error("Expected error when subscribing twice")
	}
}

// TestTransactionalConsumer_Poll_WithMessages tests Poll with actual messages (54.8% coverage)
func TestTransactionalConsumer_Poll_WithMessages(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create topic and produce messages
	err = client.CreateTopic("test-topic", 1, 1)
	if err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	producer := NewProducer(client)
	_ = producer.Send("test-topic", []byte("key1"), []byte("value1"))
	_ = producer.Send("test-topic", []byte("key2"), []byte("value2"))
	producer.Close()

	// Create transactional consumer
	tcConfig := &TransactionalConsumerConfig{
		Client:         client,
		Topics:         []string{"test-topic"},
		IsolationLevel: ReadUncommitted,
		MaxPollRecords: 10,
	}

	tc, err := NewTransactionalConsumer(tcConfig)
	if err != nil {
		t.Fatalf("Failed to create transactional consumer: %v", err)
	}
	defer tc.Close()

	// Poll for messages
	ctx := context.Background()
	records, err := tc.Poll(ctx)
	if err != nil {
		t.Errorf("Poll failed: %v", err)
	}

	if len(records) != 2 {
		t.Errorf("Expected 2 records, got %d", len(records))
	}
}

func TestTransactionalConsumer_Poll_WithFiltering(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create topic and produce messages
	err = client.CreateTopic("test-topic", 1, 1)
	if err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	producer := NewProducer(client)
	_ = producer.Send("test-topic", []byte("key1"), []byte("value1"))
	_ = producer.Send("test-topic", []byte("key2"), []byte("value2"))
	_ = producer.Send("test-topic", []byte("key3"), []byte("value3"))
	producer.Close()

	// Create transactional consumer with ReadCommitted
	tcConfig := &TransactionalConsumerConfig{
		Client:         client,
		Topics:         []string{"test-topic"},
		IsolationLevel: ReadCommitted,
		MaxPollRecords: 10,
	}

	tc, err := NewTransactionalConsumer(tcConfig)
	if err != nil {
		t.Fatalf("Failed to create transactional consumer: %v", err)
	}
	defer tc.Close()

	// Set LSO to filter some messages
	tc.UpdateLastStableOffset("test-topic", 0, 1)

	// Poll for messages
	ctx := context.Background()
	records, err := tc.Poll(ctx)
	if err != nil {
		t.Errorf("Poll failed: %v", err)
	}

	// Should filter messages beyond LSO
	if len(records) >= 3 {
		t.Errorf("Expected messages to be filtered, got %d records", len(records))
	}

	// Check filtered count
	stats := tc.Stats()
	if stats.MessagesFiltered == 0 {
		t.Error("Expected some messages to be filtered")
	}
}

func TestTransactionalConsumer_Poll_MaxPollRecords(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create topic and produce many messages
	err = client.CreateTopic("test-topic", 1, 1)
	if err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}

	producer := NewProducer(client)
	for i := 0; i < 100; i++ {
		_ = producer.Send("test-topic", []byte("key"), []byte("value"))
	}
	producer.Close()

	// Create transactional consumer with low MaxPollRecords
	tcConfig := &TransactionalConsumerConfig{
		Client:         client,
		Topics:         []string{"test-topic"},
		IsolationLevel: ReadUncommitted,
		MaxPollRecords: 5,
	}

	tc, err := NewTransactionalConsumer(tcConfig)
	if err != nil {
		t.Fatalf("Failed to create transactional consumer: %v", err)
	}
	defer tc.Close()

	// Poll for messages
	ctx := context.Background()
	records, err := tc.Poll(ctx)
	if err != nil {
		t.Errorf("Poll failed: %v", err)
	}

	// Should respect MaxPollRecords
	if len(records) > 5 {
		t.Errorf("Expected at most 5 records, got %d", len(records))
	}
}
