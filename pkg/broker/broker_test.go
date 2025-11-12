package broker

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/consensus"
	"github.com/gstreamio/streambus/pkg/logging"
	"github.com/gstreamio/streambus/pkg/security"
	"github.com/gstreamio/streambus/pkg/tenancy"
)

func newTestLogger() *logging.Logger {
	return logging.New(&logging.Config{
		Level:     logging.LevelDebug,
		Output:    os.Stdout,
		Component: "test",
	})
}

func TestBroker_SecurityManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a mock security manager
	secManager := &security.Manager{}

	broker := &Broker{
		ctx:             ctx,
		cancel:          cancel,
		logger:          newTestLogger(),
		securityManager: secManager,
		status:          StatusStopped,
	}

	result := broker.SecurityManager()

	if result != secManager {
		t.Error("SecurityManager() should return the security manager")
	}
}

func TestBroker_SecurityManager_Nil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := &Broker{
		ctx:             ctx,
		cancel:          cancel,
		logger:          newTestLogger(),
		securityManager: nil,
		status:          StatusStopped,
	}

	result := broker.SecurityManager()

	if result != nil {
		t.Error("SecurityManager() should return nil when not set")
	}
}

func TestBroker_TenancyManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a mock tenancy manager
	tenancyMgr := &tenancy.Manager{}

	broker := &Broker{
		ctx:            ctx,
		cancel:         cancel,
		logger:         newTestLogger(),
		tenancyManager: tenancyMgr,
		status:         StatusStopped,
	}

	result := broker.TenancyManager()

	if result != tenancyMgr {
		t.Error("TenancyManager() should return the tenancy manager")
	}
}

func TestBroker_TenancyManager_Nil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := &Broker{
		ctx:            ctx,
		cancel:         cancel,
		logger:         newTestLogger(),
		tenancyManager: nil,
		status:         StatusStopped,
	}

	result := broker.TenancyManager()

	if result != nil {
		t.Error("TenancyManager() should return nil when not set")
	}
}

func TestBroker_Status(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tests := []struct {
		name           string
		initialStatus  BrokerStatus
		expectedStatus BrokerStatus
	}{
		{
			name:           "StatusStopped",
			initialStatus:  StatusStopped,
			expectedStatus: StatusStopped,
		},
		{
			name:           "StatusStarting",
			initialStatus:  StatusStarting,
			expectedStatus: StatusStarting,
		},
		{
			name:           "StatusRunning",
			initialStatus:  StatusRunning,
			expectedStatus: StatusRunning,
		},
		{
			name:           "StatusStopping",
			initialStatus:  StatusStopping,
			expectedStatus: StatusStopping,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker := &Broker{
				ctx:    ctx,
				cancel: cancel,
				logger: newTestLogger(),
				status: tt.initialStatus,
			}

			result := broker.Status()

			if result != tt.expectedStatus {
				t.Errorf("Status() = %v, want %v", result, tt.expectedStatus)
			}
		})
	}
}

func TestBroker_Status_ConcurrentAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := &Broker{
		ctx:    ctx,
		cancel: cancel,
		logger: newTestLogger(),
		status: StatusRunning,
	}

	// Concurrent reads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			status := broker.Status()
			if status != StatusRunning {
				t.Errorf("Status() = %v, want StatusRunning", status)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestBroker_WaitForShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	broker := &Broker{
		ctx:    ctx,
		cancel: cancel,
		logger: newTestLogger(),
		status: StatusRunning,
	}

	// Start a goroutine that waits for shutdown
	shutdownComplete := make(chan bool)
	go func() {
		broker.WaitForShutdown()
		shutdownComplete <- true
	}()

	// Give it a moment to start waiting
	time.Sleep(50 * time.Millisecond)

	// Cancel the context to trigger shutdown
	cancel()

	// Wait for WaitForShutdown to complete
	select {
	case <-shutdownComplete:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("WaitForShutdown did not complete within timeout")
	}
}

