package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BrokerStatus represents the current status of a broker
type BrokerStatus string

const (
	BrokerStatusStarting      BrokerStatus = "STARTING"      // Broker is starting up
	BrokerStatusAlive         BrokerStatus = "ALIVE"         // Broker is healthy and serving
	BrokerStatusDraining      BrokerStatus = "DRAINING"      // Broker is being drained (no new partitions)
	BrokerStatusDecommissioned BrokerStatus = "DECOMMISSIONED" // Broker has been removed
	BrokerStatusFailed        BrokerStatus = "FAILED"        // Broker has failed health checks
)

// BrokerMetadata contains metadata about a broker
type BrokerMetadata struct {
	ID       int32  // Unique broker ID
	Host     string // Hostname or IP
	Port     int    // Port number
	Rack     string // Rack ID for rack-aware placement
	Capacity int    // Max partitions this broker can handle

	// Status tracking
	Status           BrokerStatus
	RegisteredAt     time.Time
	LastHeartbeat    time.Time
	DecommissionedAt time.Time

	// Resource information
	DiskCapacityGB   int64 // Total disk capacity in GB
	DiskUsedGB       int64 // Current disk usage in GB
	NetworkBandwidth int64 // Network bandwidth in MB/s

	// Metadata
	Version string            // Broker version
	Tags    map[string]string // Custom tags for filtering
}

// Address returns the broker's network address
func (bm *BrokerMetadata) Address() string {
	return fmt.Sprintf("%s:%d", bm.Host, bm.Port)
}

// IsHealthy checks if the broker is in a healthy state
func (bm *BrokerMetadata) IsHealthy() bool {
	return bm.Status == BrokerStatusAlive || bm.Status == BrokerStatusStarting
}

// IsActive checks if the broker is actively serving partitions
func (bm *BrokerMetadata) IsActive() bool {
	return bm.Status == BrokerStatusAlive
}

// CanAcceptPartitions checks if broker can accept new partition assignments
func (bm *BrokerMetadata) CanAcceptPartitions() bool {
	return bm.Status == BrokerStatusAlive || bm.Status == BrokerStatusStarting
}

// DiskUtilization returns disk usage percentage
func (bm *BrokerMetadata) DiskUtilization() float64 {
	if bm.DiskCapacityGB == 0 {
		return 0
	}
	return float64(bm.DiskUsedGB) / float64(bm.DiskCapacityGB) * 100
}

// BrokerRegistry manages the cluster's broker membership
type BrokerRegistry struct {
	mu sync.RWMutex

	// Brokers indexed by ID
	brokers map[int32]*BrokerMetadata

	// Metadata client for persisting to Raft
	metadataClient MetadataStore

	// Configuration
	heartbeatTimeout time.Duration
	checkInterval    time.Duration

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Callbacks
	onBrokerAdded   func(broker *BrokerMetadata)
	onBrokerRemoved func(brokerID int32)
	onBrokerFailed  func(brokerID int32)
}

// MetadataStore defines operations for persisting broker metadata
type MetadataStore interface {
	// StoreBrokerMetadata persists broker metadata via Raft
	StoreBrokerMetadata(ctx context.Context, broker *BrokerMetadata) error

	// GetBrokerMetadata retrieves broker metadata
	GetBrokerMetadata(ctx context.Context, brokerID int32) (*BrokerMetadata, error)

	// ListBrokers lists all brokers
	ListBrokers(ctx context.Context) ([]*BrokerMetadata, error)

	// DeleteBroker removes broker metadata
	DeleteBroker(ctx context.Context, brokerID int32) error
}

