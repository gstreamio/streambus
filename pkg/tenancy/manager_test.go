package tenancy

import (
	"testing"
)

func TestManager_CreateTenant(t *testing.T) {
	manager := NewManager()

	tenant, err := manager.CreateTenant("tenant1", "Test Tenant", DefaultQuotas())
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	if tenant.ID != "tenant1" {
		t.Errorf("Expected tenant ID 'tenant1', got '%s'", tenant.ID)
	}

	// Verify tracker was created
	_, err = manager.GetTracker("tenant1")
	if err != nil {
		t.Errorf("Expected tracker to be created, got error: %v", err)
	}
}

func TestManager_EnforceProduceQuota(t *testing.T) {
	manager := NewManager()

	quotas := &Quotas{
		MaxBytesPerSecond:    1000,
		MaxMessagesPerSecond: 100,
	}

	_, err := manager.CreateTenant("tenant1", "Test Tenant", quotas)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Should succeed under quota
	err = manager.EnforceProduceQuota("tenant1", 500, 50)
	if err != nil {
		t.Errorf("Expected produce quota enforcement to succeed, got error: %v", err)
	}

	// Should fail when exceeding quota
	err = manager.EnforceProduceQuota("tenant1", 600, 60)
	if err == nil {
		t.Error("Expected produce quota enforcement to fail")
	}
}

func TestManager_EnforceConsumeQuota(t *testing.T) {
	manager := NewManager()

	quotas := &Quotas{
		MaxBytesPerSecond:    1000,
		MaxMessagesPerSecond: 100,
	}

	_, err := manager.CreateTenant("tenant1", "Test Tenant", quotas)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Should succeed under quota
	err = manager.EnforceConsumeQuota("tenant1", 500, 50)
	if err != nil {
		t.Errorf("Expected consume quota enforcement to succeed, got error: %v", err)
	}

	// Should fail when exceeding quota
	err = manager.EnforceConsumeQuota("tenant1", 600, 60)
	if err == nil {
		t.Error("Expected consume quota enforcement to fail")
	}
}

func TestManager_EnforceConnectionQuota(t *testing.T) {
	manager := NewManager()

	quotas := &Quotas{
		MaxConnections: 5,
	}

	_, err := manager.CreateTenant("tenant1", "Test Tenant", quotas)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Add connections up to limit
	for i := 0; i < 5; i++ {
		err = manager.EnforceConnectionQuota("tenant1")
		if err != nil {
			t.Errorf("Failed to enforce connection quota %d: %v", i, err)
		}
	}

	// Should fail when exceeding quota
	err = manager.EnforceConnectionQuota("tenant1")
	if err == nil {
		t.Error("Expected connection quota enforcement to fail")
	}

	// Release a connection
	manager.ReleaseConnection("tenant1")

	// Should succeed now
	err = manager.EnforceConnectionQuota("tenant1")
	if err != nil {
		t.Errorf("Expected connection quota enforcement to succeed after release: %v", err)
	}
}

func TestManager_EnforceTopicQuota(t *testing.T) {
	manager := NewManager()

	quotas := &Quotas{
		MaxTopics:     10,
		MaxPartitions: 100,
	}

	_, err := manager.CreateTenant("tenant1", "Test Tenant", quotas)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Add topics up to limit
	for i := 0; i < 10; i++ {
		err = manager.EnforceTopicQuota("tenant1", 5)
		if err != nil {
			t.Errorf("Failed to enforce topic quota %d: %v", i, err)
		}
	}

	// Should fail when exceeding topic quota
	err = manager.EnforceTopicQuota("tenant1", 1)
	if err == nil {
		t.Error("Expected topic quota enforcement to fail")
	}

	// Release a topic
	manager.ReleaseTopic("tenant1", 5)

	// Should succeed now
	err = manager.EnforceTopicQuota("tenant1", 5)
	if err != nil {
		t.Errorf("Expected topic quota enforcement to succeed after release: %v", err)
	}
}

