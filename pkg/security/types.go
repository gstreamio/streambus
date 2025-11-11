package security

import (
	"context"
	"crypto/tls"
	"strings"
	"time"
)

// Principal represents an authenticated user or service
type Principal struct {
	ID           string            // Unique identifier (username, service account ID, etc.)
	Type         PrincipalType     // Type of principal
	AuthMethod   AuthMethod        // How the principal was authenticated
	Attributes   map[string]string // Additional attributes (groups, roles, etc.)
	AuthTime     time.Time         // When authentication occurred
	ExpiresAt    *time.Time        // Optional expiration time
	ClientIP     string            // Client IP address
	ClientCertCN string            // Common name from client certificate (if mTLS)
}

// PrincipalType represents the type of principal
type PrincipalType string

const (
	PrincipalTypeUser           PrincipalType = "USER"
	PrincipalTypeService        PrincipalType = "SERVICE"
	PrincipalTypeAnonymous      PrincipalType = "ANONYMOUS"
	PrincipalTypeSystemInternal PrincipalType = "SYSTEM_INTERNAL"
)

// AuthMethod represents the authentication method used
type AuthMethod string

const (
	AuthMethodNone         AuthMethod = "NONE"
	AuthMethodTLS          AuthMethod = "TLS"
	AuthMethodMTLS         AuthMethod = "MTLS"
	AuthMethodSASLPlain    AuthMethod = "SASL_PLAIN"
	AuthMethodSASLSCRAM256 AuthMethod = "SASL_SCRAM_SHA256"
	AuthMethodSASLSCRAM512 AuthMethod = "SASL_SCRAM_SHA512"
	AuthMethodAPIKey       AuthMethod = "API_KEY"
	AuthMethodOAuth        AuthMethod = "OAUTH"
	AuthMethodJWT          AuthMethod = "JWT"
)

// Authenticator authenticates a request and returns a Principal
type Authenticator interface {
	// Authenticate authenticates the request and returns a Principal
	Authenticate(ctx context.Context, credentials interface{}) (*Principal, error)

	// Method returns the authentication method supported
	Method() AuthMethod
}

// Authorizer checks if a principal has permission to perform an action
type Authorizer interface {
	// Authorize checks if the principal can perform the action on the resource
	Authorize(ctx context.Context, principal *Principal, action Action, resource Resource) (bool, error)
}

// Action represents an action that can be performed
type Action string

const (
	// Topic actions
	ActionTopicCreate    Action = "TOPIC_CREATE"
	ActionTopicDelete    Action = "TOPIC_DELETE"
	ActionTopicDescribe  Action = "TOPIC_DESCRIBE"
	ActionTopicAlter     Action = "TOPIC_ALTER"
	ActionTopicWrite     Action = "TOPIC_WRITE"
	ActionTopicRead      Action = "TOPIC_READ"
	ActionTopicReadCommit Action = "TOPIC_READ_COMMIT"

	// Consumer group actions
	ActionGroupRead     Action = "GROUP_READ"
	ActionGroupDescribe Action = "GROUP_DESCRIBE"
	ActionGroupDelete   Action = "GROUP_DELETE"

	// Cluster actions
	ActionClusterDescribe Action = "CLUSTER_DESCRIBE"
	ActionClusterAlter    Action = "CLUSTER_ALTER"
	ActionClusterAction   Action = "CLUSTER_ACTION"

	// Transaction actions
	ActionTransactionDescribe Action = "TXN_DESCRIBE"
	ActionTransactionWrite    Action = "TXN_WRITE"

	// Schema actions
	ActionSchemaRead    Action = "SCHEMA_READ"
	ActionSchemaWrite   Action = "SCHEMA_WRITE"
	ActionSchemaDelete  Action = "SCHEMA_DELETE"
	ActionSchemaCompat  Action = "SCHEMA_COMPAT"
)

// ResourceType represents the type of resource
type ResourceType string

const (
	ResourceTypeTopic         ResourceType = "TOPIC"
	ResourceTypeConsumerGroup ResourceType = "GROUP"
	ResourceTypeCluster       ResourceType = "CLUSTER"
	ResourceTypeTransaction   ResourceType = "TRANSACTION"
	ResourceTypeSchema        ResourceType = "SCHEMA"
)

// Resource represents a resource that can be accessed
type Resource struct {
	Type       ResourceType // Type of resource
	Name       string       // Resource name (topic name, group ID, etc.)
	PatternType PatternType // How to match the name
}

// PatternType defines how resource names are matched
type PatternType string

const (
	PatternTypeLiteral  PatternType = "LITERAL"  // Exact match
	PatternTypePrefix   PatternType = "PREFIX"   // Prefix match
	PatternTypeWildcard PatternType = "WILDCARD" // Wildcard match (* and ?)
)

// Permission represents a permission granted to a principal
type Permission string

const (
	PermissionAllow Permission = "ALLOW"
	PermissionDeny  Permission = "DENY"
)

