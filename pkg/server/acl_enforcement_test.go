package server

import (
	"testing"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/security"
)

// newAuthzSecurityManager creates a security manager with authorization enabled.
func newAuthzSecurityManager(t *testing.T) *security.Manager {
	t.Helper()
	mgr, err := security.NewManager(&security.SecurityConfig{
		AuthzEnabled: true,
	}, newTestLogger())
	if err != nil {
		t.Fatalf("Failed to create security manager: %v", err)
	}
	return mgr
}

// passThrough returns a mockHandler that records whether it was called.
func passThrough(called *bool) *mockHandler {
	return &mockHandler{
		handleFunc: func(req *protocol.Request) *protocol.Response {
			*called = true
			return &protocol.Response{
				Header: protocol.ResponseHeader{
					RequestID: req.Header.RequestID,
					Status:    protocol.StatusOK,
				},
			}
		},
	}
}

func TestACLEnforcement_PermissiveDefault_NoACLs(t *testing.T) {
	// When authorization is enabled but no ACL entries exist, all requests
	// should be allowed (permissive default).
	secMgr := newAuthzSecurityManager(t)

	tests := []struct {
		name    string
		reqType protocol.RequestType
		payload interface{}
	}{
		{
			name:    "produce allowed with no ACLs",
			reqType: protocol.RequestTypeProduce,
			payload: &protocol.ProduceRequest{Topic: "any-topic"},
		},
		{
			name:    "fetch allowed with no ACLs",
			reqType: protocol.RequestTypeFetch,
			payload: &protocol.FetchRequest{Topic: "any-topic"},
		},
		{
			name:    "create topic allowed with no ACLs",
			reqType: protocol.RequestTypeCreateTopic,
			payload: &protocol.CreateTopicRequest{Topic: "new-topic"},
		},
		{
			name:    "delete topic allowed with no ACLs",
			reqType: protocol.RequestTypeDeleteTopic,
			payload: &protocol.DeleteTopicRequest{Topic: "old-topic"},
		},
		{
			name:    "list topics allowed with no ACLs",
			reqType: protocol.RequestTypeListTopics,
			payload: &protocol.ListTopicsRequest{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			sh := NewSecurityHandler(passThrough(&called), secMgr, true)

			req := &protocol.Request{
				Header:  protocol.RequestHeader{RequestID: 1, Type: tt.reqType},
				Payload: tt.payload,
			}

			resp := sh.Handle(req)

			if !called {
				t.Error("Base handler should have been called (permissive default)")
			}
			if resp.Header.Status != protocol.StatusOK {
				t.Errorf("Expected OK status, got %v", resp.Header.Status)
			}
		})
	}
}

func TestACLEnforcement_ProduceRequiresWrite(t *testing.T) {
	secMgr := newAuthzSecurityManager(t)

	// Grant READ (not WRITE) on "orders" to all principals
	err := secMgr.AddACL(&security.ACLEntry{
		ID:           "read-orders",
		Principal:    "*",
		ResourceType: security.ResourceTypeTopic,
		ResourceName: "orders",
		PatternType:  security.PatternTypeLiteral,
		Action:       security.ActionTopicRead,
		Permission:   security.PermissionAllow,
	})
	if err != nil {
		t.Fatalf("Failed to add ACL: %v", err)
	}

	called := false
	sh := NewSecurityHandler(passThrough(&called), secMgr, true)

	req := &protocol.Request{
		Header:  protocol.RequestHeader{RequestID: 1, Type: protocol.RequestTypeProduce},
		Payload: &protocol.ProduceRequest{Topic: "orders"},
	}

	resp := sh.Handle(req)

	if called {
		t.Error("Base handler should NOT have been called (no WRITE permission)")
	}
	if resp.Header.ErrorCode != protocol.ErrAuthorizationFailed {
		t.Errorf("Expected ErrAuthorizationFailed, got %v", resp.Header.ErrorCode)
	}
}

