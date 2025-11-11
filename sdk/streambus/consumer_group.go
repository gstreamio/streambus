package streambus

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ConsumerGroup manages a group of consumers for coordinated consumption
type ConsumerGroup struct {
	client  *Client
	groupID string
	topics  []string
	logger  Logger

	// State management
	mu        sync.RWMutex
	consumers map[string]*Consumer
	running   bool
	cancel    context.CancelFunc
}

// ConsumerGroupConfig holds consumer group configuration
type ConsumerGroupConfig struct {
	// Rebalance strategy
	RebalanceStrategy string

	// Session timeout
	SessionTimeout time.Duration

	// Heartbeat interval
	HeartbeatInterval time.Duration

	// Auto commit
	AutoCommit bool

	// Auto commit interval
	AutoCommitInterval time.Duration
}

// DefaultConsumerGroupConfig returns default consumer group configuration
func DefaultConsumerGroupConfig() *ConsumerGroupConfig {
	return &ConsumerGroupConfig{
		RebalanceStrategy:  "range",
		SessionTimeout:     30 * time.Second,
		HeartbeatInterval:  3 * time.Second,
		AutoCommit:         true,
		AutoCommitInterval: 5 * time.Second,
	}
}

// Start starts the consumer group
func (cg *ConsumerGroup) Start(ctx context.Context, handler MessageHandler) error {
	cg.mu.Lock()
	if cg.running {
		cg.mu.Unlock()
		return fmt.Errorf("consumer group already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	cg.cancel = cancel
	cg.running = true
	cg.consumers = make(map[string]*Consumer)
	cg.mu.Unlock()

	// Create consumers for each topic
	for _, topic := range cg.topics {
		consumer := cg.client.NewConsumer(topic)
		cg.mu.Lock()
		cg.consumers[topic] = consumer
		cg.mu.Unlock()

		// Start consuming in a goroutine
		go func(topic string, consumer *Consumer) {
			if err := consumer.Consume(ctx, handler); err != nil {
				if cg.logger != nil {
					cg.logger.Error("Consumer error",
						"group", cg.groupID,
						"topic", topic,
						"error", err)
				}
			}
		}(topic, consumer)
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Clean up
	cg.Stop()

	return nil
}

// Subscribe subscribes to additional topics
func (cg *ConsumerGroup) Subscribe(topics ...string) error {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	// Add new topics
	for _, topic := range topics {
		// Check if already subscribed
		found := false
		for _, existing := range cg.topics {
			if existing == topic {
				found = true
				break
			}
		}

		if !found {
			cg.topics = append(cg.topics, topic)

			// If running, create a new consumer for this topic
			if cg.running {
				consumer := cg.client.NewConsumer(topic)
				cg.consumers[topic] = consumer
			}
		}
	}

	return nil
}

// Unsubscribe removes topics from subscription
func (cg *ConsumerGroup) Unsubscribe(topics ...string) error {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	for _, topic := range topics {
		// Remove from topics list
		newTopics := make([]string, 0, len(cg.topics))
		for _, t := range cg.topics {
			if t != topic {
				newTopics = append(newTopics, t)
			}
		}
		cg.topics = newTopics

		// Close consumer if running
		if consumer, exists := cg.consumers[topic]; exists {
			if err := consumer.Close(); err != nil {
				if cg.logger != nil {
					cg.logger.Warn("Failed to close consumer",
						"topic", topic,
						"error", err)
				}
			}
			delete(cg.consumers, topic)
		}
	}

	return nil
}

// Stop stops the consumer group
func (cg *ConsumerGroup) Stop() error {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	if !cg.running {
		return nil
	}

	// Cancel context
	if cg.cancel != nil {
		cg.cancel()
	}

	// Close all consumers
	var lastErr error
	for topic, consumer := range cg.consumers {
		if err := consumer.Close(); err != nil {
			lastErr = err
			if cg.logger != nil {
				cg.logger.Error("Failed to close consumer",
					"topic", topic,
					"error", err)
			}
		}
	}

	cg.running = false
	cg.consumers = nil

	return lastErr
}

// CommitOffsets manually commits offsets for all consumers
func (cg *ConsumerGroup) CommitOffsets() error {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	var lastErr error
	for topic, consumer := range cg.consumers {
		if err := consumer.Commit(); err != nil {
			lastErr = err
			if cg.logger != nil {
				cg.logger.Error("Failed to commit offset",
					"group", cg.groupID,
					"topic", topic,
					"error", err)
			}
		}
	}

	return lastErr
}

// GetMetrics returns consumer group metrics
func (cg *ConsumerGroup) GetMetrics() ConsumerGroupMetrics {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	return ConsumerGroupMetrics{
		GroupID:       cg.groupID,
		Topics:        cg.topics,
		ConsumerCount: len(cg.consumers),
		Running:       cg.running,
	}
}

// ConsumerGroupMetrics holds consumer group metrics
type ConsumerGroupMetrics struct {
	GroupID       string
	Topics        []string
	ConsumerCount int
	Running       bool
}

// ConsumerGroupBuilder provides a fluent interface for building consumer groups
type ConsumerGroupBuilder struct {
	client  *Client
	groupID string
	topics  []string
	config  *ConsumerGroupConfig
}

// NewConsumerGroupBuilder creates a new consumer group builder
func (c *Client) NewConsumerGroupBuilder(groupID string) *ConsumerGroupBuilder {
	return &ConsumerGroupBuilder{
		client:  c,
		groupID: groupID,
		config:  DefaultConsumerGroupConfig(),
	}
}

// WithTopics adds topics to the consumer group
func (cgb *ConsumerGroupBuilder) WithTopics(topics ...string) *ConsumerGroupBuilder {
	cgb.topics = append(cgb.topics, topics...)
	return cgb
}

// WithAutoCommit enables auto commit
func (cgb *ConsumerGroupBuilder) WithAutoCommit(interval time.Duration) *ConsumerGroupBuilder {
	cgb.config.AutoCommit = true
	cgb.config.AutoCommitInterval = interval
	return cgb
}

// WithSessionTimeout sets the session timeout
func (cgb *ConsumerGroupBuilder) WithSessionTimeout(timeout time.Duration) *ConsumerGroupBuilder {
	cgb.config.SessionTimeout = timeout
	return cgb
}

// Build creates the consumer group
func (cgb *ConsumerGroupBuilder) Build() *ConsumerGroup {
	return &ConsumerGroup{
		client:  cgb.client,
		groupID: cgb.groupID,
		topics:  cgb.topics,
		logger:  cgb.client.config.Logger,
	}
}