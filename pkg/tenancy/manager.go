package tenancy

import (
	"fmt"
	"sync"
)

// Manager manages all tenants and their quota tracking
type Manager struct {
	store    *TenantStore
	trackers map[TenantID]*QuotaTracker
	mu       sync.RWMutex
}

// NewManager creates a new tenant manager
func NewManager() *Manager {
	return &Manager{
		store:    NewTenantStore(),
		trackers: make(map[TenantID]*QuotaTracker),
	}
}

// CreateTenant creates a new tenant with specified quotas
func (m *Manager) CreateTenant(id TenantID, name string, quotas *Quotas) (*Tenant, error) {
	// Create tenant in store
	tenant, err := m.store.CreateTenant(id, name, quotas)
	if err != nil {
		return nil, err
	}

	// Create quota tracker
	m.mu.Lock()
	m.trackers[id] = NewQuotaTracker(id, tenant.Quotas)
	m.mu.Unlock()

	return tenant, nil
}

// GetTenant retrieves a tenant
func (m *Manager) GetTenant(id TenantID) (*Tenant, error) {
	return m.store.GetTenant(id)
}

// UpdateTenant updates a tenant's configuration
func (m *Manager) UpdateTenant(id TenantID, name string, quotas *Quotas) error {
	err := m.store.UpdateTenant(id, name, quotas)
	if err != nil {
		return err
	}

	// Update tracker with new quotas if provided
	if quotas != nil {
		m.mu.Lock()
		if tracker, exists := m.trackers[id]; exists {
			tracker.quotas = quotas
		}
		m.mu.Unlock()
	}

	return nil
}

// DeleteTenant deletes a tenant
func (m *Manager) DeleteTenant(id TenantID) error {
	err := m.store.DeleteTenant(id)
	if err != nil {
		return err
	}

	// Remove tracker
	m.mu.Lock()
	delete(m.trackers, id)
	m.mu.Unlock()

	return nil
}

// ListTenants returns all tenants
func (m *Manager) ListTenants() []*Tenant {
	return m.store.ListTenants()
}

// SuspendTenant suspends a tenant
func (m *Manager) SuspendTenant(id TenantID) error {
	return m.store.SuspendTenant(id)
}

// ActivateTenant activates a tenant
func (m *Manager) ActivateTenant(id TenantID) error {
	return m.store.ActivateTenant(id)
}

// GetTracker gets the quota tracker for a tenant
func (m *Manager) GetTracker(id TenantID) (*QuotaTracker, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tracker, exists := m.trackers[id]
	if !exists {
		return nil, fmt.Errorf("no tracker found for tenant %s", id)
	}

	return tracker, nil
}

// CheckTenantActive checks if a tenant exists and is active
func (m *Manager) CheckTenantActive(id TenantID) error {
	tenant, err := m.store.GetTenant(id)
	if err != nil {
		return err
	}

	if !tenant.IsActive() {
		return fmt.Errorf("tenant %s is not active (status: %s)", id, tenant.Status)
	}

	return nil
}

// EnforceProduceQuota checks and records produce operation
func (m *Manager) EnforceProduceQuota(id TenantID, bytes, messages int64) error {
	// Check tenant is active
	if err := m.CheckTenantActive(id); err != nil {
		return err
	}

	// Get tracker
	tracker, err := m.GetTracker(id)
	if err != nil {
		return err
	}

	// Check throughput quota
	if err := tracker.CheckThroughput(bytes, messages); err != nil {
		return err
	}

	// Record usage
	tracker.RecordThroughput(bytes, messages)
	return nil
}

// EnforceConsumeQuota checks and records consume operation
func (m *Manager) EnforceConsumeQuota(id TenantID, bytes, messages int64) error {
	// Check tenant is active
	if err := m.CheckTenantActive(id); err != nil {
		return err
	}

	// Get tracker
	tracker, err := m.GetTracker(id)
	if err != nil {
		return err
	}

	// Check throughput quota
	if err := tracker.CheckThroughput(bytes, messages); err != nil {
		return err
	}

	// Record usage
	tracker.RecordThroughput(bytes, messages)
	return nil
}

