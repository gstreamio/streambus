package security

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestNewSASLAuthenticator(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *SASLConfig
		validate func(*testing.T, *SASLAuthenticator)
	}{
		{
			name: "with users",
			cfg: &SASLConfig{
				Enabled:    true,
				Mechanisms: []AuthMethod{AuthMethodSASLPlain},
				Users: map[string]*User{
					"test": {Username: "test"},
				},
			},
			validate: func(t *testing.T, auth *SASLAuthenticator) {
				if len(auth.config.Users) != 1 {
					t.Errorf("Expected 1 user, got %d", len(auth.config.Users))
				}
			},
		},
		{
			name: "nil users map",
			cfg: &SASLConfig{
				Enabled:    true,
				Mechanisms: []AuthMethod{AuthMethodSASLPlain},
				Users:      nil,
			},
			validate: func(t *testing.T, auth *SASLAuthenticator) {
				if auth.config.Users == nil {
					t.Error("Expected users map to be initialized")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := NewSASLAuthenticator(tt.cfg)
			if auth == nil {
				t.Fatal("Expected non-nil authenticator")
			}
			if tt.validate != nil {
				tt.validate(t, auth)
			}
		})
	}
}

func TestSASLAuthenticator_Authenticate_PLAIN(t *testing.T) {
	// Create test user with PLAIN authentication
	user, err := CreateUser("testuser", "testpass", AuthMethodSASLPlain, []string{"group1", "group2"})
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	cfg := &SASLConfig{
		Enabled:    true,
		Mechanisms: []AuthMethod{AuthMethodSASLPlain},
		Users: map[string]*User{
			"testuser": user,
		},
	}

	auth := NewSASLAuthenticator(cfg)

	tests := []struct {
		name        string
		creds       interface{}
		expectError bool
		validate    func(*testing.T, *Principal)
	}{
		{
			name: "valid PLAIN authentication",
			creds: &SASLCredentials{
				Username:  "testuser",
				Password:  "testpass",
				Mechanism: AuthMethodSASLPlain,
			},
			expectError: false,
			validate: func(t *testing.T, p *Principal) {
				if p.ID != "testuser" {
					t.Errorf("Expected ID 'testuser', got '%s'", p.ID)
				}
				if p.Type != PrincipalTypeUser {
					t.Errorf("Expected type USER, got %s", p.Type)
				}
				if p.AuthMethod != AuthMethodSASLPlain {
					t.Errorf("Expected auth method SASL_PLAIN, got %s", p.AuthMethod)
				}
				if p.Attributes["groups"] == "" {
					t.Error("Expected groups to be set in attributes")
				}
			},
		},
		{
			name: "invalid password",
			creds: &SASLCredentials{
				Username:  "testuser",
				Password:  "wrongpass",
				Mechanism: AuthMethodSASLPlain,
			},
			expectError: true,
		},
		{
			name: "user not found",
			creds: &SASLCredentials{
				Username:  "nonexistent",
				Password:  "testpass",
				Mechanism: AuthMethodSASLPlain,
			},
			expectError: true,
		},
		{
			name: "invalid credentials type",
			creds: &TLSCredentials{
				CommonName: "test",
			},
			expectError: true,
		},
		{
			name: "mechanism not allowed",
			creds: &SASLCredentials{
				Username:  "testuser",
				Password:  "testpass",
				Mechanism: AuthMethodSASLSCRAM256,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			principal, err := auth.Authenticate(ctx, tt.creds)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if principal == nil {
				t.Fatal("Expected non-nil principal")
			}

			if tt.validate != nil {
				tt.validate(t, principal)
			}
		})
	}
}

