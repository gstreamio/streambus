package security

import (
	"context"
	"fmt"
	"sync"

	"github.com/gstreamio/streambus/pkg/logging"
)

// Manager manages all security components
type Manager struct {
	config *SecurityConfig
	logger *logging.Logger

	// Authentication
	authenticators map[AuthMethod]Authenticator
	authEnabled    bool

	// Authorization
	authorizer     Authorizer
	authzEnabled   bool

	// Audit logging
	auditLogger  AuditLogger
	auditEnabled bool

	// Encryption
	encryption        *EncryptionService
	encryptionEnabled bool

	mu sync.RWMutex
}

// NewManager creates a new security manager
func NewManager(config *SecurityConfig, logger *logging.Logger) (*Manager, error) {
	if config == nil {
		config = DefaultSecurityConfig()
	}

	manager := &Manager{
		config:         config,
		logger:         logger,
		authenticators: make(map[AuthMethod]Authenticator),
		authEnabled:    config.TLS != nil && config.TLS.Enabled || config.SASL != nil && config.SASL.Enabled,
		authzEnabled:   config.AuthzEnabled,
		auditEnabled:   config.AuditEnabled,
	}

	// Initialize authenticators
	if err := manager.initializeAuthenticators(); err != nil {
		return nil, fmt.Errorf("failed to initialize authenticators: %w", err)
	}

	// Initialize authorizer
	if config.AuthzEnabled {
		manager.authorizer = NewACLAuthorizer(config)

		// Load default ACLs if configured
		if config.UseDefaultACLs {
			for _, acl := range DefaultACLs() {
				_ = manager.authorizer.(*ACLAuthorizer).AddACL(acl)
			}
		}
	}

	// Initialize audit logger
	if config.AuditEnabled {
		if err := manager.initializeAuditLogger(); err != nil {
			return nil, fmt.Errorf("failed to initialize audit logger: %w", err)
		}
	}

	// Initialize encryption
	if config.Encryption != nil && config.Encryption.Enabled {
		encService, err := NewEncryptionService(config.Encryption)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize encryption: %w", err)
		}
		manager.encryption = encService
		manager.encryptionEnabled = true
	}

	return manager, nil
}

// initializeAuthenticators initializes all configured authenticators
func (m *Manager) initializeAuthenticators() error {
	// TLS/mTLS authenticator
	if m.config.TLS != nil && m.config.TLS.Enabled && m.config.TLS.RequireClientCert {
		tlsAuth := NewTLSAuthenticator(m.config.TLS)
		m.authenticators[AuthMethodMTLS] = tlsAuth
		m.logger.Info("TLS/mTLS authenticator initialized")
	}

	// SASL authenticators
	if m.config.SASL != nil && m.config.SASL.Enabled {
		saslAuth := NewSASLAuthenticator(m.config.SASL)

		// Register each allowed mechanism
		for _, mechanism := range m.config.SASL.Mechanisms {
			m.authenticators[mechanism] = saslAuth
		}
		m.logger.Info("SASL authenticators initialized", logging.Fields{
			"mechanisms": m.config.SASL.Mechanisms,
		})
	}

	// API Key authenticator
	if m.config.APIKeyEnabled {
		apiKeyAuth := NewAPIKeyAuthenticator()
		// TODO: Load API keys from configuration
		m.authenticators[AuthMethodAPIKey] = apiKeyAuth
		m.logger.Info("API Key authenticator initialized")
	}

	return nil
}

// initializeAuditLogger initializes the audit logger
func (m *Manager) initializeAuditLogger() error {
	var loggers []AuditLogger

	// File audit logger
	if m.config.AuditLogFile != "" {
		fileLogger, err := NewFileAuditLogger(m.config.AuditLogFile, m.logger)
		if err != nil {
			return fmt.Errorf("failed to create file audit logger: %w", err)
		}
		loggers = append(loggers, fileLogger)
		m.logger.Info("File audit logger initialized", logging.Fields{
			"file": m.config.AuditLogFile,
		})
	}

	// Structured audit logger (always add)
	structuredLogger := NewStructuredAuditLogger(m.logger)
	loggers = append(loggers, structuredLogger)

	// Use multi-logger if multiple loggers configured
	if len(loggers) > 1 {
		m.auditLogger = NewMultiAuditLogger(loggers...)
	} else if len(loggers) == 1 {
		m.auditLogger = loggers[0]
	} else {
		m.auditLogger = NewStructuredAuditLogger(m.logger)
	}

	return nil
}

