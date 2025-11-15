package security

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/logging"
)

func TestNewManager(t *testing.T) {
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	tests := []struct {
		name        string
		config      *SecurityConfig
		expectError bool
		validate    func(*testing.T, *Manager)
	}{
		{
			name:        "nil config uses defaults",
			config:      nil,
			expectError: false,
			validate: func(t *testing.T, m *Manager) {
				if m == nil {
					t.Fatal("Expected non-nil manager")
				}
				if m.config == nil {
					t.Error("Expected config to be initialized")
				}
				if m.authenticators == nil {
					t.Error("Expected authenticators map to be initialized")
				}
			},
		},
		{
			name: "with TLS enabled",
			config: &SecurityConfig{
				TLS: &TLSConfig{
					Enabled:           true,
					RequireClientCert: true,
					CertFile:          "test.crt",
					KeyFile:           "test.key",
				},
				SASL:           nil,
				AuthzEnabled:   false,
				AllowAnonymous: true,
			},
			expectError: false,
			validate: func(t *testing.T, m *Manager) {
				if !m.authEnabled {
					t.Error("Expected auth to be enabled with TLS")
				}
				if _, exists := m.authenticators[AuthMethodMTLS]; !exists {
					t.Error("Expected mTLS authenticator to be registered")
				}
			},
		},
		{
			name: "with SASL enabled",
			config: &SecurityConfig{
				SASL: &SASLConfig{
					Enabled:    true,
					Mechanisms: []AuthMethod{AuthMethodSASLPlain, AuthMethodSASLSCRAM256},
					Users:      make(map[string]*User),
				},
				AuthzEnabled:   false,
				AllowAnonymous: true,
			},
			expectError: false,
			validate: func(t *testing.T, m *Manager) {
				if !m.authEnabled {
					t.Error("Expected auth to be enabled with SASL")
				}
				if _, exists := m.authenticators[AuthMethodSASLPlain]; !exists {
					t.Error("Expected SASL_PLAIN authenticator to be registered")
				}
				if _, exists := m.authenticators[AuthMethodSASLSCRAM256]; !exists {
					t.Error("Expected SASL_SCRAM_SHA256 authenticator to be registered")
				}
			},
		},
		{
			name: "with authorization enabled",
			config: &SecurityConfig{
				AuthzEnabled:   true,
				UseDefaultACLs: true,
				AllowAnonymous: true,
			},
			expectError: false,
			validate: func(t *testing.T, m *Manager) {
				if !m.authzEnabled {
					t.Error("Expected authorization to be enabled")
				}
				if m.authorizer == nil {
					t.Error("Expected authorizer to be initialized")
				}
				// Check that default ACLs were loaded
				acls := m.ListACLs()
				if len(acls) == 0 {
					t.Error("Expected default ACLs to be loaded")
				}
			},
		},
		{
			name: "with audit enabled",
			config: &SecurityConfig{
				AuditEnabled:   true,
				AuditLogFile:   "", // Structured logging only
				AllowAnonymous: true,
			},
			expectError: false,
			validate: func(t *testing.T, m *Manager) {
				if !m.auditEnabled {
					t.Error("Expected audit to be enabled")
				}
				if m.auditLogger == nil {
					t.Error("Expected audit logger to be initialized")
				}
			},
		},
		{
			name: "with API key authentication",
			config: &SecurityConfig{
				APIKeyEnabled:  true,
				AllowAnonymous: true,
			},
			expectError: false,
			validate: func(t *testing.T, m *Manager) {
				if !m.authEnabled {
					t.Error("Expected auth to be enabled with API keys")
				}
				if _, exists := m.authenticators[AuthMethodAPIKey]; !exists {
					t.Error("Expected API key authenticator to be registered")
				}
			},
		},
		{
			name: "with encryption enabled",
			config: &SecurityConfig{
				Encryption: &EncryptionConfig{
					Enabled:   true,
					Algorithm: EncryptionAlgorithmAES256GCM,
				},
				AllowAnonymous: true,
			},
			expectError: false,
			validate: func(t *testing.T, m *Manager) {
				if !m.encryptionEnabled {
					t.Error("Expected encryption to be enabled")
				}
				if m.encryption == nil {
					t.Error("Expected encryption service to be initialized")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(tt.config, logger)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if err == nil && tt.validate != nil {
				tt.validate(t, manager)
			}
			if manager != nil {
				_ = manager.Close()
			}
		})
	}
}

func TestManager_Authenticate(t *testing.T) {
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	// Create a test user
	testUser, err := CreateUser("testuser", "testpass", AuthMethodSASLPlain, []string{"group1"})
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	tests := []struct {
		name        string
		config      *SecurityConfig
		method      AuthMethod
		credentials interface{}
		expectError bool
		validate    func(*testing.T, *Principal)
	}{
		{
			name: "authentication disabled with anonymous allowed",
			config: &SecurityConfig{
				AllowAnonymous: true,
			},
			method:      AuthMethodNone,
			credentials: nil,
			expectError: false,
			validate: func(t *testing.T, p *Principal) {
				if p.ID != "anonymous" {
					t.Errorf("Expected anonymous principal, got %s", p.ID)
				}
				if p.Type != PrincipalTypeAnonymous {
					t.Errorf("Expected anonymous type, got %s", p.Type)
				}
			},
		},
		{
			name: "authentication disabled without anonymous",
			config: &SecurityConfig{
				AllowAnonymous: false,
			},
			method:      AuthMethodNone,
			credentials: nil,
			expectError: true,
		},
		{
			name: "valid SASL PLAIN authentication",
			config: &SecurityConfig{
				SASL: &SASLConfig{
					Enabled:    true,
					Mechanisms: []AuthMethod{AuthMethodSASLPlain},
					Users: map[string]*User{
						"testuser": testUser,
					},
				},
			},
			method: AuthMethodSASLPlain,
			credentials: &SASLCredentials{
				Username:  "testuser",
				Password:  "testpass",
				Mechanism: AuthMethodSASLPlain,
			},
			expectError: false,
			validate: func(t *testing.T, p *Principal) {
				if p.ID != "testuser" {
					t.Errorf("Expected testuser, got %s", p.ID)
				}
				if p.Type != PrincipalTypeUser {
					t.Errorf("Expected user type, got %s", p.Type)
				}
				if p.AuthMethod != AuthMethodSASLPlain {
					t.Errorf("Expected SASL_PLAIN method, got %s", p.AuthMethod)
				}
			},
		},
		{
			name: "invalid SASL credentials",
			config: &SecurityConfig{
				SASL: &SASLConfig{
					Enabled:    true,
					Mechanisms: []AuthMethod{AuthMethodSASLPlain},
					Users: map[string]*User{
						"testuser": testUser,
					},
				},
			},
			method: AuthMethodSASLPlain,
			credentials: &SASLCredentials{
				Username:  "testuser",
				Password:  "wrongpass",
				Mechanism: AuthMethodSASLPlain,
			},
			expectError: true,
		},
		{
			name: "unsupported authentication method",
			config: &SecurityConfig{
				SASL: &SASLConfig{
					Enabled:    true,
					Mechanisms: []AuthMethod{AuthMethodSASLPlain},
					Users: map[string]*User{
						"testuser": testUser,
					},
				},
			},
			method:      AuthMethodOAuth,
			credentials: nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(tt.config, logger)
			if err != nil {
				t.Fatalf("Failed to create manager: %v", err)
			}
			defer manager.Close()

			ctx := context.Background()
			principal, err := manager.Authenticate(ctx, tt.method, tt.credentials)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if err == nil && tt.validate != nil {
				tt.validate(t, principal)
			}
		})
	}
}

