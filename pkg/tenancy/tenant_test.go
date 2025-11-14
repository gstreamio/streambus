package tenancy

import (
	"testing"
	"time"
)

func TestTenantStore_CreateTenant(t *testing.T) {
	store := NewTenantStore()

	tenant, err := store.CreateTenant("tenant1", "Test Tenant", DefaultQuotas())
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	if tenant.ID != "tenant1" {
		t.Errorf("Expected tenant ID 'tenant1', got '%s'", tenant.ID)
	}

	if tenant.Name != "Test Tenant" {
		t.Errorf("Expected tenant name 'Test Tenant', got '%s'", tenant.Name)
	}

	if tenant.Status != TenantStatusActive {
		t.Errorf("Expected status ACTIVE, got %s", tenant.Status)
	}

	if tenant.Quotas == nil {
		t.Error("Expected quotas to be set")
	}
}

func TestTenantStore_CreateDuplicateTenant(t *testing.T) {
	store := NewTenantStore()

	_, err := store.CreateTenant("tenant1", "Test Tenant", nil)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	_, err = store.CreateTenant("tenant1", "Duplicate", nil)
	if err == nil {
		t.Error("Expected error when creating duplicate tenant")
	}
}

func TestTenantStore_GetTenant(t *testing.T) {
	store := NewTenantStore()

	_, err := store.CreateTenant("tenant1", "Test Tenant", nil)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	tenant, err := store.GetTenant("tenant1")
	if err != nil {
		t.Fatalf("Failed to get tenant: %v", err)
	}

	if tenant.ID != "tenant1" {
		t.Errorf("Expected tenant ID 'tenant1', got '%s'", tenant.ID)
	}
}

func TestTenantStore_GetNonexistentTenant(t *testing.T) {
	store := NewTenantStore()

	_, err := store.GetTenant("nonexistent")
	if err == nil {
		t.Error("Expected error when getting nonexistent tenant")
	}
}

func TestTenantStore_UpdateTenant(t *testing.T) {
	store := NewTenantStore()

	_, err := store.CreateTenant("tenant1", "Test Tenant", DefaultQuotas())
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Update name
	err = store.UpdateTenant("tenant1", "Updated Name", nil)
	if err != nil {
		t.Fatalf("Failed to update tenant: %v", err)
	}

	tenant, err := store.GetTenant("tenant1")
	if err != nil {
		t.Fatalf("Failed to get tenant: %v", err)
	}

	if tenant.Name != "Updated Name" {
		t.Errorf("Expected updated name 'Updated Name', got '%s'", tenant.Name)
	}

	// Update quotas
	newQuotas := &Quotas{
		MaxBytesPerSecond:    20 * 1024 * 1024,
		MaxMessagesPerSecond: 20000,
	}

	err = store.UpdateTenant("tenant1", "", newQuotas)
	if err != nil {
		t.Fatalf("Failed to update quotas: %v", err)
	}

	tenant, err = store.GetTenant("tenant1")
	if err != nil {
		t.Fatalf("Failed to get tenant: %v", err)
	}

	if tenant.Quotas.MaxBytesPerSecond != 20*1024*1024 {
		t.Errorf("Expected updated bytes quota, got %d", tenant.Quotas.MaxBytesPerSecond)
	}
}

func TestTenantStore_DeleteTenant(t *testing.T) {
	store := NewTenantStore()

	_, err := store.CreateTenant("tenant1", "Test Tenant", nil)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	err = store.DeleteTenant("tenant1")
	if err != nil {
		t.Fatalf("Failed to delete tenant: %v", err)
	}

	_, err = store.GetTenant("tenant1")
	if err == nil {
		t.Error("Expected error when getting deleted tenant")
	}
}

func TestTenantStore_ListTenants(t *testing.T) {
	store := NewTenantStore()

	_, _ = store.CreateTenant("tenant1", "Tenant 1", nil)
	_, _ = store.CreateTenant("tenant2", "Tenant 2", nil)
	_, _ = store.CreateTenant("tenant3", "Tenant 3", nil)

	tenants := store.ListTenants()
	if len(tenants) != 3 {
		t.Errorf("Expected 3 tenants, got %d", len(tenants))
	}
}