// Authenticate authenticates credentials and returns a principal
func (m *Manager) Authenticate(ctx context.Context, method AuthMethod, credentials interface{}) (*Principal, error) {
	if !m.authEnabled {
		// If authentication is disabled, return anonymous principal
		if m.config.AllowAnonymous {
			return &Principal{
				ID:   "anonymous",
				Type: PrincipalTypeAnonymous,
			}, nil
		}
		return nil, fmt.Errorf("authentication is disabled and anonymous access is not allowed")
	}

	// Get authenticator for method
	authenticator, exists := m.authenticators[method]
	if !exists {
		return nil, fmt.Errorf("authentication method %s not supported", method)
	}

	// Authenticate
	principal, err := authenticator.Authenticate(ctx, credentials)
	if err != nil {
		// Audit failed authentication
		if m.auditEnabled {
			m.auditFailedAuth(ctx, method, err)
		}
		return nil, err
	}

	// Audit successful authentication
	if m.auditEnabled {
		m.auditSuccessfulAuth(ctx, principal)
	}

	return principal, nil
}

// Authorize checks if a principal is authorized to perform an action
func (m *Manager) Authorize(ctx context.Context, principal *Principal, action Action, resource Resource) (bool, error) {
	if !m.authzEnabled {
		return true, nil
	}

	allowed, err := m.authorizer.Authorize(ctx, principal, action, resource)

	// Audit authorization decision
	if m.auditEnabled {
		result := AuditResultSuccess
		if !allowed {
			result = AuditResultDenied
		}
		if err != nil {
			result = AuditResultFailure
		}

		m.audit(ctx, &AuditEvent{
			Principal:    principal,
			Action:       action,
			Resource:     resource,
			Result:       result,
			ErrorMessage: formatError(err),
		})
	}

	return allowed, err
}

// Encrypt encrypts data if encryption is enabled
func (m *Manager) Encrypt(data []byte) (*EncryptedData, error) {
	if !m.encryptionEnabled {
		return &EncryptedData{Data: data}, nil
	}
	return m.encryption.Encrypt(data)
}

// Decrypt decrypts data if encryption is enabled
func (m *Manager) Decrypt(encrypted *EncryptedData) ([]byte, error) {
	if !m.encryptionEnabled {
		return encrypted.Data, nil
	}
	return m.encryption.Decrypt(encrypted)
}

// AuditAction audits a successful action
func (m *Manager) AuditAction(ctx context.Context, principal *Principal, action Action, resource Resource, metadata map[string]string) {
	if !m.auditEnabled {
		return
	}

	event := &AuditEvent{
		Principal: principal,
		Action:    action,
		Resource:  resource,
		Result:    AuditResultSuccess,
		Metadata:  metadata,
	}

	m.audit(ctx, event)
}

// AuditFailure audits a failed action
func (m *Manager) AuditFailure(ctx context.Context, principal *Principal, action Action, resource Resource, err error) {
	if !m.auditEnabled {
		return
	}

	event := &AuditEvent{
		Principal:    principal,
		Action:       action,
		Resource:     resource,
		Result:       AuditResultFailure,
		ErrorMessage: err.Error(),
	}

	m.audit(ctx, event)
}

// audit logs an audit event
func (m *Manager) audit(ctx context.Context, event *AuditEvent) {
	if m.auditLogger == nil {
		return
	}

	// Set client IP from context if available
	if clientIP, ok := ctx.Value("client_ip").(string); ok && event.ClientIP == "" {
		event.ClientIP = clientIP
	}

	if err := m.auditLogger.LogEvent(event); err != nil {
		m.logger.Error("Failed to log audit event", err)
	}
}

// auditSuccessfulAuth audits a successful authentication
func (m *Manager) auditSuccessfulAuth(ctx context.Context, principal *Principal) {
	event := &AuditEvent{
		Principal: principal,
		Action:    Action("AUTHENTICATE"),
		Resource: Resource{
			Type: ResourceType("AUTH"),
			Name: "authentication",
		},
		Result:   AuditResultSuccess,
		ClientIP: principal.ClientIP,
	}

	m.audit(ctx, event)
}

// auditFailedAuth audits a failed authentication
func (m *Manager) auditFailedAuth(ctx context.Context, method AuthMethod, err error) {
	event := &AuditEvent{
		Principal: &Principal{
			ID:         "unknown",
			Type:       PrincipalTypeAnonymous,
			AuthMethod: method,
		},
		Action: Action("AUTHENTICATE"),
		Resource: Resource{
			Type: ResourceType("AUTH"),
			Name: "authentication",
		},
		Result:       AuditResultFailure,
		ErrorMessage: err.Error(),
	}

	m.audit(ctx, event)
}