func TestBroker_WaitForShutdown_AlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	broker := &Broker{
		ctx:    ctx,
		cancel: cancel,
		logger: newTestLogger(),
		status: StatusStopped,
	}

	// WaitForShutdown should return immediately
	done := make(chan bool)
	go func() {
		broker.WaitForShutdown()
		done <- true
	}()

	select {
	case <-done:
		// Success - returned immediately
	case <-time.After(100 * time.Millisecond):
		t.Error("WaitForShutdown should return immediately when context is already cancelled")
	}
}

func TestBroker_Stop_NotRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := &Broker{
		ctx:    ctx,
		cancel: cancel,
		logger: newTestLogger(),
		status: StatusStopped,
	}

	err := broker.Stop()

	if err == nil {
		t.Error("Stop() should return error when broker is not running")
	}

	expectedError := "broker is not running"
	if err.Error() != expectedError {
		t.Errorf("Stop() error = %v, want %v", err.Error(), expectedError)
	}
}

func TestBroker_Stop_AlreadyStopping(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := &Broker{
		ctx:    ctx,
		cancel: cancel,
		logger: newTestLogger(),
		status: StatusStopping,
	}

	err := broker.Stop()

	if err == nil {
		t.Error("Stop() should return error when broker is already stopping")
	}

	if err.Error() != "broker is not running" {
		t.Errorf("Stop() error = %v, want 'broker is not running'", err.Error())
	}
}

func TestValidateConfig_InvalidBrokerID(t *testing.T) {
	config := &Config{
		BrokerID:    0,
		Host:        "localhost",
		Port:        9092,
		DataDir:     "/tmp/broker",
		RaftDataDir: "/tmp/raft",
		RaftPeers:   []consensus.Peer{{ID: 1, Addr: "localhost:1234"}},
	}

	err := validateConfig(config)

	if err == nil {
		t.Error("validateConfig should return error for invalid BrokerID")
	}

	if err.Error() != "broker_id must be positive" {
		t.Errorf("validateConfig error = %v, want 'broker_id must be positive'", err.Error())
	}
}

func TestValidateConfig_MissingHost(t *testing.T) {
	config := &Config{
		BrokerID:    1,
		Port:        9092,
		DataDir:     "/tmp/broker",
		RaftDataDir: "/tmp/raft",
		RaftPeers:   []consensus.Peer{{ID: 1, Addr: "localhost:1234"}},
	}

	err := validateConfig(config)

	if err == nil {
		t.Error("validateConfig should return error for missing Host")
	}

	if err.Error() != "host is required" {
		t.Errorf("validateConfig error = %v, want 'host is required'", err.Error())
	}
}

func TestValidateConfig_InvalidPort(t *testing.T) {
	config := &Config{
		BrokerID:    1,
		Host:        "localhost",
		Port:        0,
		DataDir:     "/tmp/broker",
		RaftDataDir: "/tmp/raft",
		RaftPeers:   []consensus.Peer{{ID: 1, Addr: "localhost:1234"}},
	}

	err := validateConfig(config)

	if err == nil {
		t.Error("validateConfig should return error for invalid Port")
	}

	if err.Error() != "port must be positive" {
		t.Errorf("validateConfig error = %v, want 'port must be positive'", err.Error())
	}
}

func TestValidateConfig_MissingDataDir(t *testing.T) {
	config := &Config{
		BrokerID:    1,
		Host:        "localhost",
		Port:        9092,
		DataDir:     "",
		RaftDataDir: "/tmp/raft",
		RaftPeers:   []consensus.Peer{{ID: 1, Addr: "localhost:1234"}},
	}

	err := validateConfig(config)

	if err == nil {
		t.Error("validateConfig should return error for missing DataDir")
	}

	if err.Error() != "data_dir is required" {
		t.Errorf("validateConfig error = %v, want 'data_dir is required'", err.Error())
	}
}

