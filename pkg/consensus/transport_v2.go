package consensus

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gstreamio/streambus/pkg/logger"
	"go.etcd.io/raft/v3/raftpb"
	"go.uber.org/zap"
)

// MessageHandler is a callback function that handles incoming Raft messages.
type MessageHandler func(msg raftpb.Message)

// BidirectionalTransport implements bidirectional TCP connections for Raft messages.
// Key improvement: Each peer connection is used for both sending and receiving.
type BidirectionalTransport struct {
	mu       sync.RWMutex
	nodeID   uint64
	bindAddr string
	peers    map[uint64]*bidirPeer
	handler  MessageHandler
	listener net.Listener
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// bidirPeer represents a bidirectional connection to another Raft node.
type bidirPeer struct {
	id   uint64
	addr string

	mu       sync.RWMutex
	conn     net.Conn
	lastUsed time.Time
	sendCh   chan raftpb.Message
	stopCh   chan struct{}

	// Exponential backoff for connection retries
	backoff *Backoff

	// Circuit breaker to avoid hammering dead nodes
	circuitBreaker *CircuitBreaker
}

// NewBidirectionalTransport creates a new bidirectional transport.
func NewBidirectionalTransport(nodeID uint64, bindAddr string, handler MessageHandler) *BidirectionalTransport {
	return &BidirectionalTransport{
		nodeID:   nodeID,
		bindAddr: bindAddr,
		peers:    make(map[uint64]*bidirPeer),
		handler:  handler,
		stopCh:   make(chan struct{}),
	}
}

// Start starts the transport layer.
func (t *BidirectionalTransport) Start() error {
	listener, err := net.Listen("tcp", t.bindAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", t.bindAddr, err)
	}

	t.listener = listener

	// Start accepting connections
	t.wg.Add(1)
	go t.acceptLoop()

	return nil
}

// Stop stops the transport layer.
func (t *BidirectionalTransport) Stop() error {
	close(t.stopCh)

	if t.listener != nil {
		t.listener.Close()
	}

	// Stop all peer connections
	t.mu.Lock()
	for _, p := range t.peers {
		t.stopPeer(p)
	}
	t.mu.Unlock()

	t.wg.Wait()
	return nil
}

// Send sends a message to the specified node.
func (t *BidirectionalTransport) Send(msg raftpb.Message) error {
	// Get peer
	t.mu.RLock()
	peer, exists := t.peers[msg.To]
	t.mu.RUnlock()

	if !exists {
		return fmt.Errorf("peer %d not found", msg.To)
	}

	// Ensure connection is established
	if err := t.ensureConnected(peer); err != nil {
		return fmt.Errorf("failed to connect to peer %d: %w", msg.To, err)
	}

	// Send via channel (non-blocking with timeout)
	select {
	case peer.sendCh <- msg:
		return nil
	case <-time.After(1 * time.Second):
		return fmt.Errorf("send timeout for peer %d", msg.To)
	case <-t.stopCh:
		return fmt.Errorf("transport stopped")
	}
}

// AddPeer adds a peer to the transport.
func (t *BidirectionalTransport) AddPeer(nodeID uint64, addr string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.peers[nodeID]; exists {
		return nil // Already exists
	}

	peer := &bidirPeer{
		id:             nodeID,
		addr:           addr,
		sendCh:         make(chan raftpb.Message, 256),
		stopCh:         make(chan struct{}),
		backoff:        NewBackoff(),
		circuitBreaker: NewCircuitBreaker(),
	}

	t.peers[nodeID] = peer
	return nil
}

// RemovePeer removes a peer from the transport.
func (t *BidirectionalTransport) RemovePeer(nodeID uint64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	peer, exists := t.peers[nodeID]
	if !exists {
		return nil
	}

	t.stopPeer(peer)
	delete(t.peers, nodeID)
	return nil
}

// ensureConnected ensures a connection to the peer exists.
// Implements industry-standard exponential backoff and circuit breaker pattern.
func (t *BidirectionalTransport) ensureConnected(peer *bidirPeer) error {
	peer.mu.RLock()
	if peer.conn != nil {
		peer.mu.RUnlock()
		return nil
	}
	peer.mu.RUnlock()

	// Check circuit breaker before attempting connection
	if !peer.circuitBreaker.Call() {
		logger.Debug("circuit breaker open, skipping connection attempt",
			zap.Uint64("peerId", peer.id),
			zap.String("addr", peer.addr))
		return fmt.Errorf("circuit breaker open for peer %d", peer.id)
	}

	peer.mu.Lock()
	defer peer.mu.Unlock()

	// Double-check after acquiring write lock
	if peer.conn != nil {
		return nil
	}

	// Apply exponential backoff before connection attempt
	// This prevents hammering the peer during restarts
	backoffDuration := peer.backoff.Next()
	logger.Debug("applying backoff before connection attempt",
		zap.Uint64("peerId", peer.id),
		zap.Duration("backoff", backoffDuration))
	time.Sleep(backoffDuration)

	// Try to establish connection
	conn, err := net.DialTimeout("tcp", peer.addr, 5*time.Second)
	if err != nil {
		// Record failure in circuit breaker
		peer.circuitBreaker.RecordFailure()

		// Log at debug level to avoid spam (sampled in production)
		logger.Debug("failed to connect to peer",
			zap.Uint64("peerId", peer.id),
			zap.String("addr", peer.addr),
			zap.Duration("backoff", backoffDuration),
			zap.Int("failureCount", peer.circuitBreaker.failureCount),
			zap.Error(err))

		return fmt.Errorf("failed to connect to %s: %w", peer.addr, err)
	}

	// Send handshake: our node ID
	if err := binary.Write(conn, binary.BigEndian, t.nodeID); err != nil {
		conn.Close()
		peer.circuitBreaker.RecordFailure()

		logger.Debug("failed to send handshake to peer",
			zap.Uint64("peerId", peer.id),
			zap.String("addr", peer.addr),
			zap.Error(err))

		return fmt.Errorf("failed to send handshake: %w", err)
	}

	// Connection successful!
	peer.conn = conn
	peer.lastUsed = time.Now()

	// Reset backoff and record success in circuit breaker
	peer.backoff.Reset()
	peer.circuitBreaker.RecordSuccess()

	// Log success at info level (important event)
	logger.Info("connected to raft peer",
		zap.Uint64("nodeId", t.nodeID),
		zap.Uint64("peerId", peer.id),
		zap.String("addr", peer.addr))

	// Start sender and receiver goroutines
	t.wg.Add(2)
	go t.senderLoop(peer)
	go t.receiverLoop(peer)

	return nil
}

// stopPeer stops a peer connection.
func (t *BidirectionalTransport) stopPeer(peer *bidirPeer) {
	close(peer.stopCh)

	peer.mu.Lock()
	if peer.conn != nil {
		peer.conn.Close()
		peer.conn = nil
	}
	peer.mu.Unlock()
}

// senderLoop sends messages from the send channel.
func (t *BidirectionalTransport) senderLoop(peer *bidirPeer) {
	defer t.wg.Done()

	for {
		select {
		case <-peer.stopCh:
			return
		case <-t.stopCh:
			return
		case msg := <-peer.sendCh:
			// Keep trying to send the message with backoff
			maxRetries := 3
			backoff := 10 * time.Millisecond

			for attempt := 0; attempt <= maxRetries; attempt++ {
				err := t.sendMessage(peer, msg)
				if err == nil {
					// Success!
					break
				}

				// Connection failed, close it
				peer.mu.Lock()
				if peer.conn != nil {
					peer.conn.Close()
					peer.conn = nil
				}
				peer.mu.Unlock()

				// If this was the last attempt, drop the message
				if attempt == maxRetries {
					// Log error but continue processing next messages
					// rather than exiting the loop entirely
					break
				}

				// Wait before retrying
				time.Sleep(backoff)
				backoff *= 2

				// Try to reconnect
				_ = t.ensureConnected(peer)
			}
		}
	}
}

// sendMessage sends a single message on the connection.
func (t *BidirectionalTransport) sendMessage(peer *bidirPeer, msg raftpb.Message) error {
	peer.mu.RLock()
	conn := peer.conn
	peer.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("no connection")
	}

	// Serialize message
	data, err := msg.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Write message length
	if err := binary.Write(conn, binary.BigEndian, uint32(len(data))); err != nil {
		return fmt.Errorf("failed to write message length: %w", err)
	}

	// Write message data
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("failed to write message data: %w", err)
	}

	peer.mu.Lock()
	peer.lastUsed = time.Now()
	peer.mu.Unlock()

	return nil
}