// EnforceConnectionQuota checks if a new connection can be created
func (m *Manager) EnforceConnectionQuota(id TenantID) error {
	// Check tenant is active
	if err := m.CheckTenantActive(id); err != nil {
		return err
	}

	// Get tracker
	tracker, err := m.GetTracker(id)
	if err != nil {
		return err
	}

	// Add connection
	return tracker.AddConnection()
}

// ReleaseConnection releases a connection quota
func (m *Manager) ReleaseConnection(id TenantID) {
	tracker, err := m.GetTracker(id)
	if err != nil {
		return
	}

	tracker.RemoveConnection()
}

// EnforceTopicQuota checks if a new topic can be created
func (m *Manager) EnforceTopicQuota(id TenantID, partitions int) error {
	// Check tenant is active
	if err := m.CheckTenantActive(id); err != nil {
		return err
	}

	// Get tracker
	tracker, err := m.GetTracker(id)
	if err != nil {
		return err
	}

	// Add topic
	return tracker.AddTopic(partitions)
}

// ReleaseTopic releases a topic quota
func (m *Manager) ReleaseTopic(id TenantID, partitions int) {
	tracker, err := m.GetTracker(id)
	if err != nil {
		return
	}

	tracker.RemoveTopic(partitions)
}

// EnforceStorageQuota checks if storage quota allows the operation
func (m *Manager) EnforceStorageQuota(id TenantID, bytes int64) error {
	// Check tenant is active
	if err := m.CheckTenantActive(id); err != nil {
		return err
	}

	// Get tracker
	tracker, err := m.GetTracker(id)
	if err != nil {
		return err
	}

	// Check storage
	return tracker.CheckStorage(bytes)
}

// UpdateStorageUsage updates the current storage usage for a tenant
func (m *Manager) UpdateStorageUsage(id TenantID, bytes int64) error {
	tracker, err := m.GetTracker(id)
	if err != nil {
		return err
	}

	tracker.UpdateStorage(bytes)
	return nil
}

// GetUsage returns current usage for a tenant
func (m *Manager) GetUsage(id TenantID) (*Usage, error) {
	tracker, err := m.GetTracker(id)
	if err != nil {
		return nil, err
	}

	return tracker.GetUsage(), nil
}

// GetAllUsage returns usage for all tenants
func (m *Manager) GetAllUsage() map[TenantID]*Usage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	usage := make(map[TenantID]*Usage)
	for id, tracker := range m.trackers {
		usage[id] = tracker.GetUsage()
	}

	return usage
}

// GetUtilization returns utilization percentages for a tenant
func (m *Manager) GetUtilization(id TenantID) (map[string]float64, error) {
	tracker, err := m.GetTracker(id)
	if err != nil {
		return nil, err
	}

	return map[string]float64{
		"connections": tracker.UtilizationPercent("connections"),
		"storage":     tracker.UtilizationPercent("storage"),
		"topics":      tracker.UtilizationPercent("topics"),
	}, nil
}

// TenantStats returns summary statistics for a tenant
type TenantStats struct {
	Tenant       *Tenant
	Usage        *Usage
	Utilization  map[string]float64
}

// GetTenantStats returns comprehensive stats for a tenant
func (m *Manager) GetTenantStats(id TenantID) (*TenantStats, error) {
	tenant, err := m.GetTenant(id)
	if err != nil {
		return nil, err
	}

	usage, err := m.GetUsage(id)
	if err != nil {
		return nil, err
	}

	utilization, err := m.GetUtilization(id)
	if err != nil {
		return nil, err
	}

	return &TenantStats{
		Tenant:      tenant,
		Usage:       usage,
		Utilization: utilization,
	}, nil
}