func TestValidateConfig_MissingRaftDataDir(t *testing.T) {
	config := &Config{
		BrokerID:  1,
		Host:      "localhost",
		Port:      9092,
		DataDir:   "/tmp/broker",
		RaftPeers: []consensus.Peer{{ID: 1, Addr: "localhost:1234"}},
	}

	err := validateConfig(config)

	if err == nil {
		t.Error("validateConfig should return error for missing RaftDataDir")
	}

	if err.Error() != "raft_data_dir is required" {
		t.Errorf("validateConfig error = %v, want 'raft_data_dir is required'", err.Error())
	}
}

func TestValidateConfig_MissingRaftPeers(t *testing.T) {
	config := &Config{
		BrokerID:    1,
		Host:        "localhost",
		Port:        9092,
		DataDir:     "/tmp/broker",
		RaftDataDir: "/tmp/raft",
	}

	err := validateConfig(config)

	if err == nil {
		t.Error("validateConfig should return error for missing RaftPeers")
	}

	if err.Error() != "raft_peers is required" {
		t.Errorf("validateConfig error = %v, want 'raft_peers is required'", err.Error())
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	config := &Config{
		BrokerID:    1,
		Host:        "localhost",
		Port:        9092,
		DataDir:     "/tmp/broker",
		RaftDataDir: "/tmp/raft",
		RaftPeers:   []consensus.Peer{{ID: 1, Addr: "localhost:1234"}},
	}

	err := validateConfig(config)

	if err != nil {
		t.Errorf("validateConfig should not return error for valid config: %v", err)
	}
}

func TestNew_NilConfig(t *testing.T) {
	broker, err := New(nil)

	if err == nil {
		t.Error("New(nil) should return error")
	}

	if broker != nil {
		t.Error("New(nil) should return nil broker")
	}

	if err.Error() != "config is required" {
		t.Errorf("New(nil) error = %v, want 'config is required'", err.Error())
	}
}

func TestNew_InvalidConfig(t *testing.T) {
	config := &Config{
		BrokerID: 0, // Invalid
		Host:     "localhost",
		Port:     9092,
		DataDir:  "/tmp/broker",
	}

	broker, err := New(config)

	if err == nil {
		t.Error("New with invalid config should return error")
	}

	if broker != nil {
		t.Error("New with invalid config should return nil broker")
	}
}

func TestBrokerStatus_String(t *testing.T) {
	// Note: This test assumes String() method exists on BrokerStatus
	// If it doesn't exist, this test will verify the numeric values at least

	tests := []struct {
		name   string
		status BrokerStatus
		value  int
	}{
		{
			name:   "StatusStopped",
			status: StatusStopped,
			value:  0,
		},
		{
			name:   "StatusStarting",
			status: StatusStarting,
			value:  1,
		},
		{
			name:   "StatusRunning",
			status: StatusRunning,
			value:  2,
		},
		{
			name:   "StatusStopping",
			status: StatusStopping,
			value:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.status) != tt.value {
				t.Errorf("%s = %d, want %d", tt.name, int(tt.status), tt.value)
			}
		})
	}
}