// receiverLoop receives messages from the connection.
func (t *BidirectionalTransport) receiverLoop(peer *bidirPeer) {
	defer t.wg.Done()

	peer.mu.RLock()
	conn := peer.conn
	peer.mu.RUnlock()

	if conn == nil {
		return
	}

	for {
		select {
		case <-peer.stopCh:
			return
		case <-t.stopCh:
			return
		default:
		}

		// Set read deadline
		if err := conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
			return
		}

		// Read message length
		var msgLen uint32
		if err := binary.Read(conn, binary.BigEndian, &msgLen); err != nil {
			if isTimeout(err) {
				// Timeout is expected, continue loop to check stop channel
				continue
			}
			// Connection closed or other error
			return
		}

		// Validate message size (max 10MB)
		if msgLen > 10*1024*1024 {
			return
		}

		// Read message data
		data := make([]byte, msgLen)
		if _, err := io.ReadFull(conn, data); err != nil {
			return
		}

		// Unmarshal message
		var msg raftpb.Message
		if err := msg.Unmarshal(data); err != nil {
			continue
		}

		// Pass to handler
		if t.handler != nil {
			t.handler(msg)
		}
	}
}

// acceptLoop accepts incoming connections.
func (t *BidirectionalTransport) acceptLoop() {
	defer t.wg.Done()

	for {
		select {
		case <-t.stopCh:
			return
		default:
		}

		// Set accept deadline to check stopCh periodically
		if err := t.listener.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second)); err != nil {
			continue
		}

		conn, err := t.listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // Timeout is expected, check stopCh
			}
			// Only log non-timeout errors if we're not stopping
			select {
			case <-t.stopCh:
				return
			default:
			}
			continue
		}

		t.wg.Add(1)
		go t.handleIncomingConnection(conn)
	}
}

// isTimeout checks if an error is a timeout error.
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}

// handleIncomingConnection handles a new incoming connection.
func (t *BidirectionalTransport) handleIncomingConnection(conn net.Conn) {
	defer t.wg.Done()

	// Set deadline for handshake
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		conn.Close()
		return
	}

	// Read peer node ID from handshake
	var peerID uint64
	if err := binary.Read(conn, binary.BigEndian, &peerID); err != nil {
		conn.Close()
		return
	}

	// Clear deadline
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		conn.Close()
		return
	}

	// Find peer
	t.mu.RLock()
	peer, exists := t.peers[peerID]
	t.mu.RUnlock()

	if !exists {
		conn.Close()
		return
	}

	// Set connection on peer
	peer.mu.Lock()
	if peer.conn != nil {
		// Already have a connection, close the new one
		peer.mu.Unlock()
		conn.Close()
		return
	}
	peer.conn = conn
	peer.lastUsed = time.Now()
	peer.mu.Unlock()

	// Start sender and receiver goroutines
	t.wg.Add(2)
	go t.senderLoop(peer)
	go t.receiverLoop(peer)
}
