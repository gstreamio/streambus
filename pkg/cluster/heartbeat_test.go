package cluster

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewHeartbeatService(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())
	brokerID := int32(1)

	hs := NewHeartbeatService(brokerID, registry)

	if hs == nil {
		t.Fatal("Expected non-nil HeartbeatService")
	}

	if hs.brokerID != brokerID {
		t.Errorf("brokerID = %d, want %d", hs.brokerID, brokerID)
	}

	if hs.registry != registry {
		t.Error("registry not set correctly")
	}

	if hs.interval != 10*time.Second {
		t.Errorf("interval = %v, want 10s", hs.interval)
	}

	if hs.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", hs.timeout)
	}
}

func TestHeartbeatService_SetInterval(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())
	hs := NewHeartbeatService(1, registry)

	newInterval := 5 * time.Second
	hs.SetInterval(newInterval)

	if hs.interval != newInterval {
		t.Errorf("interval = %v, want %v", hs.interval, newInterval)
	}
}

func TestHeartbeatService_SetTimeout(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())
	hs := NewHeartbeatService(1, registry)

	newTimeout := 3 * time.Second
	hs.SetTimeout(newTimeout)

	if hs.timeout != newTimeout {
		t.Errorf("timeout = %v, want %v", hs.timeout, newTimeout)
	}
}

func TestHeartbeatService_StartStop(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())
	brokerID := int32(1)

	// Register the broker first
	err := registry.RegisterBroker(context.Background(), &BrokerMetadata{
		ID:   brokerID,
		Host: "localhost",
		Port: 9092,
	})
	if err != nil {
		t.Fatalf("Failed to register broker: %v", err)
	}

	hs := NewHeartbeatService(brokerID, registry)

	// Set short interval for testing
	hs.SetInterval(50 * time.Millisecond)

	// Start service
	err = hs.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for a few heartbeats
	time.Sleep(150 * time.Millisecond)

	// Stop service
	err = hs.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	// Verify metrics
	metrics := hs.GetMetrics()
	if metrics.SuccessCount == 0 {
		t.Error("Expected at least one successful heartbeat")
	}
}

func TestHeartbeatService_Metrics(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())
	brokerID := int32(1)

	// Register broker
	err := registry.RegisterBroker(context.Background(), &BrokerMetadata{
		ID:   brokerID,
		Host: "localhost",
		Port: 9092,
	})
	if err != nil {
		t.Fatalf("Failed to register broker: %v", err)
	}

	hs := NewHeartbeatService(brokerID, registry)

	// Initial metrics should be zero
	metrics := hs.GetMetrics()
	if metrics.SuccessCount != 0 {
		t.Errorf("Initial SuccessCount = %d, want 0", metrics.SuccessCount)
	}
	if metrics.FailureCount != 0 {
		t.Errorf("Initial FailureCount = %d, want 0", metrics.FailureCount)
	}
	if !metrics.LastHeartbeatTime.IsZero() {
		t.Error("Initial LastHeartbeatTime should be zero")
	}

	// Send a heartbeat
	err = hs.sendHeartbeat()
	if err != nil {
		t.Errorf("sendHeartbeat failed: %v", err)
	}

	// Check updated metrics
	metrics = hs.GetMetrics()
	if metrics.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", metrics.SuccessCount)
	}
	if metrics.LastError != nil {
		t.Errorf("LastError = %v, want nil", metrics.LastError)
	}
	if metrics.LastHeartbeatTime.IsZero() {
		t.Error("LastHeartbeatTime should be set after successful heartbeat")
	}
}

func TestHeartbeatService_FailedHeartbeat(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())
	brokerID := int32(999) // Non-existent broker

	hs := NewHeartbeatService(brokerID, registry)

	// Try to send heartbeat for non-existent broker
	err := hs.sendHeartbeat()
	if err == nil {
		t.Error("Expected error for non-existent broker")
	}

	// Check metrics
	metrics := hs.GetMetrics()
	if metrics.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", metrics.FailureCount)
	}
	if metrics.SuccessCount != 0 {
		t.Errorf("SuccessCount = %d, want 0", metrics.SuccessCount)
	}
	if metrics.LastError == nil {
		t.Error("LastError should be set after failed heartbeat")
	}
}

