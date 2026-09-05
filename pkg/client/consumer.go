package client

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
)

// Consumer reads messages from StreamBus topics
type Consumer struct {
	client *Client
	config ConsumerConfig

	// Current position
	topic       string
	partitionID uint32
	offset      int64

	// State
	closed int32

	// Metrics
	messagesRead int64
	bytesRead    int64
	fetchCount   int64
}

// NewConsumer creates a new consumer
func NewConsumer(client *Client, topic string, partitionID uint32) *Consumer {
	return NewConsumerWithConfig(client, topic, partitionID, client.config.ConsumerConfig)
}

// NewConsumerWithConfig creates a new consumer with custom config
func NewConsumerWithConfig(client *Client, topic string, partitionID uint32, config ConsumerConfig) *Consumer {
	return &Consumer{
		client:      client,
		config:      config,
		topic:       topic,
		partitionID: partitionID,
		offset:      config.StartOffset,
	}
}

// Fetch fetches messages from the current offset
func (c *Consumer) Fetch(ctx context.Context) ([]protocol.Message, error) {
	return c.FetchN(ctx, int(c.config.MaxFetchBytes/1024)) // Estimate messages based on size
}

// FetchN fetches up to N messages
func (c *Consumer) FetchN(ctx context.Context, maxMessages int) ([]protocol.Message, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, ErrConsumerClosed
	}

	if c.topic == "" {
		return nil, ErrInvalidTopic
	}

	// The -1 "latest" sentinel from DefaultConfig is never a valid offset to
	// send to the broker - resolve it to the current end-of-log offset the
	// first time a fetch is attempted, so a consumer that never calls Seek
	// still starts consuming from "latest" as documented.
	if c.offset == -1 {
		endOffset, err := c.resolveEndOffset(ctx)
		if err != nil {
			return nil, err
		}
		c.offset = endOffset
	}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type:    protocol.RequestTypeFetch,
			Version: protocol.ProtocolVersion,
			Flags:   protocol.FlagNone,
		},
		Payload: &protocol.FetchRequest{
			Topic:          c.topic,
			PartitionID:    c.partitionID,
			Offset:         c.offset,
			MaxBytes:       c.config.MaxFetchBytes,
			IsolationLevel: c.config.IsolationLevel,
		},
	}

	// Send to first broker (later will have partition routing)
	broker := c.client.config.Brokers[0]
	resp, err := c.client.sendRequestWithRetry(ctx, broker, req)
	if err != nil {
		return nil, err
	}

	// Parse response
	fetchResp, ok := resp.Payload.(*protocol.FetchResponse)
	if !ok {
		return nil, ErrInvalidResponse
	}

	// The broker only bounds the response by byte size (MaxBytes), not
	// message count, so it can return more than maxMessages when enough
	// messages fit in that budget. Truncate here rather than advancing the
	// offset past (and discarding) messages the caller never received.
	truncated := maxMessages > 0 && len(fetchResp.Messages) > maxMessages
	messages := fetchResp.Messages
	if truncated {
		messages = messages[:maxMessages]
	}

	// Update offset for next fetch.
	//
	// A truncated response must resume from the last message we actually
	// handed back, never from NextOffset: NextOffset accounts for the whole
	// broker-side window, which can reach past messages this call withheld.
	//
	// Otherwise, NextOffset is what accounts for control records the broker
	// filtered out before we ever saw them - without it, a window that held
	// only a filtered marker would leave len(messages) == 0 and this offset
	// would never move, so the same empty-looking window would be re-fetched
	// forever. A server that predates NextOffset sends the -1 sentinel, in
	// which case the old last-message rule still applies (correct against a
	// server that never filtered anything).
	switch {
	case truncated:
		lastMessage := messages[len(messages)-1]
		c.offset = lastMessage.Offset + 1
	case fetchResp.NextOffset >= 0:
		c.offset = fetchResp.NextOffset
	case len(messages) > 0:
		lastMessage := messages[len(messages)-1]
		c.offset = lastMessage.Offset + 1
	}

	// Update metrics
	atomic.AddInt64(&c.messagesRead, int64(len(messages)))
	atomic.AddInt64(&c.fetchCount, 1)

	var bytes int64
	for _, msg := range messages {
		bytes += int64(len(msg.Key) + len(msg.Value))
	}
	atomic.AddInt64(&c.bytesRead, bytes)

	return messages, nil
}