// NewBrokerRegistry creates a new broker registry
func NewBrokerRegistry(metadataClient MetadataStore) *BrokerRegistry {
	ctx, cancel := context.WithCancel(context.Background())

	return &BrokerRegistry{
		brokers:          make(map[int32]*BrokerMetadata),
		metadataClient:   metadataClient,
		heartbeatTimeout: 30 * time.Second,
		checkInterval:    5 * time.Second,
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Start starts the broker registry
func (br *BrokerRegistry) Start() error {
	// Load existing brokers from metadata store
	if err := br.loadBrokers(); err != nil {
		return fmt.Errorf("failed to load brokers: %w", err)
	}

	// Start health monitoring
	br.wg.Add(1)
	go br.healthMonitorLoop()

	return nil
}

// Stop stops the broker registry
func (br *BrokerRegistry) Stop() error {
	br.cancel()
	br.wg.Wait()
	return nil
}

// RegisterBroker registers a new broker or updates existing broker
func (br *BrokerRegistry) RegisterBroker(ctx context.Context, broker *BrokerMetadata) error {
	br.mu.Lock()
	defer br.mu.Unlock()

	// Check if broker already exists
	existing, exists := br.brokers[broker.ID]
	if exists {
		// Update existing broker
		broker.RegisteredAt = existing.RegisteredAt
	} else {
		// New broker
		broker.RegisteredAt = time.Now()
		broker.Status = BrokerStatusStarting
	}

	broker.LastHeartbeat = time.Now()

	// Persist to metadata store
	if err := br.metadataClient.StoreBrokerMetadata(ctx, broker); err != nil {
		return fmt.Errorf("failed to persist broker metadata: %w", err)
	}

	// Update local registry
	br.brokers[broker.ID] = broker

	// Trigger callback if new broker
	if !exists && br.onBrokerAdded != nil {
		go br.onBrokerAdded(broker)
	}

	return nil
}

// DeregisterBroker marks a broker for decommissioning
func (br *BrokerRegistry) DeregisterBroker(ctx context.Context, brokerID int32) error {
	br.mu.Lock()
	defer br.mu.Unlock()

	broker, exists := br.brokers[brokerID]
	if !exists {
		return fmt.Errorf("broker %d not found", brokerID)
	}

	// Mark as decommissioned
	broker.Status = BrokerStatusDecommissioned
	broker.DecommissionedAt = time.Now()

	// Persist to metadata store
	if err := br.metadataClient.StoreBrokerMetadata(ctx, broker); err != nil {
		return fmt.Errorf("failed to persist broker decommission: %w", err)
	}

	// Trigger callback
	if br.onBrokerRemoved != nil {
		go br.onBrokerRemoved(brokerID)
	}

	return nil
}

// RecordHeartbeat records a heartbeat from a broker
func (br *BrokerRegistry) RecordHeartbeat(brokerID int32) error {
	br.mu.Lock()
	defer br.mu.Unlock()

	broker, exists := br.brokers[brokerID]
	if !exists {
		return fmt.Errorf("broker %d not registered", brokerID)
	}

	broker.LastHeartbeat = time.Now()

	// Update status if currently failed
	if broker.Status == BrokerStatusFailed || broker.Status == BrokerStatusStarting {
		broker.Status = BrokerStatusAlive
	}

	return nil
}

// GetBroker retrieves broker metadata
func (br *BrokerRegistry) GetBroker(brokerID int32) (*BrokerMetadata, error) {
	br.mu.RLock()
	defer br.mu.RUnlock()

	broker, exists := br.brokers[brokerID]
	if !exists {
		return nil, fmt.Errorf("broker %d not found", brokerID)
	}

	// Return a copy to prevent external modifications
	copy := *broker
	return &copy, nil
}

// ListBrokers returns all registered brokers
func (br *BrokerRegistry) ListBrokers() []*BrokerMetadata {
	br.mu.RLock()
	defer br.mu.RUnlock()

	brokers := make([]*BrokerMetadata, 0, len(br.brokers))
	for _, broker := range br.brokers {
		copy := *broker
		brokers = append(brokers, &copy)
	}

	return brokers
}

// ListActiveBrokers returns only active brokers
func (br *BrokerRegistry) ListActiveBrokers() []*BrokerMetadata {
	br.mu.RLock()
	defer br.mu.RUnlock()

	brokers := make([]*BrokerMetadata, 0)
	for _, broker := range br.brokers {
		if broker.IsActive() {
			copy := *broker
			brokers = append(brokers, &copy)
		}
	}

	return brokers
}

// GetBrokerCount returns the number of registered brokers
func (br *BrokerRegistry) GetBrokerCount() int {
	br.mu.RLock()
	defer br.mu.RUnlock()
	return len(br.brokers)
}

// GetActiveBrokerCount returns the number of active brokers
func (br *BrokerRegistry) GetActiveBrokerCount() int {
	br.mu.RLock()
	defer br.mu.RUnlock()

	count := 0
	for _, broker := range br.brokers {
		if broker.IsActive() {
			count++
		}
	}
	return count
}

// SetOnBrokerAdded sets callback for broker addition
func (br *BrokerRegistry) SetOnBrokerAdded(callback func(*BrokerMetadata)) {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.onBrokerAdded = callback
}

// SetOnBrokerRemoved sets callback for broker removal
func (br *BrokerRegistry) SetOnBrokerRemoved(callback func(int32)) {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.onBrokerRemoved = callback
}

// SetOnBrokerFailed sets callback for broker failure
func (br *BrokerRegistry) SetOnBrokerFailed(callback func(int32)) {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.onBrokerFailed = callback
}

// loadBrokers loads brokers from metadata store on startup
func (br *BrokerRegistry) loadBrokers() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	brokers, err := br.metadataClient.ListBrokers(ctx)
	if err != nil {
		return err
	}

	br.mu.Lock()
	defer br.mu.Unlock()

	for _, broker := range brokers {
		br.brokers[broker.ID] = broker
	}

	return nil
}

// healthMonitorLoop periodically checks broker health
func (br *BrokerRegistry) healthMonitorLoop() {
	defer br.wg.Done()

	ticker := time.NewTicker(br.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-br.ctx.Done():
			return
		case <-ticker.C:
			br.checkBrokerHealth()
		}
	}
}

