package timeout

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Config holds timeout configuration for various operations
type Config struct {
	// Consensus timeouts
	RaftProposal     time.Duration `json:"raft_proposal"`
	RaftElection     time.Duration `json:"raft_election"`
	RaftHeartbeat    time.Duration `json:"raft_heartbeat"`
	RaftSnapshot     time.Duration `json:"raft_snapshot"`

	// Metadata timeouts
	MetadataUpdate   time.Duration `json:"metadata_update"`
	MetadataSync     time.Duration `json:"metadata_sync"`

	// Cluster coordination timeouts
	RebalanceTimeout time.Duration `json:"rebalance_timeout"`
	BrokerHeartbeat  time.Duration `json:"broker_heartbeat"`
	LeaderElection   time.Duration `json:"leader_election"`

	// Health check timeouts
	HealthCheck      time.Duration `json:"health_check"`
	ReadinessCheck   time.Duration `json:"readiness_check"`

	// Network timeouts
	ConnectionTimeout time.Duration `json:"connection_timeout"`
	RequestTimeout    time.Duration `json:"request_timeout"`
	IdleTimeout       time.Duration `json:"idle_timeout"`

	// Shutdown timeouts
	GracefulShutdown time.Duration `json:"graceful_shutdown"`
	ForceShutdown    time.Duration `json:"force_shutdown"`
}

// DefaultConfig returns default timeout configuration
func DefaultConfig() *Config {
	return &Config{
		// Consensus timeouts
		RaftProposal:  5 * time.Second,
		RaftElection:  10 * time.Second,
		RaftHeartbeat: 1 * time.Second,
		RaftSnapshot:  30 * time.Second,

		// Metadata timeouts
		MetadataUpdate: 3 * time.Second,
		MetadataSync:   10 * time.Second,

		// Cluster coordination timeouts
		RebalanceTimeout: 30 * time.Second,
		BrokerHeartbeat:  2 * time.Second,
		LeaderElection:   15 * time.Second,

		// Health check timeouts
		HealthCheck:    2 * time.Second,
		ReadinessCheck: 3 * time.Second,

		// Network timeouts
		ConnectionTimeout: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		IdleTimeout:       60 * time.Second,

		// Shutdown timeouts
		GracefulShutdown: 30 * time.Second,
		ForceShutdown:    5 * time.Second,
	}
}

// ProductionConfig returns production-tuned timeout configuration
func ProductionConfig() *Config {
	return &Config{
		// Consensus timeouts (more generous for production)
		RaftProposal:  10 * time.Second,
		RaftElection:  15 * time.Second,
		RaftHeartbeat: 1 * time.Second,
		RaftSnapshot:  60 * time.Second,

		// Metadata timeouts
		MetadataUpdate: 5 * time.Second,
		MetadataSync:   15 * time.Second,

		// Cluster coordination timeouts
		RebalanceTimeout: 60 * time.Second,
		BrokerHeartbeat:  3 * time.Second,
		LeaderElection:   20 * time.Second,

		// Health check timeouts
		HealthCheck:    3 * time.Second,
		ReadinessCheck: 5 * time.Second,

		// Network timeouts
		ConnectionTimeout: 15 * time.Second,
		RequestTimeout:    60 * time.Second,
		IdleTimeout:       120 * time.Second,

		// Shutdown timeouts
		GracefulShutdown: 60 * time.Second,
		ForceShutdown:    10 * time.Second,
	}
}

// TestConfig returns test-friendly timeout configuration (shorter timeouts)
func TestConfig() *Config {
	return &Config{
		// Consensus timeouts
		RaftProposal:  1 * time.Second,
		RaftElection:  2 * time.Second,
		RaftHeartbeat: 200 * time.Millisecond,
		RaftSnapshot:  5 * time.Second,

		// Metadata timeouts
		MetadataUpdate: 1 * time.Second,
		MetadataSync:   2 * time.Second,

		// Cluster coordination timeouts
		RebalanceTimeout: 5 * time.Second,
		BrokerHeartbeat:  500 * time.Millisecond,
		LeaderElection:   3 * time.Second,

		// Health check timeouts
		HealthCheck:    500 * time.Millisecond,
		ReadinessCheck: 1 * time.Second,

		// Network timeouts
		ConnectionTimeout: 2 * time.Second,
		RequestTimeout:    5 * time.Second,
		IdleTimeout:       10 * time.Second,

		// Shutdown timeouts
		GracefulShutdown: 5 * time.Second,
		ForceShutdown:    1 * time.Second,
	}
}

// Validate validates the timeout configuration
func (c *Config) Validate() error {
	if c.RaftProposal <= 0 {
		return fmt.Errorf("raft_proposal timeout must be positive")
	}
	if c.RaftElection <= c.RaftHeartbeat {
		return fmt.Errorf("raft_election timeout must be greater than raft_heartbeat")
	}
	if c.RaftHeartbeat <= 0 {
		return fmt.Errorf("raft_heartbeat timeout must be positive")
	}
	if c.HealthCheck <= 0 {
		return fmt.Errorf("health_check timeout must be positive")
	}
	if c.GracefulShutdown <= 0 {
		return fmt.Errorf("graceful_shutdown timeout must be positive")
	}
	return nil
}