func TestSASLAuthenticator_Authenticate_SCRAM256(t *testing.T) {
	// Create test user with SCRAM-SHA-256 authentication
	user, err := CreateUser("scramuser", "scrampass", AuthMethodSASLSCRAM256, []string{"admin"})
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	cfg := &SASLConfig{
		Enabled:    true,
		Mechanisms: []AuthMethod{AuthMethodSASLSCRAM256},
		Users: map[string]*User{
			"scramuser": user,
		},
	}

	auth := NewSASLAuthenticator(cfg)

	tests := []struct {
		name        string
		creds       *SASLCredentials
		expectError bool
	}{
		{
			name: "valid SCRAM-SHA-256 authentication",
			creds: &SASLCredentials{
				Username:  "scramuser",
				Password:  "scrampass",
				Mechanism: AuthMethodSASLSCRAM256,
			},
			expectError: false,
		},
		{
			name: "invalid password",
			creds: &SASLCredentials{
				Username:  "scramuser",
				Password:  "wrongpass",
				Mechanism: AuthMethodSASLSCRAM256,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			principal, err := auth.Authenticate(ctx, tt.creds)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if principal == nil {
				t.Fatal("Expected non-nil principal")
			}

			if principal.AuthMethod != AuthMethodSASLSCRAM256 {
				t.Errorf("Expected auth method SASL_SCRAM_SHA256, got %s", principal.AuthMethod)
			}
		})
	}
}

func TestSASLAuthenticator_Authenticate_SCRAM512(t *testing.T) {
	// Create test user with SCRAM-SHA-512 authentication
	user, err := CreateUser("scram512user", "scram512pass", AuthMethodSASLSCRAM512, []string{"power-users"})
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	cfg := &SASLConfig{
		Enabled:    true,
		Mechanisms: []AuthMethod{AuthMethodSASLSCRAM512},
		Users: map[string]*User{
			"scram512user": user,
		},
	}

	auth := NewSASLAuthenticator(cfg)

	tests := []struct {
		name        string
		creds       *SASLCredentials
		expectError bool
	}{
		{
			name: "valid SCRAM-SHA-512 authentication",
			creds: &SASLCredentials{
				Username:  "scram512user",
				Password:  "scram512pass",
				Mechanism: AuthMethodSASLSCRAM512,
			},
			expectError: false,
		},
		{
			name: "invalid password",
			creds: &SASLCredentials{
				Username:  "scram512user",
				Password:  "wrongpass",
				Mechanism: AuthMethodSASLSCRAM512,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			principal, err := auth.Authenticate(ctx, tt.creds)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if principal == nil {
				t.Fatal("Expected non-nil principal")
			}

			if principal.AuthMethod != AuthMethodSASLSCRAM512 {
				t.Errorf("Expected auth method SASL_SCRAM_SHA512, got %s", principal.AuthMethod)
			}
		})
	}
}

func TestSASLAuthenticator_DisabledUser(t *testing.T) {
	user, err := CreateUser("disabled", "pass", AuthMethodSASLPlain, nil)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	user.Enabled = false

	cfg := &SASLConfig{
		Enabled:    true,
		Mechanisms: []AuthMethod{AuthMethodSASLPlain},
		Users: map[string]*User{
			"disabled": user,
		},
	}

	auth := NewSASLAuthenticator(cfg)
	ctx := context.Background()

	creds := &SASLCredentials{
		Username:  "disabled",
		Password:  "pass",
		Mechanism: AuthMethodSASLPlain,
	}

	_, err = auth.Authenticate(ctx, creds)
	if err == nil {
		t.Error("Expected error for disabled user")
	}
}

