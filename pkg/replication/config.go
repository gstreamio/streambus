package replication

import (
	"fmt"
	"time"
)

// Config holds replication configuration
type Config struct {
	// FetchMinBytes is the minimum bytes to return in a fetch response
	// Setting this higher reduces CPU usage but increases latency
	FetchMinBytes int

	// FetchMaxBytes is the maximum bytes to return in a fetch response
	FetchMaxBytes int

	// FetchMaxWaitMs is the maximum time to wait for FetchMinBytes
	// If MinBytes not reached within this time, return what we have
	FetchMaxWaitMs int

	// FetcherInterval is how often followers fetch from leader
	FetcherInterval time.Duration

	// ReplicaLagMaxMessages is max messages a replica can lag before ISR removal
	ReplicaLagMaxMessages int64

	// ReplicaLagTimeMaxMs is max time a replica can lag before ISR removal
	ReplicaLagTimeMaxMs int64

	// HighWaterMarkCheckpointIntervalMs is how often to checkpoint HW
	HighWaterMarkCheckpointIntervalMs int64

	// NumFetcherThreads is the number of fetcher threads per follower
	NumFetcherThreads int

	// MaxInflightFetches is max concurrent fetch requests per follower
	MaxInflightFetches int

	// FetchBackoffMs is backoff time after fetch error
	FetchBackoffMs int64

	// ReplicaIDForFollower is the replica ID when running as follower
	ReplicaIDForFollower ReplicaID
}

// DefaultConfig returns sensible defaults for replication
func DefaultConfig() *Config {
	return &Config{
		FetchMinBytes:                     1,                 // Return immediately with any data
		FetchMaxBytes:                     1024 * 1024,       // 1MB
		FetchMaxWaitMs:                    500,               // 500ms max wait
		FetcherInterval:                   50 * time.Millisecond, // Fetch every 50ms
		ReplicaLagMaxMessages:             4000,              // 4000 messages
		ReplicaLagTimeMaxMs:               10000,             // 10 seconds
		HighWaterMarkCheckpointIntervalMs: 5000,              // 5 seconds
		NumFetcherThreads:                 1,                 // Single-threaded initially
		MaxInflightFetches:                1,                 // No pipelining initially
		FetchBackoffMs:                    1000,              // 1 second backoff on error
		ReplicaIDForFollower:              0,                 // Must be set
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.FetchMinBytes < 0 {
		return fmt.Errorf("FetchMinBytes must be >= 0, got %d", c.FetchMinBytes)
	}
	if c.FetchMaxBytes <= 0 {
		return fmt.Errorf("FetchMaxBytes must be > 0, got %d", c.FetchMaxBytes)
	}
	if c.FetchMaxBytes < c.FetchMinBytes {
		return fmt.Errorf("FetchMaxBytes (%d) must be >= FetchMinBytes (%d)",
			c.FetchMaxBytes, c.FetchMinBytes)
	}
	if c.FetchMaxWaitMs < 0 {
		return fmt.Errorf("FetchMaxWaitMs must be >= 0, got %d", c.FetchMaxWaitMs)
	}
	if c.FetcherInterval <= 0 {
		return fmt.Errorf("FetcherInterval must be > 0, got %v", c.FetcherInterval)
	}
	if c.ReplicaLagMaxMessages <= 0 {
		return fmt.Errorf("ReplicaLagMaxMessages must be > 0, got %d", c.ReplicaLagMaxMessages)
	}
	if c.ReplicaLagTimeMaxMs <= 0 {
		return fmt.Errorf("ReplicaLagTimeMaxMs must be > 0, got %d", c.ReplicaLagTimeMaxMs)
	}
	if c.NumFetcherThreads <= 0 {
		return fmt.Errorf("NumFetcherThreads must be > 0, got %d", c.NumFetcherThreads)
	}
	if c.MaxInflightFetches <= 0 {
		return fmt.Errorf("MaxInflightFetches must be > 0, got %d", c.MaxInflightFetches)
	}
	return nil
}

// Clone returns a deep copy of the config
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	copy := *c
	return &copy
}
