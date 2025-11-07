package client

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/shawntherrien/streambus/pkg/protocol"
)

// connection represents a single connection to a broker
type connection struct {
	conn      net.Conn
	codec     *protocol.Codec
	broker    string
	lastUsed  time.Time
	inUse     bool
	mu        sync.Mutex
	requestID uint64
}

// nextRequestID returns the next request ID for this connection
func (c *connection) nextRequestID() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requestID++
	return c.requestID
}

// markUsed marks the connection as used
func (c *connection) markUsed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastUsed = time.Now()
	c.inUse = true
}

// markIdle marks the connection as idle
func (c *connection) markIdle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inUse = false
}

// isHealthy checks if the connection is healthy (must be called with lock held)
func (c *connection) isHealthyLocked() bool {
	if c.conn == nil {
		return false
	}

	// Try to set a deadline to test the connection
	err := c.conn.SetDeadline(time.Now().Add(1 * time.Second))
	if err != nil {
		return false
	}

	// Reset deadline
	c.conn.SetDeadline(time.Time{})
	return true
}

// isHealthy checks if the connection is healthy
func (c *connection) isHealthy() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isHealthyLocked()
}

// close closes the connection
func (c *connection) close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ConnectionPool manages connections to brokers
type ConnectionPool struct {
	config      *Config
	connections map[string][]*connection // broker -> connections
	mu          sync.RWMutex
	closed      bool
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(config *Config) *ConnectionPool {
	ctx, cancel := context.WithCancel(context.Background())

	pool := &ConnectionPool{
		config:      config,
		connections: make(map[string][]*connection),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start cleanup goroutine
	pool.wg.Add(1)
	go pool.cleanupLoop()

	return pool
}

// Get gets a connection to the specified broker
func (p *ConnectionPool) Get(broker string) (*connection, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, ErrConnectionPoolClosed
	}

	// Look for an idle connection
	conns := p.connections[broker]
	for _, conn := range conns {
		conn.mu.Lock()
		if !conn.inUse && conn.isHealthyLocked() {
			conn.inUse = true
			conn.lastUsed = time.Now()
			conn.mu.Unlock()
			return conn, nil
		}
		conn.mu.Unlock()
	}

	// Create new connection if under limit
	if len(conns) < p.config.MaxConnectionsPerBroker {
		conn, err := p.createConnection(broker)
		if err != nil {
			return nil, err
		}
		p.connections[broker] = append(p.connections[broker], conn)
		return conn, nil
	}

	// Wait for a connection to become available
	// For now, return error - could implement waiting in future
	return nil, ErrNoConnection
}

// Put returns a connection to the pool
func (p *ConnectionPool) Put(conn *connection) {
	if conn == nil {
		return
	}

	conn.markIdle()
}

// Remove removes a connection from the pool
func (p *ConnectionPool) Remove(conn *connection) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	conns := p.connections[conn.broker]
	for i, c := range conns {
		if c == conn {
			// Remove from slice
			p.connections[conn.broker] = append(conns[:i], conns[i+1:]...)
			return conn.close()
		}
	}

	return nil
}

// createConnection creates a new connection to a broker
func (p *ConnectionPool) createConnection(broker string) (*connection, error) {
	// Create TCP connection with timeout
	dialer := net.Dialer{
		Timeout:   p.config.ConnectTimeout,
		KeepAlive: p.config.KeepAlivePeriod,
	}

	conn, err := dialer.DialContext(p.ctx, "tcp", broker)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", broker, err)
	}

	// Configure TCP connection
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		if p.config.KeepAlive {
			tcpConn.SetKeepAlive(true)
			tcpConn.SetKeepAlivePeriod(p.config.KeepAlivePeriod)
		}
	}

	c := &connection{
		conn:     conn,
		codec:    protocol.NewCodec(),
		broker:   broker,
		lastUsed: time.Now(),
		inUse:    true,
	}

	return c, nil
}

// cleanupLoop periodically cleans up idle connections
func (p *ConnectionPool) cleanupLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.cleanup()
		}
	}
}

// cleanup removes idle and unhealthy connections
func (p *ConnectionPool) cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()

	idleTimeout := 5 * time.Minute

	for broker, conns := range p.connections {
		activeConns := make([]*connection, 0, len(conns))

		for _, conn := range conns {
			conn.mu.Lock()
			shouldRemove := !conn.inUse &&
				(time.Since(conn.lastUsed) > idleTimeout || !conn.isHealthyLocked())
			conn.mu.Unlock()

			if shouldRemove {
				conn.close()
			} else {
				activeConns = append(activeConns, conn)
			}
		}

		if len(activeConns) == 0 {
			delete(p.connections, broker)
		} else {
			p.connections[broker] = activeConns
		}
	}
}

// Close closes all connections in the pool
func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	// Cancel context to stop cleanup loop
	p.cancel()

	// Close all connections
	p.mu.Lock()
	for _, conns := range p.connections {
		for _, conn := range conns {
			conn.close()
		}
	}
	p.connections = make(map[string][]*connection)
	p.mu.Unlock()

	// Wait for cleanup goroutine
	p.wg.Wait()

	return nil
}

// Stats returns pool statistics
func (p *ConnectionPool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := PoolStats{
		BrokerStats: make(map[string]BrokerStats),
	}

	for broker, conns := range p.connections {
		brokerStats := BrokerStats{}
		for _, conn := range conns {
			conn.mu.Lock()
			brokerStats.Total++
			if conn.inUse {
				brokerStats.Active++
			} else {
				brokerStats.Idle++
			}
			conn.mu.Unlock()
		}
		stats.BrokerStats[broker] = brokerStats
		stats.TotalConnections += brokerStats.Total
		stats.ActiveConnections += brokerStats.Active
		stats.IdleConnections += brokerStats.Idle
	}

	return stats
}

// PoolStats holds pool statistics
type PoolStats struct {
	TotalConnections  int
	ActiveConnections int
	IdleConnections   int
	BrokerStats       map[string]BrokerStats
}

// BrokerStats holds per-broker statistics
type BrokerStats struct {
	Total  int
	Active int
	Idle   int
}
