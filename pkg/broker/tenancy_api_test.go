package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gstreamio/streambus/pkg/cluster"
	"github.com/gstreamio/streambus/pkg/logging"
	"github.com/gstreamio/streambus/pkg/tenancy"
)

// newTestBrokerWithTenancy creates a broker with tenancy manager for testing
func newTestBrokerWithTenancy(t *testing.T) (*Broker, *tenancy.Manager) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logger := logging.New(&logging.Config{
		Level:     logging.LevelError,
		Component: "test",
	})

	clusterMetaStore := &mockClusterMetadataStore{}
	registry := cluster.NewBrokerRegistry(clusterMetaStore)

	tenancyMgr := tenancy.NewManager()

	broker := &Broker{
		ctx:            ctx,
		cancel:         cancel,
		logger:         logger,
		registry:       registry,
		tenancyManager: tenancyMgr,
		config: &Config{
			BrokerID:           1,
			EnableMultiTenancy: true,
		},
		status: StatusRunning,
	}

	return broker, tenancyMgr
}

// TestCreateTenant_Success tests creating a tenant
func TestCreateTenant_Success(t *testing.T) {
	broker, _ := newTestBrokerWithTenancy(t)

	reqBody := map[string]interface{}{
		"id":          "test-tenant-1",
		"name":        "Test Tenant 1",
		"description": "A test tenant",
		"quotas": map[string]interface{}{
			"MaxBytesPerSecond":    5 * 1024 * 1024,
			"MaxMessagesPerSecond": 5000,
			"MaxStorageBytes":      50 * 1024 * 1024 * 1024,
			"MaxTopics":            50,
			"MaxPartitions":        500,
			"MaxConnections":       100,
		},
	}

	jsonData, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	broker.handleTenantList(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	var tenant map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&tenant); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if tenant["ID"] != "test-tenant-1" {
		t.Errorf("Expected tenant ID 'test-tenant-1', got %v", tenant["ID"])
	}

	if tenant["Name"] != "Test Tenant 1" {
		t.Errorf("Expected tenant name 'Test Tenant 1', got %v", tenant["Name"])
	}
}