func TestACLEnforcement_ProduceAllowedWithWrite(t *testing.T) {
	secMgr := newAuthzSecurityManager(t)

	// Grant WRITE on "orders" to all principals
	err := secMgr.AddACL(&security.ACLEntry{
		ID:           "write-orders",
		Principal:    "*",
		ResourceType: security.ResourceTypeTopic,
		ResourceName: "orders",
		PatternType:  security.PatternTypeLiteral,
		Action:       security.ActionTopicWrite,
		Permission:   security.PermissionAllow,
	})
	if err != nil {
		t.Fatalf("Failed to add ACL: %v", err)
	}

	called := false
	sh := NewSecurityHandler(passThrough(&called), secMgr, true)

	req := &protocol.Request{
		Header:  protocol.RequestHeader{RequestID: 1, Type: protocol.RequestTypeProduce},
		Payload: &protocol.ProduceRequest{Topic: "orders"},
	}

	resp := sh.Handle(req)

	if !called {
		t.Error("Base handler should have been called (WRITE granted)")
	}
	if resp.Header.Status != protocol.StatusOK {
		t.Errorf("Expected OK status, got %v", resp.Header.Status)
	}
}

func TestACLEnforcement_FetchRequiresRead(t *testing.T) {
	secMgr := newAuthzSecurityManager(t)

	// Grant WRITE (not READ) on "events" to all principals
	err := secMgr.AddACL(&security.ACLEntry{
		ID:           "write-events",
		Principal:    "*",
		ResourceType: security.ResourceTypeTopic,
		ResourceName: "events",
		PatternType:  security.PatternTypeLiteral,
		Action:       security.ActionTopicWrite,
		Permission:   security.PermissionAllow,
	})
	if err != nil {
		t.Fatalf("Failed to add ACL: %v", err)
	}

	called := false
	sh := NewSecurityHandler(passThrough(&called), secMgr, true)

	req := &protocol.Request{
		Header:  protocol.RequestHeader{RequestID: 2, Type: protocol.RequestTypeFetch},
		Payload: &protocol.FetchRequest{Topic: "events"},
	}

	resp := sh.Handle(req)

	if called {
		t.Error("Base handler should NOT have been called (no READ permission)")
	}
	if resp.Header.ErrorCode != protocol.ErrAuthorizationFailed {
		t.Errorf("Expected ErrAuthorizationFailed, got %v", resp.Header.ErrorCode)
	}
}

func TestACLEnforcement_FetchAllowedWithRead(t *testing.T) {
	secMgr := newAuthzSecurityManager(t)

	// Grant READ on "events" to all principals
	err := secMgr.AddACL(&security.ACLEntry{
		ID:           "read-events",
		Principal:    "*",
		ResourceType: security.ResourceTypeTopic,
		ResourceName: "events",
		PatternType:  security.PatternTypeLiteral,
		Action:       security.ActionTopicRead,
		Permission:   security.PermissionAllow,
	})
	if err != nil {
		t.Fatalf("Failed to add ACL: %v", err)
	}

	called := false
	sh := NewSecurityHandler(passThrough(&called), secMgr, true)

	req := &protocol.Request{
		Header:  protocol.RequestHeader{RequestID: 2, Type: protocol.RequestTypeFetch},
		Payload: &protocol.FetchRequest{Topic: "events"},
	}

	resp := sh.Handle(req)

	if !called {
		t.Error("Base handler should have been called (READ granted)")
	}
	if resp.Header.Status != protocol.StatusOK {
		t.Errorf("Expected OK status, got %v", resp.Header.Status)
	}
}

func TestACLEnforcement_CreateTopicRequiresCreate(t *testing.T) {
	secMgr := newAuthzSecurityManager(t)

	// Grant READ on "new-topic" but NOT CREATE
	err := secMgr.AddACL(&security.ACLEntry{
		ID:           "read-new-topic",
		Principal:    "*",
		ResourceType: security.ResourceTypeTopic,
		ResourceName: "new-topic",
		PatternType:  security.PatternTypeLiteral,
		Action:       security.ActionTopicRead,
		Permission:   security.PermissionAllow,
	})
	if err != nil {
		t.Fatalf("Failed to add ACL: %v", err)
	}

	called := false
	sh := NewSecurityHandler(passThrough(&called), secMgr, true)

	req := &protocol.Request{
		Header:  protocol.RequestHeader{RequestID: 3, Type: protocol.RequestTypeCreateTopic},
		Payload: &protocol.CreateTopicRequest{Topic: "new-topic"},
	}

	resp := sh.Handle(req)

	if called {
		t.Error("Base handler should NOT have been called (no CREATE permission)")
	}
	if resp.Header.ErrorCode != protocol.ErrAuthorizationFailed {
		t.Errorf("Expected ErrAuthorizationFailed, got %v", resp.Header.ErrorCode)
	}
}

