package server

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/gstreamio/streambus/pkg/logging"
	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/security"
)

// newTestLogger creates a test logger
func newTestLogger() *logging.Logger {
	return logging.New(&logging.Config{
		Level:     logging.LevelError, // Use error level to minimize test output
		Output:    os.Stdout,
		Component: "test",
	})
}

// mockHandler is a simple handler for testing
type mockHandler struct {
	handleFunc func(req *protocol.Request) *protocol.Response
}

func (m *mockHandler) Handle(req *protocol.Request) *protocol.Response {
	if m.handleFunc != nil {
		return m.handleFunc(req)
	}
	return &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: req.Header.RequestID,
			Status:    protocol.StatusOK,
		},
		Payload: &protocol.ProduceResponse{},
	}
}

func TestNewSecurityHandler(t *testing.T) {
	baseHandler := &mockHandler{}
	secMgr := &security.Manager{}

	sh := NewSecurityHandler(baseHandler, secMgr, true)

	if sh == nil {
		t.Fatal("Expected SecurityHandler to be created")
	}

	if sh.baseHandler != baseHandler {
		t.Error("baseHandler not set correctly")
	}

	if sh.securityManager != secMgr {
		t.Error("securityManager not set correctly")
	}

	if !sh.enabled {
		t.Error("enabled should be true")
	}
}

func TestSecurityHandler_Handle_Disabled(t *testing.T) {
	called := false
	baseHandler := &mockHandler{
		handleFunc: func(req *protocol.Request) *protocol.Response {
			called = true
			return &protocol.Response{
				Header: protocol.ResponseHeader{
					RequestID: req.Header.RequestID,
					Status:    protocol.StatusOK,
				},
			}
		},
	}

	sh := NewSecurityHandler(baseHandler, nil, false)

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 123,
			Type:      protocol.RequestTypeProduce,
		},
		Payload: &protocol.ProduceRequest{
			Topic: "test-topic",
		},
	}

	resp := sh.Handle(req)

	if !called {
		t.Error("Base handler should have been called when security is disabled")
	}

	if resp.Header.RequestID != 123 {
		t.Errorf("RequestID = %d, want 123", resp.Header.RequestID)
	}

	stats := sh.GetStats()
	if stats["requests_handled"] != 1 {
		t.Errorf("requests_handled = %d, want 1", stats["requests_handled"])
	}
}

func TestSecurityHandler_Handle_NilSecurityManager(t *testing.T) {
	called := false
	baseHandler := &mockHandler{
		handleFunc: func(req *protocol.Request) *protocol.Response {
			called = true
			return &protocol.Response{
				Header: protocol.ResponseHeader{
					RequestID: req.Header.RequestID,
					Status:    protocol.StatusOK,
				},
			}
		},
	}

	// enabled=true but securityManager is nil
	sh := NewSecurityHandler(baseHandler, nil, true)

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 456,
			Type:      protocol.RequestTypeProduce,
		},
		Payload: &protocol.ProduceRequest{
			Topic: "test-topic",
		},
	}

	resp := sh.Handle(req)

	if !called {
		t.Error("Base handler should have been called when security manager is nil")
	}

	if resp.Header.RequestID != 456 {
		t.Errorf("RequestID = %d, want 456", resp.Header.RequestID)
	}
}

func TestSecurityHandler_authenticate(t *testing.T) {
	// Create minimal security manager with authentication disabled
	secMgr, err := security.NewManager(&security.SecurityConfig{}, newTestLogger())
	if err != nil {
		t.Fatalf("Failed to create security manager: %v", err)
	}

	sh := &SecurityHandler{
		securityManager: secMgr,
	}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeProduce,
		},
		Payload: &protocol.ProduceRequest{},
	}

	principal, authErr := sh.authenticate(context.Background(), req)

	if authErr != nil {
		t.Errorf("authenticate failed: %v", authErr)
	}

	if principal == nil {
		t.Fatal("Expected principal to be returned")
	}

	if principal.ID != "anonymous" {
		t.Errorf("Principal ID = %s, want anonymous", principal.ID)
	}

	if principal.Type != security.PrincipalTypeAnonymous {
		t.Errorf("Principal Type = %s, want anonymous", principal.Type)
	}
}

