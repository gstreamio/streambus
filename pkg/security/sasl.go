package security

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"hash"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
)

// SASLAuthenticator handles SASL authentication
type SASLAuthenticator struct {
	config *SASLConfig
}

// NewSASLAuthenticator creates a new SASL authenticator
func NewSASLAuthenticator(cfg *SASLConfig) *SASLAuthenticator {
	if cfg.Users == nil {
		cfg.Users = make(map[string]*User)
	}
	return &SASLAuthenticator{
		config: cfg,
	}
}

// Authenticate authenticates using SASL
func (a *SASLAuthenticator) Authenticate(ctx context.Context, credentials interface{}) (*Principal, error) {
	saslCreds, ok := credentials.(*SASLCredentials)
	if !ok {
		return nil, fmt.Errorf("invalid credentials type for SASL authentication")
	}

	// Check if mechanism is allowed
	if !a.isMechanismAllowed(saslCreds.Mechanism) {
		return nil, fmt.Errorf("SASL mechanism %s not allowed", saslCreds.Mechanism)
	}

	// Get user
	user, exists := a.config.Users[saslCreds.Username]
	if !exists {
		return nil, fmt.Errorf("user not found")
	}

	if !user.Enabled {
		return nil, fmt.Errorf("user is disabled")
	}

	// Authenticate based on mechanism
	var err error
	switch saslCreds.Mechanism {
	case AuthMethodSASLPlain:
		err = a.authenticatePlain(saslCreds, user)
	case AuthMethodSASLSCRAM256:
		err = a.authenticateSCRAM(saslCreds, user, sha256.New)
	case AuthMethodSASLSCRAM512:
		err = a.authenticateSCRAM(saslCreds, user, sha512.New)
	default:
		return nil, fmt.Errorf("unsupported SASL mechanism: %s", saslCreds.Mechanism)
	}

	if err != nil {
		return nil, err
	}

	// Create principal
	principal := &Principal{
		ID:         user.Username,
		Type:       PrincipalTypeUser,
		AuthMethod: saslCreds.Mechanism,
		Attributes: make(map[string]string),
		AuthTime:   time.Now(),
	}

	// Copy user attributes
	for k, v := range user.Attributes {
		principal.Attributes[k] = v
	}

	// Add groups
	if len(user.Groups) > 0 {
		principal.Attributes["groups"] = fmt.Sprintf("%v", user.Groups)
	}

	return principal, nil
}

// Method returns the authentication method
func (a *SASLAuthenticator) Method() AuthMethod {
	// Return the first allowed mechanism as default
	if len(a.config.Mechanisms) > 0 {
		return a.config.Mechanisms[0]
	}
	return AuthMethodSASLPlain
}

// isMechanismAllowed checks if a mechanism is allowed
func (a *SASLAuthenticator) isMechanismAllowed(mechanism AuthMethod) bool {
	for _, m := range a.config.Mechanisms {
		if m == mechanism {
			return true
		}
	}
	return false
}

// authenticatePlain authenticates using SASL PLAIN
func (a *SASLAuthenticator) authenticatePlain(creds *SASLCredentials, user *User) error {
	// Compare password with bcrypt hash
	err := bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(creds.Password))
	if err != nil {
		return fmt.Errorf("invalid password")
	}
	return nil
}

// authenticateSCRAM authenticates using SASL SCRAM
func (a *SASLAuthenticator) authenticateSCRAM(creds *SASLCredentials, user *User, hashFunc func() hash.Hash) error {
	// Generate client key and stored key
	// This is a simplified SCRAM implementation
	// In production, this would follow RFC 5802 completely

	if len(user.Salt) == 0 || user.Iterations == 0 {
		return fmt.Errorf("user not configured for SCRAM")
	}

	// Derive salted password
	saltedPassword := pbkdf2.Key([]byte(creds.Password), user.Salt, user.Iterations, hashFunc().Size(), hashFunc)

	// Generate client key
	mac := hmac.New(hashFunc, saltedPassword)
	mac.Write([]byte("Client Key"))
	clientKey := mac.Sum(nil)

	// Hash client key to get stored key
	h := hashFunc()
	h.Write(clientKey)
	storedKey := h.Sum(nil)

	// Compare with user's stored key (password hash for SCRAM)
	if !hmac.Equal(storedKey, user.PasswordHash) {
		return fmt.Errorf("invalid password")
	}

	return nil
}