func TestCreateUser(t *testing.T) {
	tests := []struct {
		name       string
		username   string
		password   string
		authMethod AuthMethod
		groups     []string
		validate   func(*testing.T, *User)
	}{
		{
			name:       "PLAIN user",
			username:   "plainuser",
			password:   "password123",
			authMethod: AuthMethodSASLPlain,
			groups:     []string{"users"},
			validate: func(t *testing.T, u *User) {
				if u.Username != "plainuser" {
					t.Errorf("Expected username 'plainuser', got '%s'", u.Username)
				}
				if len(u.PasswordHash) == 0 {
					t.Error("Expected password hash to be set")
				}
				// Verify bcrypt hash works
				if err := bcrypt.CompareHashAndPassword(u.PasswordHash, []byte("password123")); err != nil {
					t.Error("Password hash verification failed")
				}
				if len(u.Groups) != 1 || u.Groups[0] != "users" {
					t.Error("Groups not set correctly")
				}
				if !u.Enabled {
					t.Error("Expected user to be enabled by default")
				}
			},
		},
		{
			name:       "SCRAM-SHA-256 user",
			username:   "scramuser",
			password:   "scrampass",
			authMethod: AuthMethodSASLSCRAM256,
			groups:     []string{"admin"},
			validate: func(t *testing.T, u *User) {
				if len(u.Salt) != 16 {
					t.Errorf("Expected 16-byte salt, got %d", len(u.Salt))
				}
				if u.Iterations != 4096 {
					t.Errorf("Expected 4096 iterations, got %d", u.Iterations)
				}
				if len(u.PasswordHash) != sha256.New().Size() {
					t.Errorf("Expected %d-byte hash, got %d", sha256.New().Size(), len(u.PasswordHash))
				}
			},
		},
		{
			name:       "SCRAM-SHA-512 user",
			username:   "scram512user",
			password:   "scram512pass",
			authMethod: AuthMethodSASLSCRAM512,
			groups:     nil,
			validate: func(t *testing.T, u *User) {
				if len(u.Salt) != 16 {
					t.Errorf("Expected 16-byte salt, got %d", len(u.Salt))
				}
				if u.Iterations != 4096 {
					t.Errorf("Expected 4096 iterations, got %d", u.Iterations)
				}
				if len(u.PasswordHash) != sha512.New().Size() {
					t.Errorf("Expected %d-byte hash, got %d", sha512.New().Size(), len(u.PasswordHash))
				}
			},
		},
		{
			name:       "no groups",
			username:   "nogroups",
			password:   "pass",
			authMethod: AuthMethodSASLPlain,
			groups:     nil,
			validate: func(t *testing.T, u *User) {
				if u.Groups != nil {
					t.Error("Expected nil groups")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := CreateUser(tt.username, tt.password, tt.authMethod, tt.groups)
			if err != nil {
				t.Fatalf("Failed to create user: %v", err)
			}

			if user == nil {
				t.Fatal("Expected non-nil user")
			}

			if tt.validate != nil {
				tt.validate(t, user)
			}
		})
	}
}

func TestCreateUser_UnsupportedMethod(t *testing.T) {
	_, err := CreateUser("test", "pass", AuthMethodMTLS, nil)
	if err == nil {
		t.Error("Expected error for unsupported auth method")
	}
}

func TestVerifyPassword(t *testing.T) {
	tests := []struct {
		name       string
		authMethod AuthMethod
		password   string
		wrongPass  string
	}{
		{
			name:       "PLAIN",
			authMethod: AuthMethodSASLPlain,
			password:   "correctpass",
			wrongPass:  "wrongpass",
		},
		{
			name:       "SCRAM-SHA-256",
			authMethod: AuthMethodSASLSCRAM256,
			password:   "correctpass",
			wrongPass:  "wrongpass",
		},
		{
			name:       "SCRAM-SHA-512",
			authMethod: AuthMethodSASLSCRAM512,
			password:   "correctpass",
			wrongPass:  "wrongpass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := CreateUser("testuser", tt.password, tt.authMethod, nil)
			if err != nil {
				t.Fatalf("Failed to create user: %v", err)
			}

			// Test correct password
			if err := VerifyPassword(user, tt.password, tt.authMethod); err != nil {
				t.Errorf("Failed to verify correct password: %v", err)
			}

			// Test wrong password
			if err := VerifyPassword(user, tt.wrongPass, tt.authMethod); err == nil {
				t.Error("Expected error for wrong password")
			}
		})
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key1, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("Failed to generate API key: %v", err)
	}

	if key1 == "" {
		t.Error("Expected non-empty API key")
	}

	// Generate another and ensure they're different
	key2, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("Failed to generate second API key: %v", err)
	}

	if key1 == key2 {
		t.Error("Expected different API keys")
	}
}

