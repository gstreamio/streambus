package server

import (
	"context"
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
