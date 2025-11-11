package chaos

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// FaultType represents different types of faults that can be injected
type FaultType string

const (
	FaultTypeLatency     FaultType = "latency"
	FaultTypeError       FaultType = "error"
	FaultTypeDisconnect  FaultType = "disconnect"
	FaultTypeDataLoss    FaultType = "data_loss"
	FaultTypePartition   FaultType = "partition"
	FaultTypeSlowResponse FaultType = "slow_response"
)

// FaultInjector manages fault injection for chaos testing
type FaultInjector struct {
	mu             sync.RWMutex
	enabled        bool
	faults         map[FaultType]*FaultConfig
	executionCount map[FaultType]int
	logger         Logger
}

// FaultConfig configures a specific fault type
type FaultConfig struct {
	Type        FaultType
	Probability float64       // 0.0 to 1.0
	Duration    time.Duration
	Delay       time.Duration // For latency injection
	ErrorMsg    string        // For error injection
	Enabled     bool
}

// Logger interface for fault injection logging
type Logger interface {
	Info(msg string, fields map[string]interface{})
	Warn(msg string, fields map[string]interface{})
	Error(msg string, fields map[string]interface{})
}

// NewFaultInjector creates a new fault injector
func NewFaultInjector(logger Logger) *FaultInjector {
	return &FaultInjector{
		enabled:        false,
		faults:         make(map[FaultType]*FaultConfig),
		executionCount: make(map[FaultType]int),
		logger:         logger,
	}
}

// Enable enables fault injection
func (fi *FaultInjector) Enable() {
	fi.mu.Lock()
	defer fi.mu.Unlock()
	fi.enabled = true
	fi.logger.Info("Fault injection enabled", nil)
}

// Disable disables fault injection
func (fi *FaultInjector) Disable() {
	fi.mu.Lock()
	defer fi.mu.Unlock()
	fi.enabled = false
	fi.logger.Info("Fault injection disabled", nil)
}

// AddFault adds or updates a fault configuration
func (fi *FaultInjector) AddFault(config *FaultConfig) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	fi.faults[config.Type] = config
	fi.logger.Info("Fault configured", map[string]interface{}{
		"type":        config.Type,
		"probability": config.Probability,
		"enabled":     config.Enabled,
	})
}

// RemoveFault removes a fault configuration
func (fi *FaultInjector) RemoveFault(faultType FaultType) {
	fi.mu.Lock()
	defer fi.mu.Unlock()
	delete(fi.faults, faultType)
}

// ShouldInjectFault determines if a fault should be injected based on probability
func (fi *FaultInjector) ShouldInjectFault(faultType FaultType) bool {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	if !fi.enabled {
		return false
	}

	config, exists := fi.faults[faultType]
	if !exists || !config.Enabled {
		return false
	}

	return rand.Float64() < config.Probability
}

