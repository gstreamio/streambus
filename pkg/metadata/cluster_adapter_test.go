package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/shawntherrien/streambus/pkg/cluster"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConsensusNode is a simple mock that accepts all proposals
type mockConsensusNode struct {
	fsm      *FSM
	isLeader bool
}

func newMockConsensusNode(fsm *FSM) *mockConsensusNode {
	return &mockConsensusNode{
		fsm:      fsm,
		isLeader: true,
	}
}

func (m *mockConsensusNode) Propose(ctx context.Context, data []byte) error {
	// Directly apply to FSM (simulating immediate consensus)
	return m.fsm.Apply(data)
}

func (m *mockConsensusNode) IsLeader() bool {
	return m.isLeader
}

func (m *mockConsensusNode) Leader() uint64 {
	if m.isLeader {
		return 1
	}
	return 0
}

// setupTestAdapter creates a test adapter with mock consensus
func setupTestAdapter() (*ClusterMetadataStore, *Store, *FSM) {
	fsm := NewFSM()
	consensus := newMockConsensusNode(fsm)
	store := NewStore(fsm, consensus)
	adapter := NewClusterMetadataStore(store)
	return adapter, store, fsm
}

func TestClusterMetadataStore_StoreBrokerMetadata_NewBroker(t *testing.T) {
	adapter, _, _ := setupTestAdapter()
	ctx := context.Background()

	broker := &cluster.BrokerMetadata{
		ID:               1,
		Host:             "localhost",
		Port:             9092,
		Rack:             "rack-1",
		Status:           cluster.BrokerStatusStarting,
		DiskCapacityGB:   1000,
		DiskUsedGB:       250,
		NetworkBandwidth: 1000000000,
	}

	// Store new broker
	err := adapter.StoreBrokerMetadata(ctx, broker)
	require.NoError(t, err)

	// Verify broker was stored
	retrieved, err := adapter.GetBrokerMetadata(ctx, 1)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, int32(1), retrieved.ID)
	assert.Equal(t, "localhost", retrieved.Host)
	assert.Equal(t, 9092, retrieved.Port)
	assert.Equal(t, "rack-1", retrieved.Rack)
	// Note: FSM always sets new brokers to Alive status
	assert.Equal(t, cluster.BrokerStatusAlive, retrieved.Status)
	assert.Equal(t, int64(1000), retrieved.DiskCapacityGB)
	assert.Equal(t, int64(250), retrieved.DiskUsedGB)
	assert.Equal(t, int64(1000000000), retrieved.NetworkBandwidth)
	// Note: Tags are not supported in metadata.BrokerInfo
	assert.NotNil(t, retrieved.Tags)
}

func TestClusterMetadataStore_StoreBrokerMetadata_UpdateExisting(t *testing.T) {
	adapter, _, _ := setupTestAdapter()
	ctx := context.Background()

	// Register initial broker
	broker := &cluster.BrokerMetadata{
		ID:             1,
		Host:           "localhost",
		Port:           9092,
		Status:         cluster.BrokerStatusStarting,
		DiskCapacityGB: 1000,
		DiskUsedGB:     100,
	}
	err := adapter.StoreBrokerMetadata(ctx, broker)
	require.NoError(t, err)

	// Update broker with new values
	broker.Status = cluster.BrokerStatusAlive
	broker.DiskUsedGB = 250
	err = adapter.StoreBrokerMetadata(ctx, broker)
	require.NoError(t, err)

	// Verify update
	retrieved, err := adapter.GetBrokerMetadata(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, cluster.BrokerStatusAlive, retrieved.Status)
	assert.Equal(t, int64(250), retrieved.DiskUsedGB)
}

func TestClusterMetadataStore_GetBrokerMetadata_NotFound(t *testing.T) {
	adapter, _, _ := setupTestAdapter()
	ctx := context.Background()

	retrieved, err := adapter.GetBrokerMetadata(ctx, 999)
	assert.Error(t, err)
	assert.Nil(t, retrieved)
	assert.Contains(t, err.Error(), "not found")
}