// TestCreateTenant_InvalidJSON tests creating tenant with invalid JSON
func TestCreateTenant_InvalidJSON(t *testing.T) {
	broker, _ := newTestBrokerWithTenancy(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	broker.handleTenantList(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestCreateTenant_MissingID tests creating tenant without ID
func TestCreateTenant_MissingID(t *testing.T) {
	broker, _ := newTestBrokerWithTenancy(t)

	reqBody := map[string]interface{}{
		"name": "Test Tenant",
	}

	jsonData, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	broker.handleTenantList(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestCreateTenant_MissingName tests creating tenant without name
func TestCreateTenant_MissingName(t *testing.T) {
	broker, _ := newTestBrokerWithTenancy(t)

	reqBody := map[string]interface{}{
		"id": "test-tenant",
	}

	jsonData, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	broker.handleTenantList(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestCreateTenant_DefaultQuotas tests creating tenant with default quotas
func TestCreateTenant_DefaultQuotas(t *testing.T) {
	broker, _ := newTestBrokerWithTenancy(t)

	reqBody := map[string]interface{}{
		"id":   "test-tenant",
		"name": "Test Tenant",
	}

	jsonData, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	broker.handleTenantList(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	var tenant map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&tenant); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify default quotas were applied
	if tenant["Quotas"] == nil {
		t.Error("Expected Quotas to be set")
	}
}

// TestGetTenants tests listing all tenants
func TestGetTenants(t *testing.T) {
	broker, tenancyMgr := newTestBrokerWithTenancy(t)

	// Create some test tenants
	_, _ = tenancyMgr.CreateTenant("tenant-1", "Tenant 1", tenancy.DefaultQuotas())
	_, _ = tenancyMgr.CreateTenant("tenant-2", "Tenant 2", tenancy.DefaultQuotas())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants", nil)
	w := httptest.NewRecorder()

	broker.handleTenantList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if _, ok := response["tenants"]; !ok {
		t.Error("Expected 'tenants' field in response")
	}

	if _, ok := response["count"]; !ok {
		t.Error("Expected 'count' field in response")
	}

	// Should have at least our 2 test tenants (default tenant may not always be created)
	count := int(response["count"].(float64))
	if count < 2 {
		t.Errorf("Expected at least 2 tenants, got %d", count)
	}
}

// TestGetTenant tests getting a specific tenant
func TestGetTenant(t *testing.T) {
	broker, tenancyMgr := newTestBrokerWithTenancy(t)

	// Create a test tenant
	_, err := tenancyMgr.CreateTenant("test-tenant", "Test Tenant", tenancy.DefaultQuotas())
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/test-tenant", nil)
	w := httptest.NewRecorder()

	broker.handleTenantOperations(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var tenant map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&tenant); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if tenant["ID"] != "test-tenant" {
		t.Errorf("Expected tenant ID 'test-tenant', got %v", tenant["ID"])
	}
}

// TestGetTenant_NotFound tests getting a non-existent tenant
func TestGetTenant_NotFound(t *testing.T) {
	broker, _ := newTestBrokerWithTenancy(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/nonexistent-tenant", nil)
	w := httptest.NewRecorder()

	broker.handleTenantOperations(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestUpdateTenant tests updating a tenant
func TestUpdateTenant(t *testing.T) {
	broker, tenancyMgr := newTestBrokerWithTenancy(t)

	// Create a test tenant
	_, err := tenancyMgr.CreateTenant("test-tenant", "Test Tenant", tenancy.DefaultQuotas())
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	reqBody := map[string]interface{}{
		"name": "Updated Tenant Name",
		"quotas": map[string]interface{}{
			"MaxTopics": 200,
		},
	}

	jsonData, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tenants/test-tenant", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	broker.handleTenantOperations(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var tenant map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&tenant); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if tenant["Name"] != "Updated Tenant Name" {
		t.Errorf("Expected tenant name 'Updated Tenant Name', got %v", tenant["Name"])
	}
}

// TestUpdateTenant_InvalidJSON tests updating tenant with invalid JSON
func TestUpdateTenant_InvalidJSON(t *testing.T) {
	broker, _ := newTestBrokerWithTenancy(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/tenants/test-tenant", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	broker.handleTenantOperations(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestUpdateTenant_NotFound tests updating a non-existent tenant
func TestUpdateTenant_NotFound(t *testing.T) {
	broker, _ := newTestBrokerWithTenancy(t)

	reqBody := map[string]interface{}{
		"name": "Updated Name",
	}

	jsonData, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tenants/nonexistent-tenant", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	broker.handleTenantOperations(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}

// TestDeleteTenant tests deleting a tenant
func TestDeleteTenant(t *testing.T) {
	broker, tenancyMgr := newTestBrokerWithTenancy(t)

	// Create a test tenant
	_, err := tenancyMgr.CreateTenant("test-tenant", "Test Tenant", tenancy.DefaultQuotas())
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/test-tenant", nil)
	w := httptest.NewRecorder()

	broker.handleTenantOperations(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", w.Code)
	}

	// Verify tenant was deleted
	_, err = tenancyMgr.GetTenant("test-tenant")
	if err == nil {
		t.Error("Tenant should have been deleted")
	}
}

// TestDeleteTenant_WithActiveConnections tests deleting tenant with active connections
func TestDeleteTenant_WithActiveConnections(t *testing.T) {
	broker, tenancyMgr := newTestBrokerWithTenancy(t)

	// Create a test tenant
	_, err := tenancyMgr.CreateTenant("test-tenant", "Test Tenant", tenancy.DefaultQuotas())
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Simulate active connection
	_ = tenancyMgr.EnforceConnectionQuota("test-tenant")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/test-tenant", nil)
	w := httptest.NewRecorder()

	broker.handleTenantOperations(w, req)

	// Should fail with conflict without force parameter
	if w.Code != http.StatusConflict {
		t.Errorf("Expected status 409, got %d", w.Code)
	}
}

// TestDeleteTenant_ForceDelete tests force deleting tenant with active connections
func TestDeleteTenant_ForceDelete(t *testing.T) {
	broker, tenancyMgr := newTestBrokerWithTenancy(t)

	// Create a test tenant
	_, err := tenancyMgr.CreateTenant("test-tenant", "Test Tenant", tenancy.DefaultQuotas())
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// Simulate active connection
	_ = tenancyMgr.EnforceConnectionQuota("test-tenant")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/test-tenant?force=true", nil)
	w := httptest.NewRecorder()

	broker.handleTenantOperations(w, req)

	// Should succeed with force parameter
	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", w.Code)
	}
}

// TestDeleteTenant_NotFound tests deleting a non-existent tenant
func TestDeleteTenant_NotFound(t *testing.T) {
	broker, _ := newTestBrokerWithTenancy(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/nonexistent-tenant", nil)
	w := httptest.NewRecorder()

	broker.handleTenantOperations(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}

// TestGetTenantStats tests getting tenant statistics
func TestGetTenantStats(t *testing.T) {
	broker, tenancyMgr := newTestBrokerWithTenancy(t)

	// Create a test tenant
	_, err := tenancyMgr.CreateTenant("test-tenant", "Test Tenant", tenancy.DefaultQuotas())
	if err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/test-tenant/stats", nil)
	w := httptest.NewRecorder()

	broker.handleTenantOperations(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var stats map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if _, ok := stats["Tenant"]; !ok {
		t.Error("Expected 'Tenant' field in stats")
	}

	if _, ok := stats["Usage"]; !ok {
		t.Error("Expected 'Usage' field in stats")
	}

	if _, ok := stats["Utilization"]; !ok {
		t.Error("Expected 'Utilization' field in stats")
	}
}

// TestGetTenantStats_NotFound tests getting stats for non-existent tenant
func TestGetTenantStats_NotFound(t *testing.T) {
	broker, _ := newTestBrokerWithTenancy(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/nonexistent-tenant/stats", nil)
	w := httptest.NewRecorder()

	broker.handleTenantOperations(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}

// TestHandleTenantList_MethodNotAllowed tests tenant list endpoint with wrong method
func TestHandleTenantList_MethodNotAllowed(t *testing.T) {
	broker, _ := newTestBrokerWithTenancy(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tenants", nil)
	w := httptest.NewRecorder()

	broker.handleTenantList(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestHandleTenantOperations_MissingTenantID tests operations without tenant ID
func TestHandleTenantOperations_MissingTenantID(t *testing.T) {
	broker, _ := newTestBrokerWithTenancy(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/", nil)
	w := httptest.NewRecorder()

	broker.handleTenantOperations(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestHandleTenantOperations_MethodNotAllowed tests tenant operations with wrong method
func TestHandleTenantOperations_MethodNotAllowed(t *testing.T) {
	broker, _ := newTestBrokerWithTenancy(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/test-tenant", nil)
	w := httptest.NewRecorder()

	broker.handleTenantOperations(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}
