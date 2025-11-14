package security

import (
	"context"
	"testing"
	"time"
)

func TestNewACLAuthorizer(t *testing.T) {
	config := &SecurityConfig{
		AuthzEnabled: true,
		SuperUsers:   []string{"admin"},
	}

	auth := NewACLAuthorizer(config)

	if auth == nil {
		t.Fatal("Expected non-nil authorizer")
	}

	if auth.config != config {
		t.Error("Config not set correctly")
	}

	if auth.entries == nil {
		t.Error("Entries map not initialized")
	}
}

func TestACLAuthorizer_Authorize_Disabled(t *testing.T) {
	config := &SecurityConfig{
		AuthzEnabled: false,
	}

	auth := NewACLAuthorizer(config)
	ctx := context.Background()

	principal := &Principal{
		ID:   "user1",
		Type: PrincipalTypeUser,
	}

	// Should allow all when disabled
	allowed, err := auth.Authorize(ctx, principal, ActionTopicWrite, Resource{
		Type: ResourceTypeTopic,
		Name: "test-topic",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !allowed {
		t.Error("Expected authorization to be allowed when disabled")
	}
}

func TestACLAuthorizer_Authorize_SuperUser(t *testing.T) {
	config := &SecurityConfig{
		AuthzEnabled: true,
		SuperUsers:   []string{"admin", "root"},
	}

	auth := NewACLAuthorizer(config)
	ctx := context.Background()

	principal := &Principal{
		ID:   "admin",
		Type: PrincipalTypeUser,
	}

	// Super user should bypass all ACL checks
	allowed, err := auth.Authorize(ctx, principal, ActionTopicWrite, Resource{
		Type: ResourceTypeTopic,
		Name: "any-topic",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !allowed {
		t.Error("Expected super user to be allowed")
	}
}

func TestACLAuthorizer_Authorize_SystemInternal(t *testing.T) {
	config := &SecurityConfig{
		AuthzEnabled: true,
	}

	auth := NewACLAuthorizer(config)
	ctx := context.Background()

	principal := &Principal{
		ID:   "system",
		Type: PrincipalTypeSystemInternal,
	}

	// System internal should bypass all ACL checks
	allowed, err := auth.Authorize(ctx, principal, ActionTopicWrite, Resource{
		Type: ResourceTypeTopic,
		Name: "any-topic",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !allowed {
		t.Error("Expected system internal to be allowed")
	}
}

func TestACLAuthorizer_Authorize_DenyTakesPrecedence(t *testing.T) {
	config := &SecurityConfig{
		AuthzEnabled: true,
	}

	auth := NewACLAuthorizer(config)
	ctx := context.Background()

	// Add ALLOW rule
	allowACL := &ACLEntry{
		ID:           "allow-1",
		Principal:    "user1",
		ResourceType: ResourceTypeTopic,
		ResourceName: "test-topic",
		PatternType:  PatternTypeLiteral,
		Action:       ActionTopicWrite,
		Permission:   PermissionAllow,
		CreatedAt:    time.Now(),
	}
	_ = auth.AddACL(allowACL)

	// Add DENY rule (should take precedence)
	denyACL := &ACLEntry{
		ID:           "deny-1",
		Principal:    "user1",
		ResourceType: ResourceTypeTopic,
		ResourceName: "test-topic",
		PatternType:  PatternTypeLiteral,
		Action:       ActionTopicWrite,
		Permission:   PermissionDeny,
		CreatedAt:    time.Now(),
	}
	_ = auth.AddACL(denyACL)

	principal := &Principal{
		ID:   "user1",
		Type: PrincipalTypeUser,
	}

	allowed, err := auth.Authorize(ctx, principal, ActionTopicWrite, Resource{
		Type: ResourceTypeTopic,
		Name: "test-topic",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if allowed {
		t.Error("Expected DENY to take precedence over ALLOW")
	}
}

func TestACLAuthorizer_Authorize_DefaultDeny(t *testing.T) {
	config := &SecurityConfig{
		AuthzEnabled: true,
	}

	auth := NewACLAuthorizer(config)
	ctx := context.Background()

	principal := &Principal{
		ID:   "user1",
		Type: PrincipalTypeUser,
	}

	// No ACLs defined, should default deny
	allowed, err := auth.Authorize(ctx, principal, ActionTopicWrite, Resource{
		Type: ResourceTypeTopic,
		Name: "test-topic",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if allowed {
		t.Error("Expected default deny when no matching ACL")
	}
}

func TestACLAuthorizer_Authorize_LiteralMatch(t *testing.T) {
	config := &SecurityConfig{
		AuthzEnabled: true,
	}

	auth := NewACLAuthorizer(config)
	ctx := context.Background()

	acl := &ACLEntry{
		ID:           "literal-1",
		Principal:    "user1",
		ResourceType: ResourceTypeTopic,
		ResourceName: "orders",
		PatternType:  PatternTypeLiteral,
		Action:       ActionTopicWrite,
		Permission:   PermissionAllow,
	}
	_ = auth.AddACL(acl)

	principal := &Principal{
		ID:   "user1",
		Type: PrincipalTypeUser,
	}

	// Exact match should be allowed
	allowed, _ := auth.Authorize(ctx, principal, ActionTopicWrite, Resource{
		Type: ResourceTypeTopic,
		Name: "orders",
	})

	if !allowed {
		t.Error("Expected literal match to be allowed")
	}

	// Different topic should be denied
	allowed, _ = auth.Authorize(ctx, principal, ActionTopicWrite, Resource{
		Type: ResourceTypeTopic,
		Name: "payments",
	})

	if allowed {
		t.Error("Expected non-matching topic to be denied")
	}
}

func TestACLAuthorizer_Authorize_PrefixMatch(t *testing.T) {
	config := &SecurityConfig{
		AuthzEnabled: true,
	}

	auth := NewACLAuthorizer(config)
	ctx := context.Background()

	acl := &ACLEntry{
		ID:           "prefix-1",
		Principal:    "user1",
		ResourceType: ResourceTypeTopic,
		ResourceName: "logs.",
		PatternType:  PatternTypePrefix,
		Action:       ActionTopicWrite,
		Permission:   PermissionAllow,
	}
	_ = auth.AddACL(acl)

	principal := &Principal{
		ID:   "user1",
		Type: PrincipalTypeUser,
	}

	tests := []struct {
		name     string
		topic    string
		expected bool
	}{
		{"exact prefix", "logs.", true},
		{"with suffix", "logs.app1", true},
		{"nested", "logs.app1.errors", true},
		{"no match", "metrics.app1", false},
		{"partial match", "log", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, _ := auth.Authorize(ctx, principal, ActionTopicWrite, Resource{
				Type: ResourceTypeTopic,
				Name: tt.topic,
			})

			if allowed != tt.expected {
				t.Errorf("Expected %v for topic '%s', got %v", tt.expected, tt.topic, allowed)
			}
		})
	}
}

func TestACLAuthorizer_Authorize_WildcardMatch(t *testing.T) {
	config := &SecurityConfig{
		AuthzEnabled: true,
	}

	auth := NewACLAuthorizer(config)
	ctx := context.Background()

	acl := &ACLEntry{
		ID:           "wildcard-1",
		Principal:    "user1",
		ResourceType: ResourceTypeTopic,
		ResourceName: "app-*.logs",
		PatternType:  PatternTypeWildcard,
		Action:       ActionTopicWrite,
		Permission:   PermissionAllow,
	}
	_ = auth.AddACL(acl)

	principal := &Principal{
		ID:   "user1",
		Type: PrincipalTypeUser,
	}

	tests := []struct {
		name     string
		topic    string
		expected bool
	}{
		{"match wildcard", "app-1.logs", true},
		{"match wildcard 2", "app-test.logs", true},
		{"no match", "app-1-logs", false},
		{"no match prefix", "other-app-1.logs", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, _ := auth.Authorize(ctx, principal, ActionTopicWrite, Resource{
				Type: ResourceTypeTopic,
				Name: tt.topic,
			})

			if allowed != tt.expected {
				t.Errorf("Expected %v for topic '%s', got %v", tt.expected, tt.topic, allowed)
			}
		})
	}
}

func TestACLAuthorizer_Authorize_WildcardPrincipal(t *testing.T) {
	config := &SecurityConfig{
		AuthzEnabled: true,
	}

	auth := NewACLAuthorizer(config)
	ctx := context.Background()

	acl := &ACLEntry{
		ID:           "wildcard-principal",
		Principal:    "*",
		ResourceType: ResourceTypeTopic,
		ResourceName: "public-topic",
		PatternType:  PatternTypeLiteral,
		Action:       ActionTopicRead,
		Permission:   PermissionAllow,
	}
	_ = auth.AddACL(acl)

	tests := []struct {
		name      string
		principal string
	}{
		{"user1", "user1"},
		{"user2", "user2"},
		{"service-1", "service-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			principal := &Principal{
				ID:   tt.principal,
				Type: PrincipalTypeUser,
			}

			allowed, _ := auth.Authorize(ctx, principal, ActionTopicRead, Resource{
				Type: ResourceTypeTopic,
				Name: "public-topic",
			})

			if !allowed {
				t.Errorf("Expected wildcard principal to match %s", tt.principal)
			}
		})
	}
}

func TestACLAuthorizer_Authorize_GroupMatch(t *testing.T) {
	config := &SecurityConfig{
		AuthzEnabled: true,
	}

	auth := NewACLAuthorizer(config)
	ctx := context.Background()

	acl := &ACLEntry{
		ID:           "group-1",
		Principal:    "group:developers",
		ResourceType: ResourceTypeTopic,
		ResourceName: "dev-topic",
		PatternType:  PatternTypeLiteral,
		Action:       ActionTopicWrite,
		Permission:   PermissionAllow,
	}
	_ = auth.AddACL(acl)

	principal := &Principal{
		ID:   "user1",
		Type: PrincipalTypeUser,
		Attributes: map[string]string{
			"groups": "developers",
		},
	}

	allowed, _ := auth.Authorize(ctx, principal, ActionTopicWrite, Resource{
		Type: ResourceTypeTopic,
		Name: "dev-topic",
	})

	if !allowed {
		t.Error("Expected group match to be allowed")
	}

	// User not in group
	principal2 := &Principal{
		ID:   "user2",
		Type: PrincipalTypeUser,
		Attributes: map[string]string{
			"groups": "testers",
		},
	}

	allowed, _ = auth.Authorize(ctx, principal2, ActionTopicWrite, Resource{
		Type: ResourceTypeTopic,
		Name: "dev-topic",
	})

	if allowed {
		t.Error("Expected user not in group to be denied")
	}
}

func TestACLAuthorizer_Authorize_TypeMatch(t *testing.T) {
	config := &SecurityConfig{
		AuthzEnabled: true,
	}

	auth := NewACLAuthorizer(config)
	ctx := context.Background()

	acl := &ACLEntry{
		ID:           "type-1",
		Principal:    "type:SERVICE",
		ResourceType: ResourceTypeTopic,
		ResourceName: "metrics",
		PatternType:  PatternTypeLiteral,
		Action:       ActionTopicWrite,
		Permission:   PermissionAllow,
	}
	_ = auth.AddACL(acl)

	// Service principal should match
	servicePrincipal := &Principal{
		ID:   "service-1",
		Type: PrincipalTypeService,
	}

	allowed, _ := auth.Authorize(ctx, servicePrincipal, ActionTopicWrite, Resource{
		Type: ResourceTypeTopic,
		Name: "metrics",
	})

	if !allowed {
		t.Error("Expected service type to match")
	}

	// User principal should not match
	userPrincipal := &Principal{
		ID:   "user1",
		Type: PrincipalTypeUser,
	}

	allowed, _ = auth.Authorize(ctx, userPrincipal, ActionTopicWrite, Resource{
		Type: ResourceTypeTopic,
		Name: "metrics",
	})

	if allowed {
		t.Error("Expected user type not to match")
	}
}

func TestACLAuthorizer_AddACL(t *testing.T) {
	auth := NewACLAuthorizer(&SecurityConfig{})

	acl := &ACLEntry{
		Principal:    "user1",
		ResourceType: ResourceTypeTopic,
		ResourceName: "test",
		Action:       ActionTopicWrite,
		Permission:   PermissionAllow,
	}

	err := auth.AddACL(acl)
	if err != nil {
		t.Fatalf("Failed to add ACL: %v", err)
	}

	if acl.ID == "" {
		t.Error("Expected ID to be generated")
	}

	if acl.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}

	// Verify it was added
	retrieved, err := auth.GetACL(acl.ID)
	if err != nil {
		t.Fatalf("Failed to get ACL: %v", err)
	}

	if retrieved.Principal != "user1" {
		t.Error("Retrieved ACL doesn't match")
	}
}

func TestACLAuthorizer_RemoveACL(t *testing.T) {
	auth := NewACLAuthorizer(&SecurityConfig{})

	acl := &ACLEntry{
		ID:           "test-acl",
		Principal:    "user1",
		ResourceType: ResourceTypeTopic,
		ResourceName: "test",
		Action:       ActionTopicWrite,
		Permission:   PermissionAllow,
	}

	_ = auth.AddACL(acl)

	// Remove it
	err := auth.RemoveACL("test-acl")
	if err != nil {
		t.Fatalf("Failed to remove ACL: %v", err)
	}

	// Verify it's gone
	_, err = auth.GetACL("test-acl")
	if err == nil {
		t.Error("Expected error when getting removed ACL")
	}

	// Try removing non-existent
	err = auth.RemoveACL("nonexistent")
	if err == nil {
		t.Error("Expected error when removing non-existent ACL")
	}
}

func TestACLAuthorizer_ListACLs(t *testing.T) {
	auth := NewACLAuthorizer(&SecurityConfig{})

	acl1 := &ACLEntry{
		ID:           "acl1",
		Principal:    "user1",
		ResourceType: ResourceTypeTopic,
		ResourceName: "test1",
		Action:       ActionTopicWrite,
		Permission:   PermissionAllow,
	}

	acl2 := &ACLEntry{
		ID:           "acl2",
		Principal:    "user2",
		ResourceType: ResourceTypeTopic,
		ResourceName: "test2",
		Action:       ActionTopicRead,
		Permission:   PermissionAllow,
	}

	_ = auth.AddACL(acl1)
	_ = auth.AddACL(acl2)

	acls := auth.ListACLs()
	if len(acls) != 2 {
		t.Errorf("Expected 2 ACLs, got %d", len(acls))
	}
}

func TestACLAuthorizer_FindACLs(t *testing.T) {
	auth := NewACLAuthorizer(&SecurityConfig{})

	acl1 := &ACLEntry{
		ID:           "acl1",
		Principal:    "user1",
		ResourceType: ResourceTypeTopic,
		ResourceName: "logs.app1",
		PatternType:  PatternTypePrefix,
		Action:       ActionTopicWrite,
		Permission:   PermissionAllow,
	}

	acl2 := &ACLEntry{
		ID:           "acl2",
		Principal:    "user2",
		ResourceType: ResourceTypeTopic,
		ResourceName: "logs.app2",
		PatternType:  PatternTypePrefix,
		Action:       ActionTopicWrite,
		Permission:   PermissionAllow,
	}

	acl3 := &ACLEntry{
		ID:           "acl3",
		Principal:    "user1",
		ResourceType: ResourceTypeTopic,
		ResourceName: "logs.app1",
		PatternType:  PatternTypePrefix,
		Action:       ActionTopicRead,
		Permission:   PermissionAllow,
	}

	_ = auth.AddACL(acl1)
	_ = auth.AddACL(acl2)
	_ = auth.AddACL(acl3)

	// Find ACLs for specific resource and action
	matches := auth.FindACLs(ResourceTypeTopic, "logs.app1.errors", ActionTopicWrite)

	if len(matches) != 1 {
		t.Errorf("Expected 1 match, got %d", len(matches))
	}

	if matches[0].ID != "acl1" {
		t.Error("Wrong ACL matched")
	}
}

func TestACLAuthorizer_ClearACLs(t *testing.T) {
	auth := NewACLAuthorizer(&SecurityConfig{})

	acl := &ACLEntry{
		ID:           "acl1",
		Principal:    "user1",
		ResourceType: ResourceTypeTopic,
		ResourceName: "test",
		Action:       ActionTopicWrite,
		Permission:   PermissionAllow,
	}

	_ = auth.AddACL(acl)

	// Verify it exists
	if len(auth.ListACLs()) != 1 {
		t.Error("Expected 1 ACL")
	}

	// Clear all
	auth.ClearACLs()

	// Verify empty
	if len(auth.ListACLs()) != 0 {
		t.Error("Expected no ACLs after clear")
	}
}

func TestACLBuilder(t *testing.T) {
	acl := NewACLBuilder().
		ForPrincipal("user1").
		OnResource(ResourceTypeTopic, "orders").
		WithPatternType(PatternTypeLiteral).
		ForAction(ActionTopicWrite).
		Allow().
		CreatedBy("admin").
		Build()

	if acl.Principal != "user1" {
		t.Error("Principal not set correctly")
	}

	if acl.ResourceType != ResourceTypeTopic {
		t.Error("Resource type not set correctly")
	}

	if acl.ResourceName != "orders" {
		t.Error("Resource name not set correctly")
	}

	if acl.PatternType != PatternTypeLiteral {
		t.Error("Pattern type not set correctly")
	}

	if acl.Action != ActionTopicWrite {
		t.Error("Action not set correctly")
	}

	if acl.Permission != PermissionAllow {
		t.Error("Permission not set correctly")
	}

	if acl.CreatedBy != "admin" {
		t.Error("CreatedBy not set correctly")
	}

	if acl.ID == "" {
		t.Error("ID should be generated")
	}

	if acl.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestACLBuilder_Deny(t *testing.T) {
	acl := NewACLBuilder().
		ForPrincipal("user1").
		OnResource(ResourceTypeTopic, "sensitive").
		ForAction(ActionTopicRead).
		Deny().
		Build()

	if acl.Permission != PermissionDeny {
		t.Error("Expected DENY permission")
	}
}

func TestACLBuilder_DefaultPatternType(t *testing.T) {
	acl := NewACLBuilder().
		ForPrincipal("user1").
		OnResource(ResourceTypeTopic, "test").
		ForAction(ActionTopicRead).
		Allow().
		Build()

	// Should default to literal
	if acl.PatternType != PatternTypeLiteral {
		t.Errorf("Expected default pattern type LITERAL, got %s", acl.PatternType)
	}
}

func TestDefaultACLs(t *testing.T) {
	acls := DefaultACLs()

	if len(acls) == 0 {
		t.Error("Expected default ACLs")
	}

	// All should be ALLOW
	for _, acl := range acls {
		if acl.Permission != PermissionAllow {
			t.Error("Expected all default ACLs to be ALLOW")
		}
		if acl.CreatedBy != "system" {
			t.Error("Expected CreatedBy to be 'system'")
		}
	}
}

func TestRestrictiveACLs(t *testing.T) {
	acls := RestrictiveACLs()

	// Should return empty (default deny)
	if len(acls) != 0 {
		t.Error("Expected empty restrictive ACLs")
	}
}

func TestPrincipal_IsSuperUser(t *testing.T) {
	principal := &Principal{
		ID: "admin",
	}

	superUsers := []string{"admin", "root"}

	if !principal.IsSuperUser(superUsers) {
		t.Error("Expected 'admin' to be a super user")
	}

	principal.ID = "regular-user"
	if principal.IsSuperUser(superUsers) {
		t.Error("Expected 'regular-user' not to be a super user")
	}
}

func TestPrincipal_HasGroup(t *testing.T) {
	principal := &Principal{
		ID: "user1",
		Attributes: map[string]string{
			"groups": "developers",
		},
	}

	if !principal.HasGroup("developers") {
		t.Error("Expected principal to have group 'developers'")
	}

	if principal.HasGroup("admins") {
		t.Error("Expected principal not to have group 'admins'")
	}

	// No groups attribute
	principal2 := &Principal{
		ID:         "user2",
		Attributes: map[string]string{},
	}

	if principal2.HasGroup("any") {
		t.Error("Expected principal with no groups not to match")
	}
}

func TestPrincipal_IsExpired(t *testing.T) {
	// No expiration
	principal1 := &Principal{
		ID: "user1",
	}

	if principal1.IsExpired() {
		t.Error("Expected principal with no expiration not to be expired")
	}

	// Future expiration
	future := time.Now().Add(1 * time.Hour)
	principal2 := &Principal{
		ID:        "user2",
		ExpiresAt: &future,
	}

	if principal2.IsExpired() {
		t.Error("Expected principal with future expiration not to be expired")
	}

	// Past expiration
	past := time.Now().Add(-1 * time.Hour)
	principal3 := &Principal{
		ID:        "user3",
		ExpiresAt: &past,
	}

	if !principal3.IsExpired() {
		t.Error("Expected principal with past expiration to be expired")
	}
}
