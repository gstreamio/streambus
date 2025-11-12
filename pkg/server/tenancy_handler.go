package server

import (
	"sync/atomic"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/tenancy"
)

// TenancyHandler wraps a handler with multi-tenancy and quota enforcement
type TenancyHandler struct {
	baseHandler      *Handler
	tenancyManager   *tenancy.Manager
	enabled          bool
	quotaViolations  int64
	requestsHandled  int64
}

// NewTenancyHandler creates a new tenancy-aware handler
func NewTenancyHandler(baseHandler *Handler, tenancyManager *tenancy.Manager, enabled bool) *TenancyHandler {
	return &TenancyHandler{
		baseHandler:    baseHandler,
		tenancyManager: tenancyManager,
		enabled:        enabled,
	}
}

// Handle handles a request with tenancy enforcement
func (h *TenancyHandler) Handle(req *protocol.Request) *protocol.Response {
	atomic.AddInt64(&h.requestsHandled, 1)

	// If tenancy is disabled, pass through to base handler
	if !h.enabled || h.tenancyManager == nil {
		return h.baseHandler.Handle(req)
	}

	// Extract tenant ID from request
	tenantID := h.extractTenantID(req)

	// Enforce quotas based on request type
	if err := h.enforceQuotas(tenantID, req); err != nil {
		atomic.AddInt64(&h.quotaViolations, 1)
		return h.quotaExceededResponse(req.Header.RequestID, tenantID, err)
	}

	// Pass to base handler
	return h.baseHandler.Handle(req)
}

// extractTenantID extracts tenant ID from request
// It checks multiple sources in order:
// 1. Message headers (for produce/fetch requests)
// 2. Defaults to "default" tenant if not specified
func (h *TenancyHandler) extractTenantID(req *protocol.Request) tenancy.TenantID {
	// Check message headers for tenant ID
	switch payload := req.Payload.(type) {
	case *protocol.ProduceRequest:
		if len(payload.Messages) > 0 && payload.Messages[0].Headers != nil {
			if tenantIDBytes, ok := payload.Messages[0].Headers["tenant_id"]; ok {
				return tenancy.TenantID(tenantIDBytes)
			}
		}
	case *protocol.FetchRequest:
		// For fetch requests, we could check topic ownership
		// For now, use default tenant
	}

	// Default to "default" tenant for backward compatibility
	return "default"
}

// enforceQuotas enforces tenant quotas based on request type
func (h *TenancyHandler) enforceQuotas(tenantID tenancy.TenantID, req *protocol.Request) error {
	switch req.Header.Type {
	case protocol.RequestTypeProduce:
		return h.enforceProduceQuota(tenantID, req)
	case protocol.RequestTypeFetch:
		return h.enforceFetchQuota(tenantID, req)
	case protocol.RequestTypeCreateTopic:
		return h.enforceTopicQuota(tenantID, req)
	default:
		// No quota enforcement for other request types
		return nil
	}
}

// enforceProduceQuota enforces produce quotas
func (h *TenancyHandler) enforceProduceQuota(tenantID tenancy.TenantID, req *protocol.Request) error {
	payload := req.Payload.(*protocol.ProduceRequest)

	// Calculate bytes and message count
	var totalBytes int64
	for _, msg := range payload.Messages {
		totalBytes += int64(len(msg.Key) + len(msg.Value))
	}
	messageCount := int64(len(payload.Messages))

	// Enforce quota
	return h.tenancyManager.EnforceProduceQuota(tenantID, totalBytes, messageCount)
}

// enforceFetchQuota enforces fetch quotas
func (h *TenancyHandler) enforceFetchQuota(tenantID tenancy.TenantID, req *protocol.Request) error {
	payload := req.Payload.(*protocol.FetchRequest)

	// Estimate fetch size (actual size will be smaller, but this is conservative)
	estimatedBytes := int64(payload.MaxBytes)
	estimatedMessages := int64(estimatedBytes / 1024) // Assume avg message size of 1KB

	// Enforce quota
	return h.tenancyManager.EnforceConsumeQuota(tenantID, estimatedBytes, estimatedMessages)
}

// enforceTopicQuota enforces topic creation quotas
func (h *TenancyHandler) enforceTopicQuota(tenantID tenancy.TenantID, req *protocol.Request) error {
	payload := req.Payload.(*protocol.CreateTopicRequest)

	// Enforce topic quota (includes partition count)
	return h.tenancyManager.EnforceTopicQuota(tenantID, int(payload.NumPartitions))
}

// quotaExceededResponse returns a quota exceeded error response
func (h *TenancyHandler) quotaExceededResponse(requestID uint64, tenantID tenancy.TenantID, err error) *protocol.Response {
	// Convert tenancy error to protocol error
	quotaErr, ok := err.(*tenancy.QuotaError)
	if !ok {
		return &protocol.Response{
			Header: protocol.ResponseHeader{
				RequestID: requestID,
				Status:    protocol.StatusError,
				ErrorCode: protocol.ErrInvalidRequest,
			},
			Payload: nil,
		}
	}

	// Create detailed error response
	errorMessage := quotaErr.Error()

	return &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: requestID,
			Status:    protocol.StatusError,
			ErrorCode: protocol.ErrorCode(1000 + 100), // Custom quota error code
		},
		Payload: &protocol.ErrorResponse{
			ErrorCode: protocol.ErrorCode(1000 + 100),
			Message:   errorMessage,
		},
	}
}

// GetStats returns tenancy handler statistics
func (h *TenancyHandler) GetStats() map[string]int64 {
	return map[string]int64{
		"requests_handled":  atomic.LoadInt64(&h.requestsHandled),
		"quota_violations":  atomic.LoadInt64(&h.quotaViolations),
	}
}
