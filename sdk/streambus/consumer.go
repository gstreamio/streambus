package streambus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gstreamio/streambus/pkg/client"
	"github.com/gstreamio/streambus/pkg/protocol"
)

// Consumer provides a simple interface for consuming messages
type Consumer struct {
	client     *Client
	topic      string
	underlying *client.Consumer
	logger     Logger

	// Processing options
	autoCommit bool
	maxRetries int
}

// ConsumerOption allows customizing consumer behavior
type ConsumerOption func(*Consumer)

// WithAutoCommit enables automatic offset commits
func WithAutoCommit(enabled bool) ConsumerOption {
	return func(c *Consumer) {
		c.autoCommit = enabled
	}
}

// WithMaxRetries sets the maximum number of retries for message processing
func WithMaxRetries(retries int) ConsumerOption {
	return func(c *Consumer) {
		c.maxRetries = retries
	}
}

// ReceivedMessage represents a message received from StreamBus
type ReceivedMessage struct {
	Topic     string
	Partition int32
	Offset    int64
	Key       []byte
	Value     []byte
	Headers   map[string]string
	Timestamp time.Time
}

// MessageHandler is a function that processes a message
type MessageHandler func(msg *ReceivedMessage) error

// Consume starts consuming messages and calls the handler for each
func (c *Consumer) Consume(ctx context.Context, handler MessageHandler) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Fetch messages
			messages, err := c.underlying.Fetch()
			if err != nil {
				if c.logger != nil {
					c.logger.Error("Failed to fetch messages", "error", err)
				}
				time.Sleep(1 * time.Second) // Back off on error
				continue
			}

			// Process each message
			for _, msg := range messages {
				receivedMsg := &ReceivedMessage{
					Topic:     c.topic,
					Partition: 0, // Will be updated when we have partition info
					Offset:    msg.Offset,
					Key:       msg.Key,
					Value:     msg.Value,
					Timestamp: time.Unix(0, msg.Timestamp),
				}

				// Process with retries
				if err := c.processWithRetries(receivedMsg, handler); err != nil {
					if c.logger != nil {
						c.logger.Error("Failed to process message",
							"offset", msg.Offset,
							"error", err)
					}
					// Decide whether to continue or stop on error
					// For now, we'll continue
					continue
				}

				// Note: Auto-commit not yet implemented in underlying consumer
			}
		}
	}
}

// ConsumeN consumes up to N messages
func (c *Consumer) ConsumeN(n int, handler MessageHandler) error {
	messages, err := c.underlying.FetchN(n)
	if err != nil {
		return fmt.Errorf("failed to fetch messages: %w", err)
	}

	for _, msg := range messages {
		receivedMsg := &ReceivedMessage{
			Topic:     c.topic,
			Partition: 0,
			Offset:    msg.Offset,
			Key:       msg.Key,
			Value:     msg.Value,
			Timestamp: time.Unix(0, msg.Timestamp),
		}

		if err := handler(receivedMsg); err != nil {
			return fmt.Errorf("handler failed for message at offset %d: %w", msg.Offset, err)
		}
	}

	return nil
}

// ConsumeJSON consumes messages and unmarshals them as JSON
func (c *Consumer) ConsumeJSON(ctx context.Context, handler func(msg *ReceivedMessage, data interface{}) error, target interface{}) error {
	return c.Consume(ctx, func(msg *ReceivedMessage) error {
		if err := json.Unmarshal(msg.Value, target); err != nil {
			return fmt.Errorf("failed to unmarshal JSON: %w", err)
		}
		return handler(msg, target)
	})
}

// Subscribe subscribes to receive messages with a callback
func (c *Consumer) Subscribe(handler MessageHandler) error {
	// Start polling in the background
	return c.underlying.Poll(5*time.Second, func(messages []protocol.Message) error {
		for _, msg := range messages {
			receivedMsg := &ReceivedMessage{
				Topic:     c.topic,
				Partition: 0,
				Offset:    msg.Offset,
				Key:       msg.Key,
				Value:     msg.Value,
				Timestamp: time.Unix(0, msg.Timestamp),
			}

			if err := handler(receivedMsg); err != nil {
				return err
			}
		}
		return nil
	})
}

// Commit commits the current offset
// Note: Not yet implemented in underlying consumer
func (c *Consumer) Commit() error {
	// TODO: Implement when underlying consumer supports commit
	return nil
}

// SeekToBeginning seeks to the beginning of the partition
func (c *Consumer) SeekToBeginning() error {
	return c.underlying.Seek(0)
}

// SeekToEnd seeks to the end of the partition
func (c *Consumer) SeekToEnd() error {
	return c.underlying.Seek(-1)
}

// SeekToOffset seeks to a specific offset
func (c *Consumer) SeekToOffset(offset int64) error {
	return c.underlying.Seek(offset)
}

// Close closes the consumer
func (c *Consumer) Close() error {
	return c.underlying.Close()
}

// processWithRetries processes a message with retry logic
func (c *Consumer) processWithRetries(msg *ReceivedMessage, handler MessageHandler) error {
	var lastErr error
	maxRetries := c.maxRetries
	if maxRetries == 0 {
		maxRetries = 1 // At least try once
	}

	for i := 0; i < maxRetries; i++ {
		if err := handler(msg); err != nil {
			lastErr = err
			if i < maxRetries-1 {
				// Exponential backoff
				time.Sleep(time.Duration(1<<uint(i)) * time.Second)
				if c.logger != nil {
					c.logger.Warn("Retrying message processing",
						"attempt", i+1,
						"offset", msg.Offset,
						"error", err)
				}
			}
		} else {
			return nil
		}
	}

	return lastErr
}

// ConsumerBuilder provides a fluent interface for building consumers
type ConsumerBuilder struct {
	client  *Client
	topic   string
	options []ConsumerOption
}

// NewConsumerBuilder creates a new consumer builder
func (c *Client) NewConsumerBuilder(topic string) *ConsumerBuilder {
	return &ConsumerBuilder{
		client: c,
		topic:  topic,
	}
}

// WithAutoCommit enables automatic offset commits
func (cb *ConsumerBuilder) WithAutoCommit() *ConsumerBuilder {
	cb.options = append(cb.options, WithAutoCommit(true))
	return cb
}

// WithRetries sets the maximum number of retries
func (cb *ConsumerBuilder) WithRetries(retries int) *ConsumerBuilder {
	cb.options = append(cb.options, WithMaxRetries(retries))
	return cb
}

// Build creates the consumer
func (cb *ConsumerBuilder) Build() *Consumer {
	consumer := cb.client.NewConsumer(cb.topic)
	for _, opt := range cb.options {
		opt(consumer)
	}
	return consumer
}