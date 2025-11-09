# StreamBus Multi-Tenancy

StreamBus provides a comprehensive multi-tenancy system that enables resource isolation, quota enforcement, and usage tracking for multiple tenants sharing the same broker infrastructure.

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Key Concepts](#key-concepts)
- [Quick Start](#quick-start)
- [API Reference](#api-reference)
- [Integration Guide](#integration-guide)
- [Best Practices](#best-practices)
- [Examples](#examples)

## Architecture Overview

The multi-tenancy system consists of three main components:

```
┌─────────────┐
│   Manager   │  ← Orchestration layer (thread-safe)
└─────┬───────┘
      │
      ├─────────────┬─────────────┐
      │             │             │
┌─────▼──────┐ ┌───▼──────┐ ┌───▼──────┐
│TenantStore │ │ Tracker1 │ │ Tracker2 │  ← Per-tenant tracking
└────────────┘ └──────────┘ └──────────┘
```

### Components

1. **TenantStore**: Manages tenant metadata, configuration, and lifecycle
2. **QuotaTracker**: Tracks real-time resource usage and enforces quotas per tenant
3. **Manager**: Coordinates tenant management and quota enforcement across all tenants

## Key Concepts

### Tenants

A tenant represents an isolated namespace with its own:
- Unique identifier and metadata
- Resource quotas
- Usage statistics
- Lifecycle state (Active, Suspended, Deleted)

### Quotas

Quotas define resource limits for each tenant:

| Quota Type | Description | Unit |
|------------|-------------|------|
| **MaxBytesPerSecond** | Maximum data throughput | bytes/sec |
| **MaxMessagesPerSecond** | Maximum message rate | messages/sec |
| **MaxStorageBytes** | Maximum storage usage | bytes |
| **MaxTopics** | Maximum number of topics | count |
| **MaxPartitions** | Maximum total partitions across all topics | count |
| **MaxConnections** | Maximum concurrent connections | count |
| **MaxProducers** | Maximum concurrent producers | count |
| **MaxConsumers** | Maximum concurrent consumers | count |
| **MaxConsumerGroups** | Maximum consumer groups | count |
| **MaxRequestsPerSecond** | Maximum request rate | requests/sec |
| **MaxRetentionHours** | Maximum message retention | hours |

**Note**: Set any quota to `-1` for unlimited resources.

### Rate Limiting

StreamBus uses a sliding window algorithm for rate limiting:

```
Time Window (1 second)
┌────────────────────────────────┐
│ [100ms] [100ms] [100ms] ... │  ← 10 buckets
│   50      75      60     ... │  ← Bytes tracked
└────────────────────────────────┘
```

- Window size: 1 second (configurable)
- Bucket size: 100ms (10 buckets per window)
- Rate calculation: Sum of valid buckets / window duration

This provides accurate per-second rate tracking with minimal memory overhead.

## Quick Start

### Creating a Tenant

```go
package main

import (
    "github.com/shawntherrien/streambus/pkg/tenancy"
)

func main() {
    // Create manager
    manager := tenancy.NewManager()

    // Define quotas
    quotas := &tenancy.Quotas{
        MaxBytesPerSecond:    10 * 1024 * 1024,  // 10 MB/s
        MaxMessagesPerSecond: 10000,              // 10k msg/s
        MaxStorageBytes:      100 * 1024 * 1024 * 1024, // 100 GB
        MaxTopics:            100,
        MaxPartitions:        1000,
        MaxConnections:       500,
        MaxProducers:         100,
        MaxConsumers:         200,
        MaxConsumerGroups:    50,
        MaxRequestsPerSecond: 50000,
        MaxRetentionHours:    168, // 7 days
    }

    // Create tenant
    tenant, err := manager.CreateTenant("acme-corp", "ACME Corporation", quotas)
    if err != nil {
        panic(err)
    }

    println("Created tenant:", tenant.ID)
}
```

### Enforcing Quotas

```go
// Enforce produce quota
err := manager.EnforceProduceQuota("acme-corp", 1024, 1) // 1KB, 1 message
if err != nil {
    // Handle quota exceeded error
    if quotaErr, ok := err.(*tenancy.QuotaError); ok {
        log.Printf("Quota exceeded: %s (current: %d, limit: %d)",
            quotaErr.QuotaType, quotaErr.Current, quotaErr.Limit)
    }
    return err
}

// Enforce connection quota
err = manager.EnforceConnectionQuota("acme-corp")
if err != nil {
    return err
}
defer manager.ReleaseConnection("acme-corp")

// Enforce topic creation quota
err = manager.EnforceTopicQuota("acme-corp", 3) // 3 partitions
if err != nil {
    return err
}
```

### Monitoring Usage

```go
// Get current usage
usage, err := manager.GetUsage("acme-corp")
if err != nil {
    return err
}

log.Printf("Tenant Usage:")
log.Printf("  Connections: %d", usage.Connections)
log.Printf("  Topics: %d", usage.Topics)
log.Printf("  Partitions: %d", usage.Partitions)
log.Printf("  Storage: %d bytes", usage.StorageBytes)
log.Printf("  Throughput: %d bytes/sec", usage.BytesPerSecond)

// Get utilization percentages
utilization, err := manager.GetUtilization("acme-corp")
if err != nil {
    return err
}

log.Printf("Utilization:")
log.Printf("  Connections: %.1f%%", utilization["connections"])
log.Printf("  Storage: %.1f%%", utilization["storage"])
log.Printf("  Topics: %.1f%%", utilization["topics"])
```

## API Reference

### Manager

#### Tenant Management

```go
// CreateTenant creates a new tenant with specified quotas
func (m *Manager) CreateTenant(id TenantID, name string, quotas *Quotas) (*Tenant, error)

// GetTenant retrieves a tenant by ID
func (m *Manager) GetTenant(id TenantID) (*Tenant, error)

// UpdateTenant updates tenant configuration
func (m *Manager) UpdateTenant(id TenantID, name string, quotas *Quotas) error

// DeleteTenant deletes a tenant
func (m *Manager) DeleteTenant(id TenantID) error

// ListTenants returns all tenants
func (m *Manager) ListTenants() []*Tenant

// SuspendTenant suspends a tenant (blocks all operations)
func (m *Manager) SuspendTenant(id TenantID) error

// ActivateTenant activates a suspended tenant
func (m *Manager) ActivateTenant(id TenantID) error
```

#### Quota Enforcement

```go
// EnforceProduceQuota checks and records produce operation
func (m *Manager) EnforceProduceQuota(id TenantID, bytes, messages int64) error

// EnforceConsumeQuota checks and records consume operation
func (m *Manager) EnforceConsumeQuota(id TenantID, bytes, messages int64) error

// EnforceConnectionQuota checks and increments connection count
func (m *Manager) EnforceConnectionQuota(id TenantID) error

// ReleaseConnection decrements connection count
func (m *Manager) ReleaseConnection(id TenantID)

// EnforceTopicQuota checks and increments topic/partition count
func (m *Manager) EnforceTopicQuota(id TenantID, partitions int) error

// ReleaseTopic decrements topic/partition count
func (m *Manager) ReleaseTopic(id TenantID, partitions int)

// EnforceStorageQuota checks if storage quota allows operation
func (m *Manager) EnforceStorageQuota(id TenantID, bytes int64) error

// UpdateStorageUsage updates current storage usage
func (m *Manager) UpdateStorageUsage(id TenantID, bytes int64) error
```

#### Monitoring

```go
// GetUsage returns current usage for a tenant
func (m *Manager) GetUsage(id TenantID) (*Usage, error)

// GetAllUsage returns usage for all tenants
func (m *Manager) GetAllUsage() map[TenantID]*Usage

// GetUtilization returns utilization percentages
func (m *Manager) GetUtilization(id TenantID) (map[string]float64, error)

// GetTenantStats returns comprehensive statistics
func (m *Manager) GetTenantStats(id TenantID) (*TenantStats, error)
```

### QuotaTracker

The `QuotaTracker` is used internally by the `Manager` but can be accessed directly:

```go
// GetTracker gets the quota tracker for a tenant
func (m *Manager) GetTracker(id TenantID) (*QuotaTracker, error)
```

Available methods on `QuotaTracker`:

```go
// CheckThroughput checks if throughput is within quota
func (qt *QuotaTracker) CheckThroughput(bytes, messages int64) error

// RecordThroughput records usage
func (qt *QuotaTracker) RecordThroughput(bytes, messages int64)

// AddConnection increments connection count
func (qt *QuotaTracker) AddConnection() error

// RemoveConnection decrements connection count
func (qt *QuotaTracker) RemoveConnection()

// AddProducer increments producer count
func (qt *QuotaTracker) AddProducer() error

// RemoveProducer decrements producer count
func (qt *QuotaTracker) RemoveProducer()

// AddTopic increments topic/partition count
func (qt *QuotaTracker) AddTopic(partitions int) error

// RemoveTopic decrements topic/partition count
func (qt *QuotaTracker) RemoveTopic(partitions int)

// CheckStorage checks storage quota
func (qt *QuotaTracker) CheckStorage(additionalBytes int64) error

// UpdateStorage updates storage usage
func (qt *QuotaTracker) UpdateStorage(bytes int64)

// GetUsage returns current usage
func (qt *QuotaTracker) GetUsage() *Usage

// UtilizationPercent calculates quota utilization
func (qt *QuotaTracker) UtilizationPercent(quotaType string) float64
```

## Integration Guide

### Broker Integration

Integrate tenancy into your broker's request handlers:

```go
type Broker struct {
    tenancyManager *tenancy.Manager
    // ... other fields
}

func (b *Broker) HandleProduceRequest(req *ProduceRequest) error {
    // Extract tenant ID from request context
    tenantID := req.TenantID

    // Enforce quota
    err := b.tenancyManager.EnforceProduceQuota(
        tenantID,
        int64(len(req.Message.Value)),
        1,
    )
    if err != nil {
        return &QuotaExceededError{Tenant: tenantID, Err: err}
    }

    // Process request normally
    return b.processProduceRequest(req)
}

func (b *Broker) HandleConnection(conn net.Conn) error {
    tenantID := extractTenantFromAuth(conn)

    // Enforce connection quota
    err := b.tenancyManager.EnforceConnectionQuota(tenantID)
    if err != nil {
        conn.Close()
        return err
    }
    defer b.tenancyManager.ReleaseConnection(tenantID)

    // Handle connection
    return b.handleConnection(conn)
}
```

### Storage Integration

Update storage usage periodically:

```go
func (b *Broker) UpdateTenantStorage() {
    for _, tenant := range b.tenancyManager.ListTenants() {
        // Calculate storage usage
        usage := b.storage.GetTenantUsage(tenant.ID)

        // Update tracker
        b.tenancyManager.UpdateStorageUsage(tenant.ID, usage)
    }
}

// Run periodically
go func() {
    ticker := time.NewTicker(1 * time.Minute)
    for range ticker.C {
        b.UpdateTenantStorage()
    }
}()
```

### Admin API Integration

Expose tenant management via API:

```go
// POST /admin/tenants
func (api *AdminAPI) CreateTenant(w http.ResponseWriter, r *http.Request) {
    var req struct {
        ID     string          `json:"id"`
        Name   string          `json:"name"`
        Quotas *tenancy.Quotas `json:"quotas"`
    }

    json.NewDecoder(r.Body).Decode(&req)

    tenant, err := api.manager.CreateTenant(req.ID, req.Name, req.Quotas)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    json.NewEncoder(w).Encode(tenant)
}

// GET /admin/tenants/:id/stats
func (api *AdminAPI) GetTenantStats(w http.ResponseWriter, r *http.Request) {
    tenantID := mux.Vars(r)["id"]

    stats, err := api.manager.GetTenantStats(tenantID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusNotFound)
        return
    }

    json.NewEncoder(w).Encode(stats)
}
```

## Best Practices

### 1. Quota Configuration

**Start Conservative, Adjust Based on Usage**

```go
// Start with moderate quotas
initialQuotas := &tenancy.Quotas{
    MaxBytesPerSecond:    5 * 1024 * 1024,  // 5 MB/s
    MaxMessagesPerSecond: 5000,
    MaxStorageBytes:      50 * 1024 * 1024 * 1024, // 50 GB
    MaxTopics:            50,
    MaxPartitions:        500,
    MaxConnections:       100,
}

// Monitor and adjust
utilization, _ := manager.GetUtilization(tenantID)
if utilization["storage"] > 80.0 {
    // Consider increasing storage quota
}
```

### 2. Monitoring and Alerting

**Set Up Proactive Monitoring**

```go
func MonitorTenants(manager *tenancy.Manager) {
    for _, tenant := range manager.ListTenants() {
        stats, _ := manager.GetTenantStats(tenant.ID)

        // Alert if utilization is high
        for resource, utilization := range stats.Utilization {
            if utilization > 80.0 {
                log.Printf("WARNING: Tenant %s at %.1f%% %s utilization",
                    tenant.ID, utilization, resource)
            }
            if utilization > 95.0 {
                log.Printf("CRITICAL: Tenant %s at %.1f%% %s utilization",
                    tenant.ID, utilization, resource)
                // Send alert
            }
        }
    }
}
```

### 3. Graceful Degradation

**Handle Quota Errors Gracefully**

```go
func HandleProduceWithRetry(manager *tenancy.Manager, tenantID string, msg []byte) error {
    for i := 0; i < 3; i++ {
        err := manager.EnforceProduceQuota(tenantID, int64(len(msg)), 1)
        if err == nil {
            return produceMessage(msg)
        }

        if quotaErr, ok := err.(*tenancy.QuotaError); ok {
            if quotaErr.QuotaType == "bytes_per_second" {
                // Wait for rate limit window to refresh
                time.Sleep(100 * time.Millisecond)
                continue
            }
        }

        return err
    }
    return errors.New("quota enforcement failed after retries")
}
```

### 4. Tenant Lifecycle Management

**Suspend Before Deleting**

```go
func SafeDeleteTenant(manager *tenancy.Manager, tenantID string) error {
    // First suspend to prevent new operations
    if err := manager.SuspendTenant(tenantID); err != nil {
        return err
    }

    // Wait for in-flight operations to complete
    time.Sleep(5 * time.Second)

    // Verify no active connections
    usage, _ := manager.GetUsage(tenantID)
    if usage.Connections > 0 {
        return errors.New("tenant still has active connections")
    }

    // Now safe to delete
    return manager.DeleteTenant(tenantID)
}
```

### 5. Default Quotas

**Use Tiered Quota Profiles**

```go
func FreeTierQuotas() *tenancy.Quotas {
    return &tenancy.Quotas{
        MaxBytesPerSecond:    1 * 1024 * 1024,  // 1 MB/s
        MaxMessagesPerSecond: 1000,
        MaxStorageBytes:      10 * 1024 * 1024 * 1024, // 10 GB
        MaxTopics:            10,
        MaxPartitions:        50,
        MaxConnections:       10,
        MaxProducers:         5,
        MaxConsumers:         10,
    }
}

func ProTierQuotas() *tenancy.Quotas {
    return &tenancy.Quotas{
        MaxBytesPerSecond:    50 * 1024 * 1024,  // 50 MB/s
        MaxMessagesPerSecond: 50000,
        MaxStorageBytes:      500 * 1024 * 1024 * 1024, // 500 GB
        MaxTopics:            500,
        MaxPartitions:        5000,
        MaxConnections:       1000,
        MaxProducers:         200,
        MaxConsumers:         500,
    }
}

func EnterpriseTierQuotas() *tenancy.Quotas {
    return tenancy.UnlimitedQuotas() // No limits
}
```

## Examples

### Complete Integration Example

```go
package main

import (
    "log"
    "time"

    "github.com/shawntherrien/streambus/pkg/tenancy"
)

func main() {
    // Initialize manager
    manager := tenancy.NewManager()

    // Create tenants
    setupTenants(manager)

    // Start monitoring
    go monitorTenants(manager)

    // Simulate operations
    simulateOperations(manager)
}

func setupTenants(manager *tenancy.Manager) {
    tenants := []struct {
        id     string
        name   string
        quotas *tenancy.Quotas
    }{
        {"startup-a", "Startup A", tenancy.DefaultQuotas()},
        {"enterprise-b", "Enterprise B", enterpriseQuotas()},
        {"free-tier", "Free User", freeTierQuotas()},
    }

    for _, t := range tenants {
        _, err := manager.CreateTenant(t.id, t.name, t.quotas)
        if err != nil {
            log.Fatalf("Failed to create tenant %s: %v", t.id, err)
        }
        log.Printf("Created tenant: %s", t.id)
    }
}

func monitorTenants(manager *tenancy.Manager) {
    ticker := time.NewTicker(10 * time.Second)
    for range ticker.C {
        usage := manager.GetAllUsage()

        for tenantID, stats := range usage {
            log.Printf("Tenant %s: %d connections, %d topics, %d bytes/sec",
                tenantID, stats.Connections, stats.Topics, stats.BytesPerSecond)
        }
    }
}

func simulateOperations(manager *tenancy.Manager) {
    // Simulate produce
    for i := 0; i < 100; i++ {
        err := manager.EnforceProduceQuota("startup-a", 1024, 1)
        if err != nil {
            log.Printf("Produce failed: %v", err)
        } else {
            log.Printf("Produced message %d", i)
        }
        time.Sleep(100 * time.Millisecond)
    }
}

func freeTierQuotas() *tenancy.Quotas {
    return &tenancy.Quotas{
        MaxBytesPerSecond:    1 * 1024 * 1024,
        MaxMessagesPerSecond: 1000,
        MaxStorageBytes:      10 * 1024 * 1024 * 1024,
        MaxTopics:            10,
        MaxPartitions:        50,
        MaxConnections:       10,
    }
}

func enterpriseQuotas() *tenancy.Quotas {
    return tenancy.UnlimitedQuotas()
}
```

### Testing Example

```go
func TestTenantQuotaEnforcement(t *testing.T) {
    manager := tenancy.NewManager()

    quotas := &tenancy.Quotas{
        MaxBytesPerSecond:    1000,
        MaxMessagesPerSecond: 10,
    }

    _, err := manager.CreateTenant("test-tenant", "Test", quotas)
    if err != nil {
        t.Fatal(err)
    }

    // Should succeed under quota
    err = manager.EnforceProduceQuota("test-tenant", 500, 5)
    if err != nil {
        t.Errorf("Expected success under quota, got: %v", err)
    }

    // Should fail when exceeding quota
    err = manager.EnforceProduceQuota("test-tenant", 600, 10)
    if err == nil {
        t.Error("Expected quota error")
    }

    // Verify it's a QuotaError
    if _, ok := err.(*tenancy.QuotaError); !ok {
        t.Errorf("Expected QuotaError, got: %T", err)
    }
}
```

## Performance Considerations

### Memory Usage

Each `QuotaTracker` uses approximately:
- 200 bytes for counter variables
- 800 bytes for rate windows (3 windows × 10 buckets × 24 bytes)
- **Total: ~1 KB per tenant**

For 10,000 tenants: ~10 MB memory overhead.

### CPU Usage

- Rate window updates: O(1) per operation
- Quota checks: O(1) per operation
- All operations use read locks where possible for maximum concurrency

### Thread Safety

All operations are thread-safe:
- Manager uses RWMutex for tracker map
- QuotaTracker uses RWMutex for counters
- RateWindow uses Mutex for bucket updates

## Troubleshooting

### Common Issues

**1. Quota Errors Despite Low Usage**

Check rate window timing:

```go
usage, _ := manager.GetUsage(tenantID)
log.Printf("Current rate: %d bytes/sec", usage.BytesPerSecond)
```

Rate limits use a sliding window - spikes in traffic can cause temporary quota errors even if average usage is low.

**2. Storage Quota Not Working**

Storage must be updated manually:

```go
// Update storage usage periodically
manager.UpdateStorageUsage(tenantID, currentStorageBytes)
```

**3. Connection Count Drift**

Ensure connections are properly released:

```go
manager.EnforceConnectionQuota(tenantID)
defer manager.ReleaseConnection(tenantID) // Always use defer
```

---

## License

Part of the StreamBus project - See main LICENSE file.

## Contributing

See the main CONTRIBUTING.md in the repository root.