func TestACLEnforcement_CreateTopicAllowedWithCreate(t *testing.T) {
	secMgr := newAuthzSecurityManager(t)

	err := secMgr.AddACL(&security.ACLEntry{
		ID:           "create-new-topic",
		Principal:    "*",
		ResourceType: security.ResourceTypeTopic,
		ResourceName: "new-topic",
		PatternType:  security.PatternTypeLiteral,
		Action:       security.ActionTopicCreate,
		Permission:   security.PermissionAllow,
	})
	if err != nil {
		t.Fatalf("Failed to add ACL: %v", err)
	}

	called := false
	sh := NewSecurityHandler(passThrough(&called), secMgr, true)

	req := &protocol.Request{
		Header:  protocol.RequestHeader{RequestID: 3, Type: protocol.RequestTypeCreateTopic},
		Payload: &protocol.CreateTopicRequest{Topic: "new-topic"},
	}

	resp := sh.Handle(req)

	if !called {
		t.Error("Base handler should have been called (CREATE granted)")
	}
	if resp.Header.Status != protocol.StatusOK {
		t.Errorf("Expected OK status, got %v", resp.Header.Status)
	}
}

func TestACLEnforcement_DeleteTopicRequiresDelete(t *testing.T) {
	secMgr := newAuthzSecurityManager(t)

	// Grant WRITE but NOT DELETE
	err := secMgr.AddACL(&security.ACLEntry{
		ID:           "write-old-topic",
		Principal:    "*",
		ResourceType: security.ResourceTypeTopic,
		ResourceName: "old-topic",
		PatternType:  security.PatternTypeLiteral,
		Action:       security.ActionTopicWrite,
		Permission:   security.PermissionAllow,
	})
	if err != nil {
		t.Fatalf("Failed to add ACL: %v", err)
	}

	called := false
	sh := NewSecurityHandler(passThrough(&called), secMgr, true)

	req := &protocol.Request{
		Header:  protocol.RequestHeader{RequestID: 4, Type: protocol.RequestTypeDeleteTopic},
		Payload: &protocol.DeleteTopicRequest{Topic: "old-topic"},
	}

	resp := sh.Handle(req)

	if called {
		t.Error("Base handler should NOT have been called (no DELETE permission)")
	}
	if resp.Header.ErrorCode != protocol.ErrAuthorizationFailed {
		t.Errorf("Expected ErrAuthorizationFailed, got %v", resp.Header.ErrorCode)
	}
}

func TestACLEnforcement_DeleteTopicAllowedWithDelete(t *testing.T) {
	secMgr := newAuthzSecurityManager(t)

	err := secMgr.AddACL(&security.ACLEntry{
		ID:           "delete-old-topic",
		Principal:    "*",
		ResourceType: security.ResourceTypeTopic,
		ResourceName: "old-topic",
		PatternType:  security.PatternTypeLiteral,
		Action:       security.ActionTopicDelete,
		Permission:   security.PermissionAllow,
	})
	if err != nil {
		t.Fatalf("Failed to add ACL: %v", err)
	}

	called := false
	sh := NewSecurityHandler(passThrough(&called), secMgr, true)

	req := &protocol.Request{
		Header:  protocol.RequestHeader{RequestID: 4, Type: protocol.RequestTypeDeleteTopic},
		Payload: &protocol.DeleteTopicRequest{Topic: "old-topic"},
	}

	resp := sh.Handle(req)

	if !called {
		t.Error("Base handler should have been called (DELETE granted)")
	}
	if resp.Header.Status != protocol.StatusOK {
		t.Errorf("Expected OK status, got %v", resp.Header.Status)
	}
}

