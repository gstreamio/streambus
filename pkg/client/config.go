package client

import (
	"crypto/tls"
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

	// Security configuration
	Security *SecurityConfig
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
			StartOffset:        -1,          // Latest
			MaxFetchBytes:      1024 * 1024, // 1MB
			MinFetchBytes:      1,           // Return immediately
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

	// Validate security config if present
	if c.Security != nil {
		if err := c.Security.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// SecurityConfig holds client security configuration
type SecurityConfig struct {
	// TLS configuration
	TLS *TLSConfig

	// SASL configuration
	SASL *SASLConfig

	// API Key
	APIKey string
}

// TLSConfig holds TLS configuration for the client
type TLSConfig struct {
	// Enable TLS
	Enabled bool

	// Certificate file (for client certificate authentication)
	CertFile string

	// Key file (for client certificate authentication)
	KeyFile string

	// CA certificate file (to verify server certificate)
	CAFile string

	// Skip server certificate verification (not recommended for production)
	InsecureSkipVerify bool

	// Server name for certificate verification
	ServerName string

	// Parsed TLS config (internal use)
	config *tls.Config
}

// SASLConfig holds SASL authentication configuration
type SASLConfig struct {
	// Enable SASL
	Enabled bool

	// SASL mechanism (PLAIN, SCRAM-SHA-256, SCRAM-SHA-512)
	Mechanism string

	// Username
	Username string

	// Password
	Password string
}

// Validate validates security configuration
func (s *SecurityConfig) Validate() error {
	if s.TLS != nil && s.TLS.Enabled {
		if s.TLS.CertFile != "" && s.TLS.KeyFile == "" {
			return ErrInvalidTLSConfig
		}
		if s.TLS.KeyFile != "" && s.TLS.CertFile == "" {
			return ErrInvalidTLSConfig
		}
	}

	if s.SASL != nil && s.SASL.Enabled {
		if s.SASL.Username == "" {
			return ErrInvalidSASLConfig
		}
		if s.SASL.Password == "" {
			return ErrInvalidSASLConfig
		}
		if s.SASL.Mechanism == "" {
			s.SASL.Mechanism = "SCRAM-SHA-256" // Default
		}
	}

	return nil
}
