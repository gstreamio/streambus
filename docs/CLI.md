# StreamBus CLI Tools

StreamBus provides powerful command-line tools for managing and interacting with your StreamBus clusters.

## Table of Contents

- [streambus-admin](#streambus-admin) - Cluster administration and management
- [streambus-cli](#streambus-cli) - Message producer/consumer CLI
- [streambus-broker](#streambus-broker) - Broker server
- [streambus-mirror-maker](#streambus-mirror-maker) - Cross-cluster replication

---

## streambus-admin

The primary administration tool for StreamBus clusters.

### Installation

```bash
# Build from source
make build

# Or install directly
go install github.com/shawntherrien/streambus/cmd/streambus-admin@latest
```

### Global Flags

All commands support these global flags:

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--brokers` | | `localhost:9092` | Comma-separated list of broker addresses |
| `--admin-addr` | | `localhost:8080` | Admin API address |
| `--output` | `-o` | `table` | Output format: `table`, `json`, `yaml`, `wide` |
| `--verbose` | `-v` | `false` | Enable verbose output |
| `--no-color` | | `false` | Disable colored output |
| `--config` | | `~/.streambus-admin.yaml` | Config file path |

### Configuration File

Create `~/.streambus-admin.yaml` for persistent settings:

```yaml
brokers:
  - localhost:9092
  - localhost:9093
admin-addr: localhost:8080
output: table
verbose: false
```

---

## Topic Management

### List Topics

```bash
# Table output (default)
streambus-admin topic list

# JSON output
streambus-admin topic list --output json

# YAML output
streambus-admin topic list --output yaml
```

### Create Topic

```bash
# Create topic with default settings (1 partition, replication factor 1)
streambus-admin topic create my-topic

# Create topic with custom settings
streambus-admin topic create my-topic --partitions 3 --replication-factor 2

# Short flags
streambus-admin topic create my-topic -p 3 -r 2
```

**Flags:**
- `--partitions, -p`: Number of partitions (default: 1)
- `--replication-factor, -r`: Replication factor (default: 1)

### Delete Topic

```bash
streambus-admin topic delete my-topic

# With confirmation in verbose mode
streambus-admin topic delete my-topic --verbose
```

### Describe Topic

```bash
streambus-admin topic describe my-topic
```

---

## ACL Management

Manage Access Control Lists for topics, groups, and cluster resources.

### List ACLs

```bash
# Table view
streambus-admin acl list

# JSON view for programmatic access
streambus-admin acl list --output json
```

### Create ACL

```bash
# Allow user to write to a topic
streambus-admin acl create \
  --principal "User:alice" \
  --resource-type TOPIC \
  --resource-name orders \
  --operation WRITE \
  --permission ALLOW

# Allow user to read from a consumer group
streambus-admin acl create \
  --principal "User:bob" \
  --resource-type GROUP \
  --resource-name consumer-group-1 \
  --operation READ \
  --permission ALLOW

# Deny access with pattern matching
streambus-admin acl create \
  --principal "User:guest" \
  --resource-type TOPIC \
  --resource-name sensitive-* \
  --resource-pattern PREFIX \
  --operation READ \
  --permission DENY
```

**Flags:**
- `--principal`: Principal (e.g., `User:alice`, `Service:api`)
- `--resource-type`: Resource type: `TOPIC`, `GROUP`, `CLUSTER`
- `--resource-name`: Resource name or pattern
- `--resource-pattern`: Pattern type: `LITERAL`, `PREFIX`, `WILDCARD` (default: `LITERAL`)
- `--operation`: Operation: `READ`, `WRITE`, `CREATE`, `DELETE`, `DESCRIBE`, `ALTER`
- `--permission`: Permission: `ALLOW`, `DENY` (default: `ALLOW`)
- `--host`: Host restriction (default: `*` for all hosts)

### Delete ACL

```bash
# Delete by ACL ID (get ID from list command)
streambus-admin acl delete acl-123456
```

---

## User Management

Manage SASL/SCRAM users for authentication.

### List Users

```bash
streambus-admin user list

# JSON output for automation
streambus-admin user list --output json
```

### Create User

```bash
# Create user with SCRAM-SHA-256 (default)
streambus-admin user create alice --password secure-password

# Create user with SCRAM-SHA-512
streambus-admin user create alice \
  --password secure-password \
  --mechanism SCRAM-SHA-512
```

**Flags:**
- `--password, -p`: User password (required)
- `--mechanism, -m`: SASL mechanism: `SCRAM-SHA-256`, `SCRAM-SHA-512` (default: `SCRAM-SHA-256`)

### Delete User

```bash
streambus-admin user delete alice
```

---

## Cluster Operations

### Check Cluster Health

```bash
# Basic health check
streambus-admin cluster health

# Verbose output with security status
streambus-admin cluster health --verbose
```

Output includes:
- Broker connectivity
- Security configuration status
- Authentication/authorization status
- TLS status
- Audit logging status
- Encryption at rest status

### Cluster Info

```bash
streambus-admin cluster info
```

*Note: Full cluster metadata API is coming soon.*

---

## Consumer Group Management

### List Consumer Groups

```bash
streambus-admin group list
```

### Describe Consumer Group

```bash
streambus-admin group describe my-group
```

### Delete Consumer Group

```bash
streambus-admin group delete my-group
```

---

## Output Formats

### Table Format (Default)

Clean, human-readable tabular output:

```bash
streambus-admin topic list
```

Output:
```
TOPIC
orders
payments
users
```

### JSON Format

Machine-readable JSON for automation:

```bash
streambus-admin topic list --output json
```

Output:
```json
{
  "topics": [
    "orders",
    "payments",
    "users"
  ]
}
```

### YAML Format

YAML output for configuration management:

```bash
streambus-admin acl list --output yaml
```

Output:
```yaml
- id: acl-001
  principal: User:alice
  resource_type: TOPIC
  resource_name: orders
  pattern_type: LITERAL
  action: WRITE
  permission: ALLOW
```

---

## Shell Completion

Enable shell completion for faster command entry:

### Bash

```bash
# Generate completion script
streambus-admin completion bash > /etc/bash_completion.d/streambus-admin

# Or for current session
source <(streambus-admin completion bash)
```

### Zsh

```bash
# Add to ~/.zshrc
streambus-admin completion zsh > "${fpath[1]}/_streambus-admin"
```

### Fish

```bash
streambus-admin completion fish > ~/.config/fish/completions/streambus-admin.fish
```

### PowerShell

```powershell
streambus-admin completion powershell | Out-String | Invoke-Expression
```

---

## Examples

### Complete Workflow: Setup Secure Topic

```bash
# 1. Create topic
streambus-admin topic create secure-orders --partitions 3 --replication-factor 2

# 2. Create users
streambus-admin user create producer-service --password prod-secret
streambus-admin user create consumer-service --password cons-secret

# 3. Set up ACLs
streambus-admin acl create \
  --principal "User:producer-service" \
  --resource-type TOPIC \
  --resource-name secure-orders \
  --operation WRITE \
  --permission ALLOW

streambus-admin acl create \
  --principal "User:consumer-service" \
  --resource-type TOPIC \
  --resource-name secure-orders \
  --operation READ \
  --permission ALLOW

# 4. Verify health
streambus-admin cluster health --verbose

# 5. List configuration
streambus-admin topic list
streambus-admin user list
streambus-admin acl list
```

### Backup and Restore ACLs

```bash
# Export ACLs to JSON
streambus-admin acl list --output json > acls-backup.json

# Later, recreate ACLs from backup (requires scripting)
cat acls-backup.json | jq -r '.[] |
  "streambus-admin acl create --principal \(.principal) --resource-type \(.resource_type) --resource-name \(.resource_name) --operation \(.action) --permission \(.permission)"' | bash
```

### Monitor Multiple Clusters

```bash
# Cluster 1
streambus-admin cluster health --brokers cluster1:9092 --admin-addr cluster1:8080

# Cluster 2
streambus-admin cluster health --brokers cluster2:9092 --admin-addr cluster2:8080
```

---

## Troubleshooting

### Connection Issues

```bash
# Test broker connectivity
streambus-admin cluster health --verbose

# Verify broker addresses
streambus-admin topic list --brokers localhost:9092 --verbose
```

### Authentication Failures

```bash
# Check security status
streambus-admin cluster health

# Verify user exists
streambus-admin user list | grep username

# Check ACLs
streambus-admin acl list --output json | jq '.[] | select(.principal == "User:username")'
```

### Permission Denied

```bash
# Review all ACLs for a principal
streambus-admin acl list --output json | jq '.[] | select(.principal == "User:alice")'

# Check for DENY rules
streambus-admin acl list --output json | jq '.[] | select(.permission == "DENY")'
```

---

## Advanced Usage

### Scripting with JSON Output

```bash
#!/bin/bash

# Get all topics as JSON
TOPICS=$(streambus-admin topic list --output json)

# Process each topic
echo "$TOPICS" | jq -r '.topics[]' | while read -r topic; do
  echo "Processing topic: $topic"
  # Do something with each topic
done
```

### Automation with CI/CD

```yaml
# GitHub Actions example
steps:
  - name: Setup ACLs
    run: |
      streambus-admin acl create \
        --admin-addr ${{ secrets.BROKER_ADMIN_ADDR }} \
        --principal "Service:${{ github.repository }}" \
        --resource-type TOPIC \
        --resource-name ${{ env.TOPIC_NAME }} \
        --operation WRITE \
        --permission ALLOW
```

### Custom Configuration Profiles

```bash
# Development
export STREAMBUS_BROKERS=localhost:9092
export STREAMBUS_ADMIN_ADDR=localhost:8080

# Production
export STREAMBUS_BROKERS=prod-broker-1:9092,prod-broker-2:9092
export STREAMBUS_ADMIN_ADDR=prod-admin:8080

# Commands will use environment variables
streambus-admin cluster health
```

---

## Best Practices

1. **Use Configuration Files**: Store common settings in `~/.streambus-admin.yaml`
2. **Automate with JSON**: Use `--output json` for scripting and automation
3. **Enable Shell Completion**: Speeds up command entry and reduces errors
4. **Verbose Mode for Debugging**: Use `-v` flag when troubleshooting
5. **Secure Credentials**: Never hardcode passwords in scripts; use environment variables or secret managers
6. **Test Changes**: Use `--verbose` to preview operations before executing

---

## Related Documentation

- [Security Guide](./SECURITY.md) - Complete security configuration
- [API Reference](./API.md) - Admin HTTP API
- [Operations Guide](./OPERATIONS.md) - Running StreamBus in production

---

## Support

- **Issues**: https://github.com/shawntherrien/streambus/issues
- **Documentation**: https://docs.streambus.io
- **Community**: https://discord.gg/streambus
