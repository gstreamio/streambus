package consensus

import (
	"testing"
	"time"
)

func TestNewBackoff(t *testing.T) {
	b := NewBackoff()

	if b == nil {
		t.Fatal("NewBackoff() returned nil")
	}

	if b.InitialInterval != 100*time.Millisecond {
		t.Errorf("InitialInterval = %v, want %v", b.InitialInterval, 100*time.Millisecond)
	}

	if b.MaxInterval != 30*time.Second {
		t.Errorf("MaxInterval = %v, want %v", b.MaxInterval, 30*time.Second)
	}

	if b.Multiplier != 2.0 {
		t.Errorf("Multiplier = %v, want %v", b.Multiplier, 2.0)
	}

	if b.Jitter != 0.2 {
		t.Errorf("Jitter = %v, want %v", b.Jitter, 0.2)
	}
}

func TestBackoff_Next(t *testing.T) {
	b := NewBackoff()

	// Test exponential backoff progression
	tests := []struct {
		name        string
		minExpected time.Duration
		maxExpected time.Duration
	}{
		{
			name:        "First attempt",
			minExpected: 80 * time.Millisecond,  // 100ms - 20% jitter
			maxExpected: 120 * time.Millisecond, // 100ms + 20% jitter
		},
		{
			name:        "Second attempt",
			minExpected: 160 * time.Millisecond, // 200ms - 20% jitter
			maxExpected: 240 * time.Millisecond, // 200ms + 20% jitter
		},
		{
			name:        "Third attempt",
			minExpected: 320 * time.Millisecond, // 400ms - 20% jitter
			maxExpected: 480 * time.Millisecond, // 400ms + 20% jitter
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duration := b.Next()

			if duration < tt.minExpected || duration > tt.maxExpected {
				t.Errorf("Next() = %v, want between %v and %v",
					duration, tt.minExpected, tt.maxExpected)
			}
		})
	}
}

func TestBackoff_NextMaxCap(t *testing.T) {
	b := NewBackoff()

	// Keep calling Next() until we hit the max
	var lastDuration time.Duration
	for i := 0; i < 20; i++ {
		lastDuration = b.Next()
	}

	// After many attempts, should be capped at MaxInterval with jitter
	maxWithJitter := time.Duration(float64(b.MaxInterval) * 1.2)
	if lastDuration > maxWithJitter {
		t.Errorf("Next() after many attempts = %v, should be capped around %v",
			lastDuration, b.MaxInterval)
	}
}

func TestBackoff_Reset(t *testing.T) {
	b := NewBackoff()

	// Progress the backoff
	b.Next()
	b.Next()
	b.Next()

	// Reset
	b.Reset()

	// Next call should be back to initial interval
	duration := b.Next()
	minExpected := 80 * time.Millisecond  // 100ms - 20% jitter
	maxExpected := 120 * time.Millisecond // 100ms + 20% jitter

	if duration < minExpected || duration > maxExpected {
		t.Errorf("After Reset(), Next() = %v, want between %v and %v",
			duration, minExpected, maxExpected)
	}
}

func TestBackoff_Jitter(t *testing.T) {
	b := NewBackoff()

	// Call Next() multiple times and ensure we get different values due to jitter
	durations := make(map[time.Duration]bool)
	for i := 0; i < 10; i++ {
		b.Reset()
		duration := b.Next()
		durations[duration] = true
	}

	// Should have at least 2 different durations due to jitter
	// (statistically, should have 10 different values, but being conservative)
	if len(durations) < 2 {
		t.Error("Jitter not working - all backoff durations are identical")
	}
}

func TestNewCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker()

	if cb == nil {
		t.Fatal("NewCircuitBreaker() returned nil")
	}

	if cb.maxFailures != 5 {
		t.Errorf("maxFailures = %v, want %v", cb.maxFailures, 5)
	}

	if cb.resetTimeout != 30*time.Second {
		t.Errorf("resetTimeout = %v, want %v", cb.resetTimeout, 30*time.Second)
	}

	if cb.state != StateClosed {
		t.Errorf("initial state = %v, want %v", cb.state, StateClosed)
	}
}

func TestCircuitBreaker_CallClosed(t *testing.T) {
	cb := NewCircuitBreaker()

	// Circuit should be closed initially, allowing calls
	if !cb.Call() {
		t.Error("Call() returned false for closed circuit")
	}
}

func TestCircuitBreaker_RecordSuccess(t *testing.T) {
	cb := NewCircuitBreaker()

	// Record some failures
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.failureCount != 2 {
		t.Errorf("failureCount = %v, want 2", cb.failureCount)
	}

	// Record success
	cb.RecordSuccess()

	// Failure count should be reset
	if cb.failureCount != 0 {
		t.Errorf("After RecordSuccess(), failureCount = %v, want 0", cb.failureCount)
	}

	// State should be closed
	if cb.state != StateClosed {
		t.Errorf("After RecordSuccess(), state = %v, want %v", cb.state, StateClosed)
	}
}

func TestCircuitBreaker_RecordFailure(t *testing.T) {
	cb := NewCircuitBreaker()

	// Record failures up to max
	for i := 1; i <= 5; i++ {
		cb.RecordFailure()

		if i < 5 {
			if cb.state != StateClosed {
				t.Errorf("After %d failures, state = %v, want %v", i, cb.state, StateClosed)
			}
		} else {
			if cb.state != StateOpen {
				t.Errorf("After %d failures, state = %v, want %v", i, cb.state, StateOpen)
			}
		}
	}
}

func TestCircuitBreaker_CallOpen(t *testing.T) {
	cb := NewCircuitBreaker()

	// Open the circuit by recording max failures
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}

	// Call() should return false for open circuit
	if cb.Call() {
		t.Error("Call() returned true for open circuit (should be false)")
	}
}

