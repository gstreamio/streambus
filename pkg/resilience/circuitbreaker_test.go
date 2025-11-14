package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := New(DefaultConfig("test"))

	assert.Equal(t, StateClosed, cb.State())
	stats := cb.Stats()
	assert.Equal(t, uint64(0), stats.TotalRequests)
	assert.Equal(t, uint64(0), stats.TotalSuccesses)
	assert.Equal(t, uint64(0), stats.TotalFailures)
}

func TestCircuitBreaker_SuccessfulCalls(t *testing.T) {
	cb := New(DefaultConfig("test"))
	ctx := context.Background()

	// Make 10 successful calls
	for i := 0; i < 10; i++ {
		err := cb.Call(ctx, func() error {
			return nil
		})
		require.NoError(t, err)
	}

	// Should still be closed
	assert.Equal(t, StateClosed, cb.State())

	stats := cb.Stats()
	assert.Equal(t, uint64(10), stats.TotalRequests)
	assert.Equal(t, uint64(10), stats.TotalSuccesses)
	assert.Equal(t, uint64(0), stats.TotalFailures)
}

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	config := DefaultConfig("test")
	config.MaxFailures = 3
	cb := New(config)
	ctx := context.Background()

	testErr := errors.New("test error")

	// Make 3 failed calls (should open circuit)
	for i := 0; i < 3; i++ {
		err := cb.Call(ctx, func() error {
			return testErr
		})
		assert.Equal(t, testErr, err)
	}

	// Circuit should now be open
	assert.Equal(t, StateOpen, cb.State())

	// Next call should fail immediately with ErrCircuitOpen
	err := cb.Call(ctx, func() error {
		t.Fatal("should not be called when circuit is open")
		return nil
	})
	assert.ErrorIs(t, err, ErrCircuitOpen)

	stats := cb.Stats()
	assert.Equal(t, uint64(4), stats.TotalRequests)
	assert.Equal(t, uint32(3), stats.ConsecutiveFailures)
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	config := DefaultConfig("test")
	config.MaxFailures = 2
	config.Timeout = 100 * time.Millisecond
	cb := New(config)
	ctx := context.Background()

	testErr := errors.New("test error")

	// Fail twice to open circuit
	for i := 0; i < 2; i++ {
		_ = cb.Call(ctx, func() error { return testErr })
	}

	assert.Equal(t, StateOpen, cb.State())

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Next call should transition to half-open
	called := false
	err := cb.Call(ctx, func() error {
		called = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called, "function should be called in half-open state")
	assert.Equal(t, StateHalfOpen, cb.State())
}

func TestCircuitBreaker_HalfOpenToClosedOnSuccess(t *testing.T) {
	config := DefaultConfig("test")
	config.MaxFailures = 2
	config.Timeout = 50 * time.Millisecond
	config.MaxRequestsHalfOpen = 3
	cb := New(config)
	ctx := context.Background()

	testErr := errors.New("test error")

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Call(ctx, func() error { return testErr })
	}

	assert.Equal(t, StateOpen, cb.State())

	// Wait for timeout to transition to half-open
	time.Sleep(100 * time.Millisecond)

	// Make successful calls in half-open state
	for i := 0; i < 3; i++ {
		err := cb.Call(ctx, func() error { return nil })
		require.NoError(t, err)
	}

	// Should transition to closed after 3 successes
	assert.Equal(t, StateClosed, cb.State())

	stats := cb.Stats()
	// After transitioning to closed, consecutive counters are reset
	assert.Equal(t, uint32(0), stats.ConsecutiveSuccesses)
	assert.Equal(t, uint32(0), stats.ConsecutiveFailures)
	// But total successes should include all successful calls
	assert.GreaterOrEqual(t, stats.TotalSuccesses, uint64(3))
}

func TestCircuitBreaker_HalfOpenToOpenOnFailure(t *testing.T) {
	config := DefaultConfig("test")
	config.MaxFailures = 2
	config.Timeout = 50 * time.Millisecond
	cb := New(config)
	ctx := context.Background()

	testErr := errors.New("test error")

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Call(ctx, func() error { return testErr })
	}

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// First call transitions to half-open and succeeds
	_ = cb.Call(ctx, func() error { return nil })
	assert.Equal(t, StateHalfOpen, cb.State())

	// Next call fails, should immediately open circuit
	_ = cb.Call(ctx, func() error { return testErr })
	assert.Equal(t, StateOpen, cb.State())
}

