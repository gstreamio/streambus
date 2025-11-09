package resilience

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// State represents the circuit breaker state
type State int

const (
	// StateClosed allows all requests through
	StateClosed State = iota
	// StateOpen rejects all requests
	StateOpen
	// StateHalfOpen allows limited requests to test recovery
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Stats contains circuit breaker statistics
type Stats struct {
	State                State
	TotalRequests        uint64
	TotalSuccesses       uint64
	TotalFailures        uint64
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
	LastStateChange      time.Time
	LastFailure          time.Time
}

// Config contains circuit breaker configuration
type Config struct {
	// Name identifies this circuit breaker
	Name string

	// MaxFailures is the number of consecutive failures before opening
	MaxFailures uint32

	// Timeout is how long to wait in open state before trying half-open
	Timeout time.Duration

	// MaxRequestsHalfOpen is the number of successful requests needed in half-open
	// state before transitioning to closed
	MaxRequestsHalfOpen uint32

	// OnStateChange is called when the circuit breaker changes state
	OnStateChange func(from, to State)
}

// DefaultConfig returns a circuit breaker config with sensible defaults
func DefaultConfig(name string) *Config {
	return &Config{
		Name:                name,
		MaxFailures:         5,
		Timeout:             30 * time.Second,
		MaxRequestsHalfOpen: 3,
		OnStateChange:       nil,
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	mu     sync.RWMutex
	config *Config

	state                State
	totalRequests        uint64
	totalSuccesses       uint64
	totalFailures        uint64
	consecutiveSuccesses uint32
	consecutiveFailures  uint32
	lastStateChange      time.Time
	lastFailure          time.Time
	openUntil            time.Time
}

// Common errors
var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// New creates a new circuit breaker with the given configuration
func New(config *Config) *CircuitBreaker {
	if config == nil {
		config = DefaultConfig("default")
	}

	return &CircuitBreaker{
		config:          config,
		state:           StateClosed,
		lastStateChange: time.Now(),
	}
}

// Call executes the given function with circuit breaker protection
func (cb *CircuitBreaker) Call(ctx context.Context, fn func() error) error {
	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return err
	}

	// Check if we can proceed
	if err := cb.beforeCall(); err != nil {
		return err
	}

	// Execute the function
	err := fn()

	// Record the result
	cb.afterCall(err)

	return err
}

// beforeCall checks if the request can proceed
func (cb *CircuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalRequests++

	switch cb.state {
	case StateClosed:
		// Allow request
		return nil

	case StateOpen:
		// Check if timeout has elapsed
		if time.Now().After(cb.openUntil) {
			// Transition to half-open
			cb.setState(StateHalfOpen)
			return nil
		}
		// Still open, reject request
		return ErrCircuitOpen

	case StateHalfOpen:
		// Allow limited requests to test recovery
		// In half-open, we allow requests through but track them carefully
		return nil

	default:
		return fmt.Errorf("unknown circuit breaker state: %d", cb.state)
	}
}

// afterCall records the result of the function call
func (cb *CircuitBreaker) afterCall(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err == nil {
		// Success
		cb.onSuccess()
	} else {
		// Failure
		cb.onFailure()
	}
}

// onSuccess handles a successful request
func (cb *CircuitBreaker) onSuccess() {
	cb.totalSuccesses++
	cb.consecutiveSuccesses++
	cb.consecutiveFailures = 0

	switch cb.state {
	case StateClosed:
		// Stay closed
		return

	case StateHalfOpen:
		// Check if we've had enough successes to close
		if cb.consecutiveSuccesses >= cb.config.MaxRequestsHalfOpen {
			cb.setState(StateClosed)
		}

	case StateOpen:
		// Shouldn't happen, but if it does, stay open
		return
	}
}

// onFailure handles a failed request
func (cb *CircuitBreaker) onFailure() {
	cb.totalFailures++
	cb.consecutiveFailures++
	cb.consecutiveSuccesses = 0
	cb.lastFailure = time.Now()

	switch cb.state {
	case StateClosed:
		// Check if we should open
		if cb.consecutiveFailures >= cb.config.MaxFailures {
			cb.setState(StateOpen)
			cb.openUntil = time.Now().Add(cb.config.Timeout)
		}

	case StateHalfOpen:
		// Any failure in half-open immediately opens the circuit
		cb.setState(StateOpen)
		cb.openUntil = time.Now().Add(cb.config.Timeout)

	case StateOpen:
		// Stay open and reset the timeout
		cb.openUntil = time.Now().Add(cb.config.Timeout)
	}
}

// setState changes the circuit breaker state and triggers callback
func (cb *CircuitBreaker) setState(newState State) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState
	cb.lastStateChange = time.Now()

	// Reset counters on state change
	if newState == StateClosed {
		cb.consecutiveFailures = 0
		cb.consecutiveSuccesses = 0
	}

	// Trigger callback if configured
	if cb.config.OnStateChange != nil {
		// Call callback without holding lock
		go cb.config.OnStateChange(oldState, newState)
	}
}

// State returns the current circuit breaker state
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Stats returns current circuit breaker statistics
func (cb *CircuitBreaker) Stats() Stats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return Stats{
		State:                cb.state,
		TotalRequests:        cb.totalRequests,
		TotalSuccesses:       cb.totalSuccesses,
		TotalFailures:        cb.totalFailures,
		ConsecutiveSuccesses: cb.consecutiveSuccesses,
		ConsecutiveFailures:  cb.consecutiveFailures,
		LastStateChange:      cb.lastStateChange,
		LastFailure:          cb.lastFailure,
	}
}

// Reset resets the circuit breaker to closed state with zero stats
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	oldState := cb.state
	cb.state = StateClosed
	cb.totalRequests = 0
	cb.totalSuccesses = 0
	cb.totalFailures = 0
	cb.consecutiveSuccesses = 0
	cb.consecutiveFailures = 0
	cb.lastStateChange = time.Now()
	cb.lastFailure = time.Time{}
	cb.openUntil = time.Time{}

	if oldState != StateClosed && cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(oldState, StateClosed)
	}
}

// Name returns the circuit breaker name
func (cb *CircuitBreaker) Name() string {
	return cb.config.Name
}