func TestACLEnforcement_DenyOverridesAllow(t *testing.T) {
	secMgr := newAuthzSecurityManager(t)

	// Add ALLOW for WRITE on "sensitive"
	err := secMgr.AddACL(&security.ACLEntry{
		ID:           "allow-write",
		Principal:    "*",
		ResourceType: security.ResourceTypeTopic,
		ResourceName: "sensitive",
		PatternType:  security.PatternTypeLiteral,
		Action:       security.ActionTopicWrite,
		Permission:   security.PermissionAllow,
	})
	if err != nil {
		t.Fatalf("Failed to add ALLOW ACL: %v", err)
	}

	// Add explicit DENY for WRITE on "sensitive" (takes precedence)
	err = secMgr.AddACL(&security.ACLEntry{
		ID:           "deny-write",
		Principal:    "*",
		ResourceType: security.ResourceTypeTopic,
		ResourceName: "sensitive",
		PatternType:  security.PatternTypeLiteral,
		Action:       security.ActionTopicWrite,
		Permission:   security.PermissionDeny,
	})
	if err != nil {
		t.Fatalf("Failed to add DENY ACL: %v", err)
	}

	called := false
	sh := NewSecurityHandler(passThrough(&called), secMgr, true)

	req := &protocol.Request{
		Header:  protocol.RequestHeader{RequestID: 5, Type: protocol.RequestTypeProduce},
		Payload: &protocol.ProduceRequest{Topic: "sensitive"},
	}

	resp := sh.Handle(req)

	if called {
		t.Error("Base handler should NOT be called (DENY overrides ALLOW)")
	}
	if resp.Header.ErrorCode != protocol.ErrAuthorizationFailed {
		t.Errorf("Expected ErrAuthorizationFailed, got %v", resp.Header.ErrorCode)
	}
}

func TestACLEnforcement_WrongTopicDenied(t *testing.T) {
	secMgr := newAuthzSecurityManager(t)

	// Grant WRITE on "allowed-topic" only
	err := secMgr.AddACL(&security.ACLEntry{
		ID:           "write-allowed",
		Principal:    "*",
		ResourceType: security.ResourceTypeTopic,
		ResourceName: "allowed-topic",
		PatternType:  security.PatternTypeLiteral,
		Action:       security.ActionTopicWrite,
		Permission:   security.PermissionAllow,
	})
	if err != nil {
		t.Fatalf("Failed to add ACL: %v", err)
	}

	called := false
	sh := NewSecurityHandler(passThrough(&called), secMgr, true)

	// Try to produce to a different topic
	req := &protocol.Request{
		Header:  protocol.RequestHeader{RequestID: 6, Type: protocol.RequestTypeProduce},
		Payload: &protocol.ProduceRequest{Topic: "other-topic"},
	}

	resp := sh.Handle(req)

	if called {
		t.Error("Base handler should NOT be called (wrong topic)")
	}
	if resp.Header.ErrorCode != protocol.ErrAuthorizationFailed {
		t.Errorf("Expected ErrAuthorizationFailed, got %v", resp.Header.ErrorCode)
	}
}

func TestACLEnforcement_PrefixACL(t *testing.T) {
	secMgr := newAuthzSecurityManager(t)

	// Grant WRITE on topics starting with "logs."
	err := secMgr.AddACL(&security.ACLEntry{
		ID:           "write-logs-prefix",
		Principal:    "*",
		ResourceType: security.ResourceTypeTopic,
		ResourceName: "logs.",
		PatternType:  security.PatternTypePrefix,
		Action:       security.ActionTopicWrite,
		Permission:   security.PermissionAllow,
	})
	if err != nil {
		t.Fatalf("Failed to add ACL: %v", err)
	}

	tests := []struct {
		name        string
		topic       string
		expectAllow bool
	}{
		{"matching prefix", "logs.app1", true},
		{"non-matching", "metrics.app1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			sh := NewSecurityHandler(passThrough(&called), secMgr, true)

			req := &protocol.Request{
				Header:  protocol.RequestHeader{RequestID: 7, Type: protocol.RequestTypeProduce},
				Payload: &protocol.ProduceRequest{Topic: tt.topic},
			}

			resp := sh.Handle(req)

			if called != tt.expectAllow {
				t.Errorf("expected handler called=%v, got %v", tt.expectAllow, called)
			}
			if tt.expectAllow && resp.Header.Status != protocol.StatusOK {
				t.Errorf("Expected OK status, got %v", resp.Header.Status)
			}
			if !tt.expectAllow && resp.Header.ErrorCode != protocol.ErrAuthorizationFailed {
				t.Errorf("Expected ErrAuthorizationFailed, got %v", resp.Header.ErrorCode)
			}
		})
	}
}

func TestACLEnforcement_SecurityDisabledPassesThrough(t *testing.T) {
	// When the security handler is disabled, all requests pass through
	// regardless of ACL configuration.
	called := false
	sh := NewSecurityHandler(passThrough(&called), nil, false)

	req := &protocol.Request{
		Header:  protocol.RequestHeader{RequestID: 8, Type: protocol.RequestTypeProduce},
		Payload: &protocol.ProduceRequest{Topic: "any-topic"},
	}

	resp := sh.Handle(req)

	if !called {
		t.Error("Base handler should have been called (security disabled)")
	}
	if resp.Header.Status != protocol.StatusOK {
		t.Errorf("Expected OK status, got %v", resp.Header.Status)
	}
}

