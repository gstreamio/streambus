package client

import (
	"time"
)

// Config holds client configuration
type Config struct {
	// Broker addresses (e.g., []string{"localhost:9092", "localhost:9093"})
	Brokers []string

	// Connection timeout
	ConnectTimeout time.Duration

	// Read timeout for requests
	ReadTimeout time.Duration

	// Write timeout for requests
	WriteTimeout time.Duration

	// Maximum number of connections per broker
	MaxConnectionsPerBroker int

	// Retry configuration
	MaxRetries    int
	RetryBackoff  time.Duration
	RetryMaxDelay time.Duration

	// Request timeout
	RequestTimeout time.Duration

	// Keep-alive configuration
	KeepAlive       bool
	KeepAlivePeriod time.Duration

	// Producer configuration
	ProducerConfig ProducerConfig

	// Consumer configuration
	ConsumerConfig ConsumerConfig
}

// ProducerConfig holds producer-specific configuration
type ProducerConfig struct {
	// Whether to require acknowledgment from server
	RequireAck bool

	// Batch size for batching messages (0 = no batching)
	BatchSize int

	// Maximum time to wait before flushing batch
	BatchTimeout time.Duration

	// Compression type (none, gzip, snappy, lz4)
	Compression string

	// Maximum number of in-flight requests
	MaxInFlightRequests int
}

// ConsumerConfig holds consumer-specific configuration
type ConsumerConfig struct {
	// Consumer group ID
	GroupID string

	// Offset to start consuming from (earliest, latest, or specific offset)
	StartOffset int64

	// Maximum bytes to fetch per request
	MaxFetchBytes uint32

	// Minimum bytes before server responds
	MinFetchBytes uint32

	// Maximum wait time for server to accumulate min bytes
	MaxWaitTime time.Duration

	// Auto-commit offset interval
	AutoCommitInterval time.Duration
}

// DefaultConfig returns default client configuration
func DefaultConfig() *Config {
	return &Config{
		Brokers:                 []string{"localhost:9092"},
		ConnectTimeout:          10 * time.Second,
		ReadTimeout:             30 * time.Second,
		WriteTimeout:            30 * time.Second,
		MaxConnectionsPerBroker: 5,
		MaxRetries:              3,
		RetryBackoff:            100 * time.Millisecond,
		RetryMaxDelay:           30 * time.Second,
		RequestTimeout:          30 * time.Second,
		KeepAlive:               true,
		KeepAlivePeriod:         30 * time.Second,
		ProducerConfig: ProducerConfig{
			RequireAck:          true,
			BatchSize:           100,
			BatchTimeout:        10 * time.Millisecond,
			Compression:         "none",
			MaxInFlightRequests: 5,
		},
		ConsumerConfig: ConsumerConfig{
			GroupID:            "",
			StartOffset:        -1, // Latest
			MaxFetchBytes:      1024 * 1024,     // 1MB
			MinFetchBytes:      1,               // Return immediately
			MaxWaitTime:        500 * time.Millisecond,
			AutoCommitInterval: 5 * time.Second,
		},
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if len(c.Brokers) == 0 {
		return ErrNoBrokers
	}

	if c.ConnectTimeout <= 0 {
		return ErrInvalidTimeout
	}

	if c.MaxConnectionsPerBroker <= 0 {
		return ErrInvalidMaxConnections
	}

	if c.MaxRetries < 0 {
		return ErrInvalidRetries
	}

	return nil
}