func TestClusterMetadataStore_ListBrokers(t *testing.T) {
	adapter, _, _ := setupTestAdapter()
	ctx := context.Background()

	// Register multiple brokers
	for i := int32(1); i <= 3; i++ {
		broker := &cluster.BrokerMetadata{
			ID:     i,
			Host:   "localhost",
			Port:   9090 + int(i),
			Rack:   "rack-1",
			Status: cluster.BrokerStatusAlive,
		}
		err := adapter.StoreBrokerMetadata(ctx, broker)
		require.NoError(t, err)
	}

	// List all brokers
	brokers, err := adapter.ListBrokers(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, len(brokers))

	// Verify broker IDs
	ids := make(map[int32]bool)
	for _, b := range brokers {
		ids[b.ID] = true
	}
	assert.True(t, ids[1])
	assert.True(t, ids[2])
	assert.True(t, ids[3])
}

func TestClusterMetadataStore_DeleteBroker(t *testing.T) {
	adapter, _, _ := setupTestAdapter()
	ctx := context.Background()

	// Register broker
	broker := &cluster.BrokerMetadata{
		ID:     1,
		Host:   "localhost",
		Port:   9092,
		Status: cluster.BrokerStatusAlive,
	}
	err := adapter.StoreBrokerMetadata(ctx, broker)
	require.NoError(t, err)

	// Delete broker
	err = adapter.DeleteBroker(ctx, 1)
	require.NoError(t, err)

	// Verify deletion
	brokers, err := adapter.ListBrokers(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, len(brokers))
}

func TestClusterMetadataStore_StatusConversion(t *testing.T) {
	adapter, _, _ := setupTestAdapter()
	ctx := context.Background()

	// Test 1: Register new broker (FSM always sets to Alive)
	t.Run("new_broker_always_alive", func(t *testing.T) {
		broker := &cluster.BrokerMetadata{
			ID:     1,
			Host:   "localhost",
			Port:   9092,
			Status: cluster.BrokerStatusFailed, // Try to register as failed
		}

		err := adapter.StoreBrokerMetadata(ctx, broker)
		require.NoError(t, err)

		retrieved, err := adapter.GetBrokerMetadata(ctx, 1)
		require.NoError(t, err)
		// FSM overrides to Alive for new brokers
		assert.Equal(t, cluster.BrokerStatusAlive, retrieved.Status)
		assert.Equal(t, "localhost:9092", retrieved.Address())
	})

	// Test 2: Update existing broker status
	testCases := []struct {
		name                string
		updateStatus        cluster.BrokerStatus
		expectedAfterUpdate cluster.BrokerStatus
	}{
		{"update_to_failed", cluster.BrokerStatusFailed, cluster.BrokerStatusFailed},
		{"update_to_draining", cluster.BrokerStatusDraining, cluster.BrokerStatusDraining},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// First register broker (will be Alive)
			broker := &cluster.BrokerMetadata{
				ID:     2,
				Host:   "localhost",
				Port:   9093,
				Status: cluster.BrokerStatusAlive,
			}
			err := adapter.StoreBrokerMetadata(ctx, broker)
			require.NoError(t, err)

			// Now update status
			broker.Status = tc.updateStatus
			err = adapter.StoreBrokerMetadata(ctx, broker)
			require.NoError(t, err)

			// Verify status was updated
			retrieved, err := adapter.GetBrokerMetadata(ctx, 2)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedAfterUpdate, retrieved.Status)

			// Clean up
			adapter.DeleteBroker(ctx, 2)
		})
	}
}

func TestClusterMetadataStore_StorePartitionAssignment(t *testing.T) {
	adapter, store, _ := setupTestAdapter()
	ctx := context.Background()

	// Register brokers first
	for i := int32(1); i <= 3; i++ {
		broker := &cluster.BrokerMetadata{
			ID:     i,
			Host:   "localhost",
			Port:   9090 + int(i),
			Status: cluster.BrokerStatusAlive,
		}
		err := adapter.StoreBrokerMetadata(ctx, broker)
		require.NoError(t, err)
	}

	// Create assignment
	assignment := cluster.NewAssignment()
	assignment.Version = 1

	// Add partitions to assignment
	key1 := cluster.PartitionKey("test-topic", 0)
	assignment.Partitions[key1] = []int32{1, 2}
	assignment.Leaders[key1] = 1

	key2 := cluster.PartitionKey("test-topic", 1)
	assignment.Partitions[key2] = []int32{2, 3}
	assignment.Leaders[key2] = 2

	// Store assignment
	err := adapter.StorePartitionAssignment(ctx, assignment)
	require.NoError(t, err)

	// Verify partitions were created
	partitions := store.ListPartitions("test-topic")
	assert.Equal(t, 2, len(partitions))

	// Check partition 0
	part0, exists := store.GetPartition("test-topic", 0)
	require.True(t, exists)
	assert.Equal(t, "test-topic", part0.Topic)
	assert.Equal(t, 0, part0.Partition)
	assert.Equal(t, uint64(1), part0.Leader)
	assert.Equal(t, []uint64{1, 2}, part0.Replicas)
	assert.Equal(t, uint64(1), part0.LeaderEpoch)

	// Check partition 1
	part1, exists := store.GetPartition("test-topic", 1)
	require.True(t, exists)
	assert.Equal(t, "test-topic", part1.Topic)
	assert.Equal(t, 1, part1.Partition)
	assert.Equal(t, uint64(2), part1.Leader)
	assert.Equal(t, []uint64{2, 3}, part1.Replicas)
}