// checkBrokerHealth checks health of all brokers
func (br *BrokerRegistry) checkBrokerHealth() {
	br.mu.Lock()
	defer br.mu.Unlock()

	now := time.Now()

	for _, broker := range br.brokers {
		// Skip decommissioned brokers
		if broker.Status == BrokerStatusDecommissioned {
			continue
		}

		// Check if heartbeat timeout exceeded
		if now.Sub(broker.LastHeartbeat) > br.heartbeatTimeout {
			if broker.Status != BrokerStatusFailed {
				broker.Status = BrokerStatusFailed

				// Trigger failure callback
				if br.onBrokerFailed != nil {
					go br.onBrokerFailed(broker.ID)
				}
			}
		}
	}
}

// GetBrokerStats returns cluster-wide broker statistics
func (br *BrokerRegistry) GetBrokerStats() BrokerStats {
	br.mu.RLock()
	defer br.mu.RUnlock()

	stats := BrokerStats{}

	for _, broker := range br.brokers {
		stats.TotalBrokers++

		switch broker.Status {
		case BrokerStatusAlive:
			stats.AliveBrokers++
		case BrokerStatusStarting:
			stats.StartingBrokers++
		case BrokerStatusFailed:
			stats.FailedBrokers++
		case BrokerStatusDraining:
			stats.DrainingBrokers++
		case BrokerStatusDecommissioned:
			stats.DecommissionedBrokers++
		}

		stats.TotalDiskCapacityGB += broker.DiskCapacityGB
		stats.TotalDiskUsedGB += broker.DiskUsedGB
	}

	if stats.TotalDiskCapacityGB > 0 {
		stats.AvgDiskUtilization = float64(stats.TotalDiskUsedGB) / float64(stats.TotalDiskCapacityGB) * 100
	}

	return stats
}

// BrokerStats contains cluster-wide broker statistics
type BrokerStats struct {
	TotalBrokers          int
	AliveBrokers          int
	StartingBrokers       int
	FailedBrokers         int
	DrainingBrokers       int
	DecommissionedBrokers int

	TotalDiskCapacityGB int64
	TotalDiskUsedGB     int64
	AvgDiskUtilization  float64
}