func TestAPIKeyAuthenticator(t *testing.T) {
	auth := NewAPIKeyAuthenticator()

	// Create test API key
	key := &APIKey{
		Key:         "test-key-123",
		Secret:      "test-secret-456",
		PrincipalID: "service-1",
		Type:        PrincipalTypeService,
		Groups:      []string{"services"},
		Attributes: map[string]string{
			"env": "production",
		},
		Enabled:   true,
		CreatedAt: time.Now(),
	}

	auth.AddAPIKey(key)

	tests := []struct {
		name        string
		creds       interface{}
		expectError bool
		validate    func(*testing.T, *Principal)
	}{
		{
			name: "valid API key",
			creds: &APIKeyCredentials{
				APIKey:    "test-key-123",
				APISecret: "test-secret-456",
			},
			expectError: false,
			validate: func(t *testing.T, p *Principal) {
				if p.ID != "service-1" {
					t.Errorf("Expected ID 'service-1', got '%s'", p.ID)
				}
				if p.Type != PrincipalTypeService {
					t.Errorf("Expected type SERVICE, got %s", p.Type)
				}
				if p.AuthMethod != AuthMethodAPIKey {
					t.Errorf("Expected auth method API_KEY, got %s", p.AuthMethod)
				}
				if p.Attributes["env"] != "production" {
					t.Error("Attributes not copied correctly")
				}
			},
		},
		{
			name: "invalid API key",
			creds: &APIKeyCredentials{
				APIKey:    "invalid-key",
				APISecret: "test-secret-456",
			},
			expectError: true,
		},
		{
			name: "invalid API secret",
			creds: &APIKeyCredentials{
				APIKey:    "test-key-123",
				APISecret: "wrong-secret",
			},
			expectError: true,
		},
		{
			name: "invalid credentials type",
			creds: &SASLCredentials{
				Username: "test",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			principal, err := auth.Authenticate(ctx, tt.creds)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if principal == nil {
				t.Fatal("Expected non-nil principal")
			}

			if tt.validate != nil {
				tt.validate(t, principal)
			}
		})
	}
}

func TestAPIKeyAuthenticator_DisabledKey(t *testing.T) {
	auth := NewAPIKeyAuthenticator()

	key := &APIKey{
		Key:         "disabled-key",
		Secret:      "secret",
		PrincipalID: "service-1",
		Type:        PrincipalTypeService,
		Enabled:     false,
		CreatedAt:   time.Now(),
	}

	auth.AddAPIKey(key)

	creds := &APIKeyCredentials{
		APIKey:    "disabled-key",
		APISecret: "secret",
	}

	ctx := context.Background()
	_, err := auth.Authenticate(ctx, creds)
	if err == nil {
		t.Error("Expected error for disabled API key")
	}
}

func TestAPIKeyAuthenticator_ExpiredKey(t *testing.T) {
	auth := NewAPIKeyAuthenticator()

	expiredTime := time.Now().Add(-1 * time.Hour)
	key := &APIKey{
		Key:         "expired-key",
		Secret:      "secret",
		PrincipalID: "service-1",
		Type:        PrincipalTypeService,
		Enabled:     true,
		ExpiresAt:   &expiredTime,
		CreatedAt:   time.Now().Add(-2 * time.Hour),
	}

	auth.AddAPIKey(key)

	creds := &APIKeyCredentials{
		APIKey:    "expired-key",
		APISecret: "secret",
	}

	ctx := context.Background()
	_, err := auth.Authenticate(ctx, creds)
	if err == nil {
		t.Error("Expected error for expired API key")
	}
}

func TestAPIKeyAuthenticator_Method(t *testing.T) {
	auth := NewAPIKeyAuthenticator()
	if auth.Method() != AuthMethodAPIKey {
		t.Errorf("Expected method API_KEY, got %s", auth.Method())
	}
}

func TestSASLAuthenticator_Method(t *testing.T) {
	cfg := &SASLConfig{
		Mechanisms: []AuthMethod{AuthMethodSASLPlain, AuthMethodSASLSCRAM256},
	}
	auth := NewSASLAuthenticator(cfg)

	// Should return first mechanism
	if auth.Method() != AuthMethodSASLPlain {
		t.Errorf("Expected method SASL_PLAIN, got %s", auth.Method())
	}
}

func TestCredentialsType(t *testing.T) {
	tests := []struct {
		name     string
		creds    Credentials
		expected AuthMethod
	}{
		{
			name: "TLS credentials",
			creds: &TLSCredentials{
				CommonName: "test",
			},
			expected: AuthMethodMTLS,
		},
		{
			name: "SASL PLAIN credentials",
			creds: &SASLCredentials{
				Mechanism: AuthMethodSASLPlain,
			},
			expected: AuthMethodSASLPlain,
		},
		{
			name: "SASL SCRAM credentials",
			creds: &SASLCredentials{
				Mechanism: AuthMethodSASLSCRAM256,
			},
			expected: AuthMethodSASLSCRAM256,
		},
		{
			name:     "API key credentials",
			creds:    &APIKeyCredentials{},
			expected: AuthMethodAPIKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.creds.Type() != tt.expected {
				t.Errorf("Expected type %s, got %s", tt.expected, tt.creds.Type())
			}
		})
	}
}