// ACLEntry represents an Access Control List entry
type ACLEntry struct {
	ID           string       // Unique ID
	Principal    string       // Principal ID (user, service, group)
	ResourceType ResourceType // Type of resource
	ResourceName string       // Resource name or pattern
	PatternType  PatternType  // How to match resource name
	Action       Action       // Action being controlled
	Permission   Permission   // Allow or deny
	CreatedAt    time.Time    // When ACL was created
	CreatedBy    string       // Who created the ACL
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	// Enable TLS
	Enabled bool

	// Certificate and key files
	CertFile string
	KeyFile  string

	// CA certificate file (for verifying client certificates)
	CAFile string

	// Require client certificates (mTLS)
	RequireClientCert bool

	// Verify client certificates
	VerifyClientCert bool

	// Allowed client certificate CNs (for mTLS)
	AllowedClientCNs []string

	// Minimum TLS version
	MinVersion uint16 // tls.VersionTLS12, tls.VersionTLS13

	// Cipher suites (nil = use defaults)
	CipherSuites []uint16

	// Internal TLS config (built from above)
	config *tls.Config
}

// SASLConfig holds SASL authentication configuration
type SASLConfig struct {
	// Enable SASL
	Enabled bool

	// Allowed mechanisms
	Mechanisms []AuthMethod

	// User database (for PLAIN and SCRAM)
	Users map[string]*User
}

// User represents a user for authentication
type User struct {
	Username     string
	PasswordHash []byte // bcrypt hash
	Salt         []byte // For SCRAM
	Iterations   int    // For SCRAM
	Groups       []string
	Attributes   map[string]string
	Enabled      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Credentials represents authentication credentials
type Credentials interface {
	Type() AuthMethod
}

// TLSCredentials represents TLS-based credentials
type TLSCredentials struct {
	ClientCert *tls.Certificate
	CommonName string
	DNSNames   []string
}

func (c *TLSCredentials) Type() AuthMethod {
	return AuthMethodMTLS
}

// SASLCredentials represents SASL credentials
type SASLCredentials struct {
	Username  string
	Password  string
	Mechanism AuthMethod
}

func (c *SASLCredentials) Type() AuthMethod {
	return c.Mechanism
}

// APIKeyCredentials represents API key credentials
type APIKeyCredentials struct {
	APIKey    string
	APISecret string
}

func (c *APIKeyCredentials) Type() AuthMethod {
	return AuthMethodAPIKey
}

// SecurityConfig holds overall security configuration
type SecurityConfig struct {
	// TLS configuration
	TLS *TLSConfig

	// SASL configuration
	SASL *SASLConfig

	// Enable authorization
	AuthzEnabled bool

	// Super users (bypass all ACL checks)
	SuperUsers []string

	// Allow anonymous access
	AllowAnonymous bool

	// Audit logging
	AuditEnabled bool

	// Audit log file path
	AuditLogFile string

	// Enable API key authentication
	APIKeyEnabled bool

	// Use default ACLs
	UseDefaultACLs bool

	// Encryption configuration
	Encryption *EncryptionConfig
}

// AuditEvent represents a security audit event
type AuditEvent struct {
	Timestamp    time.Time
	Principal    *Principal
	Action       Action
	Resource     Resource
	Result       AuditResult
	ClientIP     string
	ErrorMessage string
	Metadata     map[string]string
}

// AuditResult represents the result of an audited action
type AuditResult string

const (
	AuditResultSuccess AuditResult = "SUCCESS"
	AuditResultFailure AuditResult = "FAILURE"
	AuditResultDenied  AuditResult = "DENIED"
)

// AuditLogger logs security audit events
type AuditLogger interface {
	// LogEvent logs a security audit event
	LogEvent(event *AuditEvent) error
}

// contextKey is used for storing values in context
type contextKey string

const (
	// PrincipalContextKey is the key for storing principal in context
	PrincipalContextKey contextKey = "security.principal"
)

// ContextWithPrincipal adds a principal to the context
func ContextWithPrincipal(ctx context.Context, principal *Principal) context.Context {
	return context.WithValue(ctx, PrincipalContextKey, principal)
}

// PrincipalFromContext extracts the principal from the context
func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	principal, ok := ctx.Value(PrincipalContextKey).(*Principal)
	return principal, ok
}

// IsSuperUser checks if the principal is a super user
func (p *Principal) IsSuperUser(superUsers []string) bool {
	for _, su := range superUsers {
		if p.ID == su {
			return true
		}
	}
	return false
}

// HasGroup checks if the principal belongs to a group
func (p *Principal) HasGroup(group string) bool {
	groupsStr, ok := p.Attributes["groups"]
	if !ok {
		return false
	}

	// Groups are stored as a string, could be comma-separated or space-separated
	// or in fmt.Sprintf format like "[group1 group2]"
	// Simple approach: check if the group name is contained in the string
	return strings.Contains(groupsStr, group)
}

// IsExpired checks if the principal's authentication has expired
func (p *Principal) IsExpired() bool {
	if p.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*p.ExpiresAt)
}