// InjectLatency injects artificial latency
func (fi *FaultInjector) InjectLatency(ctx context.Context, operation string) error {
	if !fi.ShouldInjectFault(FaultTypeLatency) {
		return nil
	}

	fi.mu.Lock()
	config := fi.faults[FaultTypeLatency]
	fi.executionCount[FaultTypeLatency]++
	fi.mu.Unlock()

	fi.logger.Warn("Injecting latency fault", map[string]interface{}{
		"operation": operation,
		"delay":     config.Delay,
	})

	select {
	case <-time.After(config.Delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// InjectError injects an error response
func (fi *FaultInjector) InjectError(operation string) error {
	if !fi.ShouldInjectFault(FaultTypeError) {
		return nil
	}

	fi.mu.Lock()
	config := fi.faults[FaultTypeError]
	fi.executionCount[FaultTypeError]++
	fi.mu.Unlock()

	fi.logger.Warn("Injecting error fault", map[string]interface{}{
		"operation": operation,
		"error":     config.ErrorMsg,
	})

	return fmt.Errorf("injected fault: %s", config.ErrorMsg)
}

// InjectSlowResponse injects a slow response for a specific operation
func (fi *FaultInjector) InjectSlowResponse(ctx context.Context, operation string) error {
	if !fi.ShouldInjectFault(FaultTypeSlowResponse) {
		return nil
	}

	fi.mu.Lock()
	config := fi.faults[FaultTypeSlowResponse]
	fi.executionCount[FaultTypeSlowResponse]++
	fi.mu.Unlock()

	fi.logger.Warn("Injecting slow response fault", map[string]interface{}{
		"operation": operation,
		"delay":     config.Delay,
	})

	select {
	case <-time.After(config.Delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// GetStats returns fault injection statistics
func (fi *FaultInjector) GetStats() map[FaultType]int {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	stats := make(map[FaultType]int)
	for ft, count := range fi.executionCount {
		stats[ft] = count
	}
	return stats
}

// Reset resets fault injection statistics
func (fi *FaultInjector) Reset() {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	fi.executionCount = make(map[FaultType]int)
	fi.logger.Info("Fault injection stats reset", nil)
}

// NetworkPartitionSimulator simulates network partitions between nodes
type NetworkPartitionSimulator struct {
	mu         sync.RWMutex
	partitions map[string]map[string]bool // from -> to -> blocked
	logger     Logger
}

// NewNetworkPartitionSimulator creates a new partition simulator
func NewNetworkPartitionSimulator(logger Logger) *NetworkPartitionSimulator {
	return &NetworkPartitionSimulator{
		partitions: make(map[string]map[string]bool),
		logger:     logger,
	}
}

// PartitionNodes creates a network partition between two nodes
func (nps *NetworkPartitionSimulator) PartitionNodes(from, to string) {
	nps.mu.Lock()
	defer nps.mu.Unlock()

	if nps.partitions[from] == nil {
		nps.partitions[from] = make(map[string]bool)
	}
	nps.partitions[from][to] = true

	nps.logger.Warn("Network partition created", map[string]interface{}{
		"from": from,
		"to":   to,
	})
}

// HealPartition removes a network partition
func (nps *NetworkPartitionSimulator) HealPartition(from, to string) {
	nps.mu.Lock()
	defer nps.mu.Unlock()

	if nps.partitions[from] != nil {
		delete(nps.partitions[from], to)
	}

	nps.logger.Info("Network partition healed", map[string]interface{}{
		"from": from,
		"to":   to,
	})
}

// IsPartitioned checks if a path is partitioned
func (nps *NetworkPartitionSimulator) IsPartitioned(from, to string) bool {
	nps.mu.RLock()
	defer nps.mu.RUnlock()

	if nps.partitions[from] == nil {
		return false
	}
	return nps.partitions[from][to]
}

// HealAll removes all network partitions
func (nps *NetworkPartitionSimulator) HealAll() {
	nps.mu.Lock()
	defer nps.mu.Unlock()

	nps.partitions = make(map[string]map[string]bool)
	nps.logger.Info("All network partitions healed", nil)
}

// ChaosScenario represents a chaos testing scenario
type ChaosScenario struct {
	Name        string
	Description string
	Duration    time.Duration
	Faults      []*FaultConfig
	Injector    *FaultInjector
	logger      Logger
}

// NewChaosScenario creates a new chaos scenario
func NewChaosScenario(name, description string, duration time.Duration, logger Logger) *ChaosScenario {
	return &ChaosScenario{
		Name:        name,
		Description: description,
		Duration:    duration,
		Faults:      make([]*FaultConfig, 0),
		Injector:    NewFaultInjector(logger),
		logger:      logger,
	}
}

// AddFault adds a fault to the scenario
func (cs *ChaosScenario) AddFault(config *FaultConfig) {
	cs.Faults = append(cs.Faults, config)
}

// Run executes the chaos scenario
func (cs *ChaosScenario) Run(ctx context.Context, testFunc func(context.Context) error) error {
	cs.logger.Info("Starting chaos scenario", map[string]interface{}{
		"name":        cs.Name,
		"duration":    cs.Duration,
		"fault_count": len(cs.Faults),
	})

	// Configure faults
	for _, fault := range cs.Faults {
		cs.Injector.AddFault(fault)
	}

	// Enable injection
	cs.Injector.Enable()
	defer cs.Injector.Disable()

	// Create context with timeout
	testCtx, cancel := context.WithTimeout(ctx, cs.Duration)
	defer cancel()

	// Run test function
	err := testFunc(testCtx)

	// Collect stats
	stats := cs.Injector.GetStats()
	cs.logger.Info("Chaos scenario completed", map[string]interface{}{
		"name":        cs.Name,
		"stats":       stats,
		"test_error":  err != nil,
	})

	return err
}

// PredefinedScenarios

// NewRandomLatencyScenario creates a scenario with random latency injection
func NewRandomLatencyScenario(logger Logger) *ChaosScenario {
	scenario := NewChaosScenario(
		"Random Latency",
		"Randomly injects latency to test timeout handling",
		5*time.Minute,
		logger,
	)

	scenario.AddFault(&FaultConfig{
		Type:        FaultTypeLatency,
		Probability: 0.3, // 30% of operations
		Delay:       500 * time.Millisecond,
		Enabled:     true,
	})

	return scenario
}

// NewIntermittentErrorScenario creates a scenario with intermittent errors
func NewIntermittentErrorScenario(logger Logger) *ChaosScenario {
	scenario := NewChaosScenario(
		"Intermittent Errors",
		"Randomly injects errors to test error handling and retries",
		5*time.Minute,
		logger,
	)

	scenario.AddFault(&FaultConfig{
		Type:        FaultTypeError,
		Probability: 0.2, // 20% of operations
		ErrorMsg:    "chaos: simulated connection error",
		Enabled:     true,
	})

	return scenario
}

// NewSlowNetworkScenario creates a scenario simulating slow network
func NewSlowNetworkScenario(logger Logger) *ChaosScenario {
	scenario := NewChaosScenario(
		"Slow Network",
		"Simulates slow network conditions",
		5*time.Minute,
		logger,
	)

	scenario.AddFault(&FaultConfig{
		Type:        FaultTypeSlowResponse,
		Probability: 0.5, // 50% of operations
		Delay:       1 * time.Second,
		Enabled:     true,
	})

	return scenario
}

// NewCombinedChaosScenario creates a scenario with multiple fault types
func NewCombinedChaosScenario(logger Logger) *ChaosScenario {
	scenario := NewChaosScenario(
		"Combined Chaos",
		"Multiple fault types for comprehensive resilience testing",
		10*time.Minute,
		logger,
	)

	scenario.AddFault(&FaultConfig{
		Type:        FaultTypeLatency,
		Probability: 0.2,
		Delay:       300 * time.Millisecond,
		Enabled:     true,
	})

	scenario.AddFault(&FaultConfig{
		Type:        FaultTypeError,
		Probability: 0.1,
		ErrorMsg:    "chaos: random error",
		Enabled:     true,
	})

	scenario.AddFault(&FaultConfig{
		Type:        FaultTypeSlowResponse,
		Probability: 0.15,
		Delay:       800 * time.Millisecond,
		Enabled:     true,
	})

	return scenario
}