// Manager manages timeout contexts
type Manager struct {
	mu     sync.RWMutex
	config *Config
}

// NewManager creates a new timeout manager
func NewManager(config *Config) (*Manager, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Manager{
		config: config,
	}, nil
}

// GetConfig returns a copy of the current configuration
func (m *Manager) GetConfig() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external modification
	configCopy := *m.config
	return &configCopy
}

// UpdateConfig updates the timeout configuration
func (m *Manager) UpdateConfig(config *Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
	return nil
}

// WithRaftProposal creates a context with raft proposal timeout
func (m *Manager) WithRaftProposal(parent context.Context) (context.Context, context.CancelFunc) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return context.WithTimeout(parent, m.config.RaftProposal)
}

// WithRaftElection creates a context with raft election timeout
func (m *Manager) WithRaftElection(parent context.Context) (context.Context, context.CancelFunc) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return context.WithTimeout(parent, m.config.RaftElection)
}

// WithMetadataUpdate creates a context with metadata update timeout
func (m *Manager) WithMetadataUpdate(parent context.Context) (context.Context, context.CancelFunc) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return context.WithTimeout(parent, m.config.MetadataUpdate)
}

// WithMetadataSync creates a context with metadata sync timeout
func (m *Manager) WithMetadataSync(parent context.Context) (context.Context, context.CancelFunc) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return context.WithTimeout(parent, m.config.MetadataSync)
}

// WithRebalance creates a context with rebalance timeout
func (m *Manager) WithRebalance(parent context.Context) (context.Context, context.CancelFunc) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return context.WithTimeout(parent, m.config.RebalanceTimeout)
}

// WithHealthCheck creates a context with health check timeout
func (m *Manager) WithHealthCheck(parent context.Context) (context.Context, context.CancelFunc) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return context.WithTimeout(parent, m.config.HealthCheck)
}

// WithReadinessCheck creates a context with readiness check timeout
func (m *Manager) WithReadinessCheck(parent context.Context) (context.Context, context.CancelFunc) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return context.WithTimeout(parent, m.config.ReadinessCheck)
}

// WithConnection creates a context with connection timeout
func (m *Manager) WithConnection(parent context.Context) (context.Context, context.CancelFunc) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return context.WithTimeout(parent, m.config.ConnectionTimeout)
}

// WithRequest creates a context with request timeout
func (m *Manager) WithRequest(parent context.Context) (context.Context, context.CancelFunc) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return context.WithTimeout(parent, m.config.RequestTimeout)
}

// WithGracefulShutdown creates a context with graceful shutdown timeout
func (m *Manager) WithGracefulShutdown(parent context.Context) (context.Context, context.CancelFunc) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return context.WithTimeout(parent, m.config.GracefulShutdown)
}

// WithCustom creates a context with a custom timeout
func (m *Manager) WithCustom(parent context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, duration)
}

// Operation represents a timeout-aware operation
type Operation struct {
	name    string
	timeout time.Duration
}

// NewOperation creates a new operation with timeout
func NewOperation(name string, timeout time.Duration) *Operation {
	return &Operation{
		name:    name,
		timeout: timeout,
	}
}

// Execute executes the operation with timeout
func (o *Operation) Execute(parent context.Context, fn func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(parent, o.timeout)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- fn(ctx)
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return fmt.Errorf("%s: %w", o.name, ctx.Err())
	}
}

// ExecuteWithRetry executes the operation with retries and timeout
func (o *Operation) ExecuteWithRetry(parent context.Context, maxRetries int, fn func(ctx context.Context) error) error {
	var lastErr error
	for i := 0; i <= maxRetries; i++ {
		err := o.Execute(parent, fn)
		if err == nil {
			return nil
		}
		lastErr = err

		// Don't retry on context cancellation
		if parent.Err() != nil {
			return lastErr
		}

		// Exponential backoff
		if i < maxRetries {
			backoff := time.Duration(1<<uint(i)) * 100 * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-parent.Done():
				return parent.Err()
			}
		}
	}
	return lastErr
}

// Global default manager
var defaultManager *Manager

func init() {
	var err error
	defaultManager, err = NewManager(DefaultConfig())
	if err != nil {
		panic(fmt.Sprintf("failed to create default timeout manager: %v", err))
	}
}

// SetDefaultManager sets the global default timeout manager
func SetDefaultManager(manager *Manager) {
	defaultManager = manager
}

// Default returns the default timeout manager
func Default() *Manager {
	return defaultManager
}

// WithRaftProposal creates a context with raft proposal timeout using the default manager
func WithRaftProposal(parent context.Context) (context.Context, context.CancelFunc) {
	return defaultManager.WithRaftProposal(parent)
}

// WithHealthCheck creates a context with health check timeout using the default manager
func WithHealthCheck(parent context.Context) (context.Context, context.CancelFunc) {
	return defaultManager.WithHealthCheck(parent)
}

// WithRequest creates a context with request timeout using the default manager
func WithRequest(parent context.Context) (context.Context, context.CancelFunc) {
	return defaultManager.WithRequest(parent)
}