func TestSecurityHandler_authenticate_Enabled(t *testing.T) {
	// Create security manager with SASL enabled (which enables authentication)
	secMgr, err := security.NewManager(&security.SecurityConfig{
		SASL: &security.SASLConfig{
			Enabled: true,
		},
	}, newTestLogger())
	if err != nil {
		t.Fatalf("Failed to create security manager: %v", err)
	}

	sh := &SecurityHandler{
		securityManager: secMgr,
	}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeProduce,
		},
		Payload: &protocol.ProduceRequest{},
	}

	// Even with authentication enabled, should return anonymous for now (TODO in code)
	principal, authErr := sh.authenticate(context.Background(), req)

	if authErr != nil {
		t.Errorf("authenticate failed: %v", authErr)
	}

	if principal == nil {
		t.Fatal("Expected principal to be returned")
	}

	if principal.ID != "anonymous" {
		t.Errorf("Principal ID = %s, want anonymous", principal.ID)
	}
}

func TestSecurityHandler_getActionAndResource_Produce(t *testing.T) {
	sh := &SecurityHandler{}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeProduce,
		},
		Payload: &protocol.ProduceRequest{
			Topic: "my-topic",
		},
	}

	action, resource := sh.getActionAndResource(req)

	if action != security.ActionTopicWrite {
		t.Errorf("Action = %s, want TopicWrite", action)
	}

	if resource.Type != security.ResourceTypeTopic {
		t.Errorf("Resource Type = %s, want Topic", resource.Type)
	}

	if resource.Name != "my-topic" {
		t.Errorf("Resource Name = %s, want my-topic", resource.Name)
	}

	if resource.PatternType != security.PatternTypeLiteral {
		t.Errorf("PatternType = %s, want Literal", resource.PatternType)
	}
}

func TestSecurityHandler_getActionAndResource_Fetch(t *testing.T) {
	sh := &SecurityHandler{}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeFetch,
		},
		Payload: &protocol.FetchRequest{
			Topic: "read-topic",
		},
	}

	action, resource := sh.getActionAndResource(req)

	if action != security.ActionTopicRead {
		t.Errorf("Action = %s, want TopicRead", action)
	}

	if resource.Name != "read-topic" {
		t.Errorf("Resource Name = %s, want read-topic", resource.Name)
	}
}

func TestSecurityHandler_getActionAndResource_GetOffset(t *testing.T) {
	sh := &SecurityHandler{}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeGetOffset,
		},
		Payload: &protocol.GetOffsetRequest{
			Topic: "offset-topic",
		},
	}

	action, resource := sh.getActionAndResource(req)

	if action != security.ActionTopicDescribe {
		t.Errorf("Action = %s, want TopicDescribe", action)
	}

	if resource.Name != "offset-topic" {
		t.Errorf("Resource Name = %s, want offset-topic", resource.Name)
	}
}

func TestSecurityHandler_getActionAndResource_CreateTopic(t *testing.T) {
	sh := &SecurityHandler{}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeCreateTopic,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic: "new-topic",
		},
	}

	action, resource := sh.getActionAndResource(req)

	if action != security.ActionTopicCreate {
		t.Errorf("Action = %s, want TopicCreate", action)
	}

	if resource.Name != "new-topic" {
		t.Errorf("Resource Name = %s, want new-topic", resource.Name)
	}
}

func TestSecurityHandler_getActionAndResource_DeleteTopic(t *testing.T) {
	sh := &SecurityHandler{}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeDeleteTopic,
		},
		Payload: &protocol.DeleteTopicRequest{
			Topic: "old-topic",
		},
	}

	action, resource := sh.getActionAndResource(req)

	if action != security.ActionTopicDelete {
		t.Errorf("Action = %s, want TopicDelete", action)
	}

	if resource.Name != "old-topic" {
		t.Errorf("Resource Name = %s, want old-topic", resource.Name)
	}
}

func TestSecurityHandler_getActionAndResource_ListTopics(t *testing.T) {
	sh := &SecurityHandler{}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeListTopics,
		},
		Payload: &protocol.ListTopicsRequest{},
	}

	action, resource := sh.getActionAndResource(req)

	if action != security.ActionClusterDescribe {
		t.Errorf("Action = %s, want ClusterDescribe", action)
	}

	if resource.Type != security.ResourceTypeCluster {
		t.Errorf("Resource Type = %s, want Cluster", resource.Type)
	}

	if resource.Name != "cluster" {
		t.Errorf("Resource Name = %s, want cluster", resource.Name)
	}
}

func TestSecurityHandler_getActionAndResource_HealthCheck(t *testing.T) {
	sh := &SecurityHandler{}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestTypeHealthCheck,
		},
		Payload: &protocol.HealthCheckRequest{},
	}

	action, resource := sh.getActionAndResource(req)

	if action != security.ActionClusterDescribe {
		t.Errorf("Action = %s, want ClusterDescribe", action)
	}

	if resource.Name != "cluster" {
		t.Errorf("Resource Name = %s, want cluster", resource.Name)
	}
}