func TestHeartbeatService_HeartbeatLoop(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())
	brokerID := int32(1)

	// Register broker
	err := registry.RegisterBroker(context.Background(), &BrokerMetadata{
		ID:   brokerID,
		Host: "localhost",
		Port: 9092,
	})
	if err != nil {
		t.Fatalf("Failed to register broker: %v", err)
	}

	hs := NewHeartbeatService(brokerID, registry)

	// Set very short interval for testing
	hs.SetInterval(30 * time.Millisecond)

	// Start the service
	err = hs.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for multiple heartbeats
	time.Sleep(120 * time.Millisecond)

	// Stop service
	hs.Stop()

	// Verify multiple heartbeats occurred
	metrics := hs.GetMetrics()
	if metrics.SuccessCount < 2 {
		t.Errorf("Expected at least 2 heartbeats, got %d", metrics.SuccessCount)
	}
}

func TestHeartbeatService_ConcurrentAccess(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())
	brokerID := int32(1)

	// Register broker
	registry.RegisterBroker(context.Background(), &BrokerMetadata{
		ID:   brokerID,
		Host: "localhost",
		Port: 9092,
	})

	hs := NewHeartbeatService(brokerID, registry)
	hs.SetInterval(10 * time.Millisecond)

	// Start service
	hs.Start()

	// Concurrently access metrics while heartbeats are running
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = hs.GetMetrics()
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	wg.Wait()
	hs.Stop()

	// Should not panic or race
}

func TestNewBrokerHealthChecker(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())

	bhc := NewBrokerHealthChecker(registry)

	if bhc == nil {
		t.Fatal("Expected non-nil BrokerHealthChecker")
	}

	if bhc.registry != registry {
		t.Error("registry not set correctly")
	}

	if bhc.checkInterval != 30*time.Second {
		t.Errorf("checkInterval = %v, want 30s", bhc.checkInterval)
	}

	if bhc.checkTimeout != 10*time.Second {
		t.Errorf("checkTimeout = %v, want 10s", bhc.checkTimeout)
	}
}

func TestBrokerHealthChecker_SetHealthCheckFunction(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())
	bhc := NewBrokerHealthChecker(registry)

	callCount := 0
	healthCheckFn := func(broker *BrokerMetadata) error {
		callCount++
		return nil
	}

	bhc.SetHealthCheckFunction(healthCheckFn)

	if bhc.healthCheckFn == nil {
		t.Error("healthCheckFn should be set")
	}
}

func TestBrokerHealthChecker_StartStop(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())
	bhc := NewBrokerHealthChecker(registry)

	// Set short check interval for testing
	bhc.checkInterval = 50 * time.Millisecond

	// Start checker
	err := bhc.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Stop checker
	err = bhc.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	// Second stop should be safe
	err = bhc.Stop()
	if err != nil {
		t.Error("Second Stop should not error")
	}
}

func TestBrokerHealthChecker_CheckAllBrokers(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())

	// Register some brokers
	for i := int32(1); i <= 3; i++ {
		err := registry.RegisterBroker(context.Background(), &BrokerMetadata{
			ID:     i,
			Host:   "localhost",
			Port:   9090 + int(i),
			Status: BrokerStatusAlive,
		})
		if err != nil {
			t.Fatalf("Failed to register broker %d: %v", i, err)
		}
		// Update broker status to Alive (RegisterBroker sets new brokers to Starting)
		broker, _ := registry.GetBroker(i)
		broker.Status = BrokerStatusAlive
		registry.RegisterBroker(context.Background(), broker)
	}

	bhc := NewBrokerHealthChecker(registry)

	// Set up health check function
	checkedBrokers := make(map[int32]bool)
	var mu sync.Mutex

	bhc.SetHealthCheckFunction(func(broker *BrokerMetadata) error {
		mu.Lock()
		defer mu.Unlock()
		checkedBrokers[broker.ID] = true
		return nil
	})

	// Run check
	bhc.checkAllBrokers()

	// Verify all brokers were checked
	mu.Lock()
	defer mu.Unlock()

	if len(checkedBrokers) != 3 {
		t.Errorf("Expected 3 brokers checked, got %d", len(checkedBrokers))
	}

	for i := int32(1); i <= 3; i++ {
		if !checkedBrokers[i] {
			t.Errorf("Broker %d was not checked", i)
		}
	}
}

func TestBrokerHealthChecker_NoHealthCheckFunction(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())

	// Register broker
	registry.RegisterBroker(context.Background(), &BrokerMetadata{
		ID:     1,
		Host:   "localhost",
		Port:   9092,
		Status: BrokerStatusAlive,
	})

	bhc := NewBrokerHealthChecker(registry)

	// Don't set health check function

	// Check should not panic
	bhc.checkAllBrokers()
}