func TestTenantStore_SuspendAndActivateTenant(t *testing.T) {
	store := NewTenantStore()

	_, err := store.CreateTenant("tenant1", "Test Tenant", nil)
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Suspend
	err = store.SuspendTenant("tenant1")
	if err != nil {
		t.Fatalf("Failed to suspend tenant: %v", err)
	}

	tenant, err := store.GetTenant("tenant1")
	if err != nil {
		t.Fatalf("Failed to get tenant: %v", err)
	}

	if tenant.Status != TenantStatusSuspended {
		t.Errorf("Expected status SUSPENDED, got %s", tenant.Status)
	}

	// Activate
	err = store.ActivateTenant("tenant1")
	if err != nil {
		t.Fatalf("Failed to activate tenant: %v", err)
	}

	tenant, err = store.GetTenant("tenant1")
	if err != nil {
		t.Fatalf("Failed to get tenant: %v", err)
	}

	if tenant.Status != TenantStatusActive {
		t.Errorf("Expected status ACTIVE, got %s", tenant.Status)
	}
}

func TestTenant_IsActive(t *testing.T) {
	tenant := &Tenant{Status: TenantStatusActive}
	if !tenant.IsActive() {
		t.Error("Expected tenant to be active")
	}

	tenant.Status = TenantStatusSuspended
	if tenant.IsActive() {
		t.Error("Expected tenant to not be active when suspended")
	}
}

func TestTenant_Validate(t *testing.T) {
	// Valid tenant
	tenant := &Tenant{
		ID:     "tenant1",
		Name:   "Test Tenant",
		Quotas: DefaultQuotas(),
	}

	err := tenant.Validate()
	if err != nil {
		t.Errorf("Expected valid tenant, got error: %v", err)
	}

	// Missing ID
	tenant = &Tenant{
		Name:   "Test Tenant",
		Quotas: DefaultQuotas(),
	}

	err = tenant.Validate()
	if err == nil {
		t.Error("Expected error for missing ID")
	}

	// Missing name
	tenant = &Tenant{
		ID:     "tenant1",
		Quotas: DefaultQuotas(),
	}

	err = tenant.Validate()
	if err == nil {
		t.Error("Expected error for missing name")
	}

	// Missing quotas
	tenant = &Tenant{
		ID:   "tenant1",
		Name: "Test Tenant",
	}

	err = tenant.Validate()
	if err == nil {
		t.Error("Expected error for missing quotas")
	}
}

func TestDefaultQuotas(t *testing.T) {
	quotas := DefaultQuotas()

	if quotas.MaxBytesPerSecond <= 0 {
		t.Error("Expected positive default bytes per second quota")
	}

	if quotas.MaxMessagesPerSecond <= 0 {
		t.Error("Expected positive default messages per second quota")
	}

	if quotas.MaxStorageBytes <= 0 {
		t.Error("Expected positive default storage quota")
	}
}

func TestUnlimitedQuotas(t *testing.T) {
	quotas := UnlimitedQuotas()

	if quotas.MaxBytesPerSecond != -1 {
		t.Error("Expected unlimited bytes per second quota")
	}

	if quotas.MaxMessagesPerSecond != -1 {
		t.Error("Expected unlimited messages per second quota")
	}

	if quotas.MaxStorageBytes != -1 {
		t.Error("Expected unlimited storage quota")
	}
}

func TestTenantTimestamps(t *testing.T) {
	store := NewTenantStore()

	before := time.Now()
	tenant, err := store.CreateTenant("tenant1", "Test Tenant", nil)
	after := time.Now()

	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	if tenant.CreatedAt.Before(before) || tenant.CreatedAt.After(after) {
		t.Error("CreatedAt timestamp not in expected range")
	}

	if tenant.UpdatedAt.Before(before) || tenant.UpdatedAt.After(after) {
		t.Error("UpdatedAt timestamp not in expected range")
	}

	// Update tenant
	time.Sleep(10 * time.Millisecond)
	updateBefore := time.Now()
	err = store.UpdateTenant("tenant1", "Updated", nil)
	updateAfter := time.Now()

	if err != nil {
		t.Fatalf("Failed to update tenant: %v", err)
	}

	tenant, _ = store.GetTenant("tenant1")

	if tenant.UpdatedAt.Before(updateBefore) || tenant.UpdatedAt.After(updateAfter) {
		t.Error("UpdatedAt timestamp not updated correctly")
	}

	// CreatedAt should not change
	if tenant.CreatedAt.After(after) {
		t.Error("CreatedAt should not change on update")
	}
}
