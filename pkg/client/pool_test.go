package client

import (
	"testing"
	"time"
)

func TestNewConnectionPool(t *testing.T) {
	config := DefaultConfig()
	pool := NewConnectionPool(config)

	if pool == nil {
		t.Fatal("Expected non-nil connection pool")
	}

	if pool.config != config {
		t.Error("Expected config to be set")
	}

	if pool.connections == nil {
		t.Error("Expected connections map to be initialized")
	}

	if pool.closed {
		t.Error("Expected pool to not be closed initially")
	}

	// Clean up
	pool.Close()
}

func TestConnectionPool_GetAndPutDetailed(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	pool := NewConnectionPool(config)
	defer pool.Close()

	// Get a connection
	conn, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}

	if conn == nil {
		t.Fatal("Expected non-nil connection")
	}

	if conn.broker != addr {
		t.Errorf("Expected broker %s, got %s", addr, conn.broker)
	}

	// Verify connection is marked as in use
	conn.mu.Lock()
	if !conn.inUse {
		t.Error("Expected connection to be in use")
	}
	conn.mu.Unlock()

	// Put connection back
	pool.Put(conn)

	// Verify connection is now idle
	conn.mu.Lock()
	if conn.inUse {
		t.Error("Expected connection to be idle after Put")
	}
	conn.mu.Unlock()

	// Get the same connection again (should reuse)
	conn2, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("Failed to get connection second time: %v", err)
	}

	if conn2 != conn {
		t.Error("Expected to reuse the same connection")
	}
}

func TestConnectionPool_GetMultipleConnections(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}
	config.MaxConnectionsPerBroker = 3

	pool := NewConnectionPool(config)
	defer pool.Close()

	// Get multiple connections
	conns := make([]*connection, 3)
	for i := 0; i < 3; i++ {
		conn, err := pool.Get(addr)
		if err != nil {
			t.Fatalf("Failed to get connection %d: %v", i, err)
		}
		conns[i] = conn
	}

	// Try to get one more (should fail - at max limit)
	_, err := pool.Get(addr)
	if err != ErrNoConnection {
		t.Errorf("Expected ErrNoConnection when at max capacity, got %v", err)
	}

	// Put one back
	pool.Put(conns[0])

	// Now should be able to get one
	conn, err := pool.Get(addr)
	if err != nil {
		t.Errorf("Failed to get connection after putting one back: %v", err)
	}

	if conn != conns[0] {
		t.Error("Expected to reuse the connection that was put back")
	}
}

func TestConnectionPool_Remove(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	pool := NewConnectionPool(config)
	defer pool.Close()

	// Get a connection
	conn, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}

	// Check stats before removal
	statsBefore := pool.Stats()
	if statsBefore.TotalConnections != 1 {
		t.Errorf("Expected 1 connection, got %d", statsBefore.TotalConnections)
	}

	// Remove the connection
	err = pool.Remove(conn)
	if err != nil {
		t.Errorf("Failed to remove connection: %v", err)
	}

	// Check stats after removal
	statsAfter := pool.Stats()
	if statsAfter.TotalConnections != 0 {
		t.Errorf("Expected 0 connections after removal, got %d", statsAfter.TotalConnections)
	}

	// Get a new connection (should create a new one since we removed the previous)
	conn2, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("Failed to get new connection: %v", err)
	}

	if conn2 == conn {
		t.Error("Expected a different connection after removal")
	}
}

func TestConnectionPool_CloseDetailed(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	pool := NewConnectionPool(config)

	// Get some connections
	conn1, _ := pool.Get(addr)
	conn2, _ := pool.Get(addr)
	pool.Put(conn1)
	pool.Put(conn2)

	// Close the pool
	err := pool.Close()
	if err != nil {
		t.Errorf("Failed to close pool: %v", err)
	}

	// Verify pool is closed
	if !pool.closed {
		t.Error("Expected pool to be closed")
	}

	// Try to get connection after close
	_, err = pool.Get(addr)
	if err != ErrConnectionPoolClosed {
		t.Errorf("Expected ErrConnectionPoolClosed, got %v", err)
	}

	// Second close should be idempotent
	err = pool.Close()
	if err != nil {
		t.Errorf("Expected no error on second close, got %v", err)
	}
}