func TestManager_Authorize(t *testing.T) {
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	principal := &Principal{
		ID:   "testuser",
		Type: PrincipalTypeUser,
	}

	tests := []struct {
		name           string
		config         *SecurityConfig
		principal      *Principal
		action         Action
		resource       Resource
		expectAllowed  bool
		expectError    bool
	}{
		{
			name: "authorization disabled",
			config: &SecurityConfig{
				AuthzEnabled: false,
			},
			principal:     principal,
			action:        ActionTopicWrite,
			resource:      Resource{Type: ResourceTypeTopic, Name: "test-topic"},
			expectAllowed: true,
			expectError:   false,
		},
		{
			name: "authorization with allow ACL",
			config: &SecurityConfig{
				AuthzEnabled: true,
			},
			principal: principal,
			action:    ActionTopicWrite,
			resource:  Resource{Type: ResourceTypeTopic, Name: "test-topic"},
			// Will be denied by default (no matching ACL)
			expectAllowed: false,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(tt.config, logger)
			if err != nil {
				t.Fatalf("Failed to create manager: %v", err)
			}
			defer manager.Close()

			ctx := context.Background()
			allowed, err := manager.Authorize(ctx, tt.principal, tt.action, tt.resource)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if allowed != tt.expectAllowed {
				t.Errorf("Expected allowed=%v, got %v", tt.expectAllowed, allowed)
			}
		})
	}
}