func TestManager_EnforceStorageQuota(t *testing.T) {
	manager := NewManager()

	quotas := &Quotas{
		MaxStorageBytes: 1000,
	}

	_, err := manager.CreateTenant("tenant1", "Test Tenant", quotas)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Update storage
	err = manager.UpdateStorageUsage("tenant1", 500)
	if err != nil {
		t.Fatalf("Failed to update storage usage: %v", err)
	}

	// Should succeed under quota
	err = manager.EnforceStorageQuota("tenant1", 400)
	if err != nil {
		t.Errorf("Expected storage quota enforcement to succeed, got error: %v", err)
	}

	// Should fail when exceeding quota
	err = manager.EnforceStorageQuota("tenant1", 600)
	if err == nil {
		t.Error("Expected storage quota enforcement to fail")
	}
}

func TestManager_SuspendedTenant(t *testing.T) {
	manager := NewManager()

	_, err := manager.CreateTenant("tenant1", "Test Tenant", DefaultQuotas())
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Suspend tenant
	err = manager.SuspendTenant("tenant1")
	if err != nil {
		t.Fatalf("Failed to suspend tenant: %v", err)
	}

	// Operations should fail for suspended tenant
	err = manager.EnforceProduceQuota("tenant1", 100, 10)
	if err == nil {
		t.Error("Expected produce quota enforcement to fail for suspended tenant")
	}

	err = manager.EnforceConnectionQuota("tenant1")
	if err == nil {
		t.Error("Expected connection quota enforcement to fail for suspended tenant")
	}

	// Activate tenant
	err = manager.ActivateTenant("tenant1")
	if err != nil {
		t.Fatalf("Failed to activate tenant: %v", err)
	}

	// Operations should succeed now
	err = manager.EnforceProduceQuota("tenant1", 100, 10)
	if err != nil {
		t.Errorf("Expected produce quota enforcement to succeed for active tenant: %v", err)
	}
}

func TestManager_UpdateTenant(t *testing.T) {
	manager := NewManager()

	quotas := &Quotas{
		MaxBytesPerSecond: 1000,
	}

	_, err := manager.CreateTenant("tenant1", "Test Tenant", quotas)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Update quotas
	newQuotas := &Quotas{
		MaxBytesPerSecond: 2000,
	}

	err = manager.UpdateTenant("tenant1", "Updated Tenant", newQuotas)
	if err != nil {
		t.Fatalf("Failed to update tenant: %v", err)
	}

	// Verify updated quotas in tracker
	tracker, err := manager.GetTracker("tenant1")
	if err != nil {
		t.Fatalf("Failed to get tracker: %v", err)
	}

	if tracker.quotas.MaxBytesPerSecond != 2000 {
		t.Errorf("Expected updated bytes quota 2000, got %d", tracker.quotas.MaxBytesPerSecond)
	}
}

func TestManager_DeleteTenant(t *testing.T) {
	manager := NewManager()

	_, err := manager.CreateTenant("tenant1", "Test Tenant", nil)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Delete tenant
	err = manager.DeleteTenant("tenant1")
	if err != nil {
		t.Fatalf("Failed to delete tenant: %v", err)
	}

	// Verify tenant is deleted
	_, err = manager.GetTenant("tenant1")
	if err == nil {
		t.Error("Expected error when getting deleted tenant")
	}

	// Verify tracker is deleted
	_, err = manager.GetTracker("tenant1")
	if err == nil {
		t.Error("Expected error when getting tracker for deleted tenant")
	}
}

func TestManager_GetUsage(t *testing.T) {
	manager := NewManager()

	_, err := manager.CreateTenant("tenant1", "Test Tenant", DefaultQuotas())
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Add some usage
	manager.EnforceConnectionQuota("tenant1")
	manager.EnforceConnectionQuota("tenant1")
	manager.EnforceProduceQuota("tenant1", 100, 10)

	usage, err := manager.GetUsage("tenant1")
	if err != nil {
		t.Fatalf("Failed to get usage: %v", err)
	}

	if usage.Connections != 2 {
		t.Errorf("Expected 2 connections, got %d", usage.Connections)
	}

	if usage.TenantID != "tenant1" {
		t.Errorf("Expected tenant ID 'tenant1', got '%s'", usage.TenantID)
	}
}

