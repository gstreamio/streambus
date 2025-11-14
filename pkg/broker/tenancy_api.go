package broker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gstreamio/streambus/pkg/logging"
	"github.com/gstreamio/streambus/pkg/tenancy"
)

// registerTenancyAPI registers tenant management HTTP endpoints
func (b *Broker) registerTenancyAPI(mux *http.ServeMux) {
	if !b.config.EnableMultiTenancy || b.tenancyManager == nil {
		return
	}

	// Tenant CRUD operations
	mux.HandleFunc("/api/v1/tenants", b.handleTenantList)
	mux.HandleFunc("/api/v1/tenants/", b.handleTenantOperations)

	b.logger.Info("Registered tenancy management API endpoints")
}

// handleTenantList handles GET /api/v1/tenants (list all tenants)
func (b *Broker) handleTenantList(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		b.getTenants(w, r)
	case http.MethodPost:
		b.createTenant(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleTenantOperations handles tenant-specific operations
func (b *Broker) handleTenantOperations(w http.ResponseWriter, r *http.Request) {
	// Extract tenant ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/tenants/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Tenant ID required", http.StatusBadRequest)
		return
	}

	tenantID := tenancy.TenantID(parts[0])

	// Check if this is a stats request
	if len(parts) > 1 && parts[1] == "stats" {
		b.getTenantStats(w, r, tenantID)
		return
	}

	// Handle tenant operations
	switch r.Method {
	case http.MethodGet:
		b.getTenant(w, r, tenantID)
	case http.MethodPut:
		b.updateTenant(w, r, tenantID)
	case http.MethodDelete:
		b.deleteTenant(w, r, tenantID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// getTenants handles GET /api/v1/tenants
func (b *Broker) getTenants(w http.ResponseWriter, r *http.Request) {
	tenants := b.tenancyManager.ListTenants()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"tenants": tenants,
		"count":   len(tenants),
	})
}

// createTenant handles POST /api/v1/tenants
func (b *Broker) createTenant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID          string           `json:"id"`
		Name        string           `json:"name"`
		Description string           `json:"description"`
		Quotas      *tenancy.Quotas  `json:"quotas"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.ID == "" {
		http.Error(w, "Tenant ID is required", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Tenant name is required", http.StatusBadRequest)
		return
	}

	// Use default quotas if not provided
	quotas := req.Quotas
	if quotas == nil {
		quotas = tenancy.DefaultQuotas()
	}

	// Create tenant
	tenant, err := b.tenancyManager.CreateTenant(tenancy.TenantID(req.ID), req.Name, quotas)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create tenant: %v", err), http.StatusInternalServerError)
		return
	}

	b.logger.Info("Tenant created via API", logging.Fields{
		"tenant_id": tenant.ID,
		"name":      tenant.Name,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(tenant)
}

// getTenant handles GET /api/v1/tenants/:id
func (b *Broker) getTenant(w http.ResponseWriter, r *http.Request, tenantID tenancy.TenantID) {
	tenant, err := b.tenancyManager.GetTenant(tenantID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Tenant not found: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tenant)
}

// updateTenant handles PUT /api/v1/tenants/:id
func (b *Broker) updateTenant(w http.ResponseWriter, r *http.Request, tenantID tenancy.TenantID) {
	var req struct {
		Name   string          `json:"name"`
		Quotas *tenancy.Quotas `json:"quotas"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Update tenant
	err := b.tenancyManager.UpdateTenant(tenantID, req.Name, req.Quotas)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update tenant: %v", err), http.StatusInternalServerError)
		return
	}

	b.logger.Info("Tenant updated via API", logging.Fields{
		"tenant_id": tenantID,
	})

	// Return updated tenant
	tenant, _ := b.tenancyManager.GetTenant(tenantID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tenant)
}

// deleteTenant handles DELETE /api/v1/tenants/:id
func (b *Broker) deleteTenant(w http.ResponseWriter, r *http.Request, tenantID tenancy.TenantID) {
	// Check for force parameter
	force := r.URL.Query().Get("force") == "true"

	if !force {
		// Check if tenant has active connections
		usage, err := b.tenancyManager.GetUsage(tenantID)
		if err == nil && usage.Connections > 0 {
			http.Error(w, "Tenant has active connections. Use ?force=true to delete anyway", http.StatusConflict)
			return
		}
	}

	// Delete tenant
	err := b.tenancyManager.DeleteTenant(tenantID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete tenant: %v", err), http.StatusInternalServerError)
		return
	}

	b.logger.Info("Tenant deleted via API", logging.Fields{
		"tenant_id": tenantID,
		"force":     force,
	})

	w.WriteHeader(http.StatusNoContent)
}

// getTenantStats handles GET /api/v1/tenants/:id/stats
func (b *Broker) getTenantStats(w http.ResponseWriter, r *http.Request, tenantID tenancy.TenantID) {
	stats, err := b.tenancyManager.GetTenantStats(tenantID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get tenant stats: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}
