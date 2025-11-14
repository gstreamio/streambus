package server

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gstreamio/streambus/pkg/logger"
	"github.com/gstreamio/streambus/pkg/protocol"
	"go.uber.org/zap"
)

// RequestHandler defines the interface for handling requests
type RequestHandler interface {
	Handle(req *protocol.Request) *protocol.Response
}

// Server represents a TCP server
type Server struct {
	config   *Config
	listener net.Listener
	handler  RequestHandler
	codec    *protocol.Codec

	// Connection tracking
	mu          sync.RWMutex
	connections map[net.Conn]struct{}
	connCount   int64

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	totalRequests   int64
	totalErrors     int64
	totalBytesSent  int64
	totalBytesRecv  int64
	startTime       time.Time
}

// New creates a new server
func New(config *Config, handler RequestHandler) (*Server, error) {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		config:      config,
		handler:     handler,
		codec:       protocol.NewCodec(),
		connections: make(map[net.Conn]struct{}),
		ctx:         ctx,
		cancel:      cancel,
		startTime:   time.Now(),
	}

	return s, nil
}

// Start starts the server
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.config.Address)
	if err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}

	s.listener = listener
	logger.Info("server listening", zap.String("address", s.config.Address))

	// Accept connections
	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Stop stops the server gracefully
func (s *Server) Stop() error {
	logger.Info("stopping server")

	// Cancel context to signal shutdown
	s.cancel()

	// Close listener to stop accepting new connections
	if s.listener != nil {
		s.listener.Close()
	}

	// Close all active connections
	s.mu.Lock()
	for conn := range s.connections {
		conn.Close()
	}
	s.mu.Unlock()

	// Wait for all goroutines to finish
	s.wg.Wait()

	logger.Info("server stopped")
	return nil
}

// acceptLoop accepts incoming connections
func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				// Server is shutting down
				return
			default:
				logger.Debug("accept error", zap.Error(err))
				continue
			}
		}

		// Log new connection at debug level
		logger.Debug("new connection",
			zap.String("remoteAddr", conn.RemoteAddr().String()))

		// Check connection limit
		if atomic.LoadInt64(&s.connCount) >= int64(s.config.MaxConnections) {
			logger.Warn("max connections reached, rejecting connection",
				zap.String("remoteAddr", conn.RemoteAddr().String()),
				zap.Int64("maxConnections", int64(s.config.MaxConnections)))
			conn.Close()
			continue
		}

		// Configure TCP connection
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			if s.config.KeepAlive {
				tcpConn.SetKeepAlive(true)
				tcpConn.SetKeepAlivePeriod(s.config.KeepAlivePeriod)
			}
		}

		// Track connection
		s.mu.Lock()
		s.connections[conn] = struct{}{}
		s.mu.Unlock()
		atomic.AddInt64(&s.connCount, 1)

		// Handle connection in goroutine
		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection handles a single connection
func (s *Server) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		conn.Close()
		s.mu.Lock()
		delete(s.connections, conn)
		s.mu.Unlock()
		atomic.AddInt64(&s.connCount, -1)
	}()

	// Set initial timeouts
	if s.config.ReadTimeout > 0 {
		conn.SetReadDeadline(time.Now().Add(s.config.ReadTimeout))
	}
	if s.config.WriteTimeout > 0 {
		conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout))
	}

	for {
		// Check if server is shutting down
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Read request
		req, err := s.codec.DecodeRequest(conn)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout - check if idle timeout exceeded
				continue
			}
			// Connection closed or other error
			logger.Debug("connection closed or decode error",
				zap.String("remoteAddr", conn.RemoteAddr().String()),
				zap.Error(err))
			return
		}

		// Log at debug level for high-frequency events
		logger.Debug("received request",
			zap.Int("type", int(req.Header.Type)),
			zap.Uint64("id", req.Header.RequestID))

		atomic.AddInt64(&s.totalRequests, 1)

		// Update read deadline
		if s.config.ReadTimeout > 0 {
			conn.SetReadDeadline(time.Now().Add(s.config.ReadTimeout))
		}

		// Handle request
		resp := s.handler.Handle(req)

		// Update write deadline
		if s.config.WriteTimeout > 0 {
			conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout))
		}

		// Write response
		err = s.codec.EncodeResponse(conn, resp)
		if err != nil {
			logger.Debug("failed to write response",
				zap.String("remoteAddr", conn.RemoteAddr().String()),
				zap.Error(err))
			atomic.AddInt64(&s.totalErrors, 1)
			return
		}
	}
}

// Listener returns the server's listener
func (s *Server) Listener() net.Listener {
	return s.listener
}

// Stats returns server statistics
func (s *Server) Stats() ServerStats {
	return ServerStats{
		ActiveConnections: atomic.LoadInt64(&s.connCount),
		TotalRequests:     atomic.LoadInt64(&s.totalRequests),
		TotalErrors:       atomic.LoadInt64(&s.totalErrors),
		TotalBytesSent:    atomic.LoadInt64(&s.totalBytesSent),
		TotalBytesRecv:    atomic.LoadInt64(&s.totalBytesRecv),
		Uptime:            time.Since(s.startTime),
	}
}

// ServerStats holds server statistics
type ServerStats struct {
	ActiveConnections int64
	TotalRequests     int64
	TotalErrors       int64
	TotalBytesSent    int64
	TotalBytesRecv    int64
	Uptime            time.Duration
}