func TestManager_GetAllUsage(t *testing.T) {
	manager := NewManager()

	// Create multiple tenants
	manager.CreateTenant("tenant1", "Tenant 1", nil)
	manager.CreateTenant("tenant2", "Tenant 2", nil)
	manager.CreateTenant("tenant3", "Tenant 3", nil)

	// Add usage to each
	manager.EnforceConnectionQuota("tenant1")
	manager.EnforceConnectionQuota("tenant2")
	manager.EnforceConnectionQuota("tenant2")
	manager.EnforceConnectionQuota("tenant3")

	usage := manager.GetAllUsage()

	if len(usage) != 3 {
		t.Errorf("Expected usage for 3 tenants, got %d", len(usage))
	}

	if usage["tenant1"].Connections != 1 {
		t.Errorf("Expected 1 connection for tenant1, got %d", usage["tenant1"].Connections)
	}

	if usage["tenant2"].Connections != 2 {
		t.Errorf("Expected 2 connections for tenant2, got %d", usage["tenant2"].Connections)
	}
}

func TestManager_GetUtilization(t *testing.T) {
	manager := NewManager()

	quotas := &Quotas{
		MaxConnections:  100,
		MaxStorageBytes: 1000,
		MaxTopics:       10,
	}

	_, err := manager.CreateTenant("tenant1", "Test Tenant", quotas)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Add usage
	for i := 0; i < 50; i++ {
		manager.EnforceConnectionQuota("tenant1")
	}
	manager.UpdateStorageUsage("tenant1", 500)

	utilization, err := manager.GetUtilization("tenant1")
	if err != nil {
		t.Fatalf("Failed to get utilization: %v", err)
	}

	if utilization["connections"] != 50.0 {
		t.Errorf("Expected 50%% connection utilization, got %.2f%%", utilization["connections"])
	}

	if utilization["storage"] != 50.0 {
		t.Errorf("Expected 50%% storage utilization, got %.2f%%", utilization["storage"])
	}
}

func TestManager_GetTenantStats(t *testing.T) {
	manager := NewManager()

	quotas := &Quotas{
		MaxConnections: 100,
	}

	_, err := manager.CreateTenant("tenant1", "Test Tenant", quotas)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Add usage
	for i := 0; i < 25; i++ {
		manager.EnforceConnectionQuota("tenant1")
	}

	stats, err := manager.GetTenantStats("tenant1")
	if err != nil {
		t.Fatalf("Failed to get tenant stats: %v", err)
	}

	if stats.Tenant.ID != "tenant1" {
		t.Errorf("Expected tenant ID 'tenant1', got '%s'", stats.Tenant.ID)
	}

	if stats.Usage.Connections != 25 {
		t.Errorf("Expected 25 connections, got %d", stats.Usage.Connections)
	}

	if stats.Utilization["connections"] != 25.0 {
		t.Errorf("Expected 25%% connection utilization, got %.2f%%", stats.Utilization["connections"])
	}
}

func TestManager_ListTenants(t *testing.T) {
	manager := NewManager()

	// Create multiple tenants
	manager.CreateTenant("tenant1", "Tenant 1", nil)
	manager.CreateTenant("tenant2", "Tenant 2", nil)
	manager.CreateTenant("tenant3", "Tenant 3", nil)

	tenants := manager.ListTenants()

	if len(tenants) != 3 {
		t.Errorf("Expected 3 tenants, got %d", len(tenants))
	}
}

func TestManager_NonexistentTenant(t *testing.T) {
	manager := NewManager()

	// Operations on nonexistent tenant should fail
	err := manager.EnforceProduceQuota("nonexistent", 100, 10)
	if err == nil {
		t.Error("Expected error for nonexistent tenant")
	}

	_, err = manager.GetUsage("nonexistent")
	if err == nil {
		t.Error("Expected error getting usage for nonexistent tenant")
	}

	_, err = manager.GetTracker("nonexistent")
	if err == nil {
		t.Error("Expected error getting tracker for nonexistent tenant")
	}
}