func TestConnectionPool_StatsDetailed(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	pool := NewConnectionPool(config)
	defer pool.Close()

	// Initial stats
	stats := pool.Stats()
	if stats.TotalConnections != 0 {
		t.Errorf("Expected 0 total connections, got %d", stats.TotalConnections)
	}

	// Get some connections
	conn1, _ := pool.Get(addr)
	conn2, _ := pool.Get(addr)
	conn3, _ := pool.Get(addr)

	// Check active connections
	stats = pool.Stats()
	if stats.TotalConnections != 3 {
		t.Errorf("Expected 3 total connections, got %d", stats.TotalConnections)
	}
	if stats.ActiveConnections != 3 {
		t.Errorf("Expected 3 active connections, got %d", stats.ActiveConnections)
	}
	if stats.IdleConnections != 0 {
		t.Errorf("Expected 0 idle connections, got %d", stats.IdleConnections)
	}

	// Put some back
	pool.Put(conn1)
	pool.Put(conn2)

	// Check idle connections
	stats = pool.Stats()
	if stats.TotalConnections != 3 {
		t.Errorf("Expected 3 total connections, got %d", stats.TotalConnections)
	}
	if stats.ActiveConnections != 1 {
		t.Errorf("Expected 1 active connection, got %d", stats.ActiveConnections)
	}
	if stats.IdleConnections != 2 {
		t.Errorf("Expected 2 idle connections, got %d", stats.IdleConnections)
	}

	// Check broker-specific stats
	brokerStats, exists := stats.BrokerStats[addr]
	if !exists {
		t.Error("Expected broker stats to exist")
	}
	if brokerStats.Total != 3 {
		t.Errorf("Expected 3 total broker connections, got %d", brokerStats.Total)
	}
	if brokerStats.Active != 1 {
		t.Errorf("Expected 1 active broker connection, got %d", brokerStats.Active)
	}
	if brokerStats.Idle != 2 {
		t.Errorf("Expected 2 idle broker connections, got %d", brokerStats.Idle)
	}

	pool.Put(conn3)
}

func TestConnectionPool_Cleanup(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	pool := NewConnectionPool(config)
	defer pool.Close()

	// Get a connection
	conn, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}

	// Put it back
	pool.Put(conn)

	// Manually set lastUsed to trigger cleanup
	conn.mu.Lock()
	conn.lastUsed = time.Now().Add(-10 * time.Minute)
	conn.mu.Unlock()

	// Run cleanup
	pool.cleanup()

	// Connection should be removed
	stats := pool.Stats()
	if stats.TotalConnections != 0 {
		t.Errorf("Expected connection to be cleaned up, got %d connections", stats.TotalConnections)
	}
}

func TestConnectionPool_CleanupUnhealthyConnection(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	pool := NewConnectionPool(config)
	defer pool.Close()

	// Get a connection
	conn, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}

	// Put it back
	pool.Put(conn)

	// Close the underlying connection to make it unhealthy
	conn.conn.Close()

	// Run cleanup
	pool.cleanup()

	// Connection should be removed because it's unhealthy
	stats := pool.Stats()
	if stats.TotalConnections != 0 {
		t.Errorf("Expected unhealthy connection to be cleaned up, got %d connections", stats.TotalConnections)
	}
}

func TestConnection_MarkUsed(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	pool := NewConnectionPool(config)
	defer pool.Close()

	conn, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}

	// markUsed is called when getting connection, so it should be in use
	conn.mu.Lock()
	if !conn.inUse {
		t.Error("Expected connection to be in use")
	}
	beforeTime := conn.lastUsed
	conn.mu.Unlock()

	// Wait a bit and mark as used again
	time.Sleep(10 * time.Millisecond)
	conn.markUsed()

	conn.mu.Lock()
	if !conn.inUse {
		t.Error("Expected connection to still be in use")
	}
	if !conn.lastUsed.After(beforeTime) {
		t.Error("Expected lastUsed to be updated")
	}
	conn.mu.Unlock()
}