func TestCircuitBreaker_HalfOpen(t *testing.T) {
	cb := NewCircuitBreaker()
	cb.resetTimeout = 100 * time.Millisecond // Short timeout for testing

	// Open the circuit
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}

	// Verify circuit is open
	if cb.state != StateOpen {
		t.Errorf("state = %v, want %v", cb.state, StateOpen)
	}

	// Call should be rejected
	if cb.Call() {
		t.Error("Call() returned true immediately after opening")
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// Call should now succeed (half-open)
	if !cb.Call() {
		t.Error("Call() returned false after reset timeout (should enter half-open)")
	}

	// State should be half-open
	if cb.state != StateHalfOpen {
		t.Errorf("After timeout, state = %v, want %v", cb.state, StateHalfOpen)
	}
}

func TestCircuitBreaker_HalfOpenSuccess(t *testing.T) {
	cb := NewCircuitBreaker()
	cb.resetTimeout = 100 * time.Millisecond

	// Open the circuit
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// Enter half-open state
	cb.Call()

	// Record success in half-open state
	cb.RecordSuccess()

	// Circuit should be closed again
	if cb.state != StateClosed {
		t.Errorf("After success in half-open, state = %v, want %v", cb.state, StateClosed)
	}

	// Failure count should be reset
	if cb.failureCount != 0 {
		t.Errorf("failureCount = %v, want 0", cb.failureCount)
	}
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker()
	cb.resetTimeout = 100 * time.Millisecond

	// Open the circuit
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// Enter half-open state
	cb.Call()

	// Record failure in half-open state
	cb.RecordFailure()

	// Circuit should still allow the half-open test
	// (the state will remain as is, and another failure will open it again)
	if cb.failureCount == 0 {
		t.Error("Failure in half-open should increment failure count")
	}
}

func TestCircuitBreaker_IsOpen(t *testing.T) {
	cb := NewCircuitBreaker()
	cb.resetTimeout = 100 * time.Millisecond

	// Initially closed
	if cb.IsOpen() {
		t.Error("IsOpen() = true for new circuit breaker")
	}

	// Open the circuit
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}

	// Should be open
	if !cb.IsOpen() {
		t.Error("IsOpen() = false after max failures")
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// Should not be open anymore (timeout expired)
	if cb.IsOpen() {
		t.Error("IsOpen() = true after reset timeout expired")
	}
}

func TestCircuitBreaker_State(t *testing.T) {
	cb := NewCircuitBreaker()

	// Test closed state
	if cb.State() != StateClosed {
		t.Errorf("Initial State() = %v, want %v", cb.State(), StateClosed)
	}

	// Open the circuit
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}

	// Test open state
	if cb.State() != StateOpen {
		t.Errorf("After failures, State() = %v, want %v", cb.State(), StateOpen)
	}
}

func TestCircuitBreaker_ConcurrentCalls(t *testing.T) {
	cb := NewCircuitBreaker()

	// Test concurrent calls (should be thread-safe)
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			cb.Call()
			cb.RecordFailure()
			cb.RecordSuccess()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic or deadlock
}

func TestBackoffResetAfterSuccess(t *testing.T) {
	// Simulate real-world scenario: backoff increases, then connection succeeds
	b := NewBackoff()

	// Several failed attempts
	durations := []time.Duration{}
	for i := 0; i < 5; i++ {
		duration := b.Next()
		durations = append(durations, duration)
	}

	// Verify backoff is increasing
	for i := 1; i < len(durations); i++ {
		// Each duration should be roughly 2x the previous (with jitter variance)
		if durations[i] < durations[i-1] {
			t.Errorf("Backoff not increasing: durations[%d]=%v < durations[%d]=%v",
				i, durations[i], i-1, durations[i-1])
		}
	}

	// Connection succeeds, reset backoff
	b.Reset()

	// Next attempt should be back to initial interval
	duration := b.Next()
	if duration > 150*time.Millisecond { // Generous margin for jitter
		t.Errorf("After Reset(), Next() = %v, should be close to initial interval", duration)
	}
}

func TestCircuitBreakerWithBackoff(t *testing.T) {
	// Test realistic scenario: circuit breaker and backoff working together
	cb := NewCircuitBreaker()
	b := NewBackoff()

	// Simulate 10 failed connection attempts
	for i := 0; i < 10; i++ {
		// Check if circuit allows the call
		if !cb.Call() {
			// Circuit is open, don't even try
			continue
		}

		// Simulate connection attempt with backoff
		_ = b.Next()

		// Simulate failure (for first 5 attempts)
		if i < 5 {
			cb.RecordFailure()
		} else {
			// After circuit opens, this shouldn't execute
			t.Error("Should not reach here - circuit should be open")
		}
	}

	// Circuit should be open
	if cb.state != StateOpen {
		t.Errorf("After 5 failures, circuit state = %v, want %v", cb.state, StateOpen)
	}

	// Backoff should have increased
	if b.currentInterval <= b.InitialInterval {
		t.Error("Backoff should have increased after multiple failures")
	}
}

func BenchmarkBackoffNext(b *testing.B) {
	backoff := NewBackoff()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backoff.Next()
	}
}

func BenchmarkCircuitBreakerCall(b *testing.B) {
	cb := NewCircuitBreaker()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Call()
	}
}

func BenchmarkCircuitBreakerRecordFailure(b *testing.B) {
	cb := NewCircuitBreaker()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.RecordFailure()
		if cb.state == StateOpen {
			cb.RecordSuccess() // Reset to avoid staying open
		}
	}
}
