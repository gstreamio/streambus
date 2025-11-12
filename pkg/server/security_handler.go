package server

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/security"
)

// SecurityHandler wraps a handler with authentication and authorization
type SecurityHandler struct {
	baseHandler     RequestHandler
	securityManager *security.Manager
	enabled         bool
	authFailures    int64
	authzDenials    int64
	requestsHandled int64
}

// NewSecurityHandler creates a new security-aware handler
func NewSecurityHandler(baseHandler RequestHandler, securityManager *security.Manager, enabled bool) *SecurityHandler {
	return &SecurityHandler{
		baseHandler:     baseHandler,
		securityManager: securityManager,
		enabled:         enabled,
	}
}

// Handle handles a request with security enforcement
func (h *SecurityHandler) Handle(req *protocol.Request) *protocol.Response {
	atomic.AddInt64(&h.requestsHandled, 1)

	// If security is disabled, pass through to base handler
	if !h.enabled || h.securityManager == nil {
		return h.baseHandler.Handle(req)
	}

	// Create context for this request
	ctx := context.Background()

	// Extract and authenticate credentials
	principal, err := h.authenticate(ctx, req)
	if err != nil {
		atomic.AddInt64(&h.authFailures, 1)
		return h.authenticationFailedResponse(req.Header.RequestID, err)
	}

	// Check authorization for this request
	action, resource := h.getActionAndResource(req)
	allowed, err := h.securityManager.Authorize(ctx, principal, action, resource)
	if err != nil {
		return h.errorResponse(req.Header.RequestID, protocol.ErrAuthorizationFailed, "authorization error: "+err.Error())
	}
	if !allowed {
		atomic.AddInt64(&h.authzDenials, 1)
		return h.authorizationDeniedResponse(req.Header.RequestID, principal, action, resource)
	}

	// Audit the request
	if h.securityManager.IsAuditEnabled() {
		h.securityManager.AuditAction(ctx, principal, action, resource, map[string]string{
			"request_type": req.Header.Type.String(),
			"request_id":   fmt.Sprintf("%d", req.Header.RequestID),
		})
	}

	// Pass to base handler
	return h.baseHandler.Handle(req)
}

// authenticate extracts credentials from request and authenticates
func (h *SecurityHandler) authenticate(ctx context.Context, req *protocol.Request) (*security.Principal, error) {
	// Check if authentication is enabled
	if !h.securityManager.IsAuthenticationEnabled() {
		// Return anonymous principal
		return &security.Principal{
			ID:   "anonymous",
			Type: security.PrincipalTypeAnonymous,
		}, nil
	}

	// TODO: For full authentication support, we need to implement SASL handshake
	// at connection time. For now, we'll use anonymous principal if auth is enabled
	// but allow it, or we could extract credentials from a future protocol extension.

	// For now, if authentication is enabled, we return anonymous
	// This allows the system to work while we implement proper SASL handshake
	return &security.Principal{
		ID:   "anonymous",
		Type: security.PrincipalTypeAnonymous,
	}, nil
}

// getActionAndResource determines the action and resource from the request
func (h *SecurityHandler) getActionAndResource(req *protocol.Request) (security.Action, security.Resource) {
	switch req.Header.Type {
	case protocol.RequestTypeProduce:
		payload := req.Payload.(*protocol.ProduceRequest)
		return security.ActionTopicWrite, security.Resource{
			Type:        security.ResourceTypeTopic,
			Name:        payload.Topic,
			PatternType: security.PatternTypeLiteral,
		}

	case protocol.RequestTypeFetch:
		payload := req.Payload.(*protocol.FetchRequest)
		return security.ActionTopicRead, security.Resource{
			Type:        security.ResourceTypeTopic,
			Name:        payload.Topic,
			PatternType: security.PatternTypeLiteral,
		}

	case protocol.RequestTypeGetOffset:
		payload := req.Payload.(*protocol.GetOffsetRequest)
		return security.ActionTopicDescribe, security.Resource{
			Type:        security.ResourceTypeTopic,
			Name:        payload.Topic,
			PatternType: security.PatternTypeLiteral,
		}

	case protocol.RequestTypeCreateTopic:
		payload := req.Payload.(*protocol.CreateTopicRequest)
		return security.ActionTopicCreate, security.Resource{
			Type:        security.ResourceTypeTopic,
			Name:        payload.Topic,
			PatternType: security.PatternTypeLiteral,
		}

	case protocol.RequestTypeDeleteTopic:
		payload := req.Payload.(*protocol.DeleteTopicRequest)
		return security.ActionTopicDelete, security.Resource{
			Type:        security.ResourceTypeTopic,
			Name:        payload.Topic,
			PatternType: security.PatternTypeLiteral,
		}

	case protocol.RequestTypeListTopics:
		return security.ActionClusterDescribe, security.Resource{
			Type:        security.ResourceTypeCluster,
			Name:        "cluster",
			PatternType: security.PatternTypeLiteral,
		}

	case protocol.RequestTypeHealthCheck:
		return security.ActionClusterDescribe, security.Resource{
			Type:        security.ResourceTypeCluster,
			Name:        "cluster",
			PatternType: security.PatternTypeLiteral,
		}

	default:
		return security.ActionClusterAction, security.Resource{
			Type:        security.ResourceTypeCluster,
			Name:        "cluster",
			PatternType: security.PatternTypeLiteral,
		}
	}
}

// authenticationFailedResponse returns an authentication failure response
func (h *SecurityHandler) authenticationFailedResponse(requestID uint64, err error) *protocol.Response {
	return &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: requestID,
			Status:    protocol.StatusError,
			ErrorCode: protocol.ErrAuthenticationFailed,
		},
		Payload: &protocol.ErrorResponse{
			ErrorCode: protocol.ErrAuthenticationFailed,
			Message:   "Authentication failed: " + err.Error(),
		},
	}
}

// authorizationDeniedResponse returns an authorization denial response
func (h *SecurityHandler) authorizationDeniedResponse(requestID uint64, principal *security.Principal, action security.Action, resource security.Resource) *protocol.Response {
	return &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: requestID,
			Status:    protocol.StatusError,
			ErrorCode: protocol.ErrAuthorizationFailed,
		},
		Payload: &protocol.ErrorResponse{
			ErrorCode: protocol.ErrAuthorizationFailed,
			Message:   "Access denied: principal '" + principal.ID + "' is not authorized to perform '" + string(action) + "' on resource '" + resource.Name + "'",
		},
	}
}

// errorResponse returns a generic error response
func (h *SecurityHandler) errorResponse(requestID uint64, errorCode protocol.ErrorCode, message string) *protocol.Response {
	return &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: requestID,
			Status:    protocol.StatusError,
			ErrorCode: errorCode,
		},
		Payload: &protocol.ErrorResponse{
			ErrorCode: errorCode,
			Message:   message,
		},
	}
}

// GetStats returns security handler statistics
func (h *SecurityHandler) GetStats() map[string]int64 {
	return map[string]int64{
		"requests_handled":    atomic.LoadInt64(&h.requestsHandled),
		"auth_failures":       atomic.LoadInt64(&h.authFailures),
		"authz_denials":       atomic.LoadInt64(&h.authzDenials),
	}
}