// CreateUser creates a new user with hashed password
func CreateUser(username, password string, authMethod AuthMethod, groups []string) (*User, error) {
	user := &User{
		Username:   username,
		Groups:     groups,
		Attributes: make(map[string]string),
		Enabled:    true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	switch authMethod {
	case AuthMethodSASLPlain:
		// Use bcrypt for PLAIN
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		user.PasswordHash = hash

	case AuthMethodSASLSCRAM256:
		return createSCRAMUser(user, password, sha256.New, 4096)

	case AuthMethodSASLSCRAM512:
		return createSCRAMUser(user, password, sha512.New, 4096)

	default:
		return nil, fmt.Errorf("unsupported auth method: %s", authMethod)
	}

	return user, nil
}

// createSCRAMUser creates a user configured for SCRAM
func createSCRAMUser(user *User, password string, hashFunc func() hash.Hash, iterations int) (*User, error) {
	// Generate random salt
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	user.Salt = salt
	user.Iterations = iterations

	// Derive salted password
	saltedPassword := pbkdf2.Key([]byte(password), salt, iterations, hashFunc().Size(), hashFunc)

	// Generate client key
	mac := hmac.New(hashFunc, saltedPassword)
	mac.Write([]byte("Client Key"))
	clientKey := mac.Sum(nil)

	// Hash client key to get stored key
	h := hashFunc()
	h.Write(clientKey)
	storedKey := h.Sum(nil)

	user.PasswordHash = storedKey

	return user, nil
}

// VerifyPassword verifies a password against a user
func VerifyPassword(user *User, password string, authMethod AuthMethod) error {
	switch authMethod {
	case AuthMethodSASLPlain:
		return bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(password))

	case AuthMethodSASLSCRAM256:
		return verifySCRAMPassword(user, password, sha256.New)

	case AuthMethodSASLSCRAM512:
		return verifySCRAMPassword(user, password, sha512.New)

	default:
		return fmt.Errorf("unsupported auth method: %s", authMethod)
	}
}

// verifySCRAMPassword verifies a password using SCRAM
func verifySCRAMPassword(user *User, password string, hashFunc func() hash.Hash) error {
	if len(user.Salt) == 0 || user.Iterations == 0 {
		return fmt.Errorf("user not configured for SCRAM")
	}

	// Derive salted password
	saltedPassword := pbkdf2.Key([]byte(password), user.Salt, user.Iterations, hashFunc().Size(), hashFunc)

	// Generate client key
	mac := hmac.New(hashFunc, saltedPassword)
	mac.Write([]byte("Client Key"))
	clientKey := mac.Sum(nil)

	// Hash client key to get stored key
	h := hashFunc()
	h.Write(clientKey)
	storedKey := h.Sum(nil)

	// Compare
	if !hmac.Equal(storedKey, user.PasswordHash) {
		return fmt.Errorf("invalid password")
	}

	return nil
}

// GenerateAPIKey generates a random API key
func GenerateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// APIKeyAuthenticator handles API key authentication
type APIKeyAuthenticator struct {
	keys map[string]*APIKey // key -> APIKey
}

// APIKey represents an API key
type APIKey struct {
	Key        string
	Secret     string
	PrincipalID string
	Type       PrincipalType
	Groups     []string
	Attributes map[string]string
	ExpiresAt  *time.Time
	CreatedAt  time.Time
	Enabled    bool
}

// NewAPIKeyAuthenticator creates a new API key authenticator
func NewAPIKeyAuthenticator() *APIKeyAuthenticator {
	return &APIKeyAuthenticator{
		keys: make(map[string]*APIKey),
	}
}

// AddAPIKey adds an API key
func (a *APIKeyAuthenticator) AddAPIKey(key *APIKey) {
	a.keys[key.Key] = key
}

// Authenticate authenticates using API key
func (a *APIKeyAuthenticator) Authenticate(ctx context.Context, credentials interface{}) (*Principal, error) {
	apiCreds, ok := credentials.(*APIKeyCredentials)
	if !ok {
		return nil, fmt.Errorf("invalid credentials type for API key authentication")
	}

	key, exists := a.keys[apiCreds.APIKey]
	if !exists {
		return nil, fmt.Errorf("invalid API key")
	}

	if !key.Enabled {
		return nil, fmt.Errorf("API key is disabled")
	}

	// Check expiration
	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		return nil, fmt.Errorf("API key has expired")
	}

	// Verify secret (simple equality for now)
	if key.Secret != apiCreds.APISecret {
		return nil, fmt.Errorf("invalid API secret")
	}

	// Create principal
	principal := &Principal{
		ID:         key.PrincipalID,
		Type:       key.Type,
		AuthMethod: AuthMethodAPIKey,
		Attributes: make(map[string]string),
		AuthTime:   time.Now(),
		ExpiresAt:  key.ExpiresAt,
	}

	// Copy attributes
	for k, v := range key.Attributes {
		principal.Attributes[k] = v
	}

	// Add groups
	if len(key.Groups) > 0 {
		principal.Attributes["groups"] = fmt.Sprintf("%v", key.Groups)
	}

	return principal, nil
}

// Method returns the authentication method
func (a *APIKeyAuthenticator) Method() AuthMethod {
	return AuthMethodAPIKey
}
