# StreamBus Security Configuration Examples

This directory contains security configuration examples and utilities for StreamBus.

## Files

- **broker-with-tls.yaml** - TLS/mTLS authentication example
- **broker-with-sasl.yaml** - SASL/SCRAM authentication example
- **broker-production.yaml** - Complete production security configuration
- **acl-examples.json** - ACL configuration examples and API usage
- **generate-certs.sh** - Script to generate test TLS certificates

## Quick Start

### 1. Generate Test Certificates

```bash
cd config/security
./generate-certs.sh
```

This creates:
- `certs/ca.crt`, `certs/ca.key` - Certificate Authority
- `certs/broker.crt`, `certs/broker.key` - Broker certificate
- `certs/client.crt`, `certs/client.key` - Client certificates

### 2. Start Broker with TLS

```bash
streambus-broker --config config/security/broker-with-tls.yaml
```

### 3. Configure ACLs

```bash
# Check security status
curl http://localhost:8080/api/v1/security/status

# Create ACL
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

# List ACLs
curl http://localhost:8080/api/v1/security/acls
```

### 4. Create Users (SASL)

```bash
curl -X POST http://localhost:8080/api/v1/security/users \
  -H 'Content-Type: application/json' \
  -d '{
    "username": "producer1",
    "password": "secure-password",
    "auth_method": "SASL_SCRAM_SHA256",
    "groups": ["producers"]
  }'
```

## Configuration Scenarios

### Development (No Security)

Use default configuration with security disabled.

### Testing (TLS Only)

Use `broker-with-tls.yaml` for encrypted connections.

### Staging (TLS + SASL)

Use `broker-with-sasl.yaml` for authentication testing.

### Production (Full Security)

Use `broker-production.yaml` with all security features enabled.

## Security Checklist

- [ ] TLS certificates generated and configured
- [ ] SASL users created with strong passwords
- [ ] ACLs defined for all principals
- [ ] Authorization enabled
- [ ] Audit logging configured
- [ ] Encryption at rest enabled
- [ ] Anonymous access disabled
- [ ] Super users limited

## Documentation

See [docs/SECURITY.md](../../docs/SECURITY.md) for complete security documentation.

## Support

For security issues, refer to the troubleshooting section in SECURITY.md.