func TestSecurityHandler_getActionAndResource_Unknown(t *testing.T) {
	sh := &SecurityHandler{}

	// Use a request type that falls into default case
	// Using a valid but unhandled RequestType value
	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type: protocol.RequestType(99), // Cast to RequestType, but use value that's not handled
		},
		Payload: nil,
	}

	action, resource := sh.getActionAndResource(req)

	if action != security.ActionClusterAction {
		t.Errorf("Action = %s, want ClusterAction", action)
	}

	if resource.Type != security.ResourceTypeCluster {
		t.Errorf("Resource Type = %s, want Cluster", resource.Type)
	}
}

func TestSecurityHandler_GetStats(t *testing.T) {
	baseHandler := &mockHandler{}
	sh := NewSecurityHandler(baseHandler, nil, false)

	// Initially zero
	stats := sh.GetStats()
	if stats["requests_handled"] != 0 {
		t.Errorf("requests_handled = %d, want 0", stats["requests_handled"])
	}
	if stats["auth_failures"] != 0 {
		t.Errorf("auth_failures = %d, want 0", stats["auth_failures"])
	}
	if stats["authz_denials"] != 0 {
		t.Errorf("authz_denials = %d, want 0", stats["authz_denials"])
	}

	// Handle a request
	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeProduce,
		},
		Payload: &protocol.ProduceRequest{},
	}
	sh.Handle(req)

	// Check stats updated
	stats = sh.GetStats()
	if stats["requests_handled"] != 1 {
		t.Errorf("requests_handled = %d, want 1", stats["requests_handled"])
	}
}

func TestSecurityHandler_authenticationFailedResponse(t *testing.T) {
	sh := &SecurityHandler{}

	err := fmt.Errorf("invalid credentials")
	resp := sh.authenticationFailedResponse(123, err)

	if resp.Header.RequestID != 123 {
		t.Errorf("RequestID = %d, want 123", resp.Header.RequestID)
	}

	if resp.Header.Status != protocol.StatusError {
		t.Errorf("Status = %v, want StatusError", resp.Header.Status)
	}

	if resp.Header.ErrorCode != protocol.ErrAuthenticationFailed {
		t.Errorf("ErrorCode = %v, want ErrAuthenticationFailed", resp.Header.ErrorCode)
	}

	errorResp := resp.Payload.(*protocol.ErrorResponse)
	if errorResp.ErrorCode != protocol.ErrAuthenticationFailed {
		t.Errorf("Payload ErrorCode = %v, want ErrAuthenticationFailed", errorResp.ErrorCode)
	}

	if errorResp.Message == "" {
		t.Error("Expected error message in response")
	}

	// Verify the error message contains the original error
	if !containsString(errorResp.Message, "invalid credentials") {
		t.Errorf("Expected message to contain 'invalid credentials', got: %s", errorResp.Message)
	}
}

func TestSecurityHandler_authorizationDeniedResponse(t *testing.T) {
	sh := &SecurityHandler{}

	principal := &security.Principal{
		ID:   "test-user",
		Type: security.PrincipalTypeUser,
	}

	action := security.ActionTopicWrite
	resource := security.Resource{
		Type:        security.ResourceTypeTopic,
		Name:        "restricted-topic",
		PatternType: security.PatternTypeLiteral,
	}

	resp := sh.authorizationDeniedResponse(456, principal, action, resource)

	if resp.Header.RequestID != 456 {
		t.Errorf("RequestID = %d, want 456", resp.Header.RequestID)
	}

	if resp.Header.Status != protocol.StatusError {
		t.Errorf("Status = %v, want StatusError", resp.Header.Status)
	}

	if resp.Header.ErrorCode != protocol.ErrAuthorizationFailed {
		t.Errorf("ErrorCode = %v, want ErrAuthorizationFailed", resp.Header.ErrorCode)
	}

	errorResp := resp.Payload.(*protocol.ErrorResponse)
	if errorResp.ErrorCode != protocol.ErrAuthorizationFailed {
		t.Errorf("Payload ErrorCode = %v, want ErrAuthorizationFailed", errorResp.ErrorCode)
	}

	if errorResp.Message == "" {
		t.Error("Expected error message in response")
	}

	// Verify message contains principal, action, and resource details
	expectedSubstrings := []string{"test-user", "restricted-topic"}
	for _, substr := range expectedSubstrings {
		if !containsString(errorResp.Message, substr) {
			t.Errorf("Expected message to contain '%s', got: %s", substr, errorResp.Message)
		}
	}

	// Check for action (could be formatted as TOPIC_WRITE or TopicWrite)
	if !containsString(errorResp.Message, "TOPIC_WRITE") && !containsString(errorResp.Message, "TopicWrite") {
		t.Errorf("Expected message to contain action, got: %s", errorResp.Message)
	}
}