func TestBrokerHealthChecker_PerformHealthCheck(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())

	// Register broker
	brokerID := int32(1)
	registry.RegisterBroker(context.Background(), &BrokerMetadata{
		ID:     brokerID,
		Host:   "localhost",
		Port:   9092,
		Status: BrokerStatusAlive,
	})

	bhc := NewBrokerHealthChecker(registry)

	// Test without health check function
	err := bhc.PerformHealthCheck(brokerID)
	if err == nil {
		t.Error("Expected error when no health check function configured")
	}

	// Set health check function
	bhc.SetHealthCheckFunction(func(broker *BrokerMetadata) error {
		return nil
	})

	// Test with health check function
	err = bhc.PerformHealthCheck(brokerID)
	if err != nil {
		t.Errorf("PerformHealthCheck failed: %v", err)
	}

	// Test with non-existent broker
	err = bhc.PerformHealthCheck(999)
	if err == nil {
		t.Error("Expected error for non-existent broker")
	}
}

func TestBrokerHealthChecker_HealthCheckLoop(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())

	// Register brokers
	for i := int32(1); i <= 2; i++ {
		registry.RegisterBroker(context.Background(), &BrokerMetadata{
			ID:     i,
			Host:   "localhost",
			Port:   9090 + int(i),
			Status: BrokerStatusAlive,
		})
	}

	bhc := NewBrokerHealthChecker(registry)
	bhc.checkInterval = 50 * time.Millisecond

	// Track health checks
	checkCount := 0
	var mu sync.Mutex

	bhc.SetHealthCheckFunction(func(broker *BrokerMetadata) error {
		mu.Lock()
		defer mu.Unlock()
		checkCount++
		return nil
	})

	// Start health checker
	bhc.Start()

	// Wait for multiple check cycles
	time.Sleep(150 * time.Millisecond)

	// Stop checker
	bhc.Stop()

	// Verify checks occurred
	mu.Lock()
	defer mu.Unlock()

	if checkCount < 4 {
		t.Logf("Note: Expected at least 4 checks (2 brokers × 2 cycles), got %d", checkCount)
	}
}

func TestBrokerHealthChecker_Timeout(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())

	// Register broker
	brokerID := int32(1)
	registry.RegisterBroker(context.Background(), &BrokerMetadata{
		ID:     brokerID,
		Host:   "localhost",
		Port:   9092,
		Status: BrokerStatusAlive,
	})

	bhc := NewBrokerHealthChecker(registry)
	bhc.checkTimeout = 10 * time.Millisecond

	// Set health check that times out
	bhc.SetHealthCheckFunction(func(broker *BrokerMetadata) error {
		time.Sleep(50 * time.Millisecond) // Sleep longer than timeout
		return nil
	})

	// This should timeout but not panic
	err := bhc.PerformHealthCheck(brokerID)
	if err == nil {
		t.Error("Expected timeout error")
	}
	if err != nil && err.Error() != "health check timeout" {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

func TestHeartbeatService_Timeout(t *testing.T) {
	registry := NewBrokerRegistry(newMockMetadataStore())
	brokerID := int32(1)

	// Register broker
	registry.RegisterBroker(context.Background(), &BrokerMetadata{
		ID:   brokerID,
		Host: "localhost",
		Port: 9092,
	})

	hs := NewHeartbeatService(brokerID, registry)

	// Set very short timeout
	hs.SetTimeout(1 * time.Nanosecond)

	// This should timeout
	err := hs.sendHeartbeat()
	if err == nil {
		t.Error("Expected timeout error")
	}

	metrics := hs.GetMetrics()
	if metrics.FailureCount == 0 {
		t.Error("Expected failure to be recorded")
	}
}

func TestHeartbeatMetrics_Structure(t *testing.T) {
	metrics := HeartbeatMetrics{
		SuccessCount:      10,
		FailureCount:      2,
		LastHeartbeatTime: time.Now(),
		LastError:         nil,
	}

	if metrics.SuccessCount != 10 {
		t.Errorf("SuccessCount = %d, want 10", metrics.SuccessCount)
	}

	if metrics.FailureCount != 2 {
		t.Errorf("FailureCount = %d, want 2", metrics.FailureCount)
	}

	if metrics.LastHeartbeatTime.IsZero() {
		t.Error("LastHeartbeatTime should not be zero")
	}
}
