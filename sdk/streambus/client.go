package streambus

import (
	"context"
	"fmt"
	"time"

	"github.com/gstreamio/streambus/pkg/client"
)

// Client provides a high-level, easy-to-use interface to StreamBus
type Client struct {
	underlying *client.Client
	config     *Config
}

// Config holds the SDK configuration
type Config struct {
	// Broker addresses
	Brokers []string

	// Connection settings
	ConnectTimeout time.Duration
	RequestTimeout time.Duration

	// Authentication
	Username string
	Password string

	// TLS Configuration
	UseTLS   bool
	CertFile string
	KeyFile  string
	CAFile   string

	// Retry settings
	MaxRetries int
	RetryDelay time.Duration

	// Logging
	Logger Logger
}

// Logger interface for custom logging
type Logger interface {
	Debug(msg string, keysAndValues ...interface{})
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Brokers:        []string{"localhost:9092"},
		ConnectTimeout: 10 * time.Second,
		RequestTimeout: 30 * time.Second,
		MaxRetries:     3,
		RetryDelay:     1 * time.Second,
	}
}

// New creates a new StreamBus SDK client
func New(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Map SDK config to underlying client config
	clientConfig := client.DefaultConfig()
	clientConfig.Brokers = config.Brokers
	clientConfig.ConnectTimeout = config.ConnectTimeout
	clientConfig.RequestTimeout = config.RequestTimeout

	// Create the underlying client
	underlyingClient, err := client.New(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &Client{
		underlying: underlyingClient,
		config:     config,
	}, nil
}

// Connect creates a new client and connects to the brokers
func Connect(brokers ...string) (*Client, error) {
	config := DefaultConfig()
	if len(brokers) > 0 {
		config.Brokers = brokers
	}
	return New(config)
}

// NewProducer creates a new producer with the SDK's simple interface
func (c *Client) NewProducer() *Producer {
	return &Producer{
		client:     c,
		underlying: client.NewProducer(c.underlying),
		logger:     c.config.Logger,
	}
}

// NewConsumer creates a new consumer with the SDK's simple interface
func (c *Client) NewConsumer(topic string) *Consumer {
	return &Consumer{
		client:     c,
		topic:      topic,
		underlying: client.NewConsumer(c.underlying, topic, 0), // Default to partition 0
		logger:     c.config.Logger,
	}
}

// NewConsumerGroup creates a new consumer group
func (c *Client) NewConsumerGroup(groupID string, topics ...string) *ConsumerGroup {
	return &ConsumerGroup{
		client:  c,
		groupID: groupID,
		topics:  topics,
		logger:  c.config.Logger,
	}
}

// Admin returns an admin client for managing topics and metadata
func (c *Client) Admin() *Admin {
	return &Admin{
		client: c,
		logger: c.config.Logger,
	}
}

// Close closes the client and releases resources
func (c *Client) Close() error {
	if c.underlying != nil {
		return c.underlying.Close()
	}
	return nil
}

// Ping checks connectivity to the brokers
func (c *Client) Ping(ctx context.Context) error {
	// Try to list topics as a connectivity check
	_, err := c.underlying.ListTopics()
	return err
}

// WithLogger sets a custom logger for the client
func (c *Client) WithLogger(logger Logger) *Client {
	c.config.Logger = logger
	return c
}