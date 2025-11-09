package timeout

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.NotNil(t, config)
	assert.Equal(t, 5*time.Second, config.RaftProposal)
	assert.Equal(t, 10*time.Second, config.RaftElection)
	assert.Equal(t, 1*time.Second, config.RaftHeartbeat)
	assert.Equal(t, 30*time.Second, config.GracefulShutdown)
}

func TestProductionConfig(t *testing.T) {
	config := ProductionConfig()

	assert.NotNil(t, config)
	assert.Equal(t, 10*time.Second, config.RaftProposal)
	assert.Equal(t, 15*time.Second, config.RaftElection)
	assert.Equal(t, 60*time.Second, config.GracefulShutdown)
	// Production should have longer timeouts than default
	assert.Greater(t, config.RaftProposal, DefaultConfig().RaftProposal)
}

func TestTestConfig(t *testing.T) {
	config := TestConfig()

	assert.NotNil(t, config)
	assert.Equal(t, 1*time.Second, config.RaftProposal)
	assert.Equal(t, 2*time.Second, config.RaftElection)
	assert.Equal(t, 5*time.Second, config.GracefulShutdown)
	// Test should have shorter timeouts than default
	assert.Less(t, config.RaftProposal, DefaultConfig().RaftProposal)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		expectErr bool
		errMsg    string
	}{
		{
			name:      "valid default config",
			config:    DefaultConfig(),
			expectErr: false,
		},
		{
			name: "zero raft proposal",
			config: &Config{
				RaftProposal:     0,
				RaftElection:     10 * time.Second,
				RaftHeartbeat:    1 * time.Second,
				HealthCheck:      2 * time.Second,
				GracefulShutdown: 30 * time.Second,
			},
			expectErr: true,
			errMsg:    "raft_proposal",
		},
		{
			name: "election less than heartbeat",
			config: &Config{
				RaftProposal:     5 * time.Second,
				RaftElection:     500 * time.Millisecond,
				RaftHeartbeat:    1 * time.Second,
				HealthCheck:      2 * time.Second,
				GracefulShutdown: 30 * time.Second,
			},
			expectErr: true,
			errMsg:    "raft_election",
		},
		{
			name: "zero health check",
			config: &Config{
				RaftProposal:     5 * time.Second,
				RaftElection:     10 * time.Second,
				RaftHeartbeat:    1 * time.Second,
				HealthCheck:      0,
				GracefulShutdown: 30 * time.Second,
			},
			expectErr: true,
			errMsg:    "health_check",
		},
		{
			name: "zero graceful shutdown",
			config: &Config{
				RaftProposal:     5 * time.Second,
				RaftElection:     10 * time.Second,
				RaftHeartbeat:    1 * time.Second,
				HealthCheck:      2 * time.Second,
				GracefulShutdown: 0,
			},
			expectErr: true,
			errMsg:    "graceful_shutdown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewManager(t *testing.T) {
	manager, err := NewManager(DefaultConfig())
	require.NoError(t, err)
	assert.NotNil(t, manager)
}

func TestNewManager_NilConfig(t *testing.T) {
	manager, err := NewManager(nil)
	require.NoError(t, err)
	assert.NotNil(t, manager)
}

func TestNewManager_InvalidConfig(t *testing.T) {
	invalidConfig := &Config{
		RaftProposal: 0,
	}
	manager, err := NewManager(invalidConfig)
	assert.Error(t, err)
	assert.Nil(t, manager)
}

func TestManager_GetConfig(t *testing.T) {
	manager, err := NewManager(DefaultConfig())
	require.NoError(t, err)

	config := manager.GetConfig()
	assert.NotNil(t, config)
	assert.Equal(t, 5*time.Second, config.RaftProposal)

	// Verify it's a copy by modifying it
	config.RaftProposal = 1 * time.Second
	config2 := manager.GetConfig()
	assert.Equal(t, 5*time.Second, config2.RaftProposal)
}

func TestManager_UpdateConfig(t *testing.T) {
	manager, err := NewManager(DefaultConfig())
	require.NoError(t, err)

	newConfig := ProductionConfig()
	err = manager.UpdateConfig(newConfig)
	require.NoError(t, err)

	config := manager.GetConfig()
	assert.Equal(t, 10*time.Second, config.RaftProposal)
}

func TestManager_UpdateConfig_Invalid(t *testing.T) {
	manager, err := NewManager(DefaultConfig())
	require.NoError(t, err)

	invalidConfig := &Config{RaftProposal: 0}
	err = manager.UpdateConfig(invalidConfig)
	assert.Error(t, err)

	// Original config should be unchanged
	config := manager.GetConfig()
	assert.Equal(t, 5*time.Second, config.RaftProposal)
}

func TestManager_WithRaftProposal(t *testing.T) {
	manager, err := NewManager(TestConfig())
	require.NoError(t, err)

	ctx, cancel := manager.WithRaftProposal(context.Background())
	defer cancel()

	assert.NotNil(t, ctx)
	deadline, ok := ctx.Deadline()
	assert.True(t, ok)
	assert.True(t, time.Until(deadline) <= 1*time.Second)
}

func TestManager_WithHealthCheck(t *testing.T) {
	manager, err := NewManager(TestConfig())
	require.NoError(t, err)

	ctx, cancel := manager.WithHealthCheck(context.Background())
	defer cancel()

	assert.NotNil(t, ctx)
	deadline, ok := ctx.Deadline()
	assert.True(t, ok)
	assert.True(t, time.Until(deadline) <= 500*time.Millisecond)
}

func TestManager_WithRequest(t *testing.T) {
	manager, err := NewManager(TestConfig())
	require.NoError(t, err)

	ctx, cancel := manager.WithRequest(context.Background())
	defer cancel()

	assert.NotNil(t, ctx)
	deadline, ok := ctx.Deadline()
	assert.True(t, ok)
	assert.True(t, time.Until(deadline) <= 5*time.Second)
}

func TestManager_WithCustom(t *testing.T) {
	manager, err := NewManager(DefaultConfig())
	require.NoError(t, err)

	customTimeout := 123 * time.Millisecond
	ctx, cancel := manager.WithCustom(context.Background(), customTimeout)
	defer cancel()

	assert.NotNil(t, ctx)
	deadline, ok := ctx.Deadline()
	assert.True(t, ok)
	assert.True(t, time.Until(deadline) <= customTimeout)
}

func TestOperation_Execute(t *testing.T) {
	op := NewOperation("test.operation", 100*time.Millisecond)

	// Test successful execution
	err := op.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	assert.NoError(t, err)
}

func TestOperation_Execute_Error(t *testing.T) {
	op := NewOperation("test.operation", 100*time.Millisecond)

	testErr := errors.New("test error")
	err := op.Execute(context.Background(), func(ctx context.Context) error {
		return testErr
	})
	assert.Equal(t, testErr, err)
}

func TestOperation_Execute_Timeout(t *testing.T) {
	op := NewOperation("test.operation", 50*time.Millisecond)

	err := op.Execute(context.Background(), func(ctx context.Context) error {
		time.Sleep(200 * time.Millisecond)
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "test.operation")
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestOperation_Execute_ContextCancellation(t *testing.T) {
	op := NewOperation("test.operation", 1*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := op.Execute(ctx, func(ctx context.Context) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "test.operation")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestOperation_ExecuteWithRetry_Success(t *testing.T) {
	op := NewOperation("test.operation", 100*time.Millisecond)

	callCount := 0
	err := op.ExecuteWithRetry(context.Background(), 3, func(ctx context.Context) error {
		callCount++
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestOperation_ExecuteWithRetry_EventualSuccess(t *testing.T) {
	op := NewOperation("test.operation", 100*time.Millisecond)

	callCount := 0
	err := op.ExecuteWithRetry(context.Background(), 3, func(ctx context.Context) error {
		callCount++
		if callCount < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 3, callCount)
}

func TestOperation_ExecuteWithRetry_AllFail(t *testing.T) {
	op := NewOperation("test.operation", 100*time.Millisecond)

	callCount := 0
	testErr := errors.New("persistent error")
	err := op.ExecuteWithRetry(context.Background(), 2, func(ctx context.Context) error {
		callCount++
		return testErr
	})

	assert.Equal(t, testErr, err)
	assert.Equal(t, 3, callCount) // maxRetries=2 means 3 total attempts
}

func TestOperation_ExecuteWithRetry_ContextCanceled(t *testing.T) {
	op := NewOperation("test.operation", 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	err := op.ExecuteWithRetry(ctx, 5, func(ctx context.Context) error {
		callCount++
		if callCount == 2 {
			cancel() // Cancel after second attempt
		}
		return errors.New("test error")
	})

	assert.Error(t, err)
	assert.Equal(t, 2, callCount)
}

func TestOperation_ExecuteWithRetry_Backoff(t *testing.T) {
	op := NewOperation("test.operation", 100*time.Millisecond)

	start := time.Now()
	callCount := 0
	_ = op.ExecuteWithRetry(context.Background(), 3, func(ctx context.Context) error {
		callCount++
		return errors.New("test error")
	})
	duration := time.Since(start)

	// Should have some backoff delay (100ms + 200ms + 400ms = 700ms minimum)
	assert.Greater(t, duration, 600*time.Millisecond)
	assert.Equal(t, 4, callCount) // maxRetries=3 means 4 total attempts
}

func TestDefaultManager(t *testing.T) {
	manager := Default()
	assert.NotNil(t, manager)

	config := manager.GetConfig()
	assert.Equal(t, 5*time.Second, config.RaftProposal)
}

func TestSetDefaultManager(t *testing.T) {
	// Save original
	original := Default()
	defer SetDefaultManager(original)

	// Create new manager
	newManager, err := NewManager(TestConfig())
	require.NoError(t, err)

	SetDefaultManager(newManager)

	manager := Default()
	config := manager.GetConfig()
	assert.Equal(t, 1*time.Second, config.RaftProposal)
}

func TestGlobalWithRaftProposal(t *testing.T) {
	ctx, cancel := WithRaftProposal(context.Background())
	defer cancel()

	assert.NotNil(t, ctx)
	deadline, ok := ctx.Deadline()
	assert.True(t, ok)
	assert.True(t, time.Until(deadline) <= 5*time.Second)
}

func TestGlobalWithHealthCheck(t *testing.T) {
	ctx, cancel := WithHealthCheck(context.Background())
	defer cancel()

	assert.NotNil(t, ctx)
	deadline, ok := ctx.Deadline()
	assert.True(t, ok)
	assert.True(t, time.Until(deadline) <= 2*time.Second)
}

func TestGlobalWithRequest(t *testing.T) {
	ctx, cancel := WithRequest(context.Background())
	defer cancel()

	assert.NotNil(t, ctx)
	deadline, ok := ctx.Deadline()
	assert.True(t, ok)
	assert.True(t, time.Until(deadline) <= 30*time.Second)
}

func TestManager_AllTimeoutMethods(t *testing.T) {
	manager, err := NewManager(TestConfig())
	require.NoError(t, err)

	tests := []struct {
		name   string
		create func(context.Context) (context.Context, context.CancelFunc)
	}{
		{"RaftProposal", manager.WithRaftProposal},
		{"RaftElection", manager.WithRaftElection},
		{"MetadataUpdate", manager.WithMetadataUpdate},
		{"MetadataSync", manager.WithMetadataSync},
		{"Rebalance", manager.WithRebalance},
		{"HealthCheck", manager.WithHealthCheck},
		{"ReadinessCheck", manager.WithReadinessCheck},
		{"Connection", manager.WithConnection},
		{"Request", manager.WithRequest},
		{"GracefulShutdown", manager.WithGracefulShutdown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := tt.create(context.Background())
			defer cancel()

			assert.NotNil(t, ctx)
			deadline, ok := ctx.Deadline()
			assert.True(t, ok)
			assert.True(t, time.Until(deadline) > 0)
		})
	}
}

func BenchmarkOperation_Execute(b *testing.B) {
	op := NewOperation("benchmark", 1*time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = op.Execute(context.Background(), func(ctx context.Context) error {
			return nil
		})
	}
}

func BenchmarkManager_WithTimeout(b *testing.B) {
	manager, _ := NewManager(DefaultConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := manager.WithRaftProposal(context.Background())
		cancel()
		_ = ctx
	}
}

func ExampleManager() {
	manager, _ := NewManager(DefaultConfig())

	// Create a context with raft proposal timeout
	ctx, cancel := manager.WithRaftProposal(context.Background())
	defer cancel()

	// Use the context for an operation
	_ = ctx
}

func ExampleOperation() {
	op := NewOperation("database.query", 5*time.Second)

	err := op.Execute(context.Background(), func(ctx context.Context) error {
		// Perform database query
		return nil
	})

	if err != nil {
		// Handle error
	}
}

func ExampleOperation_ExecuteWithRetry() {
	op := NewOperation("api.request", 3*time.Second)

	err := op.ExecuteWithRetry(context.Background(), 3, func(ctx context.Context) error {
		// Make API request
		return nil
	})

	if err != nil {
		// All retries failed
	}
}
