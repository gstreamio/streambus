package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
)

// Client represents a StreamBus client
type Client struct {
	config *Config
	pool   *ConnectionPool

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
	closed bool

	// Metrics
	requestsSent int64
	requestsFailed int64
	bytesWritten int64
	bytesRead int64
	startTime time.Time
}

// New creates a new StreamBus client
func New(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := &Client{
		config:    config,
		pool:      NewConnectionPool(config),
		ctx:       ctx,
		cancel:    cancel,
		startTime: time.Now(),
	}

	return client, nil
}

// sendRequest sends a request to a broker and returns the response
func (c *Client) sendRequest(ctx context.Context, broker string, req *protocol.Request) (*protocol.Response, error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, ErrClientClosed
	}
	c.mu.RUnlock()

	// Get connection from pool
	conn, err := c.pool.Get(broker)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Return connection to pool when done
	defer c.pool.Put(conn)

	// Set request ID
	req.Header.RequestID = conn.nextRequestID()

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	// Send request with timeout
	errChan := make(chan error, 1)
	respChan := make(chan *protocol.Response, 1)

	go func() {
		// Set write deadline
		if c.config.WriteTimeout > 0 {
			_ = conn.conn.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout))
		}

		// Encode and send request
		err := conn.codec.EncodeRequest(conn.conn, req)
		if err != nil {
			errChan <- fmt.Errorf("failed to encode request: %w", err)
			return
		}

		// Set read deadline
		if c.config.ReadTimeout > 0 {
			_ = conn.conn.SetReadDeadline(time.Now().Add(c.config.ReadTimeout))
		}

		// Read response
		resp, err := conn.codec.DecodeResponse(conn.conn)
		if err != nil {
			errChan <- fmt.Errorf("failed to decode response: %w", err)
			return
		}

		// Decode response payload
		err = conn.codec.DecodeResponsePayload(resp, req.Header.Type)
		if err != nil {
			errChan <- fmt.Errorf("failed to decode response payload: %w", err)
			return
		}

		respChan <- resp
	}()

	// Wait for response or timeout
	select {
	case <-timeoutCtx.Done():
		// Connection is likely dead, remove from pool
		_ = c.pool.Remove(conn)
		return nil, ErrRequestTimeout
	case err := <-errChan:
		// Connection had an error, remove from pool
		_ = c.pool.Remove(conn)
		return nil, err
	case resp := <-respChan:
		// Verify request ID matches
		if resp.Header.RequestID != req.Header.RequestID {
			return nil, ErrInvalidResponse
		}

		// Check for error status
		if resp.Header.Status == protocol.StatusError {
			if errorResp, ok := resp.Payload.(*protocol.ErrorResponse); ok {
				return nil, fmt.Errorf("server error: %s (code: %d)", errorResp.Message, errorResp.ErrorCode)
			}
			return nil, ErrRequestFailed
		}

		return resp, nil
	}
}

// sendRequestWithRetry sends a request with retry logic
func (c *Client) sendRequestWithRetry(broker string, req *protocol.Request) (*protocol.Response, error) {
	var lastErr error
	backoff := c.config.RetryBackoff

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry
			time.Sleep(backoff)
			backoff = backoff * 2
			if backoff > c.config.RetryMaxDelay {
				backoff = c.config.RetryMaxDelay
			}
		}

		resp, err := c.sendRequest(c.ctx, broker, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Don't retry on certain errors
		if err == ErrClientClosed || err == ErrInvalidResponse {
			break
		}
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", c.config.MaxRetries+1, lastErr)
}

// HealthCheck sends a health check request to a broker
func (c *Client) HealthCheck(broker string) error {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return ErrClientClosed
	}
	c.mu.RUnlock()

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type:    protocol.RequestTypeHealthCheck,
			Version: protocol.ProtocolVersion,
			Flags:   protocol.FlagNone,
		},
		Payload: &protocol.HealthCheckRequest{},
	}

	resp, err := c.sendRequest(c.ctx, broker, req)
	if err != nil {
		return err
	}

	if resp.Header.Status != protocol.StatusOK {
		return fmt.Errorf("health check failed: status %v", resp.Header.Status)
	}

	return nil
}