func TestManager_EncryptDecrypt(t *testing.T) {
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	tests := []struct {
		name        string
		config      *SecurityConfig
		data        []byte
		expectError bool
	}{
		{
			name: "encryption disabled",
			config: &SecurityConfig{
				Encryption: nil,
			},
			data:        []byte("test data"),
			expectError: false,
		},
		{
			name: "encryption enabled",
			config: &SecurityConfig{
				Encryption: &EncryptionConfig{
					Enabled:   true,
					Algorithm: EncryptionAlgorithmAES256GCM,
				},
			},
			data:        []byte("sensitive test data"),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(tt.config, logger)
			if err != nil {
				t.Fatalf("Failed to create manager: %v", err)
			}
			defer manager.Close()

			// Encrypt
			encrypted, err := manager.Encrypt(tt.data)
			if tt.expectError && err == nil {
				t.Error("Expected error during encryption but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error during encryption: %v", err)
			}

			// Decrypt
			if err == nil {
				decrypted, err := manager.Decrypt(encrypted)
				if err != nil {
					t.Errorf("Unexpected error during decryption: %v", err)
				}
				if string(decrypted) != string(tt.data) {
					t.Errorf("Expected decrypted data %q, got %q", string(tt.data), string(decrypted))
				}
			}
		})
	}
}

func TestManager_AuditAction(t *testing.T) {
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	principal := &Principal{
		ID:   "testuser",
		Type: PrincipalTypeUser,
	}

	tests := []struct {
		name   string
		config *SecurityConfig
	}{
		{
			name: "audit disabled",
			config: &SecurityConfig{
				AuditEnabled: false,
			},
		},
		{
			name: "audit enabled",
			config: &SecurityConfig{
				AuditEnabled: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(tt.config, logger)
			if err != nil {
				t.Fatalf("Failed to create manager: %v", err)
			}
			defer manager.Close()

			ctx := context.Background()
			manager.AuditAction(ctx, principal, ActionTopicWrite, Resource{
				Type: ResourceTypeTopic,
				Name: "test-topic",
			}, map[string]string{"key": "value"})

			// If audit is enabled, this should not panic or error
		})
	}
}

func TestManager_AuditFailure(t *testing.T) {
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	principal := &Principal{
		ID:   "testuser",
		Type: PrincipalTypeUser,
	}

	tests := []struct {
		name   string
		config *SecurityConfig
	}{
		{
			name: "audit disabled",
			config: &SecurityConfig{
				AuditEnabled: false,
			},
		},
		{
			name: "audit enabled",
			config: &SecurityConfig{
				AuditEnabled: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(tt.config, logger)
			if err != nil {
				t.Fatalf("Failed to create manager: %v", err)
			}
			defer manager.Close()

			ctx := context.Background()
			testErr := fmt.Errorf("authentication failed")
			manager.AuditFailure(ctx, principal, ActionTopicWrite, Resource{
				Type: ResourceTypeTopic,
				Name: "test-topic",
			}, testErr)

			// If audit is enabled, this should not panic or error
		})
	}
}

func TestManager_AddRemoveACL(t *testing.T) {
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	tests := []struct {
		name        string
		config      *SecurityConfig
		expectError bool
	}{
		{
			name: "authorization disabled",
			config: &SecurityConfig{
				AuthzEnabled: false,
			},
			expectError: true,
		},
		{
			name: "authorization enabled",
			config: &SecurityConfig{
				AuthzEnabled: true,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(tt.config, logger)
			if err != nil {
				t.Fatalf("Failed to create manager: %v", err)
			}
			defer manager.Close()

			acl := &ACLEntry{
				ID:           "test-acl-1",
				Principal:    "testuser",
				ResourceType: ResourceTypeTopic,
				ResourceName: "test-topic",
				PatternType:  PatternTypeLiteral,
				Action:       ActionTopicWrite,
				Permission:   PermissionAllow,
				CreatedAt:    time.Now(),
				CreatedBy:    "admin",
			}

			// Test AddACL
			err = manager.AddACL(acl)
			if tt.expectError && err == nil {
				t.Error("Expected error when adding ACL but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error when adding ACL: %v", err)
			}

			// Test ListACLs
			acls := manager.ListACLs()
			if !tt.expectError && len(acls) == 0 {
				t.Error("Expected at least one ACL in list")
			}
			if tt.expectError && acls != nil {
				t.Error("Expected nil ACL list when authz disabled")
			}

			// Test RemoveACL
			err = manager.RemoveACL("test-acl-1")
			if tt.expectError && err == nil {
				t.Error("Expected error when removing ACL but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error when removing ACL: %v", err)
			}
		})
	}
}

