package security

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ACLAuthorizer implements ACL-based authorization
type ACLAuthorizer struct {
	mu      sync.RWMutex
	entries map[string]*ACLEntry // ID -> entry
	config  *SecurityConfig
}

// NewACLAuthorizer creates a new ACL authorizer
func NewACLAuthorizer(config *SecurityConfig) *ACLAuthorizer {
	return &ACLAuthorizer{
		entries: make(map[string]*ACLEntry),
		config:  config,
	}
}

// Authorize checks if the principal can perform the action on the resource
func (a *ACLAuthorizer) Authorize(ctx context.Context, principal *Principal, action Action, resource Resource) (bool, error) {
	// Check if authorization is disabled
	if !a.config.AuthzEnabled {
		return true, nil
	}

	// Check if principal is a super user
	if principal.IsSuperUser(a.config.SuperUsers) {
		return true, nil
	}

	// System internal principals have full access
	if principal.Type == PrincipalTypeSystemInternal {
		return true, nil
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	// Check for explicit DENY first (takes precedence)
	for _, entry := range a.entries {
		if a.matchesEntry(principal, action, resource, entry) {
			if entry.Permission == PermissionDeny {
				return false, nil
			}
		}
	}

	// Check for explicit ALLOW
	for _, entry := range a.entries {
		if a.matchesEntry(principal, action, resource, entry) {
			if entry.Permission == PermissionAllow {
				return true, nil
			}
		}
	}

	// Default deny if no matching ACL found
	return false, nil
}

// matchesEntry checks if an ACL entry matches the authorization request
func (a *ACLAuthorizer) matchesEntry(principal *Principal, action Action, resource Resource, entry *ACLEntry) bool {
	// Check if principal matches
	if !a.matchesPrincipal(principal, entry.Principal) {
		return false
	}

	// Check if resource type matches
	if entry.ResourceType != resource.Type {
		return false
	}

	// Check if action matches
	if entry.Action != action {
		return false
	}

	// Check if resource name matches based on pattern type
	return a.matchesResourceName(resource.Name, entry.ResourceName, entry.PatternType)
}

// matchesPrincipal checks if a principal matches an ACL principal pattern
func (a *ACLAuthorizer) matchesPrincipal(principal *Principal, pattern string) bool {
	// Check for wildcard
	if pattern == "*" {
		return true
	}

	// Exact match on principal ID
	if pattern == principal.ID {
		return true
	}

	// Check for group match (pattern like "group:admin")
	if strings.HasPrefix(pattern, "group:") {
		groupName := strings.TrimPrefix(pattern, "group:")
		return principal.HasGroup(groupName)
	}

	// Check for type match (pattern like "type:SERVICE")
	if strings.HasPrefix(pattern, "type:") {
		typeName := strings.TrimPrefix(pattern, "type:")
		return string(principal.Type) == typeName
	}

	return false
}

// matchesResourceName checks if a resource name matches a pattern
func (a *ACLAuthorizer) matchesResourceName(name, pattern string, patternType PatternType) bool {
	switch patternType {
	case PatternTypeLiteral:
		return name == pattern

	case PatternTypePrefix:
		return strings.HasPrefix(name, pattern)

	case PatternTypeWildcard:
		matched, _ := filepath.Match(pattern, name)
		return matched

	default:
		return false
	}
}

// AddACL adds an ACL entry
func (a *ACLAuthorizer) AddACL(entry *ACLEntry) error {
	if entry.ID == "" {
		entry.ID = generateACLID()
	}

	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.entries[entry.ID] = entry
	return nil
}

// RemoveACL removes an ACL entry by ID
func (a *ACLAuthorizer) RemoveACL(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.entries[id]; !exists {
		return fmt.Errorf("ACL entry not found: %s", id)
	}

	delete(a.entries, id)
	return nil
}

// ListACLs lists all ACL entries
func (a *ACLAuthorizer) ListACLs() []*ACLEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()

	entries := make([]*ACLEntry, 0, len(a.entries))
	for _, entry := range a.entries {
		entries = append(entries, entry)
	}
	return entries
}

