package consensus

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/raft/v3/raftpb"
)

func TestBidirectionalTransport_SendReceive(t *testing.T) {
	// Create two transports
	var mu sync.Mutex
	var received []raftpb.Message

	handler := func(msg raftpb.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	transport1 := NewBidirectionalTransport(1, "localhost:31001", handler)
	transport2 := NewBidirectionalTransport(2, "localhost:31002", handler)

	// Add peers (bidirectional)
	_ = transport1.AddPeer(2, "localhost:31002")
	_ = transport2.AddPeer(1, "localhost:31001")

	// Start transports
	err := transport1.Start()
	require.NoError(t, err)
	defer func() { _ = transport1.Stop() }()

	err = transport2.Start()
	require.NoError(t, err)
	defer func() { _ = transport2.Stop() }()

	// Give time for listeners to start
	time.Sleep(200 * time.Millisecond)

	// Send message from node 1 to node 2
	msg := raftpb.Message{
		Type: raftpb.MsgHeartbeat,
		From: 1,
		To:   2,
		Term: 1,
	}

	err = transport1.Send(msg)
	assert.NoError(t, err)

	// Wait for message to be received
	time.Sleep(500 * time.Millisecond)

	// Check if message was received
	mu.Lock()
	count := len(received)
	mu.Unlock()

	assert.Equal(t, 1, count, "should have received 1 message")

	if count > 0 {
		mu.Lock()
		receivedMsg := received[0]
		mu.Unlock()

		assert.Equal(t, raftpb.MsgHeartbeat, receivedMsg.Type)
		assert.Equal(t, uint64(1), receivedMsg.From)
		assert.Equal(t, uint64(2), receivedMsg.To)
		assert.Equal(t, uint64(1), receivedMsg.Term)
	}
}

func TestBidirectionalTransport_Bidirectional(t *testing.T) {
	// Test that both nodes can send and receive on the SAME connection
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

	transport1 := NewBidirectionalTransport(1, "localhost:31011", handler1)
	transport2 := NewBidirectionalTransport(2, "localhost:31012", handler2)

	_ = transport1.AddPeer(2, "localhost:31012")
	_ = transport2.AddPeer(1, "localhost:31011")

	require.NoError(t, transport1.Start())
	defer func() { _ = transport1.Stop() }()
	require.NoError(t, transport2.Start())
	defer func() { _ = transport2.Stop() }()

	time.Sleep(200 * time.Millisecond)

	// Node 1 sends to node 2
	msg1to2 := raftpb.Message{
		Type: raftpb.MsgHeartbeat,
		From: 1,
		To:   2,
		Term: 1,
	}
	require.NoError(t, transport1.Send(msg1to2))

	time.Sleep(300 * time.Millisecond)

	// Node 2 sends to node 1 (should use the SAME connection)
	msg2to1 := raftpb.Message{
		Type: raftpb.MsgHeartbeatResp,
		From: 2,
		To:   1,
		Term: 1,
	}
	require.NoError(t, transport2.Send(msg2to1))

	time.Sleep(300 * time.Millisecond)

	// Check node 1 received from node 2
	mu1.Lock()
	count1 := len(received1)
	mu1.Unlock()
	assert.Equal(t, 1, count1, "node 1 should have received 1 message")

	// Check node 2 received from node 1
	mu2.Lock()
	count2 := len(received2)
	mu2.Unlock()
	assert.Equal(t, 1, count2, "node 2 should have received 1 message")
}

func TestBidirectionalTransport_ThreeNodes(t *testing.T) {
	// Test 3-node cluster where all nodes can communicate
	var mu1, mu2, mu3 sync.Mutex
	var received1, received2, received3 []raftpb.Message

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

	handler3 := func(msg raftpb.Message) {
		mu3.Lock()
		received3 = append(received3, msg)
		mu3.Unlock()
	}

	transport1 := NewBidirectionalTransport(1, "localhost:31021", handler1)
	transport2 := NewBidirectionalTransport(2, "localhost:31022", handler2)
	transport3 := NewBidirectionalTransport(3, "localhost:31023", handler3)

	// Add all peers
	_ = transport1.AddPeer(2, "localhost:31022")
	_ = transport1.AddPeer(3, "localhost:31023")

	_ = transport2.AddPeer(1, "localhost:31021")
	_ = transport2.AddPeer(3, "localhost:31023")

	_ = transport3.AddPeer(1, "localhost:31021")
	_ = transport3.AddPeer(2, "localhost:31022")

	// Start all transports
	require.NoError(t, transport1.Start())
	defer func() { _ = transport1.Stop() }()
	require.NoError(t, transport2.Start())
	defer func() { _ = transport2.Stop() }()
	require.NoError(t, transport3.Start())
	defer func() { _ = transport3.Stop() }()

	time.Sleep(300 * time.Millisecond)

	// Node 1 broadcasts to nodes 2 and 3
	msg1to2 := raftpb.Message{
		Type: raftpb.MsgHeartbeat,
		From: 1,
		To:   2,
		Term: 1,
	}
	msg1to3 := raftpb.Message{
		Type: raftpb.MsgHeartbeat,
		From: 1,
		To:   3,
		Term: 1,
	}

	require.NoError(t, transport1.Send(msg1to2))
	require.NoError(t, transport1.Send(msg1to3))

	time.Sleep(500 * time.Millisecond)

	// Node 2 sends to node 3
	msg2to3 := raftpb.Message{
		Type: raftpb.MsgVote,
		From: 2,
		To:   3,
		Term: 2,
	}
	require.NoError(t, transport2.Send(msg2to3))

	time.Sleep(500 * time.Millisecond)

	// Verify deliveries
	mu2.Lock()
	count2 := len(received2)
	mu2.Unlock()
	assert.Equal(t, 1, count2, "node 2 should have received 1 message from node 1")

	mu3.Lock()
	count3 := len(received3)
	mu3.Unlock()
	assert.Equal(t, 2, count3, "node 3 should have received 2 messages (from nodes 1 and 2)")

	t.Logf("Node 2 received %d messages", count2)
	t.Logf("Node 3 received %d messages", count3)
}

func TestBidirectionalTransport_MultipleMessages(t *testing.T) {
	// Test sending multiple messages in quick succession
	var mu sync.Mutex
	var received []raftpb.Message

	handler := func(msg raftpb.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	transport1 := NewBidirectionalTransport(1, "localhost:31031", handler)
	transport2 := NewBidirectionalTransport(2, "localhost:31032", handler)

	_ = transport1.AddPeer(2, "localhost:31032")
	_ = transport2.AddPeer(1, "localhost:31031")

	require.NoError(t, transport1.Start())
	defer func() { _ = transport1.Stop() }()
	require.NoError(t, transport2.Start())
	defer func() { _ = transport2.Stop() }()

	time.Sleep(200 * time.Millisecond)

	// Send 10 messages quickly
	for i := 0; i < 10; i++ {
		msg := raftpb.Message{
			Type: raftpb.MsgHeartbeat,
			From: 1,
			To:   2,
			Term: uint64(i + 1),
		}
		require.NoError(t, transport1.Send(msg))
	}

	// Give time for all messages to be received
	time.Sleep(1 * time.Second)

	mu.Lock()
	count := len(received)
	mu.Unlock()

	assert.Equal(t, 10, count, "should have received all 10 messages")
	t.Logf("Received %d messages", count)
}

func TestBidirectionalTransport_ConnectionReuse(t *testing.T) {
	// Test that the same connection is reused for multiple sends
	transport1 := NewBidirectionalTransport(1, "localhost:31041", func(msg raftpb.Message) {})
	transport2 := NewBidirectionalTransport(2, "localhost:31042", func(msg raftpb.Message) {})

	_ = transport1.AddPeer(2, "localhost:31042")
	_ = transport2.AddPeer(1, "localhost:31041")

	require.NoError(t, transport1.Start())
	defer func() { _ = transport1.Stop() }()
	require.NoError(t, transport2.Start())
	defer func() { _ = transport2.Stop() }()

	time.Sleep(200 * time.Millisecond)

	// Send first message
	msg1 := raftpb.Message{Type: raftpb.MsgHeartbeat, From: 1, To: 2, Term: 1}
	require.NoError(t, transport1.Send(msg1))

	time.Sleep(100 * time.Millisecond)

	// Get peer connection
	transport1.mu.RLock()
	peer := transport1.peers[2]
	transport1.mu.RUnlock()

	peer.mu.RLock()
	firstConn := peer.conn
	peer.mu.RUnlock()

	assert.NotNil(t, firstConn, "connection should exist after first send")

	// Send second message
	msg2 := raftpb.Message{Type: raftpb.MsgHeartbeat, From: 1, To: 2, Term: 2}
	require.NoError(t, transport1.Send(msg2))

	time.Sleep(100 * time.Millisecond)

	// Check that it's the same connection
	peer.mu.RLock()
	secondConn := peer.conn
	peer.mu.RUnlock()

	assert.Equal(t, firstConn, secondConn, "should reuse the same connection")
}