func TestManager_AddUser(t *testing.T) {
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	tests := []struct {
		name        string
		config      *SecurityConfig
		username    string
		password    string
		authMethod  AuthMethod
		groups      []string
		expectError bool
	}{
		{
			name: "SASL disabled",
			config: &SecurityConfig{
				SASL: nil,
			},
			username:    "testuser",
			password:    "testpass",
			authMethod:  AuthMethodSASLPlain,
			groups:      []string{"group1"},
			expectError: true,
		},
		{
			name: "SASL enabled",
			config: &SecurityConfig{
				SASL: &SASLConfig{
					Enabled:    true,
					Mechanisms: []AuthMethod{AuthMethodSASLPlain},
					Users:      make(map[string]*User),
				},
			},
			username:    "testuser",
			password:    "testpass",
			authMethod:  AuthMethodSASLPlain,
			groups:      []string{"group1"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(tt.config, logger)
			if err != nil {
				t.Fatalf("Failed to create manager: %v", err)
			}
			defer manager.Close()

			err = manager.AddUser(tt.username, tt.password, tt.authMethod, tt.groups)
			if tt.expectError && err == nil {
				t.Error("Expected error when adding user but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error when adding user: %v", err)
			}

			// Verify user was added
			if !tt.expectError {
				if user, exists := manager.config.SASL.Users[tt.username]; !exists {
					t.Error("Expected user to be added to SASL config")
				} else if user.Username != tt.username {
					t.Errorf("Expected username %s, got %s", tt.username, user.Username)
				}
			}
		})
	}
}

func TestManager_AddAPIKey(t *testing.T) {
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	tests := []struct {
		name        string
		config      *SecurityConfig
		expectError bool
	}{
		{
			name: "API key disabled",
			config: &SecurityConfig{
				APIKeyEnabled: false,
			},
			expectError: true,
		},
		{
			name: "API key enabled",
			config: &SecurityConfig{
				APIKeyEnabled: true,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewManager(tt.config, logger)
			if err != nil {
				t.Fatalf("Failed to create manager: %v", err)
			}
			defer manager.Close()

			apiKey := &APIKey{
				Key:         "test-api-key-123",
				Secret:      "test-secret",
				PrincipalID: "test-service",
				Type:        PrincipalTypeService,
				CreatedAt:   time.Now(),
				Enabled:     true,
			}

			err = manager.AddAPIKey(apiKey)
			if tt.expectError && err == nil {
				t.Error("Expected error when adding API key but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error when adding API key: %v", err)
			}
		})
	}
}

func TestManager_GettersAndFlags(t *testing.T) {
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	config := &SecurityConfig{
		SASL: &SASLConfig{
			Enabled:    true,
			Mechanisms: []AuthMethod{AuthMethodSASLPlain},
			Users:      make(map[string]*User),
		},
		AuthzEnabled: true,
		AuditEnabled: true,
		Encryption: &EncryptionConfig{
			Enabled:   true,
			Algorithm: EncryptionAlgorithmAES256GCM,
		},
	}

	manager, err := NewManager(config, logger)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Test IsAuthenticationEnabled
	if !manager.IsAuthenticationEnabled() {
		t.Error("Expected authentication to be enabled")
	}

	// Test IsAuthorizationEnabled
	if !manager.IsAuthorizationEnabled() {
		t.Error("Expected authorization to be enabled")
	}

	// Test IsAuditEnabled
	if !manager.IsAuditEnabled() {
		t.Error("Expected audit to be enabled")
	}

	// Test IsEncryptionEnabled
	if !manager.IsEncryptionEnabled() {
		t.Error("Expected encryption to be enabled")
	}

	// Test GetAuthorizer
	if manager.GetAuthorizer() == nil {
		t.Error("Expected non-nil authorizer")
	}

	// Test GetEncryption
	if manager.GetEncryption() == nil {
		t.Error("Expected non-nil encryption service")
	}
}