func TestSecurityHandler_errorResponse(t *testing.T) {
	sh := &SecurityHandler{}

	resp := sh.errorResponse(789, protocol.ErrStorageError, "test error message")

	if resp.Header.RequestID != 789 {
		t.Errorf("RequestID = %d, want 789", resp.Header.RequestID)
	}

	if resp.Header.Status != protocol.StatusError {
		t.Errorf("Status = %v, want StatusError", resp.Header.Status)
	}

	if resp.Header.ErrorCode != protocol.ErrStorageError {
		t.Errorf("ErrorCode = %v, want ErrStorageError", resp.Header.ErrorCode)
	}

	errorResp := resp.Payload.(*protocol.ErrorResponse)
	if errorResp.ErrorCode != protocol.ErrStorageError {
		t.Errorf("Payload ErrorCode = %v, want ErrStorageError", errorResp.ErrorCode)
	}

	if errorResp.Message != "test error message" {
		t.Errorf("Message = %s, want 'test error message'", errorResp.Message)
	}
}

func TestSecurityHandler_Handle_WithAuthorizationDenied(t *testing.T) {
	// Create security manager with authorization enabled but deny all
	secMgr, err := security.NewManager(&security.SecurityConfig{
		AuthzEnabled: true,
		// No ACLs configured, so authorization will be denied by default
	}, newTestLogger())
	if err != nil {
		t.Fatalf("Failed to create security manager: %v", err)
	}

	baseHandler := &mockHandler{
		handleFunc: func(req *protocol.Request) *protocol.Response {
			t.Error("Base handler should not be called when authorization is denied")
			return &protocol.Response{
				Header: protocol.ResponseHeader{
					RequestID: req.Header.RequestID,
					Status:    protocol.StatusOK,
				},
			}
		},
	}

	sh := NewSecurityHandler(baseHandler, secMgr, true)

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 999,
			Type:      protocol.RequestTypeProduce,
		},
		Payload: &protocol.ProduceRequest{
			Topic: "test-topic",
		},
	}

	resp := sh.Handle(req)

	if resp.Header.Status != protocol.StatusError {
		t.Errorf("Expected error status for denied authorization, got %v", resp.Header.Status)
	}

	if resp.Header.ErrorCode != protocol.ErrAuthorizationFailed {
		t.Errorf("Expected ErrAuthorizationFailed, got %v", resp.Header.ErrorCode)
	}

	stats := sh.GetStats()
	if stats["authz_denials"] != 1 {
		t.Errorf("authz_denials = %d, want 1", stats["authz_denials"])
	}
}

func TestSecurityHandler_Handle_WithAuditEnabled(t *testing.T) {
	// Create security manager with audit enabled
	secMgr, err := security.NewManager(&security.SecurityConfig{
		AuthzEnabled:   false, // Disable authz to allow requests through
		AuditEnabled:   true,
		AllowAnonymous: true,
	}, newTestLogger())
	if err != nil {
		t.Fatalf("Failed to create security manager: %v", err)
	}

	called := false
	baseHandler := &mockHandler{
		handleFunc: func(req *protocol.Request) *protocol.Response {
			called = true
			return &protocol.Response{
				Header: protocol.ResponseHeader{
					RequestID: req.Header.RequestID,
					Status:    protocol.StatusOK,
				},
			}
		},
	}

	sh := NewSecurityHandler(baseHandler, secMgr, true)

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 111,
			Type:      protocol.RequestTypeProduce,
		},
		Payload: &protocol.ProduceRequest{
			Topic: "test-topic",
		},
	}

	resp := sh.Handle(req)

	if !called {
		t.Error("Base handler should have been called")
	}

	if resp.Header.Status != protocol.StatusOK {
		t.Errorf("Expected OK status, got %v", resp.Header.Status)
	}

	// Verify audit was called (no errors should occur)
	if resp.Header.ErrorCode != protocol.ErrNone {
		t.Errorf("Expected no error code, got %v", resp.Header.ErrorCode)
	}
}

// containsString checks if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