func TestClusterMetadataStore_GetPartitionAssignment(t *testing.T) {
	adapter, store, _ := setupTestAdapter()
	ctx := context.Background()

	// Create partitions directly in store
	partitions := []*PartitionInfo{
		{
			Topic:       "test-topic",
			Partition:   0,
			Leader:      1,
			Replicas:    []uint64{1, 2},
			ISR:         []uint64{1, 2},
			LeaderEpoch: 1,
		},
		{
			Topic:       "test-topic",
			Partition:   1,
			Leader:      2,
			Replicas:    []uint64{2, 3},
			ISR:         []uint64{2, 3},
			LeaderEpoch: 1,
		},
	}

	err := store.BatchCreatePartitions(ctx, partitions)
	require.NoError(t, err)

	// Get assignment via adapter
	assignment, err := adapter.GetPartitionAssignment(ctx, "test-topic")
	require.NoError(t, err)
	require.NotNil(t, assignment)

	// Verify assignment
	assert.Equal(t, 2, assignment.TotalPartitions())

	key0 := cluster.PartitionKey("test-topic", 0)
	assert.Equal(t, []int32{1, 2}, assignment.Partitions[key0])
	assert.Equal(t, int32(1), assignment.Leaders[key0])

	key1 := cluster.PartitionKey("test-topic", 1)
	assert.Equal(t, []int32{2, 3}, assignment.Partitions[key1])
	assert.Equal(t, int32(2), assignment.Leaders[key1])

	// Verify broker load
	// Partition 0: replicas [1, 2] → broker 1 += 1, broker 2 += 1
	// Partition 1: replicas [2, 3] → broker 2 += 1, broker 3 += 1
	assert.Equal(t, 1, assignment.BrokerLoad[1]) // Appears in 1 partition
	assert.Equal(t, 2, assignment.BrokerLoad[2]) // Appears in 2 partitions
	assert.Equal(t, 1, assignment.BrokerLoad[3]) // Appears in 1 partition
}

func TestClusterMetadataStore_ParsePartitionKey(t *testing.T) {
	adapter, _, _ := setupTestAdapter()

	testCases := []struct {
		key             string
		expectedTopic   string
		expectedPartID  int
	}{
		{"test-topic:0", "test-topic", 0},
		{"test-topic:5", "test-topic", 5},
		{"my-topic:123", "my-topic", 123},
		{"topic-with-dashes:42", "topic-with-dashes", 42},
	}

	for _, tc := range testCases {
		t.Run(tc.key, func(t *testing.T) {
			topic, partID := adapter.parsePartitionKey(tc.key)
			assert.Equal(t, tc.expectedTopic, topic)
			assert.Equal(t, tc.expectedPartID, partID)
		})
	}
}

func TestClusterMetadataStore_ParseAddress(t *testing.T) {
	adapter, _, _ := setupTestAdapter()

	testCases := []struct {
		addr         string
		expectedHost string
		expectedPort int
	}{
		{"localhost:9092", "localhost", 9092},
		{"192.168.1.1:8080", "192.168.1.1", 8080},
		{"broker1.example.com:19092", "broker1.example.com", 19092},
		{"invalid", "invalid", 0}, // Invalid format
		{"host:abc", "host:abc", 0}, // Invalid port
	}

	for _, tc := range testCases {
		t.Run(tc.addr, func(t *testing.T) {
			host, port := adapter.parseAddress(tc.addr)
			assert.Equal(t, tc.expectedHost, host)
			assert.Equal(t, tc.expectedPort, port)
		})
	}
}