func TestCircuitBreaker_StateChangeCallback(t *testing.T) {
	var stateChanges []struct {
		from State
		to   State
	}

	config := DefaultConfig("test")
	config.MaxFailures = 2
	config.OnStateChange = func(from, to State) {
		stateChanges = append(stateChanges, struct {
			from State
			to   State
		}{from, to})
	}

	cb := New(config)
	ctx := context.Background()
	testErr := errors.New("test error")

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Call(ctx, func() error { return testErr })
	}

	// Give callback time to execute
	time.Sleep(10 * time.Millisecond)

	require.Len(t, stateChanges, 1)
	assert.Equal(t, StateClosed, stateChanges[0].from)
	assert.Equal(t, StateOpen, stateChanges[0].to)
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := DefaultConfig("test")
	config.MaxFailures = 2
	cb := New(config)
	ctx := context.Background()

	testErr := errors.New("test error")

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Call(ctx, func() error { return testErr })
	}

	assert.Equal(t, StateOpen, cb.State())

	// Reset
	cb.Reset()

	assert.Equal(t, StateClosed, cb.State())
	stats := cb.Stats()
	assert.Equal(t, uint64(0), stats.TotalRequests)
	assert.Equal(t, uint64(0), stats.TotalFailures)
	assert.Equal(t, uint32(0), stats.ConsecutiveFailures)
}

func TestCircuitBreaker_ContextCancellation(t *testing.T) {
	cb := New(DefaultConfig("test"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := cb.Call(ctx, func() error {
		t.Fatal("should not be called with cancelled context")
		return nil
	})

	assert.ErrorIs(t, err, context.Canceled)
}

func TestCircuitBreaker_Stats(t *testing.T) {
	cb := New(DefaultConfig("test"))
	ctx := context.Background()

	// Make some successful calls
	for i := 0; i < 5; i++ {
		_ = cb.Call(ctx, func() error { return nil })
	}

	// Make some failed calls
	testErr := errors.New("test error")
	for i := 0; i < 3; i++ {
		_ = cb.Call(ctx, func() error { return testErr })
	}

	stats := cb.Stats()
	assert.Equal(t, uint64(8), stats.TotalRequests)
	assert.Equal(t, uint64(5), stats.TotalSuccesses)
	assert.Equal(t, uint64(3), stats.TotalFailures)
	assert.Equal(t, uint32(3), stats.ConsecutiveFailures)
	assert.Equal(t, uint32(0), stats.ConsecutiveSuccesses)
}

func TestCircuitBreaker_ConcurrentCalls(t *testing.T) {
	cb := New(DefaultConfig("test"))
	ctx := context.Background()

	const goroutines = 100
	const callsPerGoroutine = 100

	done := make(chan bool, goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			for i := 0; i < callsPerGoroutine; i++ {
				_ = cb.Call(ctx, func() error {
					return nil
				})
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for g := 0; g < goroutines; g++ {
		<-done
	}

	stats := cb.Stats()
	assert.Equal(t, uint64(goroutines*callsPerGoroutine), stats.TotalRequests)
	assert.Equal(t, uint64(goroutines*callsPerGoroutine), stats.TotalSuccesses)
	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_StateString(t *testing.T) {
	assert.Equal(t, "closed", StateClosed.String())
	assert.Equal(t, "open", StateOpen.String())
	assert.Equal(t, "half-open", StateHalfOpen.String())
}

func TestCircuitBreaker_Name(t *testing.T) {
	cb := New(DefaultConfig("my-service"))
	assert.Equal(t, "my-service", cb.Name())
}

func TestCircuitBreaker_OpenCircuitResetsTimeoutOnFailure(t *testing.T) {
	config := DefaultConfig("test")
	config.MaxFailures = 1
	config.Timeout = 200 * time.Millisecond
	cb := New(config)
	ctx := context.Background()

	testErr := errors.New("test error")

	// Open the circuit
	_ = cb.Call(ctx, func() error { return testErr })
	assert.Equal(t, StateOpen, cb.State())

	// Wait half the timeout
	time.Sleep(100 * time.Millisecond)

	// Try to call (will fail with ErrCircuitOpen)
	// This updates openUntil, but since we're still open, it extends the timeout
	_ = cb.Call(ctx, func() error { return nil })

	// Wait another 150ms (total 250ms from first failure)
	// If timeout wasn't reset, we'd be past the original 200ms timeout
	time.Sleep(150 * time.Millisecond)

	// Circuit should still be open
	assert.Equal(t, StateOpen, cb.State())
}

func BenchmarkCircuitBreaker_ClosedState(b *testing.B) {
	cb := New(DefaultConfig("benchmark"))
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.Call(ctx, func() error { return nil })
	}
}

func BenchmarkCircuitBreaker_OpenState(b *testing.B) {
	config := DefaultConfig("benchmark")
	config.MaxFailures = 1
	cb := New(config)
	ctx := context.Background()

	// Open the circuit
	_ = cb.Call(ctx, func() error { return errors.New("fail") })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.Call(ctx, func() error { return nil })
	}
}
