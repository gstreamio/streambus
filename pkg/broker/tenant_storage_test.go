package broker

import (
	"context"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/server"
	"github.com/gstreamio/streambus/pkg/storage"
	"github.com/gstreamio/streambus/pkg/tenancy"
)

// newStorageTrackingBroker builds a broker with real storage and a tenancy
// manager, ready for per-tenant storage accounting tests.
func newStorageTrackingBroker(t *testing.T) (*Broker, *server.TopicManager, *tenancy.Manager) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	dataDir := t.TempDir()
	topicManager := server.NewTopicManager(dataDir)
	t.Cleanup(func() { _ = topicManager.Close() })

	tenancyMgr := tenancy.NewManager()

	broker := &Broker{
		ctx:            ctx,
		cancel:         cancel,
		logger:         newTestLogger(),
		tenancyManager: tenancyMgr,
		topicManager:   topicManager,
		config:         &Config{BrokerID: 1, DataDir: dataDir},
		status:         StatusRunning,
	}

	return broker, topicManager, tenancyMgr
}

// writeBytesToTopic creates a topic and writes a message of the given size to
// partition 0, flushing so the bytes land on disk.
func writeBytesToTopic(t *testing.T, tm *server.TopicManager, topic string, payloadSize int) {
	t.Helper()

	if err := tm.CreateTopic(topic, 1); err != nil {
		t.Fatalf("Failed to create topic %s: %v", topic, err)
	}

	partition, err := tm.GetPartition(topic, 0)
	if err != nil {
		t.Fatalf("Failed to get partition for %s: %v", topic, err)
	}

	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte('a' + i%26)
	}

	if _, err := partition.Log().Append(&storage.MessageBatch{
		Messages:  []storage.Message{{Value: payload, Timestamp: time.Now()}},
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("Failed to append to %s: %v", topic, err)
	}

	if err := partition.Log().Flush(); err != nil {
		t.Fatalf("Failed to flush %s: %v", topic, err)
	}
}

func TestUpdateTenantStorageUsage_ReportsRealBytes(t *testing.T) {
	broker, tm, tenancyMgr := newStorageTrackingBroker(t)

	quotas := &tenancy.Quotas{MaxTopics: 100, MaxPartitions: 1000, MaxStorageBytes: 1 << 30}
	if _, err := tenancyMgr.CreateTenant("tenant-a", "Tenant A", quotas); err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	writeBytesToTopic(t, tm, "owned-topic", 4096)
	tenancyMgr.RegisterTopic("tenant-a", "owned-topic")

	broker.updateTenantStorageUsage()

	usage, err := tenancyMgr.GetUsage("tenant-a")
	if err != nil {
		t.Fatalf("Failed to read usage: %v", err)
	}
	if usage.StorageBytes <= 0 {
		t.Fatalf("StorageBytes = %d, expected the tenant's topic bytes to be counted", usage.StorageBytes)
	}
}

func TestUpdateTenantStorageUsage_IgnoresOtherTenantsTopics(t *testing.T) {
	broker, tm, tenancyMgr := newStorageTrackingBroker(t)

	quotas := &tenancy.Quotas{MaxTopics: 100, MaxPartitions: 1000, MaxStorageBytes: 1 << 30}
	for _, id := range []tenancy.TenantID{"tenant-a", "tenant-b"} {
		if _, err := tenancyMgr.CreateTenant(id, string(id), quotas); err != nil {
			t.Fatalf("Failed to create tenant %s: %v", id, err)
		}
	}

	writeBytesToTopic(t, tm, "a-topic", 4096)
	tenancyMgr.RegisterTopic("tenant-a", "a-topic")

	broker.updateTenantStorageUsage()

	usageB, err := tenancyMgr.GetUsage("tenant-b")
	if err != nil {
		t.Fatalf("Failed to read usage: %v", err)
	}
	if usageB.StorageBytes != 0 {
		t.Errorf("tenant-b StorageBytes = %d, want 0: it owns no topics", usageB.StorageBytes)
	}
}

func TestUpdateTenantStorageUsage_ExcludesUnownedTopics(t *testing.T) {
	broker, tm, tenancyMgr := newStorageTrackingBroker(t)

	quotas := &tenancy.Quotas{MaxTopics: 100, MaxPartitions: 1000, MaxStorageBytes: 1 << 30}
	if _, err := tenancyMgr.CreateTenant("tenant-a", "Tenant A", quotas); err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	// A topic with no ownership record - e.g. created before multi-tenancy
	// was switched on - must not be billed to an arbitrary tenant.
	writeBytesToTopic(t, tm, "legacy-topic", 4096)

	broker.updateTenantStorageUsage()

	usage, err := tenancyMgr.GetUsage("tenant-a")
	if err != nil {
		t.Fatalf("Failed to read usage: %v", err)
	}
	if usage.StorageBytes != 0 {
		t.Errorf("StorageBytes = %d, want 0: the topic has no owner", usage.StorageBytes)
	}
}

func TestUpdateTenantStorageUsage_TracksGrowth(t *testing.T) {
	broker, tm, tenancyMgr := newStorageTrackingBroker(t)

	quotas := &tenancy.Quotas{MaxTopics: 100, MaxPartitions: 1000, MaxStorageBytes: 1 << 30}
	if _, err := tenancyMgr.CreateTenant("tenant-a", "Tenant A", quotas); err != nil {
		t.Fatalf("Failed to create tenant: %v", err)
	}

	writeBytesToTopic(t, tm, "topic-one", 4096)
	tenancyMgr.RegisterTopic("tenant-a", "topic-one")

	broker.updateTenantStorageUsage()
	before, err := tenancyMgr.GetUsage("tenant-a")
	if err != nil {
		t.Fatalf("Failed to read usage: %v", err)
	}

	writeBytesToTopic(t, tm, "topic-two", 8192)
	tenancyMgr.RegisterTopic("tenant-a", "topic-two")

	broker.updateTenantStorageUsage()
	after, err := tenancyMgr.GetUsage("tenant-a")
	if err != nil {
		t.Fatalf("Failed to read usage: %v", err)
	}

	if after.StorageBytes <= before.StorageBytes {
		t.Errorf("StorageBytes did not grow after a second topic: %d -> %d",
			before.StorageBytes, after.StorageBytes)
	}
}

func TestUpdateTenantStorageUsage_NoStorage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := &Broker{
		ctx:            ctx,
		cancel:         cancel,
		logger:         newTestLogger(),
		tenancyManager: tenancy.NewManager(),
		status:         StatusRunning,
	}

	// No topicManager: must be a no-op rather than a panic.
	broker.updateTenantStorageUsage()
}
