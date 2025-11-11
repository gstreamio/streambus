package streambus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shawntherrien/streambus/pkg/client"
)

// Producer provides a simple interface for producing messages
type Producer struct {
	client     *Client
	underlying *client.Producer
	logger     Logger

	// Default settings that can be overridden per-send
	defaultTopic     string
	defaultPartition int32
}

// ProducerOption allows customizing producer behavior
type ProducerOption func(*Producer)

// WithDefaultTopic sets a default topic for the producer
func WithDefaultTopic(topic string) ProducerOption {
	return func(p *Producer) {
		p.defaultTopic = topic
	}
}

// WithDefaultPartition sets a default partition for the producer
func WithDefaultPartition(partition int32) ProducerOption {
	return func(p *Producer) {
		p.defaultPartition = partition
	}
}

// Message represents a message to be sent
type Message struct {
	Topic     string
	Key       []byte
	Value     []byte
	Headers   map[string]string
	Partition *int32
	Timestamp *time.Time
}

// Send sends a simple string message
func (p *Producer) Send(topic, message string) error {
	return p.underlying.Send(topic, nil, []byte(message))
}

// SendWithKey sends a message with a key
func (p *Producer) SendWithKey(topic, key, message string) error {
	return p.underlying.Send(topic, []byte(key), []byte(message))
}

// SendJSON sends an object as JSON
func (p *Producer) SendJSON(topic string, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	var keyBytes []byte
	if key != "" {
		keyBytes = []byte(key)
	}

	return p.underlying.Send(topic, keyBytes, data)
}

// SendMessage sends a structured message
func (p *Producer) SendMessage(msg *Message) error {
	if msg.Topic == "" {
		if p.defaultTopic == "" {
			return fmt.Errorf("no topic specified")
		}
		msg.Topic = p.defaultTopic
	}

	partition := p.defaultPartition
	if msg.Partition != nil {
		partition = *msg.Partition
	}

	if partition == 0 {
		return p.underlying.Send(msg.Topic, msg.Key, msg.Value)
	}

	return p.underlying.SendToPartition(msg.Topic, uint32(partition), msg.Key, msg.Value)
}

// SendBatch sends multiple messages in a batch
func (p *Producer) SendBatch(topic string, messages []Message) error {
	for _, msg := range messages {
		if msg.Topic == "" {
			msg.Topic = topic
		}
		if err := p.SendMessage(&msg); err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}
	}

	// Flush after batch
	return p.Flush(topic)
}

// SendAsync sends a message asynchronously
func (p *Producer) SendAsync(ctx context.Context, topic, message string) <-chan error {
	errCh := make(chan error, 1)

	go func() {
		select {
		case <-ctx.Done():
			errCh <- ctx.Err()
		default:
			errCh <- p.Send(topic, message)
		}
	}()

	return errCh
}

// Flush flushes any pending messages for a topic
func (p *Producer) Flush(topic string) error {
	return p.underlying.Flush(topic)
}

// FlushAll flushes all pending messages
func (p *Producer) FlushAll() error {
	// This would ideally flush all topics, but for now we'll return nil
	// The underlying implementation would need to track all topics
	return nil
}

// Close closes the producer
func (p *Producer) Close() error {
	return p.underlying.Close()
}

// ProducerBuilder provides a fluent interface for building producers
type ProducerBuilder struct {
	client  *Client
	options []ProducerOption
}

// NewProducerBuilder creates a new producer builder
func (c *Client) NewProducerBuilder() *ProducerBuilder {
	return &ProducerBuilder{
		client: c,
	}
}

// WithTopic sets the default topic
func (pb *ProducerBuilder) WithTopic(topic string) *ProducerBuilder {
	pb.options = append(pb.options, WithDefaultTopic(topic))
	return pb
}

// WithPartition sets the default partition
func (pb *ProducerBuilder) WithPartition(partition int32) *ProducerBuilder {
	pb.options = append(pb.options, WithDefaultPartition(partition))
	return pb
}

// Build creates the producer
func (pb *ProducerBuilder) Build() *Producer {
	producer := pb.client.NewProducer()
	for _, opt := range pb.options {
		opt(producer)
	}
	return producer
}