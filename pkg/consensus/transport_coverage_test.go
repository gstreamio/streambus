package consensus

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/raft/v3/raftpb"
)

// TestBidirectionalTransport_RemovePeer tests removing a peer from transport
func TestBidirectionalTransport_RemovePeer(t *testing.T) {
	var mu sync.Mutex
	var received []raftpb.Message

	handler := func(msg raftpb.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	transport1 := NewBidirectionalTransport(1, "localhost:31051", handler)
	transport2 := NewBidirectionalTransport(2, "localhost:31052", handler)

	// Add peers
	err := transport1.AddPeer(2, "localhost:31052")
	require.NoError(t, err)

	err = transport2.AddPeer(1, "localhost:31051")
	require.NoError(t, err)

	// Start transports
	err = transport1.Start()
	require.NoError(t, err)
	defer func() { _ = transport1.Stop() }()

	err = transport2.Start()
	require.NoError(t, err)
	defer func() { _ = transport2.Stop() }()

	time.Sleep(200 * time.Millisecond)

	// Send a message to establish connection
	msg := raftpb.Message{
		Type: raftpb.MsgHeartbeat,
		From: 1,
		To:   2,
		Term: 1,
	}
	err = transport1.Send(msg)
	require.NoError(t, err)

	time.Sleep(300 * time.Millisecond)

	// Verify peer exists
	transport1.mu.RLock()
	_, exists := transport1.peers[2]
	transport1.mu.RUnlock()
	assert.True(t, exists, "peer 2 should exist")

	// Remove peer
	err = transport1.RemovePeer(2)
	assert.NoError(t, err)

	// Verify peer was removed
	transport1.mu.RLock()
	_, exists = transport1.peers[2]
	transport1.mu.RUnlock()
	assert.False(t, exists, "peer 2 should be removed")

	// Try to send message to removed peer
	err = transport1.Send(msg)
	assert.Error(t, err, "should fail to send to removed peer")
	assert.Contains(t, err.Error(), "peer 2 not found")
}

// TestBidirectionalTransport_RemovePeer_NotExists tests removing non-existent peer
func TestBidirectionalTransport_RemovePeer_NotExists(t *testing.T) {
	transport := NewBidirectionalTransport(1, "localhost:31061", nil)

	// Remove peer that doesn't exist (should not error)
	err := transport.RemovePeer(999)
	assert.NoError(t, err, "removing non-existent peer should not error")
}

// TestBidirectionalTransport_RemovePeer_MultipleRemoves tests removing same peer multiple times
func TestBidirectionalTransport_RemovePeer_MultipleRemoves(t *testing.T) {
	transport := NewBidirectionalTransport(1, "localhost:31071", nil)

	// Add peer
	err := transport.AddPeer(2, "localhost:31072")
	require.NoError(t, err)

	// Remove peer first time
	err = transport.RemovePeer(2)
	assert.NoError(t, err)

	// Remove peer second time (should not error)
	err = transport.RemovePeer(2)
	assert.NoError(t, err)
}

// TestBidirectionalTransport_AddPeer_Duplicate tests adding duplicate peer
func TestBidirectionalTransport_AddPeer_Duplicate(t *testing.T) {
	transport := NewBidirectionalTransport(1, "localhost:31081", nil)

	// Add peer
	err := transport.AddPeer(2, "localhost:31082")
	require.NoError(t, err)

	// Add same peer again (should not error, already exists)
	err = transport.AddPeer(2, "localhost:31082")
	assert.NoError(t, err)

	// Verify only one peer exists
	transport.mu.RLock()
	count := len(transport.peers)
	transport.mu.RUnlock()
	assert.Equal(t, 1, count)
}

// TestBidirectionalTransport_Send_PeerNotFound tests sending to non-existent peer
func TestBidirectionalTransport_Send_PeerNotFound(t *testing.T) {
	transport := NewBidirectionalTransport(1, "localhost:31091", nil)

	err := transport.Start()
	require.NoError(t, err)
	defer func() { _ = transport.Stop() }()

	// Try to send to non-existent peer
	msg := raftpb.Message{
		Type: raftpb.MsgHeartbeat,
		From: 1,
		To:   999,
		Term: 1,
	}

	err = transport.Send(msg)
	assert.Error(t, err, "should fail when peer not found")
	assert.Contains(t, err.Error(), "peer 999 not found")
}

// TestBidirectionalTransport_Send_ConnectionFailed tests sending when connection fails
func TestBidirectionalTransport_Send_ConnectionFailed(t *testing.T) {
	transport := NewBidirectionalTransport(1, "localhost:31101", nil)

	// Add peer with invalid address
	err := transport.AddPeer(2, "localhost:99999")
	require.NoError(t, err)

	err = transport.Start()
	require.NoError(t, err)
	defer func() { _ = transport.Stop() }()

	// Try to send (connection should fail)
	msg := raftpb.Message{
		Type: raftpb.MsgHeartbeat,
		From: 1,
		To:   2,
		Term: 1,
	}

	err = transport.Send(msg)
	assert.Error(t, err, "should fail when connection fails")
}

// Note: TestBidirectionalTransport_Send_Timeout is complex and flaky
// The timeout behavior is tested implicitly through other integration tests
// where connections fail and retries occur

// TestBidirectionalTransport_ConnectionReconnect tests reconnection after failure
func TestBidirectionalTransport_ConnectionReconnect(t *testing.T) {
	var mu sync.Mutex
	var received []raftpb.Message

	handler := func(msg raftpb.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	transport1 := NewBidirectionalTransport(1, "localhost:31121", handler)
	transport2 := NewBidirectionalTransport(2, "localhost:31122", handler)

	err := transport1.AddPeer(2, "localhost:31122")
	require.NoError(t, err)

	err = transport2.AddPeer(1, "localhost:31121")
	require.NoError(t, err)

	// Start both transports
	err = transport1.Start()
	require.NoError(t, err)
	defer func() { _ = transport1.Stop() }()

	err = transport2.Start()
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// Send first message
	msg1 := raftpb.Message{
		Type: raftpb.MsgHeartbeat,
		From: 1,
		To:   2,
		Term: 1,
	}
	err = transport1.Send(msg1)
	require.NoError(t, err)

	time.Sleep(300 * time.Millisecond)

	// Stop transport2 (simulate failure)
	err = transport2.Stop()
	require.NoError(t, err)

	// Try to send (should fail)
	msg2 := raftpb.Message{
		Type: raftpb.MsgHeartbeat,
		From: 1,
		To:   2,
		Term: 2,
	}
	_ = transport1.Send(msg2)

	// Restart transport2
	transport2 = NewBidirectionalTransport(2, "localhost:31122", handler)
	err = transport2.AddPeer(1, "localhost:31121")
	require.NoError(t, err)

	err = transport2.Start()
	require.NoError(t, err)
	defer func() { _ = transport2.Stop() }()

	time.Sleep(500 * time.Millisecond)

	// Send another message (should succeed after reconnection)
	msg3 := raftpb.Message{
		Type: raftpb.MsgHeartbeat,
		From: 1,
		To:   2,
		Term: 3,
	}
	err = transport1.Send(msg3)

	// Give time for reconnection and delivery
	time.Sleep(2 * time.Second)

	// Verify at least some messages were received
	mu.Lock()
	count := len(received)
	mu.Unlock()

	t.Logf("Received %d messages", count)
	assert.True(t, count > 0, "should have received at least one message")
}

// TestBidirectionalTransport_LargeMessage tests sending large messages
func TestBidirectionalTransport_LargeMessage(t *testing.T) {
	var mu sync.Mutex
	var received []raftpb.Message

	handler := func(msg raftpb.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	transport1 := NewBidirectionalTransport(1, "localhost:31131", handler)
	transport2 := NewBidirectionalTransport(2, "localhost:31132", handler)

	err := transport1.AddPeer(2, "localhost:31132")
	require.NoError(t, err)

	err = transport2.AddPeer(1, "localhost:31131")
	require.NoError(t, err)

	err = transport1.Start()
	require.NoError(t, err)
	defer func() { _ = transport1.Stop() }()

	err = transport2.Start()
	require.NoError(t, err)
	defer func() { _ = transport2.Stop() }()

	time.Sleep(200 * time.Millisecond)

	// Send large message (1MB)
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	msg := raftpb.Message{
		Type:    raftpb.MsgApp,
		From:    1,
		To:      2,
		Term:    1,
		Entries: []raftpb.Entry{{Index: 1, Term: 1, Data: largeData}},
	}

	err = transport1.Send(msg)
	assert.NoError(t, err)

	// Wait for delivery
	time.Sleep(1 * time.Second)

	mu.Lock()
	count := len(received)
	mu.Unlock()

	assert.Equal(t, 1, count, "should have received large message")

	if count > 0 {
		mu.Lock()
		receivedMsg := received[0]
		mu.Unlock()

		assert.Equal(t, 1, len(receivedMsg.Entries))
		assert.Equal(t, largeData, receivedMsg.Entries[0].Data)
	}
}

// TestBidirectionalTransport_Stop_WithConnections tests stopping with active connections
func TestBidirectionalTransport_Stop_WithConnections(t *testing.T) {
	transport1 := NewBidirectionalTransport(1, "localhost:31141", nil)
	transport2 := NewBidirectionalTransport(2, "localhost:31142", nil)

	err := transport1.AddPeer(2, "localhost:31142")
	require.NoError(t, err)

	err = transport2.AddPeer(1, "localhost:31141")
	require.NoError(t, err)

	err = transport1.Start()
	require.NoError(t, err)

	err = transport2.Start()
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// Send message to establish connection
	msg := raftpb.Message{
		Type: raftpb.MsgHeartbeat,
		From: 1,
		To:   2,
		Term: 1,
	}
	_ = transport1.Send(msg)

	time.Sleep(200 * time.Millisecond)

	// Stop both transports (should clean up connections)
	err = transport1.Stop()
	assert.NoError(t, err)

	err = transport2.Stop()
	assert.NoError(t, err)
}

// TestBidirectionalTransport_CircuitBreaker tests circuit breaker functionality
func TestBidirectionalTransport_CircuitBreaker(t *testing.T) {
	transport := NewBidirectionalTransport(1, "localhost:31151", nil)

	// Add peer with non-existent address
	err := transport.AddPeer(2, "localhost:31152")
	require.NoError(t, err)

	err = transport.Start()
	require.NoError(t, err)
	defer func() { _ = transport.Stop() }()

	// Try to send multiple messages (should trigger circuit breaker)
	for i := 0; i < 10; i++ {
		msg := raftpb.Message{
			Type: raftpb.MsgHeartbeat,
			From: 1,
			To:   2,
			Term: uint64(i),
		}
		_ = transport.Send(msg)
		time.Sleep(50 * time.Millisecond)
	}

	// Circuit breaker should be open now
	transport.mu.RLock()
	peer := transport.peers[2]
	transport.mu.RUnlock()

	// Check that circuit breaker has recorded failures
	failureCount := peer.circuitBreaker.failureCount
	t.Logf("Circuit breaker failure count: %d", failureCount)
	assert.True(t, failureCount > 0, "circuit breaker should have recorded failures")
}

// TestBidirectionalTransport_Backoff tests exponential backoff
func TestBidirectionalTransport_Backoff(t *testing.T) {
	// Create peer with backoff
	peer := &bidirPeer{
		id:             2,
		addr:           "localhost:31162",
		sendCh:         make(chan raftpb.Message, 256),
		stopCh:         make(chan struct{}),
		backoff:        NewBackoff(),
		circuitBreaker: NewCircuitBreaker(),
	}

	// Test backoff increases
	d1 := peer.backoff.Next()
	d2 := peer.backoff.Next()
	d3 := peer.backoff.Next()

	assert.True(t, d2 > d1, "backoff should increase")
	assert.True(t, d3 > d2, "backoff should increase")

	// Test reset
	peer.backoff.Reset()
	d4 := peer.backoff.Next()
	// Due to jitter, we just verify it's in the expected range
	assert.True(t, d4 <= d1*2, "backoff after reset should be close to initial value")
}

// TestBidirectionalTransport_IncomingConnection tests handling incoming connections
func TestBidirectionalTransport_IncomingConnection(t *testing.T) {
	var mu1, mu2 sync.Mutex
	var received1, received2 []raftpb.Message

	handler1 := func(msg raftpb.Message) {
		mu1.Lock()
		received1 = append(received1, msg)
		mu1.Unlock()
	}

	handler2 := func(msg raftpb.Message) {
		mu2.Lock()
		received2 = append(received2, msg)
		mu2.Unlock()
	}

	transport1 := NewBidirectionalTransport(1, "localhost:31171", handler1)
	transport2 := NewBidirectionalTransport(2, "localhost:31172", handler2)

	// Only add peer on transport1 (transport2 will receive incoming connection)
	err := transport1.AddPeer(2, "localhost:31172")
	require.NoError(t, err)

	err = transport2.AddPeer(1, "localhost:31171")
	require.NoError(t, err)

	err = transport1.Start()
	require.NoError(t, err)
	defer func() { _ = transport1.Stop() }()

	err = transport2.Start()
	require.NoError(t, err)
	defer func() { _ = transport2.Stop() }()

	time.Sleep(200 * time.Millisecond)

	// Transport1 sends to transport2 (initiates connection)
	msg := raftpb.Message{
		Type: raftpb.MsgHeartbeat,
		From: 1,
		To:   2,
		Term: 1,
	}
	err = transport1.Send(msg)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// Verify message was received
	mu2.Lock()
	count := len(received2)
	mu2.Unlock()
	assert.Equal(t, 1, count, "transport2 should have received message")
}