// FetchOne fetches a single message
func (c *Consumer) FetchOne(ctx context.Context) (*protocol.Message, error) {
	messages, err := c.FetchN(ctx, 1)
	if err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, ErrNoMessages
	}

	return &messages[0], nil
}

// Seek seeks to a specific offset
func (c *Consumer) Seek(offset int64) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return ErrConsumerClosed
	}

	if offset < 0 {
		return ErrInvalidOffset
	}

	c.offset = offset
	return nil
}

// SeekToBeginning seeks to the beginning of the partition
func (c *Consumer) SeekToBeginning() error {
	return c.Seek(0)
}

// SeekToEnd seeks to the end of the partition
func (c *Consumer) SeekToEnd(ctx context.Context) error {
	// Get the latest offset from the server
	offset, err := c.GetEndOffset(ctx)
	if err != nil {
		return err
	}

	return c.Seek(offset)
}

// GetEndOffset gets the end offset for the partition
func (c *Consumer) GetEndOffset(ctx context.Context) (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrConsumerClosed
	}

	return c.resolveEndOffset(ctx)
}

// resolveEndOffset asks the broker for the current end-of-log (high water
// mark) offset for this consumer's topic/partition. It does not check
// c.closed - callers that need that check should do so themselves.
func (c *Consumer) resolveEndOffset(ctx context.Context) (int64, error) {
	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type:    protocol.RequestTypeGetOffset,
			Version: protocol.ProtocolVersion,
			Flags:   protocol.FlagNone,
		},
		Payload: &protocol.GetOffsetRequest{
			Topic:       c.topic,
			PartitionID: c.partitionID,
		},
	}

	// Send to first broker
	broker := c.client.config.Brokers[0]
	resp, err := c.client.sendRequestWithRetry(ctx, broker, req)
	if err != nil {
		return 0, err
	}

	// Parse response
	offsetResp, ok := resp.Payload.(*protocol.GetOffsetResponse)
	if !ok {
		return 0, ErrInvalidResponse
	}

	return offsetResp.EndOffset, nil
}

// CurrentOffset returns the current offset
func (c *Consumer) CurrentOffset() int64 {
	return c.offset
}

// Topic returns the topic being consumed
func (c *Consumer) Topic() string {
	return c.topic
}

// Partition returns the partition being consumed
func (c *Consumer) Partition() uint32 {
	return c.partitionID
}

// Poll continuously polls for messages with a callback
func (c *Consumer) Poll(ctx context.Context, interval time.Duration, handler func([]protocol.Message) error) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return ErrConsumerClosed
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if atomic.LoadInt32(&c.closed) == 1 {
			return ErrConsumerClosed
		}

		messages, err := c.Fetch(ctx)
		if err != nil {
			return fmt.Errorf("fetch error: %w", err)
		}

		if len(messages) > 0 {
			if err := handler(messages); err != nil {
				return fmt.Errorf("handler error: %w", err)
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Close closes the consumer
func (c *Consumer) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return ErrConsumerClosed
	}

	return nil
}

// Stats returns consumer statistics
func (c *Consumer) Stats() ConsumerStats {
	return ConsumerStats{
		Topic:         c.topic,
		PartitionID:   c.partitionID,
		CurrentOffset: c.offset,
		MessagesRead:  atomic.LoadInt64(&c.messagesRead),
		BytesRead:     atomic.LoadInt64(&c.bytesRead),
		FetchCount:    atomic.LoadInt64(&c.fetchCount),
	}
}

// ConsumerStats holds consumer statistics
type ConsumerStats struct {
	Topic         string
	PartitionID   uint32
	CurrentOffset int64
	MessagesRead  int64
	BytesRead     int64
	FetchCount    int64
}
