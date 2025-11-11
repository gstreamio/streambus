package tenancy

import (
	"fmt"
	"sync"
	"time"
)

// TenantID is a unique identifier for a tenant
type TenantID string

// Tenant represents a multi-tenant namespace in StreamBus
type Tenant struct {
	ID          TenantID
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// Quotas define resource limits for this tenant
	Quotas *Quotas

	// Metadata for custom tenant properties
	Metadata map[string]string

	// Status indicates if tenant is active, suspended, etc.
	Status TenantStatus
}

// TenantStatus represents the operational status of a tenant
type TenantStatus string

const (
	TenantStatusActive    TenantStatus = "ACTIVE"
	TenantStatusSuspended TenantStatus = "SUSPENDED"
	TenantStatusDeleted   TenantStatus = "DELETED"
)

// Quotas define resource limits for a tenant
type Quotas struct {
	// Throughput quotas (per second)
	MaxBytesPerSecond    int64 // Maximum bytes/sec for produce + consume
	MaxMessagesPerSecond int64 // Maximum messages/sec for produce + consume

	// Storage quotas
	MaxStorageBytes int64 // Maximum total storage in bytes
	MaxTopics       int   // Maximum number of topics
	MaxPartitions   int   // Maximum total partitions across all topics

	// Connection quotas
	MaxConnections      int // Maximum concurrent connections
	MaxProducers        int // Maximum concurrent producers
	MaxConsumers        int // Maximum concurrent consumers
	MaxConsumerGroups   int // Maximum consumer groups

	// Request quotas
	MaxRequestsPerSecond int64 // Maximum requests/sec

	// Advanced quotas
	MaxRetentionHours int64 // Maximum message retention in hours
}

// DefaultQuotas returns sensible default quotas for a new tenant
func DefaultQuotas() *Quotas {
	return &Quotas{
		MaxBytesPerSecond:    10 * 1024 * 1024,    // 10 MB/s
		MaxMessagesPerSecond: 10000,                // 10k msgs/s
		MaxStorageBytes:      100 * 1024 * 1024 * 1024, // 100 GB
		MaxTopics:            100,
		MaxPartitions:        1000,
		MaxConnections:       1000,
		MaxProducers:         500,
		MaxConsumers:         500,
		MaxConsumerGroups:    50,
		MaxRequestsPerSecond: 10000,
		MaxRetentionHours:    168, // 7 days
	}
}

// UnlimitedQuotas returns quotas with no limits (for testing or admin tenants)
func UnlimitedQuotas() *Quotas {
	return &Quotas{
		MaxBytesPerSecond:    -1,
		MaxMessagesPerSecond: -1,
		MaxStorageBytes:      -1,
		MaxTopics:            -1,
		MaxPartitions:        -1,
		MaxConnections:       -1,
		MaxProducers:         -1,
		MaxConsumers:         -1,
		MaxConsumerGroups:    -1,
		MaxRequestsPerSecond: -1,
		MaxRetentionHours:    -1,
	}
}

// TenantStore manages tenants in the system
type TenantStore struct {
	mu      sync.RWMutex
	tenants map[TenantID]*Tenant
}

// NewTenantStore creates a new tenant store
func NewTenantStore() *TenantStore {
	return &TenantStore{
		tenants: make(map[TenantID]*Tenant),
	}
}

// CreateTenant creates a new tenant
func (s *TenantStore) CreateTenant(id TenantID, name string, quotas *Quotas) (*Tenant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if tenant already exists
	if _, exists := s.tenants[id]; exists {
		return nil, fmt.Errorf("tenant %s already exists", id)
	}

	// Use default quotas if none provided
	if quotas == nil {
		quotas = DefaultQuotas()
	}

	tenant := &Tenant{
		ID:        id,
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Quotas:    quotas,
		Metadata:  make(map[string]string),
		Status:    TenantStatusActive,
	}

	s.tenants[id] = tenant
	return tenant, nil
}

// GetTenant retrieves a tenant by ID
func (s *TenantStore) GetTenant(id TenantID) (*Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenant, exists := s.tenants[id]
	if !exists {
		return nil, fmt.Errorf("tenant %s not found", id)
	}

	return tenant, nil
}

// UpdateTenant updates a tenant's configuration
func (s *TenantStore) UpdateTenant(id TenantID, name string, quotas *Quotas) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tenant, exists := s.tenants[id]
	if !exists {
		return fmt.Errorf("tenant %s not found", id)
	}

	if name != "" {
		tenant.Name = name
	}

	if quotas != nil {
		tenant.Quotas = quotas
	}

	tenant.UpdatedAt = time.Now()
	return nil
}

// DeleteTenant deletes a tenant
func (s *TenantStore) DeleteTenant(id TenantID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tenant, exists := s.tenants[id]
	if !exists {
		return fmt.Errorf("tenant %s not found", id)
	}

	tenant.Status = TenantStatusDeleted
	tenant.UpdatedAt = time.Now()

	// Actually remove from map
	delete(s.tenants, id)
	return nil
}

// ListTenants returns all tenants
func (s *TenantStore) ListTenants() []*Tenant {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tenants := make([]*Tenant, 0, len(s.tenants))
	for _, tenant := range s.tenants {
		tenants = append(tenants, tenant)
	}

	return tenants
}

// SuspendTenant suspends a tenant
func (s *TenantStore) SuspendTenant(id TenantID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tenant, exists := s.tenants[id]
	if !exists {
		return fmt.Errorf("tenant %s not found", id)
	}

	tenant.Status = TenantStatusSuspended
	tenant.UpdatedAt = time.Now()
	return nil
}

// ActivateTenant activates a suspended tenant
func (s *TenantStore) ActivateTenant(id TenantID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tenant, exists := s.tenants[id]
	if !exists {
		return fmt.Errorf("tenant %s not found", id)
	}

	tenant.Status = TenantStatusActive
	tenant.UpdatedAt = time.Now()
	return nil
}

// IsActive checks if a tenant is active
func (t *Tenant) IsActive() bool {
	return t.Status == TenantStatusActive
}

// Validate validates the tenant configuration
func (t *Tenant) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("tenant ID cannot be empty")
	}

	if t.Name == "" {
		return fmt.Errorf("tenant name cannot be empty")
	}

	if t.Quotas == nil {
		return fmt.Errorf("tenant quotas cannot be nil")
	}

	return nil
}
