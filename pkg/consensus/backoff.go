package consensus

import (
	"math/rand/v2"
	"sync"
	"time"
)

// Backoff implements exponential backoff with jitter.
// This is industry standard from:
// - AWS SDK
// - gRPC
// - Kubernetes client-go
// - etcd client
type Backoff struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	Jitter          float64
	currentInterval time.Duration
}

// NewBackoff creates a new backoff with sensible defaults.
// Defaults are based on AWS SDK and gRPC recommendations.
func NewBackoff() *Backoff {
	return &Backoff{
		InitialInterval: 100 * time.Millisecond, // Start small
		MaxInterval:     30 * time.Second,        // Cap at 30s (Kafka default)
		Multiplier:      2.0,                      // Double each time
		Jitter:          0.2,                      // 20% jitter to avoid thundering herd
		currentInterval: 100 * time.Millisecond,
	}
}

// Next returns the next backoff duration.
func (b *Backoff) Next() time.Duration {
	// Calculate next interval
	interval := b.currentInterval

	// Apply jitter: random value between (1-jitter) and (1+jitter)
	// This prevents thundering herd problem
	jitterRange := float64(interval) * b.Jitter
	jitterOffset := (rand.Float64() * 2 * jitterRange) - jitterRange
	jitteredInterval := time.Duration(float64(interval) + jitterOffset)

	// Update for next call (exponential)
	b.currentInterval = time.Duration(float64(b.currentInterval) * b.Multiplier)
	if b.currentInterval > b.MaxInterval {
		b.currentInterval = b.MaxInterval
	}

	return jitteredInterval
}

// Reset resets the backoff to initial interval.
// Call this after a successful operation.
func (b *Backoff) Reset() {
	b.currentInterval = b.InitialInterval
}

// CircuitBreaker implements the circuit breaker pattern.
// This is industry standard from:
// - Netflix Hystrix
// - Kafka
// - Cassandra
// - Google SRE book
type CircuitBreaker struct {
	mu              sync.RWMutex
	maxFailures     int
	resetTimeout    time.Duration
	failureCount    int
	lastFailureTime time.Time
	state           CircuitState
}

// CircuitState represents the state of the circuit breaker.
type CircuitState int

const (
	StateClosed CircuitState = iota // Normal operation
	StateOpen                        // Circuit open, rejecting requests
	StateHalfOpen                    // Testing if service recovered
)

// NewCircuitBreaker creates a new circuit breaker with sensible defaults.
// Defaults are based on Hystrix recommendations.
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  5,              // Open circuit after 5 failures
		resetTimeout: 30 * time.Second, // Try again after 30s (Kafka default)
		state:        StateClosed,
	}
}

// Call attempts to execute the operation through the circuit breaker.
// Returns true if the call should proceed, false if circuit is open.
func (cb *CircuitBreaker) Call() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case StateClosed:
		// Normal operation, allow call
		return true

	case StateOpen:
		// Check if we should try again (half-open)
		if now.Sub(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = StateHalfOpen
			return true
		}
		// Circuit is open, reject call
		return false

	case StateHalfOpen:
		// Allow one test request
		return true
	}

	return false
}

// RecordSuccess records a successful operation.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount = 0
	cb.state = StateClosed
}

// RecordFailure records a failed operation.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.lastFailureTime = time.Now()
	cb.failureCount++

	if cb.failureCount >= cb.maxFailures {
		cb.state = StateOpen
	}
}

// IsOpen returns true if the circuit is open (rejecting requests).
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state == StateOpen && time.Since(cb.lastFailureTime) < cb.resetTimeout
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}