func TestClusterMetadataStore_TypeConversions(t *testing.T) {
	adapter, _, _ := setupTestAdapter()

	// Test int32 to uint64 conversion
	int32Slice := []int32{1, 2, 3, 4, 5}
	uint64Slice := adapter.int32ToUint64(int32Slice)
	assert.Equal(t, 5, len(uint64Slice))
	assert.Equal(t, uint64(1), uint64Slice[0])
	assert.Equal(t, uint64(5), uint64Slice[4])

	// Test uint64 to int32 conversion
	converted := adapter.uint64ToInt32(uint64Slice)
	assert.Equal(t, int32Slice, converted)
}

func TestClusterMetadataStore_BrokerResourceTracking(t *testing.T) {
	adapter, _, _ := setupTestAdapter()
	ctx := context.Background()

	broker := &cluster.BrokerMetadata{
		ID:               1,
		Host:             "localhost",
		Port:             9092,
		Status:           cluster.BrokerStatusAlive,
		DiskCapacityGB:   2000,
		DiskUsedGB:       1500,
		NetworkBandwidth: 10000000000, // 10 Gbps
	}

	err := adapter.StoreBrokerMetadata(ctx, broker)
	require.NoError(t, err)

	retrieved, err := adapter.GetBrokerMetadata(ctx, 1)
	require.NoError(t, err)

	// Verify resource tracking
	assert.Equal(t, int64(2000), retrieved.DiskCapacityGB)
	assert.Equal(t, int64(1500), retrieved.DiskUsedGB)
	assert.Equal(t, int64(10000000000), retrieved.NetworkBandwidth)

	// Verify disk utilization calculation
	expectedUtil := float64(1500) / float64(2000) * 100.0
	assert.Equal(t, expectedUtil, retrieved.DiskUtilization())
}

func TestClusterMetadataStore_BrokerTimestampHandling(t *testing.T) {
	adapter, _, _ := setupTestAdapter()
	ctx := context.Background()

	now := time.Now()
	broker := &cluster.BrokerMetadata{
		ID:           1,
		Host:         "localhost",
		Port:         9092,
		Status:       cluster.BrokerStatusAlive,
		RegisteredAt: now,
	}

	err := adapter.StoreBrokerMetadata(ctx, broker)
	require.NoError(t, err)

	retrieved, err := adapter.GetBrokerMetadata(ctx, 1)
	require.NoError(t, err)

	// Note: FSM sets its own RegisteredAt, so we can't compare exact times
	// Just verify the timestamp was set
	assert.False(t, retrieved.RegisteredAt.IsZero())

	// LastHeartbeat should be set to current time (approximation in adapter)
	assert.False(t, retrieved.LastHeartbeat.IsZero())
	assert.True(t, time.Since(retrieved.LastHeartbeat) < 1*time.Second)
}

func TestClusterMetadataStore_EmptyAssignment(t *testing.T) {
	adapter, _, _ := setupTestAdapter()
	ctx := context.Background()

	// Get assignment for non-existent topic
	assignment, err := adapter.GetPartitionAssignment(ctx, "non-existent-topic")
	require.NoError(t, err)
	require.NotNil(t, assignment)

	// Should return empty assignment
	assert.Equal(t, 0, assignment.TotalPartitions())
	assert.Equal(t, 0, len(assignment.Partitions))
	assert.Equal(t, 0, len(assignment.Leaders))
}

func TestClusterMetadataStore_ConcurrentOperations(t *testing.T) {
	adapter, _, _ := setupTestAdapter()
	ctx := context.Background()

	// Register multiple brokers concurrently
	done := make(chan bool, 10)
	for i := int32(1); i <= 10; i++ {
		go func(id int32) {
			broker := &cluster.BrokerMetadata{
				ID:     id,
				Host:   "localhost",
				Port:   9090 + int(id),
				Status: cluster.BrokerStatusAlive,
			}
			err := adapter.StoreBrokerMetadata(ctx, broker)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all operations
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all brokers registered
	brokers, err := adapter.ListBrokers(ctx)
	require.NoError(t, err)
	assert.Equal(t, 10, len(brokers))
}