// GetAuthorizer returns the authorizer
func (m *Manager) GetAuthorizer() Authorizer {
	return m.authorizer
}

// GetEncryption returns the encryption service
func (m *Manager) GetEncryption() *EncryptionService {
	return m.encryption
}

// AddACL adds an ACL entry (if authorization is enabled)
func (m *Manager) AddACL(entry *ACLEntry) error {
	if !m.authzEnabled {
		return fmt.Errorf("authorization is not enabled")
	}

	aclAuth, ok := m.authorizer.(*ACLAuthorizer)
	if !ok {
		return fmt.Errorf("authorizer does not support ACLs")
	}

	return aclAuth.AddACL(entry)
}

// RemoveACL removes an ACL entry
func (m *Manager) RemoveACL(id string) error {
	if !m.authzEnabled {
		return fmt.Errorf("authorization is not enabled")
	}

	aclAuth, ok := m.authorizer.(*ACLAuthorizer)
	if !ok {
		return fmt.Errorf("authorizer does not support ACLs")
	}

	return aclAuth.RemoveACL(id)
}

// ListACLs lists all ACL entries
func (m *Manager) ListACLs() []*ACLEntry {
	if !m.authzEnabled {
		return nil
	}

	aclAuth, ok := m.authorizer.(*ACLAuthorizer)
	if !ok {
		return nil
	}

	return aclAuth.ListACLs()
}

// AddUser adds a SASL user
func (m *Manager) AddUser(username, password string, authMethod AuthMethod, groups []string) error {
	if m.config.SASL == nil || !m.config.SASL.Enabled {
		return fmt.Errorf("SASL authentication is not enabled")
	}

	user, err := CreateUser(username, password, authMethod, groups)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.config.SASL.Users[username] = user
	return nil
}

// AddAPIKey adds an API key
func (m *Manager) AddAPIKey(key *APIKey) error {
	if !m.config.APIKeyEnabled {
		return fmt.Errorf("API key authentication is not enabled")
	}

	authenticator, exists := m.authenticators[AuthMethodAPIKey]
	if !exists {
		return fmt.Errorf("API key authenticator not initialized")
	}

	apiKeyAuth, ok := authenticator.(*APIKeyAuthenticator)
	if !ok {
		return fmt.Errorf("invalid authenticator type")
	}

	apiKeyAuth.AddAPIKey(key)
	return nil
}

// Close closes the security manager and all its components
func (m *Manager) Close() error {
	m.logger.Info("Closing security manager")

	// Close encryption service
	if m.encryption != nil {
		m.encryption.Close()
	}

	// Close file audit logger if it exists
	if m.auditLogger != nil {
		if fileLogger, ok := m.auditLogger.(*FileAuditLogger); ok {
			if err := fileLogger.Close(); err != nil {
				m.logger.Error("Failed to close file audit logger", err)
			}
		} else if multiLogger, ok := m.auditLogger.(*MultiAuditLogger); ok {
			// Close all file loggers in multi-logger
			for _, logger := range multiLogger.loggers {
				if fileLogger, ok := logger.(*FileAuditLogger); ok {
					if err := fileLogger.Close(); err != nil {
						m.logger.Error("Failed to close file audit logger", err)
					}
				}
			}
		}
	}

	return nil
}

// IsAuthenticationEnabled returns whether authentication is enabled
func (m *Manager) IsAuthenticationEnabled() bool {
	return m.authEnabled
}

// IsAuthorizationEnabled returns whether authorization is enabled
func (m *Manager) IsAuthorizationEnabled() bool {
	return m.authzEnabled
}

// IsAuditEnabled returns whether audit logging is enabled
func (m *Manager) IsAuditEnabled() bool {
	return m.auditEnabled
}

// IsEncryptionEnabled returns whether encryption is enabled
func (m *Manager) IsEncryptionEnabled() bool {
	return m.encryptionEnabled
}

// formatError formats an error for audit logging
func formatError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// DefaultSecurityConfig returns a default security configuration
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		TLS:              DefaultTLSConfig(),
		SASL:             nil,
		AuthzEnabled:     false,
		SuperUsers:       []string{},
		AllowAnonymous:   true,
		AuditEnabled:     false,
		APIKeyEnabled:    false,
		UseDefaultACLs:   false,
		Encryption:       DefaultEncryptionConfig(),
	}
}