// GetACL retrieves an ACL entry by ID
func (a *ACLAuthorizer) GetACL(id string) (*ACLEntry, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	entry, exists := a.entries[id]
	if !exists {
		return nil, fmt.Errorf("ACL entry not found: %s", id)
	}

	return entry, nil
}

// FindACLs finds ACL entries matching the criteria
func (a *ACLAuthorizer) FindACLs(resourceType ResourceType, resourceName string, action Action) []*ACLEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var matches []*ACLEntry
	for _, entry := range a.entries {
		if entry.ResourceType == resourceType &&
			entry.Action == action &&
			a.matchesResourceName(resourceName, entry.ResourceName, entry.PatternType) {
			matches = append(matches, entry)
		}
	}

	return matches
}

// ClearACLs removes all ACL entries
func (a *ACLAuthorizer) ClearACLs() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.entries = make(map[string]*ACLEntry)
}

// generateACLID generates a unique ACL ID
func generateACLID() string {
	return fmt.Sprintf("acl-%d", time.Now().UnixNano())
}

// ACLBuilder helps build ACL entries fluently
type ACLBuilder struct {
	entry *ACLEntry
}

// NewACLBuilder creates a new ACL builder
func NewACLBuilder() *ACLBuilder {
	return &ACLBuilder{
		entry: &ACLEntry{},
	}
}

// ForPrincipal sets the principal
func (b *ACLBuilder) ForPrincipal(principal string) *ACLBuilder {
	b.entry.Principal = principal
	return b
}

// OnResource sets the resource
func (b *ACLBuilder) OnResource(resourceType ResourceType, resourceName string) *ACLBuilder {
	b.entry.ResourceType = resourceType
	b.entry.ResourceName = resourceName
	return b
}

// WithPatternType sets the pattern type
func (b *ACLBuilder) WithPatternType(patternType PatternType) *ACLBuilder {
	b.entry.PatternType = patternType
	return b
}

// ForAction sets the action
func (b *ACLBuilder) ForAction(action Action) *ACLBuilder {
	b.entry.Action = action
	return b
}

// Allow sets permission to allow
func (b *ACLBuilder) Allow() *ACLBuilder {
	b.entry.Permission = PermissionAllow
	return b
}

// Deny sets permission to deny
func (b *ACLBuilder) Deny() *ACLBuilder {
	b.entry.Permission = PermissionDeny
	return b
}

// CreatedBy sets who created the ACL
func (b *ACLBuilder) CreatedBy(creator string) *ACLBuilder {
	b.entry.CreatedBy = creator
	return b
}

// Build builds the ACL entry
func (b *ACLBuilder) Build() *ACLEntry {
	if b.entry.ID == "" {
		b.entry.ID = generateACLID()
	}
	if b.entry.CreatedAt.IsZero() {
		b.entry.CreatedAt = time.Now()
	}
	if b.entry.PatternType == "" {
		b.entry.PatternType = PatternTypeLiteral
	}
	return b.entry
}

// DefaultACLs returns a set of default ACLs for common scenarios
func DefaultACLs() []*ACLEntry {
	return []*ACLEntry{
		// Allow all authenticated users to read topics
		NewACLBuilder().
			ForPrincipal("*").
			OnResource(ResourceTypeTopic, "*").
			WithPatternType(PatternTypeWildcard).
			ForAction(ActionTopicRead).
			Allow().
			CreatedBy("system").
			Build(),

		// Allow all authenticated users to describe topics
		NewACLBuilder().
			ForPrincipal("*").
			OnResource(ResourceTypeTopic, "*").
			WithPatternType(PatternTypeWildcard).
			ForAction(ActionTopicDescribe).
			Allow().
			CreatedBy("system").
			Build(),

		// Allow all authenticated users to read consumer groups
		NewACLBuilder().
			ForPrincipal("*").
			OnResource(ResourceTypeConsumerGroup, "*").
			WithPatternType(PatternTypeWildcard).
			ForAction(ActionGroupRead).
			Allow().
			CreatedBy("system").
			Build(),
	}
}

// RestrictiveACLs returns a set of restrictive default ACLs
func RestrictiveACLs() []*ACLEntry {
	return []*ACLEntry{
		// Deny all by default - must explicitly grant permissions
		// (This is handled by the default deny behavior)
	}
}
