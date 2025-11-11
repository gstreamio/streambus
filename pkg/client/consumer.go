package client

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/shawntherrien/streambus/pkg/protocol"
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
func (c *Consumer) Fetch() ([]protocol.Message, error) {
	return c.FetchN(int(c.config.MaxFetchBytes / 1024)) // Estimate messages based on size
}

// FetchN fetches up to N messages
func (c *Consumer) FetchN(maxMessages int) ([]protocol.Message, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, ErrConsumerClosed
	}

	if c.topic == "" {
		return nil, ErrInvalidTopic
	}

	if c.partitionID < 0 {
		return nil, ErrInvalidPartition
	}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type:    protocol.RequestTypeFetch,
			Version: protocol.ProtocolVersion,
			Flags:   protocol.FlagNone,
		},
		Payload: &protocol.FetchRequest{
			Topic:       c.topic,
			PartitionID: c.partitionID,
			Offset:      c.offset,
			MaxBytes:    c.config.MaxFetchBytes,
		},
	}

	// Send to first broker (later will have partition routing)
	broker := c.client.config.Brokers[0]
	resp, err := c.client.sendRequestWithRetry(broker, req)
	if err != nil {
		return nil, err
	}

	// Parse response
	fetchResp, ok := resp.Payload.(*protocol.FetchResponse)
	if !ok {
		return nil, ErrInvalidResponse
	}

	// Update offset for next fetch
	if len(fetchResp.Messages) > 0 {
		// Set offset to the last message's offset + 1 for the next fetch
		lastMessage := fetchResp.Messages[len(fetchResp.Messages)-1]
		c.offset = lastMessage.Offset + 1
	}

	// Update metrics
	atomic.AddInt64(&c.messagesRead, int64(len(fetchResp.Messages)))
	atomic.AddInt64(&c.fetchCount, 1)

	var bytes int64
	for _, msg := range fetchResp.Messages {
		bytes += int64(len(msg.Key) + len(msg.Value))
	}
	atomic.AddInt64(&c.bytesRead, bytes)

	return fetchResp.Messages, nil
}

// FetchOne fetches a single message
func (c *Consumer) FetchOne() (*protocol.Message, error) {
	messages, err := c.FetchN(1)
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
func (c *Consumer) SeekToEnd() error {
	// Get the latest offset from the server
	offset, err := c.GetEndOffset()
	if err != nil {
		return err
	}

	return c.Seek(offset)
}

// GetEndOffset gets the end offset for the partition
func (c *Consumer) GetEndOffset() (int64, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, ErrConsumerClosed
	}

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
	resp, err := c.client.sendRequestWithRetry(broker, req)
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
func (c *Consumer) Poll(interval time.Duration, handler func([]protocol.Message) error) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return ErrConsumerClosed
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if atomic.LoadInt32(&c.closed) == 1 {
			return ErrConsumerClosed
		}

		messages, err := c.Fetch()
		if err != nil {
			return fmt.Errorf("fetch error: %w", err)
		}

		if len(messages) > 0 {
			if err := handler(messages); err != nil {
				return fmt.Errorf("handler error: %w", err)
			}
		}

		<-ticker.C
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
		Topic:        c.topic,
		PartitionID:  c.partitionID,
		CurrentOffset: c.offset,
		MessagesRead: atomic.LoadInt64(&c.messagesRead),
		BytesRead:    atomic.LoadInt64(&c.bytesRead),
		FetchCount:   atomic.LoadInt64(&c.fetchCount),
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
