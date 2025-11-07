package server

import (
	"time"
)

// Config holds server configuration
type Config struct {
	// Network address to bind to (e.g., "localhost:9092")
	Address string

	// Maximum number of concurrent connections
	MaxConnections int

	// Read timeout for connections
	ReadTimeout time.Duration

	// Write timeout for connections
	WriteTimeout time.Duration

	// Idle timeout before closing inactive connections
	IdleTimeout time.Duration

	// Maximum message size
	MaxMessageSize uint32

	// Number of worker goroutines for handling requests
	NumWorkers int

	// Enable TCP keep-alive
	KeepAlive bool

	// Keep-alive period
	KeepAlivePeriod time.Duration
}

// DefaultConfig returns default server configuration
func DefaultConfig() *Config {
	return &Config{
		Address:         ":9092",
		MaxConnections:  10000,
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		IdleTimeout:     5 * time.Minute,
		MaxMessageSize:  1024 * 1024 * 10, // 10MB
		NumWorkers:      100,
		KeepAlive:       true,
		KeepAlivePeriod: 30 * time.Second,
	}
}