func TestConvertUint64SliceToInt32(t *testing.T) {
	tests := []struct {
		name     string
		input    []uint64
		expected []int32
	}{
		{
			name:     "empty slice",
			input:    []uint64{},
			expected: []int32{},
		},
		{
			name:     "single element",
			input:    []uint64{42},
			expected: []int32{42},
		},
		{
			name:     "multiple elements",
			input:    []uint64{1, 2, 3, 4, 5},
			expected: []int32{1, 2, 3, 4, 5},
		},
		{
			name:     "large numbers",
			input:    []uint64{1000, 2000, 3000},
			expected: []int32{1000, 2000, 3000},
		},
		{
			name:     "zero values",
			input:    []uint64{0, 0, 0},
			expected: []int32{0, 0, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertUint64SliceToInt32(tt.input)

			if len(result) != len(tt.expected) {
				t.Fatalf("Length mismatch: got %d, want %d", len(result), len(tt.expected))
			}

			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("Element %d: got %d, want %d", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestConvertUint64SliceToInt32_Nil(t *testing.T) {
	result := convertUint64SliceToInt32(nil)

	if result == nil {
		t.Error("Expected non-nil result for nil input")
	}

	if len(result) != 0 {
		t.Errorf("Expected empty slice, got length %d", len(result))
	}
}

func TestBroker_UpdateTenantStorageUsage_NoTenancyManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := &Broker{
		ctx:            ctx,
		cancel:         cancel,
		logger:         newTestLogger(),
		tenancyManager: nil, // No tenancy manager
		status:         StatusRunning,
	}

	// Should not panic when tenancyManager is nil
	broker.updateTenantStorageUsage()
}

func TestBroker_UpdateTenantStorageUsage_WithTenants(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create tenancy manager
	tenancyMgr := tenancy.NewManager()

	// Create a test tenant with quotas
	quotas := &tenancy.Quotas{
		MaxTopics:       100,
		MaxPartitions:   1000,
		MaxStorageBytes: 1024 * 1024 * 1024, // 1GB
	}

	_, err := tenancyMgr.CreateTenant("test-tenant", "Test Tenant", quotas)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	broker := &Broker{
		ctx:            ctx,
		cancel:         cancel,
		logger:         newTestLogger(),
		tenancyManager: tenancyMgr,
		status:         StatusRunning,
	}

	// Should not panic and should iterate through tenants
	broker.updateTenantStorageUsage()

	// Verify tenant still exists (function should not delete it)
	retrievedTenant, err := tenancyMgr.GetTenant("test-tenant")
	if err != nil {
		t.Errorf("Tenant should still exist after updateTenantStorageUsage: %v", err)
	}

	if retrievedTenant == nil {
		t.Error("Tenant should not be nil")
	}
}

func TestBroker_UpdateTenantStorageUsage_EmptyTenantsList(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create tenancy manager with no tenants
	tenancyMgr := tenancy.NewManager()

	broker := &Broker{
		ctx:            ctx,
		cancel:         cancel,
		logger:         newTestLogger(),
		tenancyManager: tenancyMgr,
		status:         StatusRunning,
	}

	// Should not panic with empty tenant list
	broker.updateTenantStorageUsage()
}
func TestBroker_IsReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := &Broker{
		ctx:    ctx,
		cancel: cancel,
		logger: newTestLogger(),
		status: StatusStarting,
	}

	// Initially not ready (StatusStarting)
	if broker.IsReady() {
		t.Error("Broker should not be ready when status is StatusStarting")
	}

	// Set to running
	broker.mu.Lock()
	broker.status = StatusRunning
	broker.mu.Unlock()

	if !broker.IsReady() {
		t.Error("Broker should be ready when status is StatusRunning")
	}

	// Set to stopping
	broker.mu.Lock()
	broker.status = StatusStopping
	broker.mu.Unlock()

	if broker.IsReady() {
		t.Error("Broker should not be ready when status is StatusStopping")
	}
}

func TestBroker_IsAlive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := &Broker{
		ctx:    ctx,
		cancel: cancel,
		logger: newTestLogger(),
		status: StatusStarting,
	}

	// Alive in starting state
	if !broker.IsAlive() {
		t.Error("Broker should be alive when status is StatusStarting")
	}

	// Alive in running state
	broker.mu.Lock()
	broker.status = StatusRunning
	broker.mu.Unlock()

	if !broker.IsAlive() {
		t.Error("Broker should be alive when status is StatusRunning")
	}

	// Alive in stopping state
	broker.mu.Lock()
	broker.status = StatusStopping
	broker.mu.Unlock()

	if !broker.IsAlive() {
		t.Error("Broker should be alive when status is StatusStopping")
	}

	// Not alive when stopped
	broker.mu.Lock()
	broker.status = StatusStopped
	broker.mu.Unlock()

	if broker.IsAlive() {
		t.Error("Broker should not be alive when status is StatusStopped")
	}
}

func TestBroker_IsLeader_NoRaftNode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := &Broker{
		ctx:      ctx,
		cancel:   cancel,
		logger:   newTestLogger(),
		raftNode: nil,
	}

	// Should return false when raft node is nil
	if broker.IsLeader() {
		t.Error("Broker should not be leader when raft node is nil")
	}
}