func TestManager_Close(t *testing.T) {
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	// Create temp directory for audit log
	tempDir := t.TempDir()
	auditLogFile := filepath.Join(tempDir, "audit.log")

	config := &SecurityConfig{
		AuditEnabled: true,
		AuditLogFile: auditLogFile,
		Encryption: &EncryptionConfig{
			Enabled:   true,
			Algorithm: EncryptionAlgorithmAES256GCM,
		},
	}

	manager, err := NewManager(config, logger)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Write some audit events
	ctx := context.Background()
	principal := &Principal{ID: "testuser", Type: PrincipalTypeUser}
	manager.AuditAction(ctx, principal, ActionTopicWrite, Resource{
		Type: ResourceTypeTopic,
		Name: "test-topic",
	}, nil)

	// Close should not error
	err = manager.Close()
	if err != nil {
		t.Errorf("Unexpected error during close: %v", err)
	}

	// Verify audit log file was created
	if _, err := os.Stat(auditLogFile); os.IsNotExist(err) {
		t.Error("Expected audit log file to be created")
	}
}

func TestManager_AuthenticateWithAudit(t *testing.T) {
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	testUser, err := CreateUser("testuser", "testpass", AuthMethodSASLPlain, []string{"group1"})
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	config := &SecurityConfig{
		SASL: &SASLConfig{
			Enabled:    true,
			Mechanisms: []AuthMethod{AuthMethodSASLPlain},
			Users: map[string]*User{
				"testuser": testUser,
			},
		},
		AuditEnabled: true,
	}

	manager, err := NewManager(config, logger)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	ctx := context.Background()

	// Test successful authentication with audit
	principal, err := manager.Authenticate(ctx, AuthMethodSASLPlain, &SASLCredentials{
		Username:  "testuser",
		Password:  "testpass",
		Mechanism: AuthMethodSASLPlain,
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if principal == nil {
		t.Fatal("Expected non-nil principal")
	}

	// Test failed authentication with audit
	_, err = manager.Authenticate(ctx, AuthMethodSASLPlain, &SASLCredentials{
		Username:  "testuser",
		Password:  "wrongpass",
		Mechanism: AuthMethodSASLPlain,
	})
	if err == nil {
		t.Error("Expected error for wrong password")
	}
}

func TestManager_AuthorizeWithAudit(t *testing.T) {
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	config := &SecurityConfig{
		AuthzEnabled: true,
		AuditEnabled: true,
	}

	manager, err := NewManager(config, logger)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Add an ACL
	acl := &ACLEntry{
		ID:           "test-acl",
		Principal:    "testuser",
		ResourceType: ResourceTypeTopic,
		ResourceName: "test-topic",
		PatternType:  PatternTypeLiteral,
		Action:       ActionTopicWrite,
		Permission:   PermissionAllow,
		CreatedAt:    time.Now(),
		CreatedBy:    "admin",
	}
	err = manager.AddACL(acl)
	if err != nil {
		t.Fatalf("Failed to add ACL: %v", err)
	}

	ctx := context.Background()
	principal := &Principal{
		ID:   "testuser",
		Type: PrincipalTypeUser,
	}

	// Test authorization with audit (should succeed)
	allowed, err := manager.Authorize(ctx, principal, ActionTopicWrite, Resource{
		Type: ResourceTypeTopic,
		Name: "test-topic",
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !allowed {
		t.Error("Expected authorization to be allowed")
	}

	// Test authorization with audit (should fail - different action)
	allowed, err = manager.Authorize(ctx, principal, ActionTopicDelete, Resource{
		Type: ResourceTypeTopic,
		Name: "test-topic",
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if allowed {
		t.Error("Expected authorization to be denied")
	}
}

func TestDefaultSecurityConfig(t *testing.T) {
	config := DefaultSecurityConfig()

	if config == nil {
		t.Fatal("Expected non-nil default config")
	}

	if config.TLS == nil {
		t.Error("Expected default TLS config")
	}

	if !config.AllowAnonymous {
		t.Error("Expected anonymous access to be allowed by default")
	}

	if config.AuthzEnabled {
		t.Error("Expected authorization to be disabled by default")
	}

	if config.AuditEnabled {
		t.Error("Expected audit to be disabled by default")
	}

	if config.APIKeyEnabled {
		t.Error("Expected API key auth to be disabled by default")
	}
}
