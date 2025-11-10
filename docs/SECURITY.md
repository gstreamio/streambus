# StreamBus Security Guide

This guide covers all security features in StreamBus, including authentication, authorization, encryption, and audit logging.

## Table of Contents

1. [Security Overview](#security-overview)
2. [Authentication](#authentication)
3. [Authorization & Access Control](#authorization--access-control)
4. [Encryption](#encryption)
5. [Audit Logging](#audit-logging)
6. [Configuration Guide](#configuration-guide)
7. [API Reference](#api-reference)
8. [Best Practices](#best-practices)
9. [Troubleshooting](#troubleshooting)

---

## Security Overview

StreamBus provides enterprise-grade security features:

- **Authentication**: SASL/SCRAM, TLS/mTLS, API Keys
- **Authorization**: Fine-grained ACLs with pattern matching
- **Encryption**: TLS for data in transit, AES-256-GCM for data at rest
- **Audit Logging**: Comprehensive security event logging
- **Multi-layer Security**: Defense in depth with multiple security layers

### Security Architecture

```
Client Request
     ↓
[TLS/mTLS Layer] ← Encryption & Client Cert Validation
     ↓
[Authentication] ← SASL/SCRAM or Certificate-based
     ↓
[Authorization]  ← ACL-based access control
     ↓
[Audit Logging]  ← Record all security events
     ↓
Request Processing
```

---

## Authentication

StreamBus supports multiple authentication mechanisms that can be used individually or combined.

### 1. TLS/mTLS Authentication

TLS provides encryption in transit. Mutual TLS (mTLS) also authenticates clients via certificates.

**Configuration:**

```yaml
security:
  tls:
    enabled: true
    cert_file: "/path/to/broker.crt"
    key_file: "/path/to/broker.key"
    ca_file: "/path/to/ca.crt"

    # Enable mTLS
    require_client_cert: true
    verify_client_cert: true

    # Restrict by certificate CN
    allowed_client_cns:
      - "producer-service"
      - "consumer-service"

    # TLS version
    min_version: 1.3  # Use TLS 1.3 for production
```

**Generating Test Certificates:**

```bash
cd config/security
./generate-certs.sh
```

### 2. SASL/SCRAM Authentication

SASL (Simple Authentication and Security Layer) with SCRAM provides username/password authentication with salted challenge-response.

**Supported Mechanisms:**
- `SASL_SCRAM_SHA256` - SCRAM with SHA-256 (recommended)
- `SASL_SCRAM_SHA512` - SCRAM with SHA-512 (most secure)
- `SASL_PLAIN` - Plain text (not recommended for production)

**Configuration:**

```yaml
security:
  sasl:
    enabled: true
    mechanisms:
      - "SASL_SCRAM_SHA512"
      - "SASL_SCRAM_SHA256"

    users:
      admin:
        username: "admin"
        enabled: true
        groups:
          - "admins"
```

**Creating Users via API:**

```bash
curl -X POST http://localhost:8080/api/v1/security/users \
  -H 'Content-Type: application/json' \
  -d '{
    "username": "producer1",
    "password": "secure-password-here",
    "auth_method": "SASL_SCRAM_SHA256",
    "groups": ["producers"]
  }'
```

### 3. API Key Authentication

API keys provide token-based authentication for programmatic access.

**Configuration:**

```yaml
security:
  api_key_enabled: true
```

API keys can be created and managed through the security API.

### Authentication Priority

When multiple authentication methods are enabled, the priority is:

1. TLS client certificates (mTLS)
2. SASL credentials
3. API keys
4. Anonymous (if allowed)

---

## Authorization & Access Control

StreamBus uses an Access Control List (ACL) system for fine-grained authorization.

### ACL Components

Each ACL entry consists of:

- **Principal**: User, service, or group identifier
- **Resource Type**: `TOPIC`, `GROUP`, `CLUSTER`, `TRANSACTION`, `SCHEMA`
- **Resource Name**: Specific resource identifier or pattern
- **Pattern Type**: `LITERAL`, `PREFIX`, `WILDCARD`
- **Action**: Specific operation (e.g., `TOPIC_READ`, `TOPIC_WRITE`)
- **Permission**: `ALLOW` or `DENY`

### Supported Actions

**Topic Actions:**
- `TOPIC_CREATE` - Create topics
- `TOPIC_DELETE` - Delete topics
- `TOPIC_DESCRIBE` - Describe topic metadata
- `TOPIC_ALTER` - Modify topic configuration
- `TOPIC_WRITE` - Produce messages
- `TOPIC_READ` - Consume messages
- `TOPIC_READ_COMMIT` - Commit consumer offsets

**Consumer Group Actions:**
- `GROUP_READ` - Read from consumer group
- `GROUP_DESCRIBE` - Describe consumer group
- `GROUP_DELETE` - Delete consumer group

**Cluster Actions:**
- `CLUSTER_DESCRIBE` - View cluster metadata
- `CLUSTER_ALTER` - Modify cluster configuration
- `CLUSTER_ACTION` - Administrative operations

**Transaction Actions:**
- `TXN_DESCRIBE` - View transaction state
- `TXN_WRITE` - Perform transactional operations

**Schema Actions:**
- `SCHEMA_READ` - Read schemas
- `SCHEMA_WRITE` - Register schemas
- `SCHEMA_DELETE` - Delete schemas
- `SCHEMA_COMPAT` - Check schema compatibility

### Pattern Types

**LITERAL**: Exact match
```json
{
  "resource_name": "orders",
  "pattern_type": "LITERAL"
}
```

**PREFIX**: Prefix match
```json
{
  "resource_name": "logs.",
  "pattern_type": "PREFIX"
}
```
Matches: `logs.app`, `logs.system`, `logs.audit`

**WILDCARD**: Pattern with `*` and `?`
```json
{
  "resource_name": "events-*-prod",
  "pattern_type": "WILDCARD"
}
```
Matches: `events-orders-prod`, `events-users-prod`

### Configuration

```yaml
security:
  authz_enabled: true

  # Super users bypass all ACL checks
  super_users:
    - "admin"

  # Use default ACLs (basic permissions)
  use_default_acls: false
```

### Creating ACLs

**Via API:**

```bash
curl -X POST http://localhost:8080/api/v1/security/acls \
  -H 'Content-Type: application/json' \
  -d '{
    "principal": "producer-service",
    "resource_type": "TOPIC",
    "resource_name": "orders",
    "pattern_type": "LITERAL",
    "action": "TOPIC_WRITE",
    "permission": "ALLOW"
  }'
```

**Common ACL Patterns:**

```bash
# Allow producer to write to all topics with prefix
{
  "principal": "producer-service",
  "resource_type": "TOPIC",
  "resource_name": "app.",
  "pattern_type": "PREFIX",
  "action": "TOPIC_WRITE",
  "permission": "ALLOW"
}

# Allow consumer to read specific topic
{
  "principal": "consumer-service",
  "resource_type": "TOPIC",
  "resource_name": "orders",
  "pattern_type": "LITERAL",
  "action": "TOPIC_READ",
  "permission": "ALLOW"
}

# Deny access to sensitive data
{
  "principal": "public-service",
  "resource_type": "TOPIC",
  "resource_name": "pii-data",
  "pattern_type": "LITERAL",
  "action": "TOPIC_READ",
  "permission": "DENY"
}
```

### Authorization Flow

1. Request arrives with authenticated principal
2. Extract action and resource from request
3. Check if principal is a super user (bypass ACLs)
4. Query ACLs for matching entries
5. Apply DENY rules first (explicit deny)
6. Apply ALLOW rules
7. Default deny if no matching ALLOW rule

---

## Encryption

### TLS Encryption (Data in Transit)

TLS encrypts all data between clients and brokers.

**Configuration:**

```yaml
security:
  tls:
    enabled: true
    cert_file: "/path/to/broker.crt"
    key_file: "/path/to/broker.key"
    min_version: 1.3  # TLS 1.3 recommended
```

**Supported Cipher Suites:**

TLS 1.3 cipher suites are used by default. For TLS 1.2, secure cipher suites are automatically selected.

### Encryption at Rest

Data stored on disk is encrypted using AES-256-GCM with key rotation.

**Configuration:**

```yaml
security:
  encryption:
    enabled: true
    algorithm: "AES-256-GCM"

    # Local key management
    kms_type: "local"
    master_key_path: "/path/to/master.key"

    # Key rotation interval
    key_rotation_interval: "720h"  # 30 days

    # PBKDF2 parameters
    pbkdf2_iterations: 100000
```

**Key Management Options:**

- **Local**: Keys stored on local filesystem (development/testing)
- **AWS KMS**: Integration with AWS Key Management Service (coming soon)
- **GCP KMS**: Integration with Google Cloud KMS (coming soon)
- **Vault**: HashiCorp Vault integration (coming soon)

**Key Rotation:**

Keys are automatically rotated based on `key_rotation_interval`. Old keys are retained to decrypt existing data.

**Manual Key Rotation:**

```bash
# Trigger manual key rotation via API (coming soon)
curl -X POST http://localhost:8080/api/v1/security/encryption/rotate-key
```

---

## Audit Logging

Audit logging records all security-relevant events for compliance and forensics.

### Configuration

```yaml
security:
  audit_enabled: true
  audit_log_file: "/var/log/streambus/audit.log"
```

### Audit Events

**Authentication Events:**
- Successful authentication
- Failed authentication attempts
- Certificate validation failures

**Authorization Events:**
- Access granted
- Access denied
- ACL violations

**Administrative Events:**
- User created/deleted
- ACL created/modified/deleted
- Topic created/deleted
- Configuration changes

### Audit Log Format

```json
{
  "timestamp": "2025-11-09T20:00:00Z",
  "principal": {
    "id": "producer-service",
    "type": "SERVICE",
    "auth_method": "MTLS"
  },
  "action": "TOPIC_WRITE",
  "resource": {
    "type": "TOPIC",
    "name": "orders"
  },
  "result": "SUCCESS",
  "client_ip": "192.168.1.100",
  "metadata": {
    "request_id": "12345",
    "request_type": "Produce"
  }
}
```

### Querying Audit Logs

```bash
# Filter by principal
grep '"id":"producer-service"' /var/log/streambus/audit.log

# Filter by action
grep '"action":"TOPIC_WRITE"' /var/log/streambus/audit.log

# Filter by result (failures only)
grep '"result":"DENIED"' /var/log/streambus/audit.log
```

---

## Configuration Guide

### Minimal Security (Development)

```yaml
security:
  tls:
    enabled: false
  authz_enabled: false
  allow_anonymous: true
```

### TLS Only (Basic Production)

```yaml
security:
  tls:
    enabled: true
    cert_file: "/path/to/broker.crt"
    key_file: "/path/to/broker.key"
  authz_enabled: false
  allow_anonymous: true
```

### TLS + SASL (Recommended)

```yaml
security:
  tls:
    enabled: true
    cert_file: "/path/to/broker.crt"
    key_file: "/path/to/broker.key"

  sasl:
    enabled: true
    mechanisms:
      - "SASL_SCRAM_SHA256"

  authz_enabled: true
  allow_anonymous: false
  audit_enabled: true
  audit_log_file: "/var/log/streambus/audit.log"
```

### Full Security (Production)

See `config/security/broker-production.yaml` for a complete production configuration example.

---

## API Reference

### Security Status

**GET /api/v1/security/status**

Returns the current security configuration status.

```bash
curl http://localhost:8080/api/v1/security/status
```

Response:
```json
{
  "enabled": true,
  "authentication": true,
  "authorization": true,
  "audit": true,
  "encryption": true
}
```

### ACL Management

**List ACLs:**
```bash
GET /api/v1/security/acls
```

**Create ACL:**
```bash
POST /api/v1/security/acls
Content-Type: application/json

{
  "principal": "user1",
  "resource_type": "TOPIC",
  "resource_name": "topic1",
  "pattern_type": "LITERAL",
  "action": "TOPIC_READ",
  "permission": "ALLOW"
}
```

**Delete ACL:**
```bash
DELETE /api/v1/security/acls/{acl-id}
```

### User Management

**List Users:**
```bash
GET /api/v1/security/users
```

**Create User:**
```bash
POST /api/v1/security/users
Content-Type: application/json

{
  "username": "user1",
  "password": "secure-password",
  "auth_method": "SASL_SCRAM_SHA256",
  "groups": ["producers"]
}
```

**Delete User:**
```bash
DELETE /api/v1/security/users/{username}
```

---

## Best Practices

### Authentication

1. **Always use TLS in production** - Protects credentials and data in transit
2. **Prefer SCRAM-SHA-512** over SCRAM-SHA-256 for maximum security
3. **Never use SASL_PLAIN** in production without TLS
4. **Rotate passwords regularly** - Every 90 days minimum
5. **Use strong passwords** - Minimum 16 characters, mixed case, numbers, symbols

### Authorization

1. **Principle of least privilege** - Grant minimum required permissions
2. **Use groups for role-based access** - Easier to manage than individual users
3. **Regular ACL audits** - Review and remove unnecessary permissions
4. **Explicit deny for sensitive resources** - Add DENY rules for critical data
5. **Separate production and development** - Use different principals/ACLs

### Encryption

1. **Use TLS 1.3** - More secure and performant than TLS 1.2
2. **Enable encryption at rest** - Protect data on disk
3. **Secure key storage** - Use KMS in production, never commit keys to git
4. **Regular key rotation** - Rotate encryption keys monthly
5. **Secure certificate management** - Use proper CA-signed certificates

### Audit Logging

1. **Always enable audit logging in production**
2. **Ship logs to SIEM** - Centralize security monitoring
3. **Set up alerts** - Monitor for suspicious activity
4. **Regular log reviews** - Check for anomalies and policy violations
5. **Retain logs** - Keep audit logs for compliance requirements (90+ days)

### Operational Security

1. **Principle of defense in depth** - Use multiple security layers
2. **Least privilege for service accounts** - Minimize blast radius
3. **Network segmentation** - Isolate brokers in secure network zones
4. **Regular security updates** - Keep StreamBus and dependencies updated
5. **Security testing** - Regular penetration testing and security audits

---

## Troubleshooting

### TLS Issues

**Problem:** Connection refused or TLS handshake failure

**Solutions:**
1. Verify certificate paths are correct
2. Check certificate validity: `openssl x509 -in broker.crt -text -noout`
3. Verify CA chain: `openssl verify -CAfile ca.crt broker.crt`
4. Check TLS version compatibility
5. Review broker logs for TLS errors

**Problem:** mTLS client certificate rejected

**Solutions:**
1. Verify client certificate is signed by the same CA
2. Check `allowed_client_cns` configuration
3. Ensure `require_client_cert` and `verify_client_cert` are set correctly
4. Verify client certificate CN matches expected value

### SASL Issues

**Problem:** Authentication failed

**Solutions:**
1. Verify username exists: `GET /api/v1/security/users`
2. Check password is correct
3. Ensure user is enabled
4. Verify SASL mechanism is allowed in broker config
5. Review audit logs for authentication failures

**Problem:** User cannot be created

**Solutions:**
1. Check SASL is enabled in configuration
2. Verify API request format
3. Ensure username doesn't already exist
4. Review broker logs for errors

### Authorization Issues

**Problem:** Access denied

**Solutions:**
1. List ACLs: `GET /api/v1/security/acls`
2. Verify principal has required ACL entry
3. Check pattern matching (LITERAL vs PREFIX vs WILDCARD)
4. Ensure no DENY rule is blocking access
5. Verify user groups if using group-based ACLs

**Problem:** Super user doesn't bypass ACLs

**Solutions:**
1. Check `super_users` configuration
2. Verify principal ID matches exactly
3. Restart broker if configuration was changed

### Audit Logging Issues

**Problem:** Audit logs not being written

**Solutions:**
1. Verify `audit_enabled: true` in configuration
2. Check `audit_log_file` path is writable
3. Ensure disk space available
4. Review broker logs for audit errors
5. Check file permissions on log directory

### Encryption Issues

**Problem:** Encryption initialization failed

**Solutions:**
1. Verify `master_key_path` exists and is readable
2. Check disk space for encrypted data
3. Review encryption algorithm support
4. Ensure PBKDF2 iterations is reasonable (50000-100000)

**Problem:** Unable to decrypt existing data

**Solutions:**
1. Verify master key hasn't changed
2. Check key rotation didn't lose old keys
3. Review encryption metadata in data files
4. Restore from backup if necessary

---

## Security Checklist

### Pre-Production

- [ ] TLS enabled with valid certificates
- [ ] SASL/SCRAM configured with strong passwords
- [ ] Authorization enabled with appropriate ACLs
- [ ] Audit logging enabled
- [ ] Encryption at rest enabled
- [ ] Super users limited to admin accounts only
- [ ] Anonymous access disabled
- [ ] Security configuration reviewed
- [ ] Penetration testing completed
- [ ] Incident response plan documented

### Production Monitoring

- [ ] Monitor authentication failures
- [ ] Alert on authorization denials
- [ ] Review audit logs weekly
- [ ] Track certificate expiration
- [ ] Monitor disk encryption overhead
- [ ] Check for unusual access patterns
- [ ] Verify backup encryption
- [ ] Test security incident response

### Maintenance

- [ ] Rotate passwords every 90 days
- [ ] Rotate encryption keys monthly
- [ ] Review and update ACLs quarterly
- [ ] Update TLS certificates before expiration
- [ ] Apply security patches promptly
- [ ] Audit user accounts monthly
- [ ] Review security logs weekly
- [ ] Test disaster recovery procedures

---

## Additional Resources

- [TLS Best Practices](https://wiki.mozilla.org/Security/Server_Side_TLS)
- [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)
- [NIST Cryptographic Standards](https://csrc.nist.gov/projects/cryptographic-standards-and-guidelines)

For security issues, please report to: security@streambus.io (example)

---

**Document Version:** 1.0
**Last Updated:** 2025-11-09
**Applies to:** StreamBus v1.0.0+