// CreateTopic creates a new topic
func (c *Client) CreateTopic(topic string, numPartitions uint32, replicationFactor uint16) error {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return ErrClientClosed
	}
	c.mu.RUnlock()

	if topic == "" {
		return ErrInvalidTopic
	}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type:    protocol.RequestTypeCreateTopic,
			Version: protocol.ProtocolVersion,
			Flags:   protocol.FlagNone,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:             topic,
			NumPartitions:     numPartitions,
			ReplicationFactor: replicationFactor,
		},
	}

	// Send to first broker (later will have broker discovery)
	broker := c.config.Brokers[0]
	_, err := c.sendRequestWithRetry(broker, req)
	return err
}

// DeleteTopic deletes a topic
func (c *Client) DeleteTopic(topic string) error {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return ErrClientClosed
	}
	c.mu.RUnlock()

	if topic == "" {
		return ErrInvalidTopic
	}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type:    protocol.RequestTypeDeleteTopic,
			Version: protocol.ProtocolVersion,
			Flags:   protocol.FlagNone,
		},
		Payload: &protocol.DeleteTopicRequest{
			Topic: topic,
		},
	}

	// Send to first broker
	broker := c.config.Brokers[0]
	_, err := c.sendRequestWithRetry(broker, req)
	return err
}

// ListTopics lists all topics
func (c *Client) ListTopics() ([]string, error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, ErrClientClosed
	}
	c.mu.RUnlock()

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type:    protocol.RequestTypeListTopics,
			Version: protocol.ProtocolVersion,
			Flags:   protocol.FlagNone,
		},
		Payload: &protocol.ListTopicsRequest{},
	}

	// Send to first broker
	broker := c.config.Brokers[0]
	resp, err := c.sendRequestWithRetry(broker, req)
	if err != nil {
		return nil, err
	}

	// Parse response
	if listResp, ok := resp.Payload.(*protocol.ListTopicsResponse); ok {
		topics := make([]string, len(listResp.Topics))
		for i, topicInfo := range listResp.Topics {
			topics[i] = topicInfo.Name
		}
		return topics, nil
	}

	return nil, ErrInvalidResponse
}

// Close closes the client and releases resources
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	// Cancel context
	c.cancel()

	// Close connection pool
	if err := c.pool.Close(); err != nil {
		return err
	}

	// Wait for all operations to complete
	c.wg.Wait()

	return nil
}

// Fetch fetches messages from a topic partition
func (c *Client) Fetch(ctx context.Context, req *FetchRequest) (*FetchResponse, error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, ErrClientClosed
	}
	c.mu.RUnlock()

	if req.Topic == "" {
		return nil, ErrInvalidTopic
	}

	if req.Partition < 0 {
		return nil, ErrInvalidPartition
	}

	if req.Offset < 0 {
		return nil, ErrInvalidOffset
	}

	protocolReq := &protocol.Request{
		Header: protocol.RequestHeader{
			Type:    protocol.RequestTypeFetch,
			Version: protocol.ProtocolVersion,
			Flags:   protocol.FlagNone,
		},
		Payload: &protocol.FetchRequest{
			Topic:       req.Topic,
			PartitionID: uint32(req.Partition),
			Offset:      req.Offset,
			MaxBytes:    uint32(req.MaxBytes),
		},
	}

	// Send to first broker
	broker := c.config.Brokers[0]
	resp, err := c.sendRequest(ctx, broker, protocolReq)
	if err != nil {
		return nil, err
	}

	// Parse response
	if fetchResp, ok := resp.Payload.(*protocol.FetchResponse); ok {
		return &FetchResponse{
			Topic:         fetchResp.Topic,
			Partition:     int32(fetchResp.PartitionID),
			Messages:      fetchResp.Messages,
			HighWaterMark: fetchResp.HighWaterMark,
		}, nil
	}

	return nil, ErrInvalidResponse
}

// Stats returns client statistics
func (c *Client) Stats() ClientStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return ClientStats{
		RequestsSent:   c.requestsSent,
		RequestsFailed: c.requestsFailed,
		BytesWritten:   c.bytesWritten,
		BytesRead:      c.bytesRead,
		Uptime:         time.Since(c.startTime),
		PoolStats:      c.pool.Stats(),
	}
}

// ClientStats holds client statistics
type ClientStats struct {
	RequestsSent   int64
	RequestsFailed int64
	BytesWritten   int64
	BytesRead      int64
	Uptime         time.Duration
	PoolStats      PoolStats
}

// FetchRequest represents a fetch request
type FetchRequest struct {
	Topic     string
	Partition int32
	Offset    int64
	MaxBytes  int32
}

// FetchResponse represents a fetch response
type FetchResponse struct {
	Topic         string
	Partition     int32
	Messages      []protocol.Message
	HighWaterMark int64
}