func TestConnection_IsHealthy(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	pool := NewConnectionPool(config)
	defer pool.Close()

	conn, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}

	// Connection should be healthy
	if !conn.isHealthy() {
		t.Error("Expected connection to be healthy")
	}

	// Close the connection
	conn.conn.Close()

	// Connection should now be unhealthy
	if conn.isHealthy() {
		t.Error("Expected connection to be unhealthy after closing")
	}
}

func TestConnectionPool_BuildTLSConfig(t *testing.T) {
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
		t.Fatalf("Failed to build TLS config: %v", err)
	}

	if tlsConfig == nil {
		t.Fatal("Expected non-nil TLS config")
	}

	if !tlsConfig.InsecureSkipVerify {
		t.Error("Expected InsecureSkipVerify to be true")
	}

	if tlsConfig.ServerName != "test-server" {
		t.Errorf("Expected ServerName 'test-server', got '%s'", tlsConfig.ServerName)
	}

	// Second call should return cached config
	tlsConfig2, err := pool.buildTLSConfig()
	if err != nil {
		t.Fatalf("Failed to build TLS config second time: %v", err)
	}

	if tlsConfig2 != tlsConfig {
		t.Error("Expected cached TLS config to be returned")
	}
}

func TestConnectionPool_BuildTLSConfig_InvalidCAFile(t *testing.T) {
	config := DefaultConfig()
	config.Security = &SecurityConfig{
		TLS: &TLSConfig{
			Enabled: true,
			CAFile:  "/nonexistent/ca.pem",
		},
	}

	pool := NewConnectionPool(config)
	defer pool.Close()

	_, err := pool.buildTLSConfig()
	if err == nil {
		t.Error("Expected error when CA file doesn't exist")
	}
}

func TestConnectionPool_CreateConnection_InvalidBroker(t *testing.T) {
	config := DefaultConfig()
	config.ConnectTimeout = 100 * time.Millisecond

	pool := NewConnectionPool(config)
	defer pool.Close()

	// Try to connect to invalid broker
	_, err := pool.createConnection("invalid-broker:9999")
	if err == nil {
		t.Error("Expected error when connecting to invalid broker")
	}
}

func TestConnectionPool_PutNilConnection(t *testing.T) {
	config := DefaultConfig()
	pool := NewConnectionPool(config)
	defer pool.Close()

	// Put nil connection should not panic
	pool.Put(nil)
}

func TestConnection_NextRequestID(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	pool := NewConnectionPool(config)
	defer pool.Close()

	conn, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}

	// Get multiple request IDs
	id1 := conn.nextRequestID()
	id2 := conn.nextRequestID()
	id3 := conn.nextRequestID()

	if id1 >= id2 {
		t.Errorf("Expected request ID to increment, got %d then %d", id1, id2)
	}

	if id2 >= id3 {
		t.Errorf("Expected request ID to increment, got %d then %d", id2, id3)
	}
}

func TestConnection_Close(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	pool := NewConnectionPool(config)
	defer pool.Close()

	conn, err := pool.Get(addr)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}

	// Close should not error
	err = conn.close()
	if err != nil {
		t.Errorf("Unexpected error closing connection: %v", err)
	}

	// Second close will error (connection already closed) - that's expected
	// Just verify it doesn't panic
	_ = conn.close()
}

func TestConnectionPool_MultipleCleanupCycles(t *testing.T) {
	srv, addr := setupTestServer(t)
	defer func() { _ = srv.Stop() }()

	config := DefaultConfig()
	config.Brokers = []string{addr}

	pool := NewConnectionPool(config)
	defer pool.Close()

	// Get and put connections multiple times
	for i := 0; i < 5; i++ {
		conn, err := pool.Get(addr)
		if err != nil {
			t.Fatalf("Failed to get connection: %v", err)
		}
		pool.Put(conn)
	}

	// Run cleanup multiple times
	for i := 0; i < 3; i++ {
		pool.cleanup()
		time.Sleep(10 * time.Millisecond)
	}

	// Pool should still be functional
	conn, err := pool.Get(addr)
	if err != nil {
		t.Errorf("Failed to get connection after cleanup cycles: %v", err)
	}

	if conn == nil {
		t.Error("Expected non-nil connection after cleanup cycles")
	}
}