func TestACLEnforcement_DenialCountsInStats(t *testing.T) {
	secMgr := newAuthzSecurityManager(t)

	// Add an ACL entry for another topic so entries exist (no permissive default)
	err := secMgr.AddACL(&security.ACLEntry{
		ID:           "unrelated",
		Principal:    "other-user",
		ResourceType: security.ResourceTypeTopic,
		ResourceName: "unrelated-topic",
		PatternType:  security.PatternTypeLiteral,
		Action:       security.ActionTopicWrite,
		Permission:   security.PermissionAllow,
	})
	if err != nil {
		t.Fatalf("Failed to add ACL: %v", err)
	}

	called := false
	sh := NewSecurityHandler(passThrough(&called), secMgr, true)

	// Send 3 produce requests that will all be denied
	for i := 0; i < 3; i++ {
		req := &protocol.Request{
			Header:  protocol.RequestHeader{RequestID: uint64(i), Type: protocol.RequestTypeProduce},
			Payload: &protocol.ProduceRequest{Topic: "blocked-topic"},
		}
		sh.Handle(req)
	}

	stats := sh.GetStats()
	if stats["authz_denials"] != 3 {
		t.Errorf("authz_denials = %d, want 3", stats["authz_denials"])
	}
	if stats["requests_handled"] != 3 {
		t.Errorf("requests_handled = %d, want 3", stats["requests_handled"])
	}
}

func TestACLEnforcement_MultipleOperationsSameTopicTable(t *testing.T) {
	secMgr := newAuthzSecurityManager(t)

	// Grant WRITE and READ on "data-topic"
	for _, entry := range []*security.ACLEntry{
		{
			ID: "write-data", Principal: "*",
			ResourceType: security.ResourceTypeTopic, ResourceName: "data-topic",
			PatternType: security.PatternTypeLiteral, Action: security.ActionTopicWrite,
			Permission: security.PermissionAllow,
		},
		{
			ID: "read-data", Principal: "*",
			ResourceType: security.ResourceTypeTopic, ResourceName: "data-topic",
			PatternType: security.PatternTypeLiteral, Action: security.ActionTopicRead,
			Permission: security.PermissionAllow,
		},
	} {
		if err := secMgr.AddACL(entry); err != nil {
			t.Fatalf("Failed to add ACL: %v", err)
		}
	}

	tests := []struct {
		name        string
		reqType     protocol.RequestType
		payload     interface{}
		expectAllow bool
	}{
		{
			name:        "produce allowed (WRITE granted)",
			reqType:     protocol.RequestTypeProduce,
			payload:     &protocol.ProduceRequest{Topic: "data-topic"},
			expectAllow: true,
		},
		{
			name:        "fetch allowed (READ granted)",
			reqType:     protocol.RequestTypeFetch,
			payload:     &protocol.FetchRequest{Topic: "data-topic"},
			expectAllow: true,
		},
		{
			name:        "create denied (no CREATE granted)",
			reqType:     protocol.RequestTypeCreateTopic,
			payload:     &protocol.CreateTopicRequest{Topic: "data-topic"},
			expectAllow: false,
		},
		{
			name:        "delete denied (no DELETE granted)",
			reqType:     protocol.RequestTypeDeleteTopic,
			payload:     &protocol.DeleteTopicRequest{Topic: "data-topic"},
			expectAllow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			sh := NewSecurityHandler(passThrough(&called), secMgr, true)

			req := &protocol.Request{
				Header:  protocol.RequestHeader{RequestID: 1, Type: tt.reqType},
				Payload: tt.payload,
			}

			resp := sh.Handle(req)

			if called != tt.expectAllow {
				t.Errorf("expected handler called=%v, got %v", tt.expectAllow, called)
			}
			if tt.expectAllow && resp.Header.Status != protocol.StatusOK {
				t.Errorf("Expected OK, got %v", resp.Header.Status)
			}
			if !tt.expectAllow && resp.Header.ErrorCode != protocol.ErrAuthorizationFailed {
				t.Errorf("Expected ErrAuthorizationFailed, got %v", resp.Header.ErrorCode)
			}
		})
	}
}
