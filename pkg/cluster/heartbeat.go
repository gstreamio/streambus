package cluster

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// HeartbeatService manages sending heartbeats from a broker
type HeartbeatService struct {
	brokerID int32
	registry *BrokerRegistry

	// Configuration
	interval time.Duration
	timeout  time.Duration

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	mu                sync.RWMutex
	successCount      int64
	failureCount      int64
	lastHeartbeatTime time.Time
	lastError         error
}

// NewHeartbeatService creates a new heartbeat service
func NewHeartbeatService(brokerID int32, registry *BrokerRegistry) *HeartbeatService {
	ctx, cancel := context.WithCancel(context.Background())

	return &HeartbeatService{
		brokerID: brokerID,
		registry: registry,
		interval: 10 * time.Second, // Send heartbeat every 10 seconds
		timeout:  5 * time.Second,  // Heartbeat timeout
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start starts the heartbeat service
func (hs *HeartbeatService) Start() error {
	// Send initial heartbeat
	if err := hs.sendHeartbeat(); err != nil {
		log.Printf("[Heartbeat] Initial heartbeat failed: %v", err)
	}

	// Start heartbeat loop
	hs.wg.Add(1)
	go hs.heartbeatLoop()

	return nil
}

// Stop stops the heartbeat service
func (hs *HeartbeatService) Stop() error {
	hs.cancel()
	hs.wg.Wait()
	return nil
}

// SetInterval sets the heartbeat interval
func (hs *HeartbeatService) SetInterval(interval time.Duration) {
	hs.interval = interval
}

// SetTimeout sets the heartbeat timeout
func (hs *HeartbeatService) SetTimeout(timeout time.Duration) {
	hs.timeout = timeout
}

// heartbeatLoop periodically sends heartbeats
func (hs *HeartbeatService) heartbeatLoop() {
	defer hs.wg.Done()

	ticker := time.NewTicker(hs.interval)
	defer ticker.Stop()

	for {
		select {
		case <-hs.ctx.Done():
			return
		case <-ticker.C:
			if err := hs.sendHeartbeat(); err != nil {
				log.Printf("[Heartbeat %d] Failed to send heartbeat: %v", hs.brokerID, err)
			}
		}
	}
}

// sendHeartbeat sends a single heartbeat
func (hs *HeartbeatService) sendHeartbeat() error {
	ctx, cancel := context.WithTimeout(hs.ctx, hs.timeout)
	defer cancel()

	// Send heartbeat with timeout
	errChan := make(chan error, 1)
	go func() {
		errChan <- hs.registry.RecordHeartbeat(hs.brokerID)
	}()

	select {
	case err := <-errChan:
		hs.mu.Lock()
		defer hs.mu.Unlock()

		if err != nil {
			hs.failureCount++
			hs.lastError = err
			return err
		}

		hs.successCount++
		hs.lastHeartbeatTime = time.Now()
		hs.lastError = nil
		return nil

	case <-ctx.Done():
		hs.mu.Lock()
		defer hs.mu.Unlock()

		hs.failureCount++
		err := fmt.Errorf("heartbeat timeout after %v", hs.timeout)
		hs.lastError = err
		return err
	}
}

// GetMetrics returns heartbeat metrics
func (hs *HeartbeatService) GetMetrics() HeartbeatMetrics {
	hs.mu.RLock()
	defer hs.mu.RUnlock()

	return HeartbeatMetrics{
		SuccessCount:      hs.successCount,
		FailureCount:      hs.failureCount,
		LastHeartbeatTime: hs.lastHeartbeatTime,
		LastError:         hs.lastError,
	}
}

// HeartbeatMetrics contains heartbeat statistics
type HeartbeatMetrics struct {
	SuccessCount      int64
	FailureCount      int64
	LastHeartbeatTime time.Time
	LastError         error
}

// BrokerHealthChecker performs active health checks on brokers
type BrokerHealthChecker struct {
	registry *BrokerRegistry

	// Configuration
	checkInterval time.Duration
	checkTimeout  time.Duration

	// Health check function
	healthCheckFn func(broker *BrokerMetadata) error

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewBrokerHealthChecker creates a new health checker
func NewBrokerHealthChecker(registry *BrokerRegistry) *BrokerHealthChecker {
	ctx, cancel := context.WithCancel(context.Background())

	return &BrokerHealthChecker{
		registry:      registry,
		checkInterval: 30 * time.Second,
		checkTimeout:  10 * time.Second,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// SetHealthCheckFunction sets the custom health check function
func (bhc *BrokerHealthChecker) SetHealthCheckFunction(fn func(*BrokerMetadata) error) {
	bhc.healthCheckFn = fn
}

// Start starts the health checker
func (bhc *BrokerHealthChecker) Start() error {
	bhc.wg.Add(1)
	go bhc.healthCheckLoop()
	return nil
}

// Stop stops the health checker
func (bhc *BrokerHealthChecker) Stop() error {
	bhc.cancel()
	bhc.wg.Wait()
	return nil
}

// healthCheckLoop periodically checks broker health
func (bhc *BrokerHealthChecker) healthCheckLoop() {
	defer bhc.wg.Done()

	ticker := time.NewTicker(bhc.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-bhc.ctx.Done():
			return
		case <-ticker.C:
			bhc.checkAllBrokers()
		}
	}
}

// checkAllBrokers checks health of all active brokers
func (bhc *BrokerHealthChecker) checkAllBrokers() {
	brokers := bhc.registry.ListActiveBrokers()

	for _, broker := range brokers {
		// Skip if no health check function configured
		if bhc.healthCheckFn == nil {
			continue
		}

		// Perform health check with timeout
		ctx, cancel := context.WithTimeout(bhc.ctx, bhc.checkTimeout)

		errChan := make(chan error, 1)
		go func() {
			errChan <- bhc.healthCheckFn(broker)
		}()

		select {
		case err := <-errChan:
			if err != nil {
				log.Printf("[HealthChecker] Broker %d health check failed: %v", broker.ID, err)
			}
		case <-ctx.Done():
			log.Printf("[HealthChecker] Broker %d health check timeout", broker.ID)
		}

		cancel()
	}
}

// PerformHealthCheck performs a single health check on a broker
func (bhc *BrokerHealthChecker) PerformHealthCheck(brokerID int32) error {
	broker, err := bhc.registry.GetBroker(brokerID)
	if err != nil {
		return err
	}

	if bhc.healthCheckFn == nil {
		return fmt.Errorf("no health check function configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), bhc.checkTimeout)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- bhc.healthCheckFn(broker)
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return fmt.Errorf("health check timeout")
	}
}
